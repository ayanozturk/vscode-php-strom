package phpstrom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/ayanozturk/vscode-php-strom/lsp"
)

func TestServerPreservesDocumentNotificationOrder(t *testing.T) {
	uri := "file:///workspace/Ordered.php"
	openedText := "<?php\n" + strings.Repeat("// startup payload\n", 20_000)
	changedText := "<?php\nclass CurrentVersion {}\n"

	openParams, err := json.Marshal(lsp.DidOpenTextDocumentParams{TextDocument: lsp.TextDocumentItem{
		URI: uri, LanguageID: "php", Version: 1, Text: openedText,
	}})
	if err != nil {
		t.Fatalf("marshal didOpen: %v", err)
	}
	changeParams, err := json.Marshal(lsp.DidChangeTextDocumentParams{
		TextDocument:   lsp.VersionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []lsp.TextDocumentContentChangeEvent{{Text: changedText}},
	})
	if err != nil {
		t.Fatalf("marshal didChange: %v", err)
	}

	input := bytes.NewBuffer(nil)
	writeTestNotification(t, input, "textDocument/didOpen", openParams)
	writeTestNotification(t, input, "textDocument/didChange", changeParams)

	server := NewServer(input, io.Discard)
	if err := server.Run(); err != nil {
		t.Fatalf("server run: %v", err)
	}

	doc, ok := server.handler.documents.Snapshot(uri)
	if !ok {
		t.Fatal("expected ordered document notifications to leave the document open")
	}
	if doc.Version != 2 || doc.Text != changedText {
		t.Fatalf("expected latest document version after ordered dispatch, got version %d", doc.Version)
	}
}

func writeTestNotification(t *testing.T, target *bytes.Buffer, method string, params json.RawMessage) {
	t.Helper()
	payload, err := json.Marshal(lsp.NotificationMessage{JSONRPC: "2.0", Method: method, Params: params})
	if err != nil {
		t.Fatalf("marshal %s notification: %v", method, err)
	}
	if _, err := fmt.Fprintf(target, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		t.Fatalf("write %s header: %v", method, err)
	}
	if _, err := target.Write(payload); err != nil {
		t.Fatalf("write %s payload: %v", method, err)
	}
}
