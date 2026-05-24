package indexer

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

func (wi *WorkspaceIndexer) loadConfiguredStubs() {
	if wi.cfg.StubsPath == "" || len(wi.cfg.Stubs) == 0 {
		return
	}

	for _, name := range wi.cfg.Stubs {
		stubPath := wi.stubFilePath(name)
		data, err := os.ReadFile(stubPath)
		if err != nil {
			continue
		}
		parsed := ParseSourceForIndex(pathToURI(stubPath), string(data))
		if len(parsed.Errors) > 0 {
			log.Printf("[indexer] skipping invalid stub %s: %s", name, strings.Join(parsed.Errors, "; "))
			continue
		}
		wi.index.PutFile(parsed.URI, parsed.Symbols)
		wi.mu.Lock()
		wi.stubURIs = append(wi.stubURIs, parsed.URI)
		wi.mu.Unlock()
	}
}

func (wi *WorkspaceIndexer) stubFilePath(name string) string {
	if wi.cfg.PHPVersion != "" {
		versioned := filepath.Join(wi.cfg.StubsPath, wi.cfg.PHPVersion, name+".php")
		if _, err := os.Stat(versioned); err == nil {
			return versioned
		}
	}
	return filepath.Join(wi.cfg.StubsPath, name+".php")
}
