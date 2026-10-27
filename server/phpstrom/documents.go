package phpstrom

import (
	"strings"
	"sync"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/ayanozturk/vscode-php-strom/lsp"
)

// Document represents an open text document managed by the client.
type Document struct {
	URI     string
	Version int
	Text    string
}

// DocumentStore is a thread-safe in-memory store of open documents.
type DocumentStore struct {
	mu   sync.RWMutex
	docs map[string]*Document
}

func NewDocumentStore() *DocumentStore {
	return &DocumentStore{docs: make(map[string]*Document)}
}

func (s *DocumentStore) Open(item lsp.TextDocumentItem) *Document {
	doc := &Document{URI: item.URI, Version: item.Version, Text: item.Text}
	s.mu.Lock()
	s.docs[item.URI] = doc
	s.mu.Unlock()
	return doc
}

func (s *DocumentStore) Change(uri string, version int, changes []lsp.TextDocumentContentChangeEvent) *Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.docs[uri]
	if !ok {
		doc = &Document{URI: uri}
		s.docs[uri] = doc
	}
	doc.Version = version
	for _, c := range changes {
		if c.Range == nil {
			doc.Text = c.Text
		} else {
			doc.Text = applyEdit(doc.Text, *c.Range, c.Text)
		}
	}
	return doc
}

func (s *DocumentStore) Get(uri string) (*Document, bool) {
	s.mu.RLock()
	doc, ok := s.docs[uri]
	s.mu.RUnlock()
	return doc, ok
}

func (s *DocumentStore) Snapshot(uri string) (Document, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, ok := s.docs[uri]
	if !ok {
		return Document{}, false
	}
	return *doc, true
}

func (s *DocumentStore) SetText(uri string, text string) (*Document, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.docs[uri]
	if !ok {
		return nil, false
	}
	doc.Text = text
	return doc, true
}

func (s *DocumentStore) Close(uri string) {
	s.mu.Lock()
	delete(s.docs, uri)
	s.mu.Unlock()
}

func applyEdit(src string, r lsp.Range, newText string) string {
	startOff := byteOffsetFromUTF16Position(src, int(r.Start.Line), int(r.Start.Character))
	endOff := byteOffsetFromUTF16Position(src, int(r.End.Line), int(r.End.Character))
	if startOff < 0 || endOff < 0 || startOff > endOff || endOff > len(src) {
		return src
	}
	return src[:startOff] + newText + src[endOff:]
}

// byteOffsetFromUTF16Position maps an LSP (line, character) position — where
// character is a UTF-16 code unit offset — onto a byte offset in src.
func byteOffsetFromUTF16Position(src string, line, char int) int {
	if line < 0 {
		return 0
	}
	off := 0
	for i := 0; i < line; i++ {
		idx := strings.IndexByte(src[off:], '\n')
		if idx < 0 {
			return len(src)
		}
		off += idx + 1
	}
	// Exclude the trailing newline from the editable line content when present.
	content := src[off:]
	if idx := strings.IndexByte(content, '\n'); idx >= 0 {
		content = content[:idx]
		content = strings.TrimSuffix(content, "\r")
	}
	if char < 0 {
		char = 0
	}
	utf16Column := 0
	bytePos := 0
	for _, r := range content {
		if utf16Column >= char {
			return off + bytePos
		}
		size := utf8.RuneLen(r)
		utf16Column += utf16.RuneLen(r)
		bytePos += size
		if utf16Column >= char {
			return off + bytePos
		}
	}
	return off + len(content)
}
