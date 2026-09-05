package indexer

import (
	"log"
	"os"
	"path/filepath"

	"github.com/ayanozturk/go-php-parser/phpstubs"
)

func (wi *WorkspaceIndexer) loadConfiguredStubs() {
	if len(wi.cfg.Stubs) == 0 {
		return
	}

	for _, name := range wi.cfg.Stubs {
		data, uri, err := wi.readStub(name)
		if err != nil {
			continue
		}
		parsed := ParseSourceForIndex(uri, string(data))
		if len(parsed.Errors) > 0 {
			// The parser is intentionally error-tolerant and can return a useful
			// declaration AST even when a stub contains syntax it does not yet
			// understand. Keep the recovered symbols instead of dropping the
			// entire extension stub because one declaration was unsupported.
			log.Printf("[indexer] indexing recovered stub %s with %d parser error(s)", name, len(parsed.Errors))
		}
		wi.index.PutFile(parsed.URI, parsed.Symbols)
		wi.mu.Lock()
		wi.stubURIs = append(wi.stubURIs, parsed.URI)
		wi.mu.Unlock()
	}
}

func (wi *WorkspaceIndexer) readStub(name string) ([]byte, string, error) {
	if wi.cfg.StubsPath != "" {
		stubPath := wi.stubFilePath(name)
		data, err := os.ReadFile(stubPath)
		if err == nil {
			return data, pathToURI(stubPath), nil
		}
	}
	data, err := phpstubs.Read(wi.cfg.PHPVersion, name)
	if err != nil {
		return nil, "", err
	}
	return data, phpstubs.FileName(wi.cfg.PHPVersion, name), nil
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
