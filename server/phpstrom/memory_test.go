package phpstrom

import "testing"

func TestMemoryLimitBytesParsesSizedValues(t *testing.T) {
	t.Setenv("PHPSTROM_GOMEMLIMIT", "2GiB")
	if got := memoryLimitBytes(); got != 2<<30 {
		t.Fatalf("2GiB = %d, want %d", got, 2<<30)
	}
	t.Setenv("PHPSTROM_GOMEMLIMIT", "512MiB")
	if got := memoryLimitBytes(); got != 512<<20 {
		t.Fatalf("512MiB = %d, want %d", got, 512<<20)
	}
}
