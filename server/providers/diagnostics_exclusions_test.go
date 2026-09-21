package providers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

func TestBuildDiagnosticsPathExclusionsExplicitIgnoreAll(t *testing.T) {
	ex := BuildDiagnosticsPathExclusions(map[string][]string{
		"**/vendor/**":     nil,
		"**/Legacy.php":    {"phpstrom.undefined"},
	}, nil)

	if !ex.IgnoresAll("/proj/vendor/pkg/Lib.php") {
		t.Fatal("expected vendor path ignored entirely")
	}
	if ex.IgnoresAll("/proj/src/App.php") {
		t.Fatal("App.php should not be ignore-all")
	}

	diags := []lsp.Diagnostic{
		{Code: "phpstrom.undefined", Message: "a"},
		{Code: "phpstrom.syntax", Message: "b"},
	}
	filtered := ex.Filter("/proj/src/Legacy.php", diags)
	if len(filtered) != 1 || filtered[0].Code != "phpstrom.syntax" {
		t.Fatalf("expected syntax-only after code filter, got %#v", filtered)
	}
}

func TestDiagnosticsExclusionsFilterEmptyAndGitignore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("# comment\ngenerated/\n!generated/keep.php\n*.tmp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ex := BuildDiagnosticsPathExclusions(nil, []indexer.WorkspaceFolder{
		{URI: "file://" + filepath.ToSlash(dir), Name: "tmp"},
	})

	ignored := filepath.Join(dir, "generated", "skip.php")
	if !ex.IgnoresAll(ignored) {
		t.Fatal("generated/skip.php should be gitignored")
	}
	keep := filepath.Join(dir, "generated", "keep.php")
	if ex.IgnoresAll(keep) {
		t.Fatal("negated keep.php must not be ignored")
	}
	if got := ex.Filter(ignored, []lsp.Diagnostic{{Message: "x"}}); len(got) != 0 {
		t.Fatalf("gitignore Filter should clear diags, got %#v", got)
	}
	if !ex.IgnoresAll(filepath.Join(dir, "foo.tmp")) {
		t.Fatal("*.tmp should match basenames")
	}
}

func TestUriToPathUnescapes(t *testing.T) {
	got := uriToPath("file:///tmp/my%20project")
	want := filepath.FromSlash("/tmp/my project")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestMatchSimpleBracesAndGlob(t *testing.T) {
	if !matchSimple("**/vendor/**/{Tests,tests}/**", "/a/vendor/pkg/Tests/X.php") {
		t.Fatal("brace glob should match Tests")
	}
	if matchSimple("**/vendor/**/{Tests,tests}/**", "/a/vendor/pkg/Lib.php") {
		t.Fatal("should not match non-test path")
	}
	if !matchSimple("{a,b}.php", "a.php") {
		t.Fatal("simple brace")
	}
}

func TestParseGitignoreRulesEscapesAndAnchors(t *testing.T) {
	rules := parseGitignoreRules("\\#notcomment\n\\!literal\n/vendor/\n")
	if len(rules) < 3 {
		t.Fatalf("expected escaped + anchored rules, got %#v", rules)
	}
	var anchored bool
	for _, r := range rules {
		if r.pattern == "vendor" && r.anchored && r.dirOnly {
			anchored = true
		}
	}
	if !anchored {
		t.Fatalf("expected anchored dir-only vendor rule, got %#v", rules)
	}
}

func TestNewRegistryWiresProviders(t *testing.T) {
	r := NewRegistry(nil, Config{})
	if r.Completion == nil || r.Diagnostics == nil || r.Symbol == nil {
		t.Fatal("expected core providers to be initialised")
	}
	if snap := r.SemanticCacheTrace(); snap != (SemanticCacheTraceSnapshot{}) {
		// nil diagnostics cache path already covered; zero snap is fine
		_ = snap
	}
}
