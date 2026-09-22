package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
	"github.com/ayanozturk/vscode-php-strom/phpstrom"
	"github.com/ayanozturk/vscode-php-strom/providers"
)

func main() {
	cpuProfile := flag.String("cpuprofile", "", "write CPU profile to file")
	memProfile := flag.String("memprofile", "", "write heap profile to file")
	workers := flag.Int("workers", 0, "worker count override (0 = indexer.DiagnosticWorkerCountFor default)")
	dumpDiagnostics := flag.String("dump", "", "write sorted uri|message lines to this file for correctness diffing")
	prodExcludes := flag.Bool("prod-excludes", false, "use production indexing excludes and diagnostic defaults, including reportable-file filtering")
	flag.Parse()

	root := "."
	if flag.NArg() > 0 {
		root = flag.Arg(0)
	}

	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatalf("create cpu profile: %v", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatalf("start cpu profile: %v", err)
		}
		defer pprof.StopCPUProfile()
	}

	log.SetOutput(os.Stderr)
	log.Printf("Starting Diagnostics Benchmark on root: %s", root)

	exclude := []string{
		"**/vendor/**",
		"**/node_modules/**",
		"**/cache/**",
		"**/.git/**",
	}
	if *prodExcludes {
		exclude = []string{"**/.git/**", "**/node_modules/**", "**/vendor/**/{Tests,tests}/**"}
	}
	cfg := indexer.Config{
		MaxSize:      10 * 1024 * 1024,
		Associations: []string{"**/*.php", "**/*.phtml"},
		Exclude:      exclude,
	}
	folders := []indexer.WorkspaceFolder{{URI: "file://" + root, Name: "benchmark-root"}}
	wi := indexer.New(cfg)
	wi.SetWorkspaceFolders(folders)

	indexStart := time.Now()
	wi.OnIndexingProgress(func(done, total int) {})
	wi.IndexWorkspace()
	log.Printf("Indexing done in %s", time.Since(indexStart).Round(time.Millisecond))

	providerCfg := providers.Config{}
	if *prodExcludes {
		providerCfg = phpstrom.DefaultProviderConfig(folders)
	}
	prov := providers.NewRegistry(wi, providerCfg)
	diagProvider := prov.Diagnostics

	uris := wi.WorkspaceFileURIs()
	if *prodExcludes {
		uris = reportableWorkspaceURIs(uris, diagProvider)
	}
	log.Printf("Scanning %d files for diagnostics...", len(uris))

	workerCount := *workers
	if workerCount <= 0 {
		workerCount = indexer.DiagnosticWorkerCountFor(len(uris))
	}
	log.Printf("Using %d diagnostics workers (GOMAXPROCS=%d)", workerCount, runtime.GOMAXPROCS(0))

	jobs := make(chan string, len(uris))
	for _, u := range uris {
		jobs <- u
	}
	close(jobs)

	var processed int64
	var totalDiags int64
	var wg sync.WaitGroup
	var dumpMu sync.Mutex
	var dumpLines []string

	var memStart, memEnd runtime.MemStats
	runtime.ReadMemStats(&memStart)
	start := time.Now()

	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for uri := range jobs {
				path := strings.TrimPrefix(uri, "file://")
				text, err := os.ReadFile(path)
				if err != nil {
					atomic.AddInt64(&processed, 1)
					continue
				}
				diags := diagProvider.AnalyseTransient(uri, string(text))
				atomic.AddInt64(&totalDiags, int64(len(diags)))
				atomic.AddInt64(&processed, 1)
				if *dumpDiagnostics != "" {
					lines := make([]string, len(diags))
					for i, d := range diags {
						lines[i] = formatDiagnosticLine(uri, d)
					}
					dumpMu.Lock()
					dumpLines = append(dumpLines, lines...)
					dumpMu.Unlock()
				}
			}
		}()
	}
	wg.Wait()

	if *dumpDiagnostics != "" {
		sort.Strings(dumpLines)
		if err := os.WriteFile(*dumpDiagnostics, []byte(strings.Join(dumpLines, "\n")+"\n"), 0o644); err != nil {
			log.Fatalf("write dump: %v", err)
		}
	}

	elapsed := time.Since(start)
	runtime.ReadMemStats(&memEnd)

	var totalLines int64
	for _, u := range uris {
		path := strings.TrimPrefix(u, "file://")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		totalLines += int64(strings.Count(string(data), "\n"))
	}

	fmt.Printf("\n========== DIAGNOSTICS PERFORMANCE ==========\n")
	fmt.Printf("Files Processed: %d\n", processed)
	fmt.Printf("Total Diagnostics: %d\n", totalDiags)
	fmt.Printf("Total Lines:      %d\n", totalLines)
	fmt.Printf("Total Time:       %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("Files/Sec:        %.2f\n", float64(processed)/elapsed.Seconds())
	fmt.Printf("Lines/Sec:        %.2f\n", float64(totalLines)/elapsed.Seconds())
	fmt.Printf("HeapAlloc:        %.2f MB\n", float64(memEnd.HeapAlloc)/(1024*1024))
	fmt.Printf("Sys memory:       %.2f MB\n", float64(memEnd.Sys)/(1024*1024))
	fmt.Printf("==============================================\n")

	if *memProfile != "" {
		f, err := os.Create(*memProfile)
		if err != nil {
			log.Fatalf("create mem profile: %v", err)
		}
		defer f.Close()
		runtime.GC()
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatalf("write mem profile: %v", err)
		}
	}
}

func reportableWorkspaceURIs(uris []string, diagnostics *providers.DiagnosticsProvider) []string {
	reportable := make([]string, 0, len(uris))
	for _, uri := range uris {
		if diagnostics.IgnoresAll(uri) {
			continue
		}
		reportable = append(reportable, uri)
	}
	return reportable
}

func formatDiagnosticLine(uri string, d lsp.Diagnostic) string {
	return fmt.Sprintf("%v|%s|%d|%s", d.Code, uri, d.Range.Start.Line, d.Message)
}
