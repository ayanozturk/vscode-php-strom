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

type semanticAnalysisSnapshot struct {
	text             string
	semanticRevision uint64
	snapshot         *analyse.SemanticSnapshot
}

type semanticDocumentCache struct {
	mu       sync.RWMutex
	byURI    map[string]semanticSnapshot
	analysis map[string]semanticAnalysisSnapshot
}

func newSemanticDocumentCache() *semanticDocumentCache {
	return &semanticDocumentCache{
		byURI:    make(map[string]semanticSnapshot),
		analysis: make(map[string]semanticAnalysisSnapshot),
	}
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
	delete(c.analysis, uri)
	c.mu.Unlock()
	return snapshot
}

func (c *semanticDocumentCache) forget(uri string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.byURI, uri)
	delete(c.analysis, uri)
	c.mu.Unlock()
}

func (c *semanticDocumentCache) analysisContextForFile(idx *indexer.WorkspaceIndexer, cacheKey, filename, text string, nodes []ast.Node) *analyse.AnalysisContext {
	var project *analyse.ProjectIndex
	var revision uint64
	if idx != nil {
		project, revision = idx.ProjectIndexSnapshotForFile(filename, text, nodes)
	}

	if c != nil && cacheKey != "" {
		c.mu.RLock()
		cached, ok := c.analysis[cacheKey]
		c.mu.RUnlock()
		if ok && cached.text == text && cached.semanticRevision == revision && cached.snapshot != nil {
			return analysisContextFromSnapshot(cached.snapshot, project, idx)
		}
	}

	parsed := map[string][]ast.Node{filename: nodes}
	var semantic *analyse.SemanticSnapshot
	var err error
	if project != nil {
		semantic, err = analyse.NewSemanticSnapshotWithIndex(project, parsed, nil, []string{filename})
	} else {
		semantic, err = analyse.NewSemanticSnapshot(parsed, nil)
	}
	if err != nil {
		return analysisContextFromSnapshot(nil, project, idx)
	}

	if c != nil && cacheKey != "" {
		c.mu.Lock()
		c.analysis[cacheKey] = semanticAnalysisSnapshot{
			text:             text,
			semanticRevision: revision,
			snapshot:         semantic,
		}
		c.mu.Unlock()
	}
	return analysisContextFromSnapshot(semantic, project, idx)
}

func analysisContextFromSnapshot(snapshot *analyse.SemanticSnapshot, project *analyse.ProjectIndex, idx *indexer.WorkspaceIndexer) *analyse.AnalysisContext {
	ctx := &analyse.AnalysisContext{}
	if snapshot != nil {
		ctx = snapshot.NewAnalysisContext()
	}
	if idx == nil {
		return ctx
	}
	if project != nil {
		ctx.Resolver = projectFallbackResolver{project: project, fallback: workspaceSymbolResolver{idx: idx}}
	} else {
		ctx.Resolver = workspaceSymbolResolver{idx: idx}
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
