package indexer

import (
	"os"
	"strings"
	"testing"
)

func BenchmarkIndexDocumentEditWP(b *testing.B) {
	src, err := os.ReadFile(os.Getenv("PHP_BENCH_FILE"))
	if err != nil {
		b.Skip("set PHP_BENCH_FILE to a large PHP file: ", err)
	}
	idx := New(Config{})
	a := string(src)
	c := strings.Replace(a, "return true;", "return  true;", 1)
	idx.IndexDocument("file:///x/functions.php", a)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			idx.IndexDocument("file:///x/functions.php", c)
		} else {
			idx.IndexDocument("file:///x/functions.php", a)
		}
	}
}

func BenchmarkIndexDocumentUnchanged(b *testing.B) {
	src, err := os.ReadFile(os.Getenv("PHP_BENCH_FILE"))
	if err != nil {
		b.Skip("set PHP_BENCH_FILE to a large PHP file: ", err)
	}
	idx := New(Config{})
	text := string(src)
	idx.IndexDocument("file:///x/f.php", text)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.IndexDocument("file:///x/f.php", text)
	}
}
