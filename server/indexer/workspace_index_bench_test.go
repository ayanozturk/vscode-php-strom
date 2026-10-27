package indexer

import (
	"os"
	"testing"
)

// BenchmarkIndexWorkspaceCold indexes the directory named by PHP_BENCH_DIR
// from scratch on every iteration.
func BenchmarkIndexWorkspaceCold(b *testing.B) {
	dir := os.Getenv("PHP_BENCH_DIR")
	if dir == "" {
		b.Skip("set PHP_BENCH_DIR to a PHP project directory")
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		wi := New(Config{Associations: []string{"**/*.php"}, MaxSize: 2_000_000})
		wi.SetWorkspaceFolders([]WorkspaceFolder{{URI: pathToURI(dir), Name: "bench"}})
		wi.IndexWorkspace()
	}
}
