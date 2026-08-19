package providers

import (
	"sync"

	"github.com/ayanozturk/go-php-parser/analyse"
	"github.com/ayanozturk/go-php-parser/ast"
	goplexer "github.com/ayanozturk/go-php-parser/lexer"
	goparser "github.com/ayanozturk/go-php-parser/parser"

	"github.com/ayanozturk/vscode-php-strom/indexer"
)

type semanticSnapshot struct {
	text   string
	nodes  []ast.Node
	errors []string
}

type semanticDocumentCache struct {
	mu    sync.RWMutex
	byURI map[string]semanticSnapshot
}

func newSemanticDocumentCache() *semanticDocumentCache {
	return &semanticDocumentCache{byURI: make(map[string]semanticSnapshot)}
}

func (c *semanticDocumentCache) snapshot(uri, text string) semanticSnapshot {
	if c == nil {
		return parseSemanticSnapshot(text)
	}

	c.mu.RLock()
	snapshot, ok := c.byURI[uri]
	c.mu.RUnlock()
	if ok && snapshot.text == text {
		return snapshot
	}

	snapshot = parseSemanticSnapshot(text)
	snapshot.text = text

	c.mu.Lock()
	c.byURI[uri] = snapshot
	c.mu.Unlock()
	return snapshot
}

func (c *semanticDocumentCache) analysisContext(idx *indexer.WorkspaceIndexer) *analyse.AnalysisContext {
	ctx := &analyse.AnalysisContext{}
	if idx != nil {
		if project := idx.ProjectIndex(); project != nil {
			ctx.Project = project
			ctx.Resolver = projectFallbackResolver{project: project, fallback: workspaceSymbolResolver{idx: idx}}
		} else {
			ctx.Resolver = workspaceSymbolResolver{idx: idx}
		}
	}
	return ctx
}

func (c *semanticDocumentCache) analysisContextForFile(idx *indexer.WorkspaceIndexer, filename, text string, nodes []ast.Node) *analyse.AnalysisContext {
	ctx := &analyse.AnalysisContext{}
	if idx != nil {
		if project := idx.ProjectIndexForFile(filename, text, nodes); project != nil {
			ctx.Project = project
			ctx.Resolver = projectFallbackResolver{project: project, fallback: workspaceSymbolResolver{idx: idx}}
		} else {
			ctx.Resolver = workspaceSymbolResolver{idx: idx}
		}
	}
	return ctx
}

func parseSemanticSnapshot(text string) semanticSnapshot {
	l := goplexer.New(text)
	parser := goparser.New(l, false)
	nodes := parser.Parse()
	errs := append([]string(nil), parser.Errors()...)
	return semanticSnapshot{nodes: nodes, errors: errs}
}
