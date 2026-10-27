package indexer

import (
	"os"
	"strings"
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
		wi := New(Config{Associations: []string{"**/*.php", "**/*.phtml", "**/*.phar"}, Exclude: benchExcludes(), MaxSize: 1_000_000})
		wi.SetWorkspaceFolders([]WorkspaceFolder{{URI: pathToURI(dir), Name: "bench"}})
		wi.IndexWorkspace()
	}
}

// benchExcludes returns the server's default excludes, or the comma-separated
// PHP_BENCH_EXCLUDE globs when set.
func benchExcludes() []string {
	if v := os.Getenv("PHP_BENCH_EXCLUDE"); v != "" {
		return strings.Split(v, ",")
	}
	return []string{"**/.git/**", "**/node_modules/**", "**/vendor/**/{Tests,tests}/**"}
}
