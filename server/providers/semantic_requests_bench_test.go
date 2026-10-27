package providers

import (
	"os"
	"strings"
	"testing"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

func benchRegistry(b *testing.B) (*Registry, string, lsp.Position) {
	src, err := os.ReadFile(os.Getenv("PHP_BENCH_FILE"))
	if err != nil {
		b.Skip("set PHP_BENCH_FILE to a large PHP file: ", err)
	}
	text := string(src)
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///x/f.php", text)
	r := NewRegistry(idx, Config{})
	off := strings.LastIndex(text, "wp_parse_args(")
	if off < 0 {
		b.Skip("no wp_parse_args call")
	}
	line := strings.Count(text[:off], "\n")
	col := off - (strings.LastIndex(text[:off], "\n") + 1) + 3
	return r, text, lsp.Position{Line: uint32(line), Character: uint32(col)}
}

func BenchmarkHoverWP(b *testing.B) {
	r, text, pos := benchRegistry(b)
	r.Hover.Provide("file:///x/f.php", text, pos)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Hover.Provide("file:///x/f.php", text, pos)
	}
}

func BenchmarkDefinitionWP(b *testing.B) {
	r, text, pos := benchRegistry(b)
	r.Definition.Provide("file:///x/f.php", text, pos)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Definition.Provide("file:///x/f.php", text, pos)
	}
}

func BenchmarkCompletionWP(b *testing.B) {
	r, text, pos := benchRegistry(b)
	r.Completion.Provide("file:///x/f.php", text, pos, nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Completion.Provide("file:///x/f.php", text, pos, nil)
	}
}
