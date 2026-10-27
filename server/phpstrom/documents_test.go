package phpstrom

import (
	"strings"
	"testing"

	"github.com/ayanozturk/vscode-php-strom/lsp"
)

func TestDocumentStoreChangeFullReplace(t *testing.T) {
	store := NewDocumentStore()
	store.Open(lsp.TextDocumentItem{URI: "file:///a.php", Version: 1, Text: "old"})
	doc := store.Change("file:///a.php", 2, []lsp.TextDocumentContentChangeEvent{
		{Text: "<?php\necho 1;\n"},
	})
	if doc.Version != 2 || doc.Text != "<?php\necho 1;\n" {
		t.Fatalf("unexpected doc: %+v", doc)
	}
}

func TestDocumentStoreChangeCreatesMissing(t *testing.T) {
	store := NewDocumentStore()
	doc := store.Change("file:///missing.php", 1, []lsp.TextDocumentContentChangeEvent{
		{Text: "hello"},
	})
	if doc.Text != "hello" || doc.URI != "file:///missing.php" {
		t.Fatalf("unexpected created doc: %+v", doc)
	}
}

func TestDocumentStoreChangeRangedASCII(t *testing.T) {
	store := NewDocumentStore()
	store.Open(lsp.TextDocumentItem{URI: "file:///a.php", Version: 1, Text: "hello world"})
	doc := store.Change("file:///a.php", 2, []lsp.TextDocumentContentChangeEvent{
		{
			Range: &lsp.Range{
				Start: lsp.Position{Line: 0, Character: 6},
				End:   lsp.Position{Line: 0, Character: 11},
			},
			Text: "strom",
		},
	})
	if doc.Text != "hello strom" {
		t.Fatalf("got %q", doc.Text)
	}
}

func TestDocumentStoreChangeRangedUTF16(t *testing.T) {
	// "café" is 4 UTF-16 code units but 5 UTF-8 bytes. Replacing the accented
	// letter (character 3..4) must not slice mid-rune.
	store := NewDocumentStore()
	store.Open(lsp.TextDocumentItem{URI: "file:///u.php", Version: 1, Text: "café"})
	doc := store.Change("file:///u.php", 2, []lsp.TextDocumentContentChangeEvent{
		{
			Range: &lsp.Range{
				Start: lsp.Position{Line: 0, Character: 3},
				End:   lsp.Position{Line: 0, Character: 4},
			},
			Text: "e",
		},
	})
	if doc.Text != "cafe" {
		t.Fatalf("UTF-16 ranged edit failed: got %q want %q", doc.Text, "cafe")
	}
}

func TestDocumentStoreChangeMultilineAndEmoji(t *testing.T) {
	// 😀 is one rune / two UTF-16 units. Replace from after 'a' through the emoji.
	store := NewDocumentStore()
	store.Open(lsp.TextDocumentItem{
		URI:     "file:///e.php",
		Version: 1,
		Text:    "a😀\nline2",
	})
	doc := store.Change("file:///e.php", 2, []lsp.TextDocumentContentChangeEvent{
		{
			Range: &lsp.Range{
				Start: lsp.Position{Line: 0, Character: 1},
				End:   lsp.Position{Line: 0, Character: 3}, // past the surrogate pair
			},
			Text: "X",
		},
	})
	if doc.Text != "aX\nline2" {
		t.Fatalf("emoji edit failed: got %q", doc.Text)
	}
}

func TestApplyEditInvalidRangeLeavesSource(t *testing.T) {
	src := "abc"
	got := applyEdit(src, lsp.Range{
		Start: lsp.Position{Line: 0, Character: 2},
		End:   lsp.Position{Line: 0, Character: 1},
	}, "x")
	if got != src {
		t.Fatalf("inverted range should be a no-op, got %q", got)
	}
}

func TestDocumentStoreSetTextMissing(t *testing.T) {
	store := NewDocumentStore()
	if _, ok := store.SetText("file:///nope.php", "x"); ok {
		t.Fatal("expected miss")
	}
}

func TestByteOffsetFromUTF16Position(t *testing.T) {
	cases := []struct {
		name       string
		src        string
		line, char int
		want       int
	}{
		{"first line", "ab\ncd\n", 0, 1, 1},
		{"second line", "ab\ncd\n", 1, 1, 4},
		{"line end clamps before newline", "ab\ncd\n", 0, 9, 2},
		{"crlf clamps before CR", "ab\r\ncd", 0, 9, 2},
		{"empty trailing line", "a\nb\n", 2, 0, 4},
		{"past last line", "a\nb\n", 3, 0, 4},
		{"no trailing newline", "ab\ncd", 1, 9, 5},
		{"emoji is two UTF-16 units", "a\U0001F600b", 0, 3, 5},
		{"negative line", "ab", -1, 5, 0},
	}
	for _, c := range cases {
		if got := byteOffsetFromUTF16Position(c.src, c.line, c.char); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

func BenchmarkApplyEdit(b *testing.B) {
	src := strings.Repeat("<?php echo 'hello world'; // filler line\n", 5000)
	r := lsp.Range{
		Start: lsp.Position{Line: 4000, Character: 3},
		End:   lsp.Position{Line: 4000, Character: 3},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = applyEdit(src, r, "x")
	}
}
