package phpls

import (
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
