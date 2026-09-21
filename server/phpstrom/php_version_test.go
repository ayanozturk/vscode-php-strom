package phpstrom

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ayanozturk/vscode-php-strom/indexer"
)

func TestVersionFromConstraintCaretUpperBound(t *testing.T) {
	// ^8.3 ⇒ >=8.3 <9.0 → lowest supported match is 8.3
	if got := versionFromConstraint("^8.3"); got != "8.3" {
		t.Fatalf("^8.3 → %q, want 8.3", got)
	}
	// ^7.4 has no upper-bound match in the supported 8.x list
	if got := versionFromConstraint("^7.4"); got != "" {
		t.Fatalf("^7.4 should not match supported 8.x candidates, got %q", got)
	}
}

func TestVersionFromConstraintTilde(t *testing.T) {
	if got := versionFromConstraint("~8.3.12"); got != "8.3" {
		t.Fatalf("~8.3.12 → %q, want 8.3 (excludes 8.4+)", got)
	}
	if got := versionFromConstraint("~8.3"); got != "8.3" {
		t.Fatalf("~8.3 → %q, want 8.3", got)
	}
	// ~8.4.0 allows 8.4 only among supported (not 8.5)
	if got := versionFromConstraint("~8.4.0"); got != "8.4" {
		t.Fatalf("~8.4.0 → %q, want 8.4", got)
	}
}

func TestVersionFromConstraintWildcardAndOps(t *testing.T) {
	if got := versionFromConstraint("8.5.*"); got != "8.5" {
		t.Fatalf("8.5.* → %q", got)
	}
	if got := versionFromConstraint("8.2.x"); got != "8.2" {
		t.Fatalf("8.2.x → %q", got)
	}
	if got := versionFromConstraint(">=8.4"); got != "8.4" {
		t.Fatalf(">=8.4 → %q", got)
	}
	if got := versionFromConstraint("*"); got != "" {
		t.Fatalf("* → %q, want empty", got)
	}
	if got := versionFromConstraint(""); got != "" {
		t.Fatalf("empty → %q", got)
	}
}

func TestEffectivePHPVersionFallbackAndOverride(t *testing.T) {
	if got := effectivePHPVersion("auto", "", nil); got != fallbackPHPVersion {
		t.Fatalf("fallback: got %q", got)
	}
	if got := effectivePHPVersion("8.2", "8.5", nil); got != "8.5" {
		t.Fatalf("override should win: got %q", got)
	}
	if got := effectivePHPVersion("8.4", "", nil); got != "8.4" {
		t.Fatalf("configured: got %q", got)
	}
}

func TestWorkspaceFolderPathUnescapes(t *testing.T) {
	got := workspaceFolderPath("file:///tmp/my%20project")
	if got != "/tmp/my project" {
		t.Fatalf("got %q", got)
	}
	if workspaceFolderPath("http://example") != "" {
		t.Fatal("non-file URI should be empty")
	}
}

func TestDetectComposerPHPVersionCaretRejectsMajorBump(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"php":"^7.4"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := detectComposerPHPVersion([]indexer.WorkspaceFolder{{URI: "file://" + filepath.ToSlash(dir)}})
	if got != "" {
		t.Fatalf("^7.4 must not select an 8.x stub version, got %q", got)
	}
}
