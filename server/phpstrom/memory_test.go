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
	t.Setenv("PHPSTROM_GOMEMLIMIT", "1GB")
	if got := memoryLimitBytes(); got != 1<<30 {
		t.Fatalf("1GB = %d, want %d", got, 1<<30)
	}
	t.Setenv("PHPSTROM_GOMEMLIMIT", "256MB")
	if got := memoryLimitBytes(); got != 256<<20 {
		t.Fatalf("256MB = %d, want %d", got, 256<<20)
	}
	t.Setenv("PHPSTROM_GOMEMLIMIT", "1048576")
	if got := memoryLimitBytes(); got != 1048576 {
		t.Fatalf("raw bytes = %d", got)
	}
	t.Setenv("PHPSTROM_GOMEMLIMIT", "")
	if got := memoryLimitBytes(); got != defaultMemoryLimit {
		t.Fatalf("default = %d, want %d", got, defaultMemoryLimit)
	}
	t.Setenv("PHPSTROM_GOMEMLIMIT", "nope")
	if got := memoryLimitBytes(); got != defaultMemoryLimit {
		t.Fatalf("invalid = %d, want default", got)
	}
	t.Setenv("PHPSTROM_GOMEMLIMIT", "0GB")
	if got := memoryLimitBytes(); got != defaultMemoryLimit {
		t.Fatalf("zero scaled = %d, want default", got)
	}
}

func TestReleaseUnusedMemory(t *testing.T) {
	releaseUnusedMemory() // smoke: must not panic
}
