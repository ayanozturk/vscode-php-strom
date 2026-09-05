package phpstrom

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

func TestDidOpenDefersIndexingUntilInitialWorkspaceIndexCompletes(t *testing.T) {
	h := NewHandler(&Server{out: io.Discard})
	h.initialIndexing.Store(true)
	uri := "file:///workspace/Deferred.php"
	params, err := json.Marshal(lsp.DidOpenTextDocumentParams{TextDocument: lsp.TextDocumentItem{
		URI: uri, LanguageID: "php", Version: 1, Text: "<?php\nclass DeferredUntilReady {}\n",
	}})
	if err != nil {
		t.Fatalf("marshal didOpen: %v", err)
	}

	h.HandleNotification("textDocument/didOpen", params)
	time.Sleep(20 * time.Millisecond)
	if got := h.idx.GetIndex().GetByFQN(`\DeferredUntilReady`); got != nil {
		t.Fatal("expected restored document indexing to wait for workspace readiness")
	}

	h.initialIndexing.Store(false)
	h.initialIndexDoneOnce.Do(func() { close(h.initialIndexDone) })
	waitForCondition(t, func() bool {
		return h.idx.GetIndex().GetByFQN(`\DeferredUntilReady`) != nil
	})
}

func TestWorkspaceScanDoesNotAnnounceStartBeforeItOwnsTheScan(t *testing.T) {
	var out synchronizedBuffer
	h := NewHandler(&Server{out: &out})
	h.workspaceDiagnosticsMu.Lock()
	done := make(chan struct{})
	go func() {
		h.runWorkspaceDiagnosticsScan(false)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	if strings.Contains(out.String(), "phpstrom/workspaceDiagnosticsStarted") {
		t.Fatal("expected queued scan to remain invisible until it owns the scan lock")
	}
	h.workspaceDiagnosticsMu.Unlock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for workspace diagnostics scan")
	}
	if !strings.Contains(out.String(), "phpstrom/workspaceDiagnosticsFinished") {
		t.Fatal("expected acquired scan to publish a matching finish notification")
	}
}

func TestNonDiagnosticConfigurationChangeDoesNotStartWorkspaceScan(t *testing.T) {
	var out synchronizedBuffer
	h := NewHandler(&Server{out: &out})
	params, err := json.Marshal(map[string]interface{}{
		"settings": map[string]interface{}{
			"format": map[string]interface{}{"tabSize": 2},
		},
	})
	if err != nil {
		t.Fatalf("marshal configuration: %v", err)
	}

	h.HandleNotification("workspace/didChangeConfiguration", params)
	if strings.Contains(out.String(), "phpstrom/workspaceDiagnosticsStarted") {
		t.Fatal("expected unrelated configuration change not to start diagnostics")
	}
}

func TestAnalysisLevelConfigurationChangeStartsWorkspaceScan(t *testing.T) {
	var out synchronizedBuffer
	h := NewHandler(&Server{out: &out})
	params, err := json.Marshal(map[string]interface{}{
		"settings": map[string]interface{}{
			"diagnostics": map[string]interface{}{
				"analysis": map[string]interface{}{"level": float64(0)},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal configuration: %v", err)
	}

	h.HandleNotification("workspace/didChangeConfiguration", params)
	waitForCondition(t, func() bool {
		return strings.Contains(out.String(), "phpstrom/workspaceDiagnosticsStarted")
	})
}

func TestWorkspaceScanDoesNotPublishEmptyDiagnosticsForCleanFiles(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "Clean.php")
	if err := os.WriteFile(filePath, []byte("<?php\n\nclass Clean\n{\n}\n"), 0o644); err != nil {
		t.Fatalf("write clean file: %v", err)
	}

	var out synchronizedBuffer
	h := NewHandler(&Server{out: &out})
	h.idx.SetWorkspaceFolders([]indexer.WorkspaceFolder{{URI: "file://" + filepath.ToSlash(tmpDir), Name: "tmp"}})
	h.indexAndPublishWorkspaceDiagnostics()

	if strings.Contains(out.String(), "textDocument/publishDiagnostics") {
		t.Fatalf("expected clean workspace files not to emit redundant diagnostic clears, got %q", out.String())
	}
}

func TestEditorTraceIsBoundedAndReturnsAnIndependentCopy(t *testing.T) {
	h := NewHandler(&Server{out: io.Discard})
	for range maxEditorTraceEvents + 3 {
		h.trace.record(EditorTraceEvent{Operation: "synthetic", Outcome: "completed"})
	}

	trace := h.EditorTrace()
	if len(trace.Events) != maxEditorTraceEvents {
		t.Fatalf("expected trace to retain at most %d events, got %d", maxEditorTraceEvents, len(trace.Events))
	}
	trace.Events[0].Operation = "mutated"
	again := h.EditorTrace()
	if again.Events[0].Operation == "mutated" {
		t.Fatal("expected EditorTrace to return an independent event slice")
	}
}

func TestEditorTraceRecordsScheduledDocumentCancellation(t *testing.T) {
	h := NewHandler(&Server{out: io.Discard})
	uri := "file:///workspace/Scheduled.php"
	h.scheduleDocumentIndex(uri, 7, time.Hour)
	h.cancelDocumentAnalysis(uri)

	for _, event := range h.EditorTrace().Events {
		if event.Operation == "document_cancellation" && event.Outcome == "cancelled_before_start" && event.URI == uri && event.Version == 7 {
			return
		}
	}
	t.Fatalf("expected cancellation trace event, got %#v", h.EditorTrace().Events)
}

func TestEditorTraceRecordsStaleDiagnosticsPublication(t *testing.T) {
	h := NewHandler(&Server{out: io.Discard})
	uri := "file:///workspace/StaleTrace.php"
	oldText := "<?php\nclass OldTrace {}\n"
	h.documents.Open(lsp.TextDocumentItem{URI: uri, LanguageID: "php", Version: 1, Text: oldText})
	h.documents.Change(uri, 2, []lsp.TextDocumentContentChangeEvent{{Range: nil, Text: "<?php\nclass NewTrace {}\n"}})

	if published := h.publishDiagnostics(uri, oldText, 1); published {
		t.Fatal("expected stale diagnostics publication to be dropped")
	}
	for _, event := range h.EditorTrace().Events {
		if event.Operation == "diagnostics_publication" && event.Outcome == "stale_dropped" && event.URI == uri && event.Version == 1 {
			return
		}
	}
	t.Fatalf("expected stale diagnostics trace event, got %#v", h.EditorTrace().Events)
}

func waitForCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not satisfied before timeout")
}
