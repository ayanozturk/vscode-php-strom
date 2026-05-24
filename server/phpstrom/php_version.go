package phpstrom

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ayanozturk/vscode-php-strom/indexer"
)

var supportedPHPVersions = []string{"8.2", "8.3", "8.4", "8.5"}

const fallbackPHPVersion = "8.3"

type composerManifest struct {
	Require map[string]string `json:"require"`
	Config  struct {
		Platform map[string]string `json:"platform"`
	} `json:"config"`
}

func effectivePHPVersion(configured, override string, folders []indexer.WorkspaceFolder) string {
	if normalized := normalizeSupportedPHPVersion(override); normalized != "" {
		return normalized
	}
	if normalized := normalizeSupportedPHPVersion(configured); normalized != "" {
		return normalized
	}
	if detected := detectComposerPHPVersion(folders); detected != "" {
		return detected
	}
	return fallbackPHPVersion
}

func normalizeSupportedPHPVersion(version string) string {
	version = strings.TrimSpace(strings.ToLower(version))
	if version == "" || version == "auto" {
		return ""
	}
	majorMinor := majorMinorVersion(version)
	for _, supported := range supportedPHPVersions {
		if majorMinor == supported {
			return supported
		}
	}
	return ""
}

func detectComposerPHPVersion(folders []indexer.WorkspaceFolder) string {
	for _, folder := range folders {
		root := workspaceFolderPath(folder.URI)
		if root == "" {
			continue
		}
		version := detectComposerPHPVersionAt(filepath.Join(root, "composer.json"))
		if version != "" {
			return version
		}
	}
	return ""
}

func detectComposerPHPVersionAt(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var manifest composerManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ""
	}
	if manifest.Config.Platform != nil {
		if version := versionFromConstraint(manifest.Config.Platform["php"]); version != "" {
			return version
		}
	}
	if manifest.Require != nil {
		return versionFromConstraint(manifest.Require["php"])
	}
	return ""
}

func versionFromConstraint(constraint string) string {
	constraint = strings.TrimSpace(strings.ToLower(constraint))
	if constraint == "" || constraint == "*" {
		return ""
	}

	candidates := append([]string(nil), supportedPHPVersions...)
	sort.Strings(candidates)
	for _, candidate := range candidates {
		if constraintAllowsMajorMinor(constraint, candidate) {
			return candidate
		}
	}
	return ""
}

func constraintAllowsMajorMinor(constraint, candidate string) bool {
	groups := strings.Split(constraint, "||")
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if constraintGroupAllowsMajorMinor(group, candidate) {
			return true
		}
	}
	return false
}

func constraintGroupAllowsMajorMinor(group, candidate string) bool {
	tokens := strings.Fields(strings.ReplaceAll(group, ",", " "))
	if len(tokens) == 0 {
		return false
	}

	for _, token := range tokens {
		if !constraintTokenAllowsMajorMinor(token, candidate) {
			return false
		}
	}
	return true
}

func constraintTokenAllowsMajorMinor(token, candidate string) bool {
	token = strings.TrimSpace(token)
	if token == "" || token == "*" {
		return true
	}
	if strings.HasPrefix(token, "^") || strings.HasPrefix(token, "~") {
		token = strings.TrimLeft(token, "^~")
		return compareMajorMinor(candidate, majorMinorVersion(token)) >= 0
	}
	if strings.HasSuffix(token, ".*") || strings.HasSuffix(token, ".x") {
		return candidate == majorMinorVersion(strings.TrimSuffix(strings.TrimSuffix(token, ".*"), ".x"))
	}

	op := ""
	for _, candidateOp := range []string{">=", "<=", ">", "<", "="} {
		if strings.HasPrefix(token, candidateOp) {
			op = candidateOp
			token = strings.TrimSpace(strings.TrimPrefix(token, candidateOp))
			break
		}
	}
	version := majorMinorVersion(token)
	if version == "" {
		return true
	}

	cmp := compareMajorMinor(candidate, version)
	switch op {
	case "", "=":
		return cmp == 0
	case ">=":
		return cmp >= 0
	case ">":
		return cmp > 0
	case "<=":
		return cmp <= 0
	case "<":
		return cmp < 0
	default:
		return true
	}
}

var majorMinorPattern = regexp.MustCompile(`(\d+)\.(\d+)`)

func majorMinorVersion(version string) string {
	matches := majorMinorPattern.FindStringSubmatch(version)
	if len(matches) != 3 {
		return ""
	}
	return matches[1] + "." + matches[2]
}

func compareMajorMinor(a, b string) int {
	if a == b {
		return 0
	}
	for idx, version := range supportedPHPVersions {
		if version != a {
			continue
		}
		for otherIdx, other := range supportedPHPVersions {
			if other != b {
				continue
			}
			if idx < otherIdx {
				return -1
			}
			return 1
		}
	}
	return strings.Compare(a, b)
}

func workspaceFolderPath(uri string) string {
	const prefix = "file://"
	if !strings.HasPrefix(uri, prefix) {
		return ""
	}
	path, err := url.PathUnescape(strings.TrimPrefix(uri, prefix))
	if err != nil {
		return strings.TrimPrefix(uri, prefix)
	}
	return path
}
