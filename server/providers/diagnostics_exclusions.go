package providers

import (
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

type DiagnosticsPathExclusions struct {
	explicit   []diagnosticPathRule
	gitignores []workspaceGitignore
}

type diagnosticPathRule struct {
	pattern   string
	ignoreAll bool
	codes     map[string]struct{}
}

type workspaceGitignore struct {
	root  string
	rules []gitignoreRule
}

type gitignoreRule struct {
	pattern  string
	negated  bool
	dirOnly  bool
	anchored bool
	hasSlash bool
}

func BuildDiagnosticsPathExclusions(config map[string][]string, folders []indexer.WorkspaceFolder) DiagnosticsPathExclusions {
	exclusions := DiagnosticsPathExclusions{}
	for pattern, codes := range config {
		rule := diagnosticPathRule{pattern: filepath.ToSlash(pattern)}
		if len(codes) == 0 {
			rule.ignoreAll = true
		} else {
			rule.codes = make(map[string]struct{}, len(codes))
			for _, code := range codes {
				rule.codes[code] = struct{}{}
			}
		}
		exclusions.explicit = append(exclusions.explicit, rule)
	}

	for _, folder := range folders {
		matcher, ok := loadWorkspaceGitignore(folder)
		if ok {
			exclusions.gitignores = append(exclusions.gitignores, matcher)
		}
	}

	return exclusions
}

func (e DiagnosticsPathExclusions) IgnoresAll(filename string) bool {
	if filename == "" {
		return false
	}
	if e.matchesGitignore(filename) {
		return true
	}
	ignoreAll, _ := e.matchExplicit(filename)
	return ignoreAll
}

func (e DiagnosticsPathExclusions) Filter(filename string, diagnostics []lsp.Diagnostic) []lsp.Diagnostic {
	if len(diagnostics) == 0 {
		if e.IgnoresAll(filename) {
			return []lsp.Diagnostic{}
		}
		return diagnostics
	}

	if e.matchesGitignore(filename) {
		return []lsp.Diagnostic{}
	}

	ignoreAll, suppressed := e.matchExplicit(filename)
	if ignoreAll {
		return []lsp.Diagnostic{}
	}
	if len(suppressed) == 0 {
		return diagnostics
	}

	filtered := make([]lsp.Diagnostic, 0, len(diagnostics))
	for _, diag := range diagnostics {
		code, _ := diag.Code.(string)
		if _, ok := suppressed[code]; ok {
			continue
		}
		filtered = append(filtered, diag)
	}
	return filtered
}

func (e DiagnosticsPathExclusions) matchExplicit(filename string) (bool, map[string]struct{}) {
	var suppressed map[string]struct{}
	for _, rule := range e.explicit {
		if !matchSimple(rule.pattern, filename) {
			continue
		}
		if rule.ignoreAll {
			return true, nil
		}
		if len(rule.codes) == 0 {
			continue
		}
		if suppressed == nil {
			suppressed = make(map[string]struct{}, len(rule.codes))
		}
		for code := range rule.codes {
			suppressed[code] = struct{}{}
		}
	}
	return false, suppressed
}

func (e DiagnosticsPathExclusions) matchesGitignore(filename string) bool {
	for _, matcher := range e.gitignores {
		if matcher.ignores(filename) {
			return true
		}
	}
	return false
}

func loadWorkspaceGitignore(folder indexer.WorkspaceFolder) (workspaceGitignore, bool) {
	root := uriToPath(folder.URI)
	if root == "" {
		return workspaceGitignore{}, false
	}

	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return workspaceGitignore{}, false
	}

	rules := parseGitignoreRules(string(data))
	if len(rules) == 0 {
		return workspaceGitignore{}, false
	}

	return workspaceGitignore{root: filepath.Clean(root), rules: rules}, true
}

func (m workspaceGitignore) ignores(filename string) bool {
	rel, ok := relativeWorkspacePath(m.root, filename)
	if !ok {
		return false
	}

	ignored := false
	for _, rule := range m.rules {
		if !rule.matches(rel) {
			continue
		}
		ignored = !rule.negated
	}
	return ignored
}

