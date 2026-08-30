package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/ayanozturk/vscode-php-strom/lsp"
	"github.com/ayanozturk/vscode-php-strom/phpstrom"
)

type durationStats struct {
	Samples []float64 `json:"samplesMs"`
	Mean    float64   `json:"meanMs"`
	Median  float64   `json:"medianMs"`
	P95     float64   `json:"p95Ms"`
	Min     float64   `json:"minMs"`
	Max     float64   `json:"maxMs"`
	CV      float64   `json:"cvPercent"`
}

type latencyReport struct {
	SchemaVersion int       `json:"schemaVersion"`
	GeneratedAt   time.Time `json:"generatedAt"`
	Environment   struct {
		GOOS        string `json:"goos"`
		GOARCH      string `json:"goarch"`
		GoVersion   string `json:"goVersion"`
		FixturePHP  int    `json:"fixturePhpFiles"`
		ColdRuns    int    `json:"coldRuns"`
		EditRuns    int    `json:"editRuns"`
		ColdProcess bool   `json:"coldProcess"`
	} `json:"environment"`
	Measurements struct {
		ColdStart        durationStats `json:"coldStart"`
		CacheReuseSave   durationStats `json:"cacheReuseSave"`
		BodyEdit         durationStats `json:"bodyEdit"`
		DependencyEdit   durationStats `json:"dependencyEdit"`
		Cancellation     durationStats `json:"cancellation"`
		StaleResultDrop  durationStats `json:"staleResultDrop"`
		FullFallbackEdit durationStats `json:"fullFallbackEdit"`
	} `json:"measurements"`
	Accounting phpstrom.EditorTraceSnapshot `json:"accounting"`
	Gates      struct {
		MaxColdStartMs float64  `json:"maxColdStartMs"`
		MaxEditMs      float64  `json:"maxEditMs"`
		MaxCancelMs    float64  `json:"maxCancelMs"`
		Passed         bool     `json:"passed"`
		Failures       []string `json:"failures"`
	} `json:"gates"`
}

type scenario struct {
	handler *phpstrom.Handler
	root    string
	files   map[string]string
}

