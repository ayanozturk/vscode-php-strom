package phpstrom

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

type synchronizedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *synchronizedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *synchronizedBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Len()
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func TestDidSaveRefreshesDocumentFromDiskAndReindexes(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "Session.php")
	uri := "file://" + filepath.ToSlash(filePath)

	oldText := "<?php\nclass Old_Class {}\n"
	newText := "<?php\nclass RGSessionSession implements RGSessionInterface {}\n"

	if err := os.WriteFile(filePath, []byte(newText), 0o644); err != nil {
		t.Fatalf("write saved file: %v", err)
	}

	srv := &Server{out: io.Discard}
	h := NewHandler(srv)
	doc := h.documents.Open(lsp.TextDocumentItem{
		URI:        uri,
		LanguageID: "php",
		Version:    1,
		Text:       oldText,
	})
	h.idx.IndexDocument(doc.URI, doc.Text)

	params, err := json.Marshal(lsp.DidSaveTextDocumentParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		t.Fatalf("marshal didSave: %v", err)
	}

	h.HandleNotification("textDocument/didSave", params)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		updated, ok := h.documents.Get(uri)
		if ok && updated.Text == newText {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	updated, ok := h.documents.Get(uri)
	if !ok {
		t.Fatal("expected document to remain open after save")
	}
	if updated.Text != newText {
		t.Fatalf("expected saved text to be reloaded from disk, got %q", updated.Text)
	}

	if got := h.idx.GetIndex().GetByFQN(`\Old_Class`); got != nil {
		t.Fatalf("expected old symbol to be removed from index, got %+v", got)
	}
	if got := h.idx.GetIndex().GetByFQN(`\RGSessionSession`); got == nil {
		t.Fatal("expected saved file to be reindexed with new class symbol")
	}
}

func TestPublishDiagnosticsDropsStaleResults(t *testing.T) {
	var out synchronizedBuffer
	srv := &Server{out: &out}
	h := NewHandler(srv)

	uri := "file:///test.php"
	h.documents.Open(lsp.TextDocumentItem{
		URI:        uri,
		LanguageID: "php",
		Version:    1,
		Text:       "<?php\nclass Bad_Class {}\n",
	})

	h.documents.Change(uri, 2, []lsp.TextDocumentContentChangeEvent{{
		Range: nil,
		Text:  "<?php\nclass GoodClass {}\n",
	}})

	if published := h.publishDiagnostics(uri, "<?php\nclass Bad_Class {}\n", 1); published {
		t.Fatal("expected stale diagnostics publish to be dropped")
	}
	if out.Len() != 0 {
		t.Fatalf("expected no diagnostics notification for stale publish, got %q", out.String())
	}

	if published := h.publishDiagnostics(uri, "<?php\nclass GoodClass {}\n", 2); !published {
		t.Fatal("expected current diagnostics publish to succeed")
	}
	if out.Len() == 0 {
		t.Fatal("expected diagnostics notification for current document version")
	}
	if strings.Contains(out.String(), `"version"`) {
		t.Fatalf("expected diagnostics notification to omit version, got %q", out.String())
	}
}

