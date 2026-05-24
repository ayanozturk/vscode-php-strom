package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildLineOffsets(t *testing.T) {
	src := "<?php\nclass Foo\n{\n}\n"
	offsets := buildLineOffsets(src)
	// Line 0 starts at 0, line 1 starts after first \n, etc.
	expected := []int{0, 6, 16, 18, 20}
	if len(offsets) != len(expected) {
		t.Fatalf("expected %d offsets, got %d: %v", len(expected), len(offsets), offsets)
	}
	for i, want := range expected {
		if offsets[i] != want {
			t.Errorf("offsets[%d]: want %d, got %d", i, want, offsets[i])
		}
	}
}

func TestOffsetToLineChar(t *testing.T) {
	src := "<?php\nclass Foo\n{\n}\n"
	offsets := buildLineOffsets(src)

	tests := []struct {
		offset   int
		wantLine uint32
		wantChar uint32
	}{
		{0, 0, 0},  // start of file
		{5, 0, 5},  // end of first line before \n
		{6, 1, 0},  // start of "class Foo"
		{11, 1, 5}, // "F" in "Foo"
		{16, 2, 0}, // "{"
	}
	for _, tc := range tests {
		line, char := offsetToLineChar(offsets, tc.offset)
		if line != tc.wantLine || char != tc.wantChar {
			t.Errorf("offset %d: want (%d,%d), got (%d,%d)", tc.offset, tc.wantLine, tc.wantChar, line, char)
		}
	}
}

func TestExtractSymbolsHasLSPPositions(t *testing.T) {
	src := "<?php\nclass MyClass {}\n"
	syms := extractSymbols("file:///test.php", src)
	if len(syms) == 0 {
		t.Fatal("expected at least one symbol")
	}
	cls := syms[0]
	if cls.Name != "MyClass" {
		t.Fatalf("expected MyClass, got %s", cls.Name)
	}
	// "class" keyword is on line 1 (0-based), so StartLine should be 1
	if cls.StartLine != 1 {
		t.Errorf("expected StartLine=1 (0-based), got %d", cls.StartLine)
	}
}

func TestExtractSymbolsUsesGoParserForPropertyHooks(t *testing.T) {
	src := `<?php
class Example {
    public private(set) string $name {
        get => $this->name;
        set => $value;
    }
}
`

	syms := extractSymbols("file:///test.php", src)
	if len(syms) < 2 {
		t.Fatalf("expected class and property symbols, got %d", len(syms))
	}

	var property *Symbol
	for _, sym := range syms {
		if sym.Kind == KindProperty && sym.Name == "name" {
			property = sym
			break
		}
	}

	if property == nil {
		t.Fatal("expected property symbol for hook-backed property")
	}
	if property.Visibility != "public" {
		t.Fatalf("expected public visibility, got %q", property.Visibility)
	}
}

func TestExtractSymbolsCreatesPromotedPropertySymbols(t *testing.T) {
	src := `<?php
class SessionStore {}

class Session {
	public function __construct(private SessionStore $session) {}
}
`

	syms := extractSymbols("file:///test.php", src)
	for _, sym := range syms {
		if sym.Kind == KindProperty && sym.Name == "session" {
			if sym.Type != "SessionStore" {
				t.Fatalf("expected promoted property type SessionStore, got %q", sym.Type)
			}
			if sym.Visibility != "private" {
				t.Fatalf("expected promoted property visibility private, got %q", sym.Visibility)
			}
			return
		}
	}

	t.Fatal("expected promoted constructor param to be indexed as property symbol")
}

func TestExtractSymbolsResolvesAliasedMethodParameterTypes(t *testing.T) {
	src := `<?php
use Vendor\Package\SchemeFilter as SchemeFilter;

class Repository {
	public function getByFilterObject(SchemeFilter $filter): void {}
}
`

	syms := extractSymbols("file:///test.php", src)
	for _, sym := range syms {
		if sym.Kind == KindMethod && sym.Name == "getByFilterObject" {
			if len(sym.Params) != 1 {
				t.Fatalf("expected one parameter, got %#v", sym.Params)
			}
			if sym.Params[0].Type != "Vendor\\Package\\SchemeFilter" {
				t.Fatalf("expected aliased param type to resolve to Vendor\\Package\\SchemeFilter, got %q", sym.Params[0].Type)
			}
			return
		}
	}

	t.Fatal("expected method symbol")
}

