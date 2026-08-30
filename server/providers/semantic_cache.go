package providers

import (
	"sync"
	"sync/atomic"

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
	trace    semanticCacheTraceCounters
}

type semanticCacheTraceCounters struct {
	parseHits      atomic.Uint64
	parseMisses    atomic.Uint64
	semanticHits   atomic.Uint64
	semanticMisses atomic.Uint64
}

// SemanticCacheTraceSnapshot is cumulative cache accounting for interactive
// document parsing and semantic snapshot construction.
type SemanticCacheTraceSnapshot struct {
	ParseHits      uint64 `json:"parseHits"`
	ParseMisses    uint64 `json:"parseMisses"`
	SemanticHits   uint64 `json:"semanticHits"`
	SemanticMisses uint64 `json:"semanticMisses"`
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
		c.trace.parseHits.Add(1)
		return snapshot
	}
	c.trace.parseMisses.Add(1)

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
			c.trace.semanticHits.Add(1)
			return analysisContextFromSnapshot(cached.snapshot, project, idx)
		}
		c.trace.semanticMisses.Add(1)
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

func (c *semanticDocumentCache) traceSnapshot() SemanticCacheTraceSnapshot {
	if c == nil {
		return SemanticCacheTraceSnapshot{}
	}
	return SemanticCacheTraceSnapshot{
		ParseHits:      c.trace.parseHits.Load(),
		ParseMisses:    c.trace.parseMisses.Load(),
		SemanticHits:   c.trace.semanticHits.Load(),
		SemanticMisses: c.trace.semanticMisses.Load(),
	}
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