func main() {
	fileCount := flag.Int("files", 1000, "number of additional synthetic PHP files")
	coldRuns := flag.Int("cold-runs", 5, "number of fresh handler/workspace index runs")
	editRuns := flag.Int("edit-runs", 5, "number of fresh incremental editor scenarios")
	maxCold := flag.Float64("max-cold-ms", 5000, "maximum permitted cold-start median in milliseconds; zero disables")
	maxEdit := flag.Float64("max-edit-ms", 1000, "maximum permitted edit median in milliseconds; zero disables")
	maxCancel := flag.Float64("max-cancel-ms", 250, "maximum permitted cancellation median in milliseconds; zero disables")
	timeout := flag.Duration("timeout", 30*time.Second, "timeout for each asynchronous editor operation")
	output := flag.String("output", "", "optional JSON output path; stdout is always written")
	coldWorker := flag.String("cold-worker", "", "internal process-cold worker workspace")
	flag.Parse()
	log.SetOutput(io.Discard)
	if *coldWorker != "" {
		if _, err := initialiseHandler(*coldWorker, *timeout); err != nil {
			fatalf("cold worker: %v", err)
		}
		return
	}

	if *fileCount < 0 || *coldRuns < 1 || *editRuns < 1 {
		fatalf("files must be non-negative and cold-runs/edit-runs must be positive")
	}

	root, err := os.MkdirTemp("", "phpstrom-editor-trace-")
	if err != nil {
		fatalf("create fixture workspace: %v", err)
	}
	defer os.RemoveAll(root)

	fixtureFiles, err := writeFixture(root, *fileCount)
	if err != nil {
		fatalf("write fixture workspace: %v", err)
	}

	coldSamples := make([]float64, 0, *coldRuns)
	for range *coldRuns {
		started := time.Now()
		command := exec.Command(os.Args[0], "--cold-worker", root, "--timeout", timeout.String())
		command.Stdout = io.Discard
		command.Stderr = io.Discard
		if err := command.Run(); err != nil {
			fatalf("process-cold start: %v", err)
		}
		coldSamples = append(coldSamples, float64(time.Since(started).Microseconds())/1000)
	}

	measurements := make(map[string][]float64)
	var h *phpstrom.Handler
	for range *editRuns {
		h, err = initialiseHandler(root, *timeout)
		if err != nil {
			fatalf("initialise incremental scenario: %v", err)
		}
		s := scenario{handler: h, root: root, files: fixtureFiles}
		runMeasurements, runErr := s.run(*timeout)
		if runErr != nil {
			fatalf("incremental scenario: %v", runErr)
		}
		for name, samples := range runMeasurements {
			measurements[name] = append(measurements[name], samples...)
		}
	}

	report := latencyReport{SchemaVersion: 1, GeneratedAt: time.Now().UTC()}
	report.Environment.GOOS = runtime.GOOS
	report.Environment.GOARCH = runtime.GOARCH
	report.Environment.GoVersion = runtime.Version()
	report.Environment.FixturePHP = len(fixtureFiles)
	report.Environment.ColdRuns = *coldRuns
	report.Environment.EditRuns = *editRuns
	report.Environment.ColdProcess = true
	report.Measurements.ColdStart = summarize(coldSamples)
	report.Measurements.CacheReuseSave = summarize(measurements["cache_reuse"])
	report.Measurements.BodyEdit = summarize(measurements["body_edit"])
	report.Measurements.DependencyEdit = summarize(measurements["dependency_edit"])
	report.Measurements.Cancellation = summarize(measurements["cancellation"])
	report.Measurements.StaleResultDrop = summarize(measurements["stale_drop"])
	report.Measurements.FullFallbackEdit = summarize(measurements["full_fallback"])
	report.Accounting = h.EditorTrace()
	report.Gates.MaxColdStartMs = *maxCold
	report.Gates.MaxEditMs = *maxEdit
	report.Gates.MaxCancelMs = *maxCancel
	report.Gates.Failures = validateReport(report)
	report.Gates.Passed = len(report.Gates.Failures) == 0

	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fatalf("encode report: %v", err)
	}
	encoded = append(encoded, '\n')
	if _, err := os.Stdout.Write(encoded); err != nil {
		fatalf("write report: %v", err)
	}
	if *output != "" {
		if err := os.WriteFile(*output, encoded, 0o644); err != nil {
			fatalf("write %s: %v", *output, err)
		}
	}
	if !report.Gates.Passed {
		os.Exit(1)
	}
}

func writeFixture(root string, additional int) (map[string]string, error) {
	files := map[string]string{
		"Base.php":       "<?php\nclass Base { public function run(): void { echo 'base'; } }\n",
		"Consumer.php":   "<?php\nclass Consumer extends Base { public function execute(Base $value): void { $value->run(); } }\n",
		"Unrelated.php":  "<?php\nclass Unrelated {}\n",
		"CollisionA.php": "<?php\nclass Collision {}\n",
		"CollisionB.php": "<?php\nclass Distinct {}\n",
	}
	for index := range additional {
		name := fmt.Sprintf("Fixture%04d.php", index)
		files[name] = fmt.Sprintf("<?php\nclass Fixture%04d { public function value(): int { return %d; } }\n", index, index)
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o644); err != nil {
			return nil, err
		}
	}
	return files, nil
}

