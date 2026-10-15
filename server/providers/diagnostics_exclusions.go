package providers

import (
	"net/url"
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
	// variants and baseVariants are brace-expanded, segment-split patterns built once at parse time.
	variants     [][]string
	baseVariants [][]string
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
	for i := len(m.rules) - 1; i >= 0; i-- {
		if m.rules[i].matches(rel) {
			ignored = !m.rules[i].negated
			break
		}
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
		rule.variants = compileGlobVariants(rule.globVariants())
		if !rule.hasSlash {
			rule.baseVariants = compileGlobVariants([]string{rule.pattern})
		}
		rules = append(rules, rule)
	}
	return rules
}

func (r gitignoreRule) matches(rel string) bool {
	rel = filepath.ToSlash(rel)
	if rel == "" || rel == "." {
		return false
	}

	parts := strings.Split(rel, "/")
	if r.dirOnly {
		for i := 0; i < len(parts)-1; i++ {
			if r.matchesParts(parts[:i+1]) {
				return true
			}
		}
		return false
	}

	if r.matchesParts(parts) {
		return true
	}

	if !r.hasSlash {
		base := []string{parts[len(parts)-1]}
		for _, variant := range r.baseVariants {
			if matchSegmentSlice(variant, base) {
				return true
			}
		}
	}

	return false
}

func (r gitignoreRule) matchesParts(parts []string) bool {
	for _, variant := range r.variants {
		if matchSegmentSlice(variant, parts) {
			return true
		}
	}
	return false
}

func compileGlobVariants(patterns []string) [][]string {
	var compiled [][]string
	for _, pattern := range patterns {
		for _, variant := range expandBracePattern(filepath.ToSlash(pattern)) {
			variant = strings.Trim(filepath.ToSlash(variant), "/")
			if variant == "" {
				continue
			}
			compiled = append(compiled, strings.Split(variant, "/"))
		}
	}
	return compiled
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
	if unescaped, err := url.PathUnescape(p); err == nil {
		p = unescaped
	}
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
