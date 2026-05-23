package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/ayanozturk/vscode-php-strom/indexer"
)

func main() {
	root := "/Volumes/RG-DOCK/rg_core"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	log.SetOutput(os.Stderr)
	log.Printf("Starting Indexer Benchmark on root: %s", root)

	cfg := indexer.Config{
		MaxSize:      10 * 1024 * 1024, // 10MB
		Associations: []string{"**/*.php", "**/*.phtml"},
		Exclude: []string{
			"**/vendor/**",
			"**/node_modules/**",
			"**/cache/**",
			"**/.git/**",
		},
	}

	wi := indexer.New(cfg)
	wi.SetWorkspaceFolders([]indexer.WorkspaceFolder{
		{
			URI:  "file://" + root,
			Name: "rg_core",
		},
	})

	var memStart, memEnd runtime.MemStats
	runtime.ReadMemStats(&memStart)

	start := time.Now()

	wi.OnIndexingStart(func() {
		log.Printf("Indexing started...")
	})
	wi.OnIndexingProgress(func(done, total int) {
		// Suppress verbose logs to measure raw speed
	})
	var summary indexer.IndexingSummary
	wi.OnIndexingDone(func(s indexer.IndexingSummary) {
		summary = s
		log.Printf(
			"Indexing done. Indexed %d/%d files, scanned %d LOC, extracted %d symbols in %s.",
			s.FilesIndexed,
			s.FilesDiscovered,
			s.LinesScanned,
			s.SymbolsIndexed,
			s.Duration.Round(time.Millisecond),
		)
	})

	wi.IndexWorkspace()

	elapsed := time.Since(start)
	runtime.ReadMemStats(&memEnd)

	fmt.Printf("\n========== INDEXER PERFORMANCE ==========\n")
	fmt.Printf("Files Indexed:   %d / %d\n", summary.FilesIndexed, summary.FilesDiscovered)
	fmt.Printf("Lines Scanned:   %d\n", summary.LinesScanned)
	fmt.Printf("Bytes Scanned:   %.2f MB\n", float64(summary.BytesScanned)/(1024*1024))
	fmt.Printf("Symbols Indexed: %d\n", summary.SymbolsIndexed)
	fmt.Printf("Total Time:      %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("Files/Sec:       %.2f\n", float64(summary.FilesIndexed)/elapsed.Seconds())
	fmt.Printf("Lines/Sec:       %.2f\n", float64(summary.LinesScanned)/elapsed.Seconds())
	fmt.Printf("HeapAlloc:       %.2f MB\n", float64(memEnd.HeapAlloc)/(1024*1024))
	fmt.Printf("Sys memory:      %.2f MB\n", float64(memEnd.Sys)/(1024*1024))
	fmt.Printf("=========================================\n")
}
