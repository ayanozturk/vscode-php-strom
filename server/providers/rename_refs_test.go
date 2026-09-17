package providers

import (
	"fmt"
	"os"
	"testing"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

func TestRenameUsesBoundSymbolNotWordAt(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	a := "<?php\nnamespace A;\nclass Foo {}\n"
	b := "<?php\nnamespace B;\nclass Foo {}\n"
	idx.IndexDocument("file:///workspace/A.php", a)
	idx.IndexDocument("file:///workspace/B.php", b)

	provider := &RenameProvider{idx: idx}
	text := "<?php\nnamespace A;\nfunction f(Foo $x) {}\n"
	// Cursor on Foo in type hint.
	pos := lsp.Position{Line: 2, Character: 12}
	edit := provider.Provide("file:///workspace/Use.php", text, pos, "Bar")
	if edit == nil || len(edit.Changes) == 0 {
		t.Fatal("expected rename edits from bound A\\Foo")
	}
	for uri, edits := range edit.Changes {
		if uri == "file:///workspace/B.php" {
			t.Fatalf("rename must not touch B\\Foo via unqualified spelling; edits=%v", edits)
		}
	}
}

func TestReferencesUsesBoundSymbol(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/A.php", "<?php\nnamespace A;\nclass Foo {}\n")
	idx.IndexDocument("file:///workspace/B.php", "<?php\nnamespace B;\nclass Foo {}\n")

	provider := &ReferencesProvider{idx: idx}
	text := "<?php\nnamespace A;\nfunction f(Foo $x) {}\n"
	pos := lsp.Position{Line: 2, Character: 12}
	locs := provider.Provide("file:///workspace/Use.php", text, pos, true)
	if len(locs) == 0 {
		t.Fatal("expected references for A\\Foo")
	}
	for _, loc := range locs {
		if loc.URI == "file:///workspace/B.php" {
			t.Fatalf("refs must not include B\\Foo: %+v", locs)
		}
	}
}

func TestReferencesHonorsIncludeDeclaration(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	text := "<?php\nclass Foo {}\nfunction useFoo(Foo $foo) {}\n"
	uri := "file:///workspace/Foo.php"
	idx.IndexDocument(uri, text)

	provider := &ReferencesProvider{idx: idx}
	pos := lsp.Position{Line: 2, Character: 17}
	withDeclaration := provider.Provide(uri, text, pos, true)
	withoutDeclaration := provider.Provide(uri, text, pos, false)
	if len(withDeclaration) != 2 {
		t.Fatalf("includeDeclaration=true: want 2 locations, got %+v", withDeclaration)
	}
	if len(withoutDeclaration) != 1 || withoutDeclaration[0].Range.Start.Line != 2 {
		t.Fatalf("includeDeclaration=false: want only the type use, got %+v", withoutDeclaration)
	}
}

func TestReferencesFindAllBodyOnlyCandidates(t *testing.T) {
	dir := t.TempDir()
	idx := indexer.New(indexer.Config{MaxSize: 1 << 20})
	declPath := dir + "/Target.php"
	declURI := "file://" + declPath
	declSrc := "<?php\nclass Target {}\n"
	if err := os.WriteFile(declPath, []byte(declSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	idx.IndexDocument(declURI, declSrc)

	const bodyFiles = 40
	for i := 0; i < bodyFiles; i++ {
		path := fmt.Sprintf("%s/Body%02d.php", dir, i)
		uri := "file://" + path
		src := fmt.Sprintf("<?php\nfunction body%d() { return new Target(); }\n", i)
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		idx.IndexDocument(uri, src)
	}

	provider := &ReferencesProvider{idx: idx}
	locs := provider.Provide(declURI, declSrc, lsp.Position{Line: 1, Character: 7}, true)
	bodyHits := 0
	for _, loc := range locs {
		if loc.URI != declURI {
			bodyHits++
		}
	}
	if bodyHits != bodyFiles {
		t.Fatalf("want all %d body-only references, got %d in %+v", bodyFiles, bodyHits, locs)
	}
}

func TestReferencesFindShortBodyOnlyIdentifier(t *testing.T) {
	dir := t.TempDir()
	idx := indexer.New(indexer.Config{MaxSize: 1 << 20})
	declPath := dir + "/X.php"
	declURI := "file://" + declPath
	declSrc := "<?php\nclass X {}\n"
	bodyPath := dir + "/Body.php"
	bodyURI := "file://" + bodyPath
	bodySrc := "<?php\nfunction body() { return new X(); }\n"
	for path, src := range map[string]string{declPath: declSrc, bodyPath: bodySrc} {
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	idx.IndexDocument(declURI, declSrc)
	idx.IndexDocument(bodyURI, bodySrc)

	locs := (&ReferencesProvider{idx: idx}).Provide(declURI, declSrc, lsp.Position{Line: 1, Character: 7}, true)
	for _, loc := range locs {
		if loc.URI == bodyURI {
			return
		}
	}
	t.Fatalf("short body-only reference was not found: %+v", locs)
}

func TestUpgradeMatchingFilesSeesBodyRef(t *testing.T) {
	dir := t.TempDir()
	declPath := dir + "/Foo.php"
	declURI := "file://" + declPath
	if err := os.WriteFile(declPath, []byte("<?php\nclass Foo {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx := indexer.New(indexer.Config{MaxSize: 1 << 20})
	idx.IndexDocument(declURI, "<?php\nclass Foo {}\n")

	// Signature + body uses: declaration-tier keeps the type hint; upgrade fills body.
	hintPath := dir + "/Hint.php"
	hintURI := "file://" + hintPath
	hintSrc := "<?php\nfunction g(Foo $x) { return new Foo(); }\n"
	if err := os.WriteFile(hintPath, []byte(hintSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	idx.IndexDocument(hintURI, hintSrc)

	provider := &ReferencesProvider{idx: idx}
	open := "<?php\nfunction h(Foo $x) {}\n"
	pos := lsp.Position{Line: 1, Character: 12}
	locs := provider.Provide("file:///open.php", open, pos, true)
	hintHits := 0
	for _, loc := range locs {
		if loc.URI == hintURI {
			hintHits++
		}
	}
	if hintHits < 2 {
		t.Fatalf("expected ≥2 Foo uses in Hint.php after body upgrade (type hint + new Foo); got %d in %+v", hintHits, locs)
	}
}

func TestRenamePrepareUsesExactBoundSpan(t *testing.T) {
	provider := &RenameProvider{}
	text := "<?php\nclass Foo {}\n"
	pos := lsp.Position{Line: 1, Character: 7}
	r := provider.Prepare("file:///workspace/Foo.php", text, pos)
	if r == nil {
		t.Fatal("expected prepare range")
	}
	if r.Start.Line != 1 || r.End.Line != 1 {
		t.Fatalf("unexpected range lines: %+v", r)
	}
	if r.End.Character-r.Start.Character != 3 {
		t.Fatalf("expected exact Foo span width 3, got %+v", r)
	}
}
