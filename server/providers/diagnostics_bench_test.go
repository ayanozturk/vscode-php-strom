package providers

import (
	"os"
	"testing"
)

func BenchmarkAnalyseWP(b *testing.B) {
	src, err := os.ReadFile(os.Getenv("PHP_BENCH_FILE"))
	if err != nil {
		b.Skip("set PHP_BENCH_FILE to a large PHP file: ", err)
	}
	p := &DiagnosticsProvider{}
	text := string(src)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = p.Analyse("file:///x/functions.php", text)
	}
}
