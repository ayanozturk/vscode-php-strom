package phpstrom

import (
	"log"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultMemoryLimit = 4 << 30

var startMemoryWatchdogOnce sync.Once

func startMemoryWatchdog() {
	startMemoryWatchdogOnce.Do(func() {
		limit := memoryLimitBytes()
		debug.SetMemoryLimit(limit)
		log.Printf("[phpstrom] soft memory limit %d MiB (set PHPSTROM_GOMEMLIMIT to override)", limit/1024/1024)

		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			var stats runtime.MemStats
			for range ticker.C {
				runtime.ReadMemStats(&stats)
				if stats.HeapInuse < uint64(limit)/2 {
					continue
				}
				debug.FreeOSMemory()
				log.Printf("[phpstrom] heap in-use %d MiB; returned unused pages to the OS", stats.HeapInuse/1024/1024)
			}
		}()
	})
}

func memoryLimitBytes() int64 {
	raw := strings.TrimSpace(os.Getenv("PHPSTROM_GOMEMLIMIT"))
	if raw == "" {
		return defaultMemoryLimit
	}
	upper := strings.ToUpper(raw)
	switch {
	case strings.HasSuffix(upper, "GIB"):
		return parseScaledLimit(raw[:len(raw)-3], 30)
	case strings.HasSuffix(upper, "GB"):
		return parseScaledLimit(raw[:len(raw)-2], 30)
	case strings.HasSuffix(upper, "MIB"):
		return parseScaledLimit(raw[:len(raw)-3], 20)
	case strings.HasSuffix(upper, "MB"):
		return parseScaledLimit(raw[:len(raw)-2], 20)
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
		return n
	}
	return defaultMemoryLimit
}

func parseScaledLimit(raw string, shift uint) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || n <= 0 {
		return defaultMemoryLimit
	}
	return n << shift
}
