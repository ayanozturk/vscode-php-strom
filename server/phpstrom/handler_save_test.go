package phpstrom

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ayanozturk/vscode-php-strom/lsp"
)

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
	var out bytes.Buffer
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
}