func TestDidChangeDebouncesOnTypeAnalysis(t *testing.T) {
	var out synchronizedBuffer
	srv := &Server{out: &out}
	h := NewHandler(srv)

	uri := "file:///debounce.php"
	h.documents.Open(lsp.TextDocumentItem{
		URI:        uri,
		LanguageID: "php",
		Version:    1,
		Text:       "<?php\nclass InitialClass {}\n",
	})

	badChange, err := json.Marshal(lsp.DidChangeTextDocumentParams{
		TextDocument: lsp.VersionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []lsp.TextDocumentContentChangeEvent{{
			Range: nil,
			Text:  "<?php\nclass Bad_Class {}\n",
		}},
	})
	if err != nil {
		t.Fatalf("marshal bad change: %v", err)
	}
	h.HandleNotification("textDocument/didChange", badChange)

	goodChange, err := json.Marshal(lsp.DidChangeTextDocumentParams{
		TextDocument: lsp.VersionedTextDocumentIdentifier{URI: uri, Version: 3},
		ContentChanges: []lsp.TextDocumentContentChangeEvent{{
			Range: nil,
			Text:  "<?php\nclass GoodClass {}\n",
		}},
	})
	if err != nil {
		t.Fatalf("marshal good change: %v", err)
	}
	h.HandleNotification("textDocument/didChange", goodChange)

	deadline := time.Now().Add(600 * time.Millisecond)
	for time.Now().Before(deadline) {
		if h.idx.GetIndex().GetByFQN(`\GoodClass`) != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := h.idx.GetIndex().GetByFQN(`\Bad_Class`); got != nil {
		t.Fatalf("expected stale changed document not to be indexed, got %+v", got)
	}
	if got := h.idx.GetIndex().GetByFQN(`\GoodClass`); got == nil {
		t.Fatal("expected latest changed document to be indexed")
	}
	if strings.Contains(out.String(), "Bad_Class") {
		t.Fatalf("expected debounced analysis to skip stale diagnostics, got %q", out.String())
	}
}

func TestDidSavePublishesEmptyDiagnosticsWhenIssueIsFixed(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "SessionTest.php")
	uri := "file://" + filepath.ToSlash(filePath)

	badText := "<?php\nclass Foo {\n\tfunction BAD_METHOD_NAME() {}\n}\n"
	goodText := "<?php\n\nclass Foo\n{\n    public function goodMethodName(): void\n    {\n    }\n}\n"

	if err := os.WriteFile(filePath, []byte(badText), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	var out synchronizedBuffer
	srv := &Server{out: &out}
	h := NewHandler(srv)
	h.cfg.Diagnostics.Run = "onSave"
	h.documents.Open(lsp.TextDocumentItem{
		URI:        uri,
		LanguageID: "php",
		Version:    1,
		Text:       badText,
	})

	changeParams, err := json.Marshal(lsp.DidChangeTextDocumentParams{
		TextDocument: lsp.VersionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []lsp.TextDocumentContentChangeEvent{{
			Range: nil,
			Text:  goodText,
		}},
	})
	if err != nil {
		t.Fatalf("marshal didChange: %v", err)
	}
	h.HandleNotification("textDocument/didChange", changeParams)

	if err := os.WriteFile(filePath, []byte(goodText), 0o644); err != nil {
		t.Fatalf("write saved file: %v", err)
	}

	saveParams, err := json.Marshal(lsp.DidSaveTextDocumentParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		t.Fatalf("marshal didSave: %v", err)
	}

	h.HandleNotification("textDocument/didSave", saveParams)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		payload := out.String()
		if strings.Contains(payload, `"method":"textDocument/publishDiagnostics"`) && strings.Contains(payload, `"diagnostics":[]`) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("expected empty diagnostics notification after save, got %q", out.String())
}

func TestWorkspaceDiagnosticsPublishesClosedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "BadClass.php")
	uri := "file://" + filepath.ToSlash(filePath)
	text := "<?php\nclass Bad_Class {}\n"

	if err := os.WriteFile(filePath, []byte(text), 0o644); err != nil {
		t.Fatalf("write workspace file: %v", err)
	}

	var out synchronizedBuffer
	srv := &Server{out: &out}
	h := NewHandler(srv)
	h.idx.SetWorkspaceFolders([]indexer.WorkspaceFolder{{URI: "file://" + filepath.ToSlash(tmpDir), Name: "tmp"}})

	h.indexAndPublishWorkspaceDiagnostics()

	payload := out.String()
	if !strings.Contains(payload, uri) {
		t.Fatalf("expected workspace diagnostics to include %s, got %q", uri, payload)
	}
	if !strings.Contains(payload, "PSR1.Classes.ClassDeclaration.PascalCase") {
		t.Fatalf("expected workspace diagnostics payload to include the rule code, got %q", payload)
	}
}

func TestDidCloseRevertsToDiskDiagnostics(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "ClosedFile.php")
	uri := "file://" + filepath.ToSlash(filePath)
	diskText := "<?php\nclass Bad_Class {}\n"
	cleanText := "<?php\nclass GoodClass {}\n"

	if err := os.WriteFile(filePath, []byte(diskText), 0o644); err != nil {
		t.Fatalf("write workspace file: %v", err)
	}

	var out synchronizedBuffer
	srv := &Server{out: &out}
	h := NewHandler(srv)
	h.documents.Open(lsp.TextDocumentItem{
		URI:        uri,
		LanguageID: "php",
		Version:    1,
		Text:       cleanText,
	})
	h.idx.IndexDocument(uri, cleanText)

	params, err := json.Marshal(lsp.DidCloseTextDocumentParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		t.Fatalf("marshal didClose: %v", err)
	}

	h.HandleNotification("textDocument/didClose", params)

	payload := out.String()
	if !strings.Contains(payload, uri) {
		t.Fatalf("expected didClose to publish diagnostics for %s, got %q", uri, payload)
	}
	if !strings.Contains(payload, "PSR1.Classes.ClassDeclaration.PascalCase") {
		t.Fatalf("expected didClose to republish disk diagnostics, got %q", payload)
	}
	if strings.Contains(payload, `"diagnostics":[]`) {
		t.Fatalf("expected didClose to preserve disk diagnostics instead of clearing them, got %q", payload)
	}
}

func TestWorkspaceDiagnosticsStopsAtLimit(t *testing.T) {
	var out synchronizedBuffer
	srv := &Server{out: &out}
	h := NewHandler(srv)
	perFileDiagnostics := len(h.prov.Diagnostics.Analyse("file:///workspace/File0.php", "<?php\nclass Bad_Class_0 {}\n"))
	if perFileDiagnostics == 0 {
		t.Fatal("expected synthetic test file to emit diagnostics")
	}

	scan := newWorkspaceDiagnosticsScanState()
	published := 0
	for index := 0; index < workspaceDiagnosticsLimit+5; index++ {
		uri := "file:///workspace/File" + strconv.Itoa(index) + ".php"
		text := "<?php\nclass Bad_Class_" + strconv.Itoa(index) + " {}\n"
		if h.publishWorkspaceDocumentDiagnosticsForScan(uri, text, scan) {
			published++
		}
	}

	expectedPublished := workspaceDiagnosticsLimit / perFileDiagnostics
	expectedTotal := expectedPublished * perFileDiagnostics
	if published != expectedPublished {
		t.Fatalf("expected scan to stop after %d published files, got %d", expectedPublished, published)
	}
	if !scan.capped() {
		t.Fatal("expected scan state to be marked capped")
	}
	if scan.total() != expectedTotal {
		t.Fatalf("expected scan total %d, got %d", expectedTotal, scan.total())
	}
}
