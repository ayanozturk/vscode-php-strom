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