func parseGitignoreRules(content string) []gitignoreRule {
	lines := strings.Split(content, "\n")
	rules := make([]gitignoreRule, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, `\#`) || strings.HasPrefix(line, `\!`) {
			line = line[1:]
		}

		rule := gitignoreRule{negated: strings.HasPrefix(line, "!")}
		if rule.negated {
			line = strings.TrimPrefix(line, "!")
		}
		line = strings.TrimPrefix(line, "./")
		rule.anchored = strings.HasPrefix(line, "/")
		line = strings.TrimPrefix(line, "/")
		rule.dirOnly = strings.HasSuffix(line, "/")
		line = strings.TrimSuffix(line, "/")
		line = filepath.ToSlash(line)
		line = strings.TrimPrefix(line, "./")
		if line == "" {
			continue
		}
		rule.pattern = line
		rule.hasSlash = strings.Contains(line, "/")
		rules = append(rules, rule)
	}
	return rules
}

func (r gitignoreRule) matches(rel string) bool {
	rel = filepath.ToSlash(rel)
	if rel == "" || rel == "." {
		return false
	}

	if r.dirOnly {
		for _, dir := range parentDirectories(rel) {
			if r.matchesPath(dir) {
				return true
			}
		}
		return false
	}

	if r.matchesPath(rel) {
		return true
	}

	if !r.hasSlash {
		return matchSimple(r.pattern, pathpkg.Base(rel))
	}

	return false
}

func (r gitignoreRule) matchesPath(candidate string) bool {
	for _, pattern := range r.globVariants() {
		if matchSimple(pattern, candidate) {
			return true
		}
	}
	return false
}

func (r gitignoreRule) globVariants() []string {
	if r.pattern == "" {
		return nil
	}
	if r.anchored {
		return []string{r.pattern}
	}
	return []string{r.pattern, "**/" + r.pattern}
}

func parentDirectories(rel string) []string {
	dir := pathpkg.Dir(filepath.ToSlash(rel))
	if dir == "." || dir == "" {
		return nil
	}
	parts := strings.Split(dir, "/")
	dirs := make([]string, 0, len(parts))
	for i := range parts {
		dirs = append(dirs, strings.Join(parts[:i+1], "/"))
	}
	return dirs
}

func relativeWorkspacePath(root, filename string) (string, bool) {
	root = filepath.Clean(root)
	filename = filepath.Clean(filename)
	rel, err := filepath.Rel(root, filename)
	if err != nil {
		return "", false
	}
	if rel == "." || rel == "" {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func uriToPath(uri string) string {
	p := strings.TrimPrefix(uri, "file://")
	return filepath.FromSlash(p)
}

func matchSimple(pattern, name string) bool {
	variants := expandBracePattern(filepath.ToSlash(pattern))
	normalizedName := filepath.ToSlash(name)
	for _, variant := range variants {
		if matchPathSegments(variant, normalizedName) {
			return true
		}
	}
	return false
}

func expandBracePattern(pattern string) []string {
	start := strings.IndexByte(pattern, '{')
	if start == -1 {
		return []string{pattern}
	}
	end := strings.IndexByte(pattern[start:], '}')
	if end == -1 {
		return []string{pattern}
	}
	end += start
	parts := strings.Split(pattern[start+1:end], ",")
	variants := make([]string, 0, len(parts))
	for _, part := range parts {
		variants = append(variants, expandBracePattern(pattern[:start]+part+pattern[end+1:])...)
	}
	return variants
}

func matchPathSegments(pattern, candidate string) bool {
	pattern = strings.Trim(filepath.ToSlash(pattern), "/")
	candidate = strings.Trim(filepath.ToSlash(candidate), "/")
	if pattern == "" {
		return candidate == ""
	}
	patternParts := strings.Split(pattern, "/")
	candidateParts := strings.Split(candidate, "/")
	return matchSegmentSlice(patternParts, candidateParts)
}

func matchSegmentSlice(patternParts, candidateParts []string) bool {
	if len(patternParts) == 0 {
		return len(candidateParts) == 0
	}
	if patternParts[0] == "**" {
		if matchSegmentSlice(patternParts[1:], candidateParts) {
			return true
		}
		if len(candidateParts) > 0 {
			return matchSegmentSlice(patternParts, candidateParts[1:])
		}
		return false
	}
	if len(candidateParts) == 0 {
		return false
	}
	matched, err := pathpkg.Match(patternParts[0], candidateParts[0])
	if err != nil || !matched {
		return false
	}
	return matchSegmentSlice(patternParts[1:], candidateParts[1:])
}
