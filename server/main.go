package main

import (
	"log"
	"os"
	"runtime/debug"

	"github.com/ayanozturk/vscode-php-strom/phpstrom"
)

// Workspace indexing/diagnostics are allocation-heavy batch passes over
// potentially tens of thousands of files; the default GOGC=100 triggers GC
// far more often than needed for a short-lived batch pass, measured to cost
// ~30% of CPU time on large corpora. Raising GC percent trades memory for
// throughput; SetMemoryLimit is the backstop that forces GC before this
// tradeoff runs the process out of memory on very large workspaces.
const gcMemoryLimitBytes = 4 << 30 // 4 GiB

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ltime | log.Lshortfile)

	debug.SetGCPercent(200)
	debug.SetMemoryLimit(gcMemoryLimitBytes)

	srv := phpstrom.NewServer(os.Stdin, os.Stdout)
	if err := srv.Run(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
