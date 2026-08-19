package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ayanozturk/vscode-php-strom/indexer"
)

type fileResult struct {
	path     string
	duration time.Duration
	timedOut bool
	panicked bool
	panicMsg string
	errCount int
	symCount int
}

func main() {
	root := flag.String("root", ".", "Root directory to scan")
	timeout := flag.Duration("timeout", 5*time.Second, "Per-file parse timeout")
	workers := flag.Int("workers", runtime.GOMAXPROCS(0), "Number of parallel workers")
	maxErrors := flag.Int("max-errors", 50, "Stop reporting after this many error files")
	slow := flag.Duration("slow", 500*time.Millisecond, "Report files slower than this")
	flag.Parse()

	log.SetOutput(os.Stderr)
	log.SetFlags(0)

	var paths []string
	_ = filepath.WalkDir(*root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := filepath.Base(p)
			for _, skip := range []string{"node_modules", "vendor", ".git", "var", "cache"} {
				if base == skip {
					return filepath.SkipDir
				}
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if ext == ".php" || ext == ".phtml" {
			paths = append(paths, p)
		}
		return nil
	})

	total := len(paths)
	fmt.Fprintf(os.Stderr, "Found %d PHP files — running with %d workers, %s timeout\n\n", total, *workers, *timeout)

	jobs := make(chan string, total)
	for _, p := range paths {
		jobs <- p
	}
	close(jobs)

	results := make([]fileResult, 0, 64)
	var mu sync.Mutex
	var done int64
	var timeouts, panics, parseErrs int64

	var wg sync.WaitGroup
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				r := parseFile(p, *timeout)

				n := atomic.AddInt64(&done, 1)
				if n%500 == 0 || int(n) == total {
					fmt.Fprintf(os.Stderr, "\r  progress: %d/%d", n, total)
				}

				if r.timedOut || r.panicked || r.errCount > 0 || r.duration >= *slow {
					mu.Lock()
					if r.timedOut {
						atomic.AddInt64(&timeouts, 1)
					}
					if r.panicked {
						atomic.AddInt64(&panics, 1)
					}
					if r.errCount > 0 {
						atomic.AddInt64(&parseErrs, 1)
					}
					results = append(results, r)
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	fmt.Fprintln(os.Stderr)

	// Sort: timeouts first, then panics, then slowest
	sort.Slice(results, func(i, j int) bool {
		a, b := results[i], results[j]
		if a.timedOut != b.timedOut {
			return a.timedOut
		}
		if a.panicked != b.panicked {
			return a.panicked
		}
		return a.duration > b.duration
	})

	fmt.Printf("\n══════════════════════════════════════════════════════════\n")
	fmt.Printf("  Results: %d files scanned\n", total)
	fmt.Printf("  Timeouts:    %d\n", timeouts)
	fmt.Printf("  Panics:      %d\n", panics)
	fmt.Printf("  Parse errors:%d\n", parseErrs)
	fmt.Printf("══════════════════════════════════════════════════════════\n\n")

	shown := 0
	for _, r := range results {
		if shown >= *maxErrors && !r.timedOut && !r.panicked {
			break
		}
		rel, _ := filepath.Rel(*root, r.path)
		switch {
		case r.timedOut:
			fmt.Printf("TIMEOUT  (>%s)  %s\n", *timeout, rel)
		case r.panicked:
			fmt.Printf("PANIC              %s\n  %s\n", rel, r.panicMsg)
		case r.errCount > 0:
			fmt.Printf("PARSE ERR (%3d)   %s  [%s]\n", r.errCount, rel, r.duration.Round(time.Millisecond))
		default:
			fmt.Printf("SLOW     (%s)  %s\n", r.duration.Round(time.Millisecond), rel)
		}
		shown++
	}
}

func parseFile(path string, limit time.Duration) fileResult {
	r := fileResult{path: path}

	type res struct {
		parsed   indexer.ParsedFile
		duration time.Duration
		panicked bool
		panicMsg string
	}

	ch := make(chan res, 1)
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()

	go func() {
		var out res
		start := time.Now()
		defer func() {
			if rec := recover(); rec != nil {
				buf := make([]byte, 2048)
				n := runtime.Stack(buf, false)
				out.panicked = true
				out.panicMsg = fmt.Sprintf("%v\n%s", rec, buf[:n])
			}
			out.duration = time.Since(start)
			ch <- out
		}()
		data, err := os.ReadFile(path)
		if err != nil {
			ch <- out
			return
		}
		out.parsed = indexer.ParseSourceForIndexWithContext(ctx, path, string(data))
	}()

	select {
	case <-ctx.Done():
		r.timedOut = true
		r.duration = limit
	case out := <-ch:
		r.duration = out.duration
		r.panicked = out.panicked
		r.panicMsg = out.panicMsg
		r.errCount = len(out.parsed.Errors)
		r.symCount = len(out.parsed.Symbols)
	}
	return r
}