func initialiseHandler(root string, timeout time.Duration) (*phpstrom.Handler, error) {
	srv := phpstrom.NewServer(bytes.NewReader(nil), io.Discard)
	h := phpstrom.NewHandler(srv)
	params := lsp.InitializeParams{
		WorkspaceFolders: []lsp.WorkspaceFolder{{URI: fileURI(root), Name: "editor-trace-fixture"}},
		InitializationOptions: map[string]interface{}{
			"settings": map[string]interface{}{
				"diagnostics": map[string]interface{}{"workspaceScanOnStart": false, "run": "onType"},
				"stubs":       []interface{}{},
			},
		},
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if _, responseErr := h.HandleRequest("initialize", raw); responseErr != nil {
		return nil, fmt.Errorf("initialize: %s", responseErr.Message)
	}
	h.HandleNotification("initialized", json.RawMessage(`{}`))
	if _, err := waitForEvent(h, 0, timeout, func(event phpstrom.EditorTraceEvent) bool {
		return event.Operation == "workspace_index" && event.Outcome == "completed"
	}); err != nil {
		return nil, err
	}
	return h, nil
}

func (s scenario) run(timeout time.Duration) (map[string][]float64, error) {
	result := make(map[string][]float64)
	consumerURI := fileURI(filepath.Join(s.root, "Consumer.php"))
	baseURI := fileURI(filepath.Join(s.root, "Base.php"))
	unrelatedURI := fileURI(filepath.Join(s.root, "Unrelated.php"))
	collisionURI := fileURI(filepath.Join(s.root, "CollisionB.php"))

	consumerOpen := sendOpen(s.handler, consumerURI, 1, s.files["Consumer.php"])
	consumerEvent, err := waitForEvent(s.handler, consumerOpen, timeout, documentEvent(consumerURI, 1))
	if err != nil {
		return nil, err
	}

	before := consumerEvent.Sequence
	sendSave(s.handler, consumerURI)
	cacheEvent, err := waitForEvent(s.handler, before, timeout, operationEvent("save_analysis", "published"))
	if err != nil {
		return nil, err
	}
	result["cache_reuse"] = append(result["cache_reuse"], microsToMillis(cacheEvent.DurationMicros))

	baseOpen := sendOpen(s.handler, baseURI, 1, s.files["Base.php"])
	baseEvent, err := waitForEvent(s.handler, baseOpen, timeout, documentEvent(baseURI, 1))
	if err != nil {
		return nil, err
	}
	bodyText := "<?php\nclass Base { public function run(): void { echo 'body changed'; } }\n"
	sendChange(s.handler, baseURI, 2, bodyText)
	bodyEvent, err := waitForEvent(s.handler, baseEvent.Sequence, timeout, documentEvent(baseURI, 2))
	if err != nil {
		return nil, err
	}
	result["body_edit"] = append(result["body_edit"], eventLatencyMillis(bodyEvent))

	before = bodyEvent.Sequence
	sendSave(s.handler, consumerURI)
	if _, err := waitForEvent(s.handler, before, timeout, operationEvent("save_analysis", "published")); err != nil {
		return nil, err
	}

	unrelatedOpen := sendOpen(s.handler, unrelatedURI, 1, s.files["Unrelated.php"])
	unrelatedEvent, err := waitForEvent(s.handler, unrelatedOpen, timeout, documentEvent(unrelatedURI, 1))
	if err != nil {
		return nil, err
	}
	sendChange(s.handler, unrelatedURI, 2, "<?php\nclass Unrelated { public function changed(int $value): void {} }\n")
	unrelatedEvent, err = waitForEvent(s.handler, unrelatedEvent.Sequence, timeout, documentEvent(unrelatedURI, 2))
	if err != nil {
		return nil, err
	}
	before = unrelatedEvent.Sequence
	sendSave(s.handler, consumerURI)
	if _, err := waitForEvent(s.handler, before, timeout, operationEvent("save_analysis", "published")); err != nil {
		return nil, err
	}

	signatureText := "<?php\nclass Base { public function run(int $value): void { echo $value; } }\n"
	sendChange(s.handler, baseURI, 3, signatureText)
	dependencyEvent, err := waitForEvent(s.handler, unrelatedEvent.Sequence, timeout, documentEvent(baseURI, 3))
	if err != nil {
		return nil, err
	}
	result["dependency_edit"] = append(result["dependency_edit"], eventLatencyMillis(dependencyEvent))
	before = dependencyEvent.Sequence
	sendSave(s.handler, consumerURI)
	if _, err := waitForEvent(s.handler, before, timeout, operationEvent("save_analysis", "published")); err != nil {
		return nil, err
	}

	sendChange(s.handler, baseURI, 4, signatureText+"\n")
	sendChange(s.handler, baseURI, 5, strings.Replace(signatureText, "echo $value", "echo $value + 1", 1))
	cancelEvent, err := waitForEvent(s.handler, dependencyEvent.Sequence, timeout, operationEvent("document_cancellation", "superseded_before_start"))
	if err != nil {
		return nil, err
	}
	result["cancellation"] = append(result["cancellation"], microsToMillis(cancelEvent.DurationMicros))
	if _, err := waitForEvent(s.handler, cancelEvent.Sequence, timeout, documentEvent(baseURI, 5)); err != nil {
		return nil, err
	}

	staleStarted := time.Now()
	if s.handler.PublishDocumentDiagnostics(consumerURI, s.files["Consumer.php"], 0) {
		return nil, fmt.Errorf("stale diagnostic result was published")
	}
	staleEvent, err := waitForEvent(s.handler, before, timeout, operationEvent("diagnostics_publication", "stale_dropped"))
	if err != nil {
		return nil, err
	}
	result["stale_drop"] = append(result["stale_drop"], math.Max(microsToMillis(staleEvent.DurationMicros), float64(time.Since(staleStarted).Microseconds())/1000))

	collisionOpen := sendOpen(s.handler, collisionURI, 1, s.files["CollisionB.php"])
	collisionEvent, err := waitForEvent(s.handler, collisionOpen, timeout, documentEvent(collisionURI, 1))
	if err != nil {
		return nil, err
	}
	sendChange(s.handler, collisionURI, 2, "<?php\nclass Collision { public function duplicate(): void {} }\n")
	fallbackEvent, err := waitForEvent(s.handler, collisionEvent.Sequence, timeout, documentEvent(collisionURI, 2))
	if err != nil {
		return nil, err
	}
	result["full_fallback"] = append(result["full_fallback"], eventLatencyMillis(fallbackEvent))
	return result, nil
}

func sendOpen(h *phpstrom.Handler, uri string, version int, text string) uint64 {
	before := lastSequence(h.EditorTrace().Events)
	notify(h, "textDocument/didOpen", lsp.DidOpenTextDocumentParams{TextDocument: lsp.TextDocumentItem{
		URI: uri, LanguageID: "php", Version: version, Text: text,
	}})
	return before
}

func sendChange(h *phpstrom.Handler, uri string, version int, text string) {
	notify(h, "textDocument/didChange", lsp.DidChangeTextDocumentParams{
		TextDocument:   lsp.VersionedTextDocumentIdentifier{URI: uri, Version: version},
		ContentChanges: []lsp.TextDocumentContentChangeEvent{{Text: text}},
	})
}

func sendSave(h *phpstrom.Handler, uri string) {
	notify(h, "textDocument/didSave", lsp.DidSaveTextDocumentParams{TextDocument: lsp.TextDocumentIdentifier{URI: uri}})
}

func notify(h *phpstrom.Handler, method string, params interface{}) {
	raw, err := json.Marshal(params)
	if err != nil {
		panic(err)
	}
	h.HandleNotification(method, raw)
}

func waitForEvent(h *phpstrom.Handler, after uint64, timeout time.Duration, match func(phpstrom.EditorTraceEvent) bool) (phpstrom.EditorTraceEvent, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, event := range h.EditorTrace().Events {
			if event.Sequence > after && match(event) {
				return event, nil
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	return phpstrom.EditorTraceEvent{}, fmt.Errorf("timed out after %s waiting for trace event", timeout)
}

func documentEvent(uri string, version int) func(phpstrom.EditorTraceEvent) bool {
	return func(event phpstrom.EditorTraceEvent) bool {
		return event.Operation == "document_analysis" && event.URI == uri && event.Version == version && (event.Outcome == "published" || event.Outcome == "indexed")
	}
}

func operationEvent(operation, outcome string) func(phpstrom.EditorTraceEvent) bool {
	return func(event phpstrom.EditorTraceEvent) bool {
		return event.Operation == operation && event.Outcome == outcome
	}
}

func validateReport(report latencyReport) []string {
	var failures []string
	if report.Gates.MaxColdStartMs > 0 && report.Measurements.ColdStart.Median > report.Gates.MaxColdStartMs {
		failures = append(failures, fmt.Sprintf("cold-start median %.3fms exceeds %.3fms", report.Measurements.ColdStart.Median, report.Gates.MaxColdStartMs))
	}
	for name, stats := range map[string]durationStats{
		"body edit": report.Measurements.BodyEdit, "dependency edit": report.Measurements.DependencyEdit,
		"cache reuse save": report.Measurements.CacheReuseSave, "full fallback edit": report.Measurements.FullFallbackEdit,
	} {
		if report.Gates.MaxEditMs > 0 && stats.Median > report.Gates.MaxEditMs {
			failures = append(failures, fmt.Sprintf("%s median %.3fms exceeds %.3fms", name, stats.Median, report.Gates.MaxEditMs))
		}
	}
	if report.Gates.MaxCancelMs > 0 && report.Measurements.Cancellation.Median > report.Gates.MaxCancelMs {
		failures = append(failures, fmt.Sprintf("cancellation median %.3fms exceeds %.3fms", report.Measurements.Cancellation.Median, report.Gates.MaxCancelMs))
	}
	if report.Accounting.Cache.SemanticHits < 2 {
		failures = append(failures, "semantic cache reuse was not observed")
	}
	if report.Accounting.Cache.SemanticMisses < 2 {
		failures = append(failures, "semantic cache rebuild was not observed")
	}
	if report.Accounting.Indexer.DependencyMatches == 0 {
		failures = append(failures, "dependency-scoped invalidation was not observed")
	}
	if report.Accounting.Indexer.IncrementalBuilds == 0 {
		failures = append(failures, "incremental project-index updates were not observed")
	}
	if report.Accounting.Indexer.BodyOnlyUpdates == 0 {
		failures = append(failures, "body-only project-index update was not observed")
	}
	if report.Accounting.Indexer.ExportedChanges == 0 {
		failures = append(failures, "exported semantic change was not observed")
	}
	if report.Accounting.Indexer.FullFallbacks == 0 {
		failures = append(failures, "full project-index fallback was not observed")
	}
	if report.Accounting.Indexer.GlobalCompactions == 0 {
		failures = append(failures, "global semantic compaction was not observed")
	}
	if len(report.Measurements.StaleResultDrop.Samples) == 0 {
		failures = append(failures, "stale diagnostics drop was not observed")
	}
	return failures
}

func summarize(samples []float64) durationStats {
	stats := durationStats{Samples: append([]float64(nil), samples...)}
	if len(samples) == 0 {
		return stats
	}
	values := append([]float64(nil), samples...)
	sort.Float64s(values)
	stats.Min = values[0]
	stats.Max = values[len(values)-1]
	for _, value := range values {
		stats.Mean += value
	}
	stats.Mean /= float64(len(values))
	stats.Median = percentile(values, 0.5)
	stats.P95 = percentile(values, 0.95)
	if stats.Mean > 0 && len(values) > 1 {
		var variance float64
		for _, value := range values {
			delta := value - stats.Mean
			variance += delta * delta
		}
		variance /= float64(len(values) - 1)
		stats.CV = math.Sqrt(variance) / stats.Mean * 100
	}
	return stats
}

func percentile(sorted []float64, quantile float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	position := quantile * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sorted[lower]
	}
	return sorted[lower] + (sorted[upper]-sorted[lower])*(position-float64(lower))
}

func lastSequence(events []phpstrom.EditorTraceEvent) uint64 {
	if len(events) == 0 {
		return 0
	}
	return events[len(events)-1].Sequence
}

func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func microsToMillis(value int64) float64 { return float64(value) / 1000 }

func eventLatencyMillis(event phpstrom.EditorTraceEvent) float64 {
	return microsToMillis(event.QueueMicros + event.DurationMicros)
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