func TestExtractSymbolsResolvesImplementedInterfacesToFQNs(t *testing.T) {
	src := `<?php
namespace Doctrine\Common\Collections;

interface Collection {}

class ArrayCollection implements Collection {}
`

	syms := extractSymbols("file:///test.php", src)
	for _, sym := range syms {
		if sym.Kind == KindClass && sym.Name == "ArrayCollection" {
			if len(sym.Implements) != 1 || sym.Implements[0] != `\Doctrine\Common\Collections\Collection` {
				t.Fatalf("expected fully-qualified implements entry, got %#v", sym.Implements)
			}
			return
		}
	}

	t.Fatal("expected ArrayCollection symbol")
}

func TestConfiguredStubsAreIndexed(t *testing.T) {
	stubsPath, err := filepath.Abs("../../stubs")
	if err != nil {
		t.Fatalf("resolve stubs path: %v", err)
	}

	wi := New(Config{StubsPath: stubsPath, Stubs: []string{"Core", "SPL"}, PHPVersion: "8.3"})
	sym := wi.GetIndex().GetByFQN(`\ArrayIterator`)
	if sym == nil {
		t.Fatal("expected ArrayIterator stub symbol to be indexed")
	}
	if len(sym.Implements) == 0 {
		t.Fatalf("expected ArrayIterator implements metadata, got %#v", sym)
	}
}

func TestMatchSimpleMatchesVendorRootPattern(t *testing.T) {
	if !matchSimple("**/vendor/**", "/workspace/project/vendor") {
		t.Fatal("expected vendor root to match exclude pattern")
	}
	if !matchSimple("**/vendor/**", "/workspace/project/vendor/symfony/http-foundation") {
		t.Fatal("expected nested vendor path to match exclude pattern")
	}
}

func TestMatchSimpleMatchesVendorTestsPattern(t *testing.T) {
	if !matchSimple("**/vendor/**/{Tests,tests}/**", "/workspace/project/vendor/symfony/http-foundation/Tests") {
		t.Fatal("expected Tests directory under vendor to match exclude pattern")
	}
	if !matchSimple("**/vendor/**/{Tests,tests}/**", "/workspace/project/vendor/symfony/http-foundation/tests/Unit") {
		t.Fatal("expected tests subtree under vendor to match exclude pattern")
	}
	if matchSimple("**/vendor/**/{Tests,tests}/**", "/workspace/project/vendor/symfony/http-foundation") {
		t.Fatal("did not expect non-test vendor directory to match vendor test exclude pattern")
	}
}

func TestCollectWorkspaceFilePathsRespectsGitignore(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte("generated/\nignored.php\n!generated/keep.php\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "generated"), 0o755); err != nil {
		t.Fatalf("mkdir generated: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "kept.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatalf("write kept.php: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "ignored.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatalf("write ignored.php: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "generated", "skip.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatalf("write generated/skip.php: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "generated", "keep.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatalf("write generated/keep.php: %v", err)
	}

	wi := New(Config{Associations: []string{"**/*.php"}})
	folders := []WorkspaceFolder{{URI: pathToURI(tmpDir), Name: "tmp"}}
	wi.SetWorkspaceFolders(folders)

	paths := wi.collectWorkspaceFilePaths(folders, wi.gitignores)
	seen := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		seen[filepath.Base(p)] = struct{}{}
	}

	if _, ok := seen["kept.php"]; !ok {
		t.Fatal("expected kept.php to be indexed")
	}
	if _, ok := seen["ignored.php"]; ok {
		t.Fatal("expected ignored.php to be excluded by .gitignore")
	}
	if _, ok := seen["skip.php"]; ok {
		t.Fatal("expected generated/skip.php to be excluded by .gitignore")
	}
	if _, ok := seen["keep.php"]; !ok {
		t.Fatal("expected negated .gitignore rule to include generated/keep.php")
	}
}
