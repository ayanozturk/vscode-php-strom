package phpstrom

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ayanozturk/go-php-parser/overrides"
	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
	"github.com/ayanozturk/vscode-php-strom/providers"
)

// Handler routes LSP requests and notifications to the appropriate provider.
type Handler struct {
	srv       *Server
	cfg       *Config
	documents *DocumentStore
	idx       *indexer.WorkspaceIndexer
	prov      *providers.Registry
	runtimeMu sync.RWMutex

	workspaceDiagnosticsMu sync.Mutex
	workspaceIndexMu       sync.Mutex
	workspaceRequestMu     sync.Mutex
	workspaceRequestActive bool
	workspaceRequestQueued bool
	workspaceRequestIndex  bool
	publishedDiagnosticsMu sync.Mutex
	publishedDiagnostics   map[string]struct{}
	documentAnalysisMu     sync.Mutex
	documentAnalysisTimers map[string]*time.Timer
	documentAnalysisSem    chan struct{}
	initialIndexing        atomic.Bool
	initialIndexDone       chan struct{}
	initialIndexDoneOnce   sync.Once
}

const workspaceDiagnosticsLimit = 50_000
const onTypeAnalysisDelay = 150 * time.Millisecond
const maxDocumentAnalysisWorkers = 2

type saveAnalysisFinishedParams struct {
	URI       string `json:"uri"`
	Published bool   `json:"published"`
}

type workspaceDiagnosticsFinishedParams struct {
	FilesWithDiagnostics int  `json:"filesWithDiagnostics"`
	TotalDiagnostics     int  `json:"totalDiagnostics"`
	Capped               bool `json:"capped"`
	Applied              bool `json:"applied"`
}

type workspaceDiagnosticResult struct {
	diagnostics []lsp.Diagnostic
	version     int
	text        string
	open        bool
}

func NewHandler(srv *Server) *Handler {
	cfg := DefaultConfig()
	docs := NewDocumentStore()
	idx := indexer.New(cfg.toIndexerConfig())
	prov := providers.NewRegistry(idx, cfg.toProviderConfig(nil))

	h := &Handler{
		srv:                    srv,
		cfg:                    cfg,
		documents:              docs,
		idx:                    idx,
		prov:                   prov,
		publishedDiagnostics:   make(map[string]struct{}),
		documentAnalysisTimers: make(map[string]*time.Timer),
		documentAnalysisSem:    make(chan struct{}, maxDocumentAnalysisWorkers),
		initialIndexDone:       make(chan struct{}),
	}

	idx.OnIndexingStart(func() { srv.Notify("phpstrom/indexingStarted", nil) })
	idx.OnIndexingProgress(func(done, total int) {
		srv.Notify("phpstrom/indexingProgress", map[string]int{"done": done, "total": total})
	})
	idx.OnIndexingDone(func(summary indexer.IndexingSummary) {
		srv.Notify("phpstrom/indexingFinished", map[string]any{
			"fileCount":       summary.FilesIndexed,
			"filesDiscovered": summary.FilesDiscovered,
			"symbolCount":     summary.SymbolsIndexed,
			"linesScanned":    summary.LinesScanned,
			"bytesScanned":    summary.BytesScanned,
			"durationMs":      summary.Duration.Milliseconds(),
			"filesPerSecond":  filesPerSecond(summary),
			"linesPerSecond":  linesPerSecond(summary),
		})
	})

	return h
}

func filesPerSecond(summary indexer.IndexingSummary) float64 {
	seconds := summary.Duration.Seconds()
	if seconds <= 0 {
		return 0
	}
	return float64(summary.FilesIndexed) / seconds
}

func linesPerSecond(summary indexer.IndexingSummary) float64 {
	seconds := summary.Duration.Seconds()
	if seconds <= 0 {
		return 0
	}
	return float64(summary.LinesScanned) / seconds
}

// HandleRequest processes a request with an ID and returns a result.
func (h *Handler) HandleRequest(method string, raw json.RawMessage) (any, *lsp.ResponseError) {
	switch method {
	case "initialize":
		return h.initialize(raw)
	case "shutdown":
		return nil, nil
	case "textDocument/completion":
		return h.completion(raw)
	case "completionItem/resolve":
		return h.completionResolve(raw)
	case "textDocument/hover":
		return h.hover(raw)
	case "textDocument/definition":
		return h.definition(raw)
	case "textDocument/declaration":
		return h.declaration(raw)
	case "textDocument/typeDefinition":
		return h.typeDefinition(raw)
	case "textDocument/implementation":
		return h.implementation(raw)
	case "textDocument/references":
		return h.references(raw)
	case "textDocument/documentHighlight":
		return h.documentHighlight(raw)
	case "textDocument/documentSymbol":
		return h.documentSymbol(raw)
	case "textDocument/signatureHelp":
		return h.signatureHelp(raw)
	case "textDocument/formatting":
		return h.formatting(raw)
	case "textDocument/rangeFormatting":
		return h.rangeFormatting(raw)
	case "textDocument/rename":
		return h.rename(raw)
	case "textDocument/prepareRename":
		return h.prepareRename(raw)
	case "textDocument/foldingRange":
		return h.foldingRange(raw)
	case "textDocument/selectionRange":
		return h.selectionRange(raw)
	case "textDocument/codeAction":
		return h.codeAction(raw)
	case "codeAction/resolve":
		return h.codeActionResolve(raw)
	case "textDocument/codeLens":
		return h.codeLens(raw)
	case "codeLens/resolve":
		return h.codeLensResolve(raw)
	case "textDocument/inlayHint":
		return h.inlayHint(raw)
	case "textDocument/documentLink":
		return h.documentLink(raw)
	case "textDocument/inlineValue":
		return h.inlineValue(raw)
	case "textDocument/prepareTypeHierarchy":
		return h.typeHierarchyPrepare(raw)
	case "typeHierarchy/supertypes":
		return h.typeHierarchySupertypes(raw)
	case "typeHierarchy/subtypes":
		return h.typeHierarchySubtypes(raw)
	case "workspace/symbol":
		return h.workspaceSymbol(raw)
	default:
		return nil, &lsp.ResponseError{Code: lsp.MethodNotFound, Message: "method not found: " + method}
	}
}

// HandleNotification processes a notification (no response needed).
func (h *Handler) HandleNotification(method string, raw json.RawMessage) {
	switch method {
	case "initialized":
		if h.initialIndexing.CompareAndSwap(false, true) {
			go func() {
				h.runtimeMu.RLock()
				scanOnStart := h.cfg.Diagnostics.WorkspaceScanOnStart
				h.runtimeMu.RUnlock()
				if scanOnStart {
					h.indexAndPublishWorkspaceDiagnostics()
				} else {
					h.indexWorkspace()
				}
				h.initialIndexing.Store(false)
				h.initialIndexDoneOnce.Do(func() { close(h.initialIndexDone) })
			}()
		}

	case "textDocument/didOpen":
		var p lsp.DidOpenTextDocumentParams
		if err := json.Unmarshal(raw, &p); err != nil {
			log.Printf("didOpen: %v", err)
			return
		}
		doc := h.documents.Open(p.TextDocument)
		if h.cfg.Diagnostics.Run == "onType" {
			h.scheduleDocumentAnalysis(doc.URI, doc.Version, 0)
		} else {
			h.scheduleDocumentIndex(doc.URI, doc.Version, 0)
		}

	case "textDocument/didChange":
		var p lsp.DidChangeTextDocumentParams
		if err := json.Unmarshal(raw, &p); err != nil {
			log.Printf("didChange: %v", err)
			return
		}
		doc := h.documents.Change(p.TextDocument.URI, p.TextDocument.Version, p.ContentChanges)
		if h.cfg.Diagnostics.Run == "onType" {
			h.scheduleDocumentAnalysis(doc.URI, doc.Version, onTypeAnalysisDelay)
		} else {
			h.scheduleDocumentIndex(doc.URI, doc.Version, onTypeAnalysisDelay)
		}

	case "textDocument/didSave":
		var p lsp.DidSaveTextDocumentParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return
		}
		if doc, ok := h.documents.Get(p.TextDocument.URI); ok {
			h.cancelDocumentAnalysis(p.TextDocument.URI)
			if text, err := h.readDocumentTextFromDisk(p.TextDocument.URI); err == nil {
				if updated, ok := h.documents.SetText(p.TextDocument.URI, text); ok {
					doc = updated
				}
			}
			published := false
			h.runDocumentAnalysis(func() {
				h.idx.IndexDocument(doc.URI, doc.Text)
				published = h.publishDiagnostics(doc.URI, doc.Text, doc.Version)
			})
			h.srv.Notify("phpstrom/saveAnalysisFinished", saveAnalysisFinishedParams{
				URI:       doc.URI,
				Published: published,
			})
		}

	case "textDocument/didClose":
		var p lsp.DidCloseTextDocumentParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return
		}
		_, wasOpen := h.documents.Get(p.TextDocument.URI)
		h.documents.Close(p.TextDocument.URI)
		h.cancelDocumentAnalysis(p.TextDocument.URI)
		h.prov.Diagnostics.Forget(p.TextDocument.URI)
		if wasOpen {
			if text, err := h.readDocumentTextFromDisk(p.TextDocument.URI); err == nil {
				h.idx.IndexDocument(p.TextDocument.URI, text)
				h.publishWorkspaceDocumentDiagnostics(p.TextDocument.URI, text)
				return
			}
		}
		h.idx.RemoveDocument(p.TextDocument.URI)
		h.notifyDiagnostics(p.TextDocument.URI, []lsp.Diagnostic{})

	case "workspace/didChangeConfiguration":
		var p struct {
			Settings map[string]any `json:"settings"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return
		}
		h.runtimeMu.Lock()
		beforeDiagnostics := workspaceDiagnosticsFingerprint(h.cfg)
		beforeStubs := strings.Join(h.cfg.Stubs, "\x00")
		beforePHPVersion := h.cfg.resolvePHPVersion(h.idx.WorkspaceFolders())
		h.cfg.Update(p.Settings)
		folders := h.idx.WorkspaceFolders()
		phpVersion := h.cfg.resolvePHPVersion(folders)
		h.idx.UpdateConfig(h.cfg.toIndexerConfig())
		if beforeStubs != strings.Join(h.cfg.Stubs, "\x00") || beforePHPVersion != phpVersion {
			h.idx.SetStubs(h.cfg.stubsPath(), h.cfg.Stubs, phpVersion)
		}
		h.prov = providers.NewRegistry(h.idx, h.cfg.toProviderConfig(folders))
		afterDiagnostics := workspaceDiagnosticsFingerprint(h.cfg)
		h.runtimeMu.Unlock()
		if !bytes.Equal(beforeDiagnostics, afterDiagnostics) {
			h.requestWorkspaceDiagnostics(false)
		}

	case "workspace/didChangeWorkspaceFolders":
		var p lsp.DidChangeWorkspaceFoldersParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return
		}
		folders := h.idx.WorkspaceFolders()
		removed := make(map[string]struct{}, len(p.Event.Removed))
		for _, folder := range p.Event.Removed {
			removed[string(folder.URI)] = struct{}{}
		}
		next := make([]indexer.WorkspaceFolder, 0, len(folders)+len(p.Event.Added))
		for _, folder := range folders {
			if _, ok := removed[folder.URI]; !ok {
				next = append(next, folder)
			}
		}
		for _, folder := range p.Event.Added {
			next = append(next, indexer.WorkspaceFolder{URI: string(folder.URI), Name: folder.Name})
		}
		h.idx.SetWorkspaceFolders(next)
		h.runtimeMu.Lock()
		h.prov = providers.NewRegistry(h.idx, h.cfg.toProviderConfig(next))
		scanOnStart := h.cfg.Diagnostics.WorkspaceScanOnStart
		h.runtimeMu.Unlock()
		go func() {
			h.indexWorkspace()
			if scanOnStart {
				h.requestWorkspaceDiagnostics(false)
				return
			}
			seen := make(map[string]struct{})
			for _, uri := range h.idx.WorkspaceFileURIs() {
				seen[uri] = struct{}{}
			}
			h.clearDiagnosticsOutsideWorkspace(seen)
		}()

	case "phpstrom/indexWorkspace":
		go h.indexWorkspace()

	case "phpstrom/scanWorkspaceDiagnostics":
		h.requestWorkspaceDiagnostics(true)

	case "exit":
		h.cancelAllDocumentAnalysis()
	}
}

// ─── initialize ───────────────────────────────────────────────────────────────

func (h *Handler) initialize(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.InitializeParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}

	folders := make([]indexer.WorkspaceFolder, len(p.WorkspaceFolders))
	for i, f := range p.WorkspaceFolders {
		folders[i] = indexer.WorkspaceFolder{URI: f.URI, Name: f.Name}
	}
	h.idx.SetWorkspaceFolders(folders)
	h.cfg.ApplyInitOptions(p.InitializationOptions)
	h.idx.UpdateConfig(h.cfg.toIndexerConfig())
	phpVersion := h.cfg.resolvePHPVersion(folders)
	h.idx.SetStubs(h.cfg.stubsPath(), h.cfg.Stubs, phpVersion)
	h.prov = providers.NewRegistry(h.idx, h.cfg.toProviderConfig(h.idx.WorkspaceFolders()))

	return lsp.InitializeResult{
		Capabilities: lsp.ServerCapabilities{
			TextDocumentSync: &lsp.TextDocumentSyncOptions{
				OpenClose: true,
				Change:    2,
				Save: &struct {
					IncludeText bool `json:"includeText"`
				}{false},
			},
			CompletionProvider: &lsp.CompletionOptions{
				TriggerCharacters: []string{"$", ">", ":", "\\", "/", "'", `"`, "*", ".", "<"},
				ResolveProvider:   true,
			},
			SignatureHelpProvider: &lsp.SignatureHelpOptions{
				TriggerCharacters:   []string{"(", ",", ":"},
				RetriggerCharacters: []string{","},
			},
			HoverProvider:                   true,
			DefinitionProvider:              true,
			TypeDefinitionProvider:          true,
			ImplementationProvider:          true,
			ReferencesProvider:              true,
			DocumentHighlightProvider:       true,
			DocumentSymbolProvider:          true,
			WorkspaceSymbolProvider:         true,
			DocumentFormattingProvider:      true,
			DocumentRangeFormattingProvider: true,
			FoldingRangeProvider:            true,
			SelectionRangeProvider:          true,
			DeclarationProvider:             true,
			TypeHierarchyProvider:           true,
			InlineValueProvider:             true,
			RenameProvider:                  &lsp.RenameOptions{PrepareProvider: true},
			CodeActionProvider: &lsp.CodeActionOptions{
				CodeActionKinds: []string{"quickfix", "refactor", "source.organizeImports"},
				ResolveProvider: true,
			},
			CodeLensProvider:     &lsp.CodeLensOptions{ResolveProvider: true},
			DocumentLinkProvider: &lsp.DocumentLinkOptions{ResolveProvider: false},
			InlayHintProvider:    &lsp.InlayHintOptions{ResolveProvider: false},
			Workspace: &lsp.WorkspaceCapabilities{
				WorkspaceFolders: &lsp.WorkspaceFoldersServerCapabilities{
					Supported:           true,
					ChangeNotifications: true,
				},
			},
		},
		ServerInfo: &lsp.ServerInfo{Name: "phpstrom", Version: "0.1.0"},
	}, nil
}

func (h *Handler) publishDiagnostics(uri, text string, version int) bool {
	h.runtimeMu.RLock()
	diagnosticsProvider := h.prov.Diagnostics
	h.runtimeMu.RUnlock()
	diags := diagnosticsProvider.Analyse(uri, text)
	if diags == nil {
		diags = []lsp.Diagnostic{}
	}
	current, ok := h.documents.Snapshot(uri)
	if !ok || current.Version != version || current.Text != text {
		return false
	}
	h.notifyDiagnostics(uri, diags)
	return true
}

func (h *Handler) scheduleDocumentAnalysis(uri string, version int, delay time.Duration) {
	h.scheduleDocumentWork(uri, version, delay, true)
}

func (h *Handler) scheduleDocumentIndex(uri string, version int, delay time.Duration) {
	h.scheduleDocumentWork(uri, version, delay, false)
}

func (h *Handler) scheduleDocumentWork(uri string, version int, delay time.Duration, publishDiagnostics bool) {
	h.documentAnalysisMu.Lock()
	if timer := h.documentAnalysisTimers[uri]; timer != nil {
		timer.Stop()
	}
	var timer *time.Timer
	timer = time.AfterFunc(delay, func() {
		h.documentAnalysisMu.Lock()
		if h.documentAnalysisTimers[uri] == timer {
			delete(h.documentAnalysisTimers, uri)
		}
		h.documentAnalysisMu.Unlock()

		if h.initialIndexing.Load() {
			<-h.initialIndexDone
		}

		doc, ok := h.documents.Snapshot(uri)
		if !ok || doc.Version != version {
			return
		}
		h.runDocumentAnalysis(func() {
			h.idx.IndexDocument(doc.URI, doc.Text)
			if publishDiagnostics {
				h.publishDiagnostics(doc.URI, doc.Text, doc.Version)
			}
		})
	})
	h.documentAnalysisTimers[uri] = timer
	h.documentAnalysisMu.Unlock()
}

func (h *Handler) runDocumentAnalysis(fn func()) {
	h.documentAnalysisSem <- struct{}{}
	defer func() { <-h.documentAnalysisSem }()
	fn()
}

func (h *Handler) cancelDocumentAnalysis(uri string) {
	h.documentAnalysisMu.Lock()
	if timer := h.documentAnalysisTimers[uri]; timer != nil {
		timer.Stop()
		delete(h.documentAnalysisTimers, uri)
	}
	h.documentAnalysisMu.Unlock()
}

func (h *Handler) cancelAllDocumentAnalysis() {
	h.documentAnalysisMu.Lock()
	for uri, timer := range h.documentAnalysisTimers {
		timer.Stop()
		delete(h.documentAnalysisTimers, uri)
	}
	h.documentAnalysisMu.Unlock()
}

func (h *Handler) indexWorkspace() {
	h.workspaceIndexMu.Lock()
	defer h.workspaceIndexMu.Unlock()
	h.idx.IndexWorkspace()
}

func (h *Handler) requestWorkspaceDiagnostics(indexWorkspace bool) {
	h.workspaceRequestMu.Lock()
	h.workspaceRequestQueued = true
	h.workspaceRequestIndex = h.workspaceRequestIndex || indexWorkspace
	if h.workspaceRequestActive {
		h.workspaceRequestMu.Unlock()
		return
	}
	h.workspaceRequestActive = true
	h.workspaceRequestMu.Unlock()

	go func() {
		for {
			h.workspaceRequestMu.Lock()
			if !h.workspaceRequestQueued {
				h.workspaceRequestActive = false
				h.workspaceRequestMu.Unlock()
				return
			}
			index := h.workspaceRequestIndex
			h.workspaceRequestQueued = false
			h.workspaceRequestIndex = false
			h.workspaceRequestMu.Unlock()

			h.runWorkspaceDiagnosticsScan(index)
		}
	}()
}

func (h *Handler) indexAndPublishWorkspaceDiagnostics() {
	h.workspaceDiagnosticsMu.Lock()
	defer h.workspaceDiagnosticsMu.Unlock()
	h.srv.Notify("phpstrom/workspaceDiagnosticsStarted", nil)
	totalFiles := len(h.idx.WorkspaceFileURIs())
	scan := newWorkspaceDiagnosticsScanState(totalFiles, func(done, total int) {
		h.srv.Notify("phpstrom/workspaceDiagnosticsProgress", map[string]int{"done": done, "total": total})
	})

	h.runtimeMu.RLock()
	diagnosticsEnabled := h.cfg.Diagnostics.Enable
	h.runtimeMu.RUnlock()
	if !diagnosticsEnabled {
		h.indexWorkspace()
		h.clearPublishedDiagnostics()
		h.notifyWorkspaceDiagnosticsFinished(scan, true)
		return
	}

	h.indexWorkspace()
	applied := h.runWorkspaceDiagnosticsLocked(scan)
	h.notifyWorkspaceDiagnosticsFinished(scan, applied)
}

func (h *Handler) runWorkspaceDiagnosticsScan(indexWorkspace bool) {
	h.workspaceDiagnosticsMu.Lock()
	defer h.workspaceDiagnosticsMu.Unlock()
	h.srv.Notify("phpstrom/workspaceDiagnosticsStarted", nil)
	totalFiles := len(h.idx.WorkspaceFileURIs())
	scan := newWorkspaceDiagnosticsScanState(totalFiles, func(done, total int) {
		h.srv.Notify("phpstrom/workspaceDiagnosticsProgress", map[string]int{"done": done, "total": total})
	})

	if indexWorkspace {
		h.indexWorkspace()
	}
	applied := h.runWorkspaceDiagnosticsLocked(scan)
	h.notifyWorkspaceDiagnosticsFinished(scan, applied)
}

func (h *Handler) notifyWorkspaceDiagnosticsFinished(scan *workspaceDiagnosticsScanState, applied bool) {
	h.srv.Notify("phpstrom/workspaceDiagnosticsFinished", workspaceDiagnosticsFinishedParams{
		FilesWithDiagnostics: h.publishedDiagnosticsCount(),
		TotalDiagnostics:     scan.total(),
		Capped:               scan.capped(),
		Applied:              applied,
	})
}

func workspaceDiagnosticsFingerprint(cfg *Config) []byte {
	value := struct {
		Enable             bool
		UndefinedSymbols   bool
		UndefinedVariables bool
		TypeErrors         bool
		Exclude            map[string][]string
		Overrides          overrides.RuleOverrides
		PHPVersion         string
		DocumentRoot       string
		Stubs              []string
	}{
		Enable:             cfg.Diagnostics.Enable,
		UndefinedSymbols:   cfg.Diagnostics.UndefinedSymbols,
		UndefinedVariables: cfg.Diagnostics.UndefinedVariables,
		TypeErrors:         cfg.Diagnostics.TypeErrors,
		Exclude:            cfg.Diagnostics.Exclude,
		Overrides:          cfg.Diagnostics.Overrides,
		PHPVersion:         cfg.Environment.EffectivePHPVersion,
		DocumentRoot:       cfg.Environment.DocumentRoot,
		Stubs:              cfg.Stubs,
	}
	encoded, _ := json.Marshal(value)
	return encoded
}

func (h *Handler) runWorkspaceDiagnosticsLocked(scan *workspaceDiagnosticsScanState) bool {
	h.runtimeMu.RLock()
	diagnosticsEnabled := h.cfg.Diagnostics.Enable
	diagnosticsProvider := h.prov.Diagnostics
	h.runtimeMu.RUnlock()
	if !diagnosticsEnabled {
		h.clearPublishedDiagnostics()
		return true
	}

	workspaceURIs := h.idx.WorkspaceFileURIs()
	seen := make(map[string]struct{}, len(workspaceURIs))
	results := make(map[string]workspaceDiagnosticResult, len(workspaceURIs))
	var resultsMu sync.Mutex
	jobs := make(chan string, len(workspaceURIs))
	workerCount := indexer.DiagnosticWorkerCountFor(len(workspaceURIs))
	var wg sync.WaitGroup

	for _, uri := range workspaceURIs {
		seen[uri] = struct{}{}
		jobs <- uri
	}
	close(jobs)

	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for uri := range jobs {
				if scan.capped() {
					return
				}
				if diagnosticsProvider.IgnoresAll(uri) {
					resultsMu.Lock()
					results[uri] = workspaceDiagnosticResult{diagnostics: []lsp.Diagnostic{}}
					resultsMu.Unlock()
					done := int(scan.processedFiles.Add(1))
					if scan.onProgress != nil && done%50 == 0 {
						scan.onProgress(done, scan.totalFiles)
					}
					continue
				}

				if doc, ok := h.documents.Snapshot(uri); ok {
					diags := diagnosticsProvider.Analyse(uri, doc.Text)
					if diags == nil {
						diags = []lsp.Diagnostic{}
					}
					if !scan.allow(len(diags)) {
						return
					}
					resultsMu.Lock()
					results[uri] = workspaceDiagnosticResult{diagnostics: diags, version: doc.Version, text: doc.Text, open: true}
					resultsMu.Unlock()
					done := int(scan.processedFiles.Add(1))
					if scan.onProgress != nil && done%50 == 0 {
						scan.onProgress(done, scan.totalFiles)
					}
					continue
				}

				text, err := h.readDocumentTextFromDisk(uri)
				if err != nil {
					resultsMu.Lock()
					results[uri] = workspaceDiagnosticResult{diagnostics: []lsp.Diagnostic{}}
					resultsMu.Unlock()
					done := int(scan.processedFiles.Add(1))
					if scan.onProgress != nil && done%50 == 0 {
						scan.onProgress(done, scan.totalFiles)
					}
					continue
				}
				diags := diagnosticsProvider.AnalyseTransient(uri, text)
				if diags == nil {
					diags = []lsp.Diagnostic{}
				}
				if !scan.allow(len(diags)) {
					return
				}
				resultsMu.Lock()
				results[uri] = workspaceDiagnosticResult{diagnostics: diags}
				resultsMu.Unlock()
				done := int(scan.processedFiles.Add(1))
				if scan.onProgress != nil && done%50 == 0 {
					scan.onProgress(done, scan.totalFiles)
				}
			}
		}()
	}

	wg.Wait()
	if scan.capped() {
		return false
	}

	resultURIs := make([]string, 0, len(results))
	for uri := range results {
		resultURIs = append(resultURIs, uri)
	}
	sort.Strings(resultURIs)
	for _, uri := range resultURIs {
		result := results[uri]
		if result.open {
			current, ok := h.documents.Snapshot(uri)
			if !ok || current.Version != result.version || current.Text != result.text {
				continue
			}
		}
		if len(result.diagnostics) == 0 && !h.hasPublishedDiagnostics(uri) {
			continue
		}
		h.notifyDiagnostics(uri, result.diagnostics)
	}
	h.clearDiagnosticsOutsideWorkspace(seen)
	return true
}

func (h *Handler) publishWorkspaceDocumentDiagnostics(uri, text string) bool {
	diags := h.prov.Diagnostics.AnalyseTransient(uri, text)
	if diags == nil {
		diags = []lsp.Diagnostic{}
	}
	h.notifyDiagnostics(uri, diags)
	return true
}

func (h *Handler) publishWorkspaceDocumentDiagnosticsForScan(uri, text string, scan *workspaceDiagnosticsScanState) bool {
	diags := h.prov.Diagnostics.AnalyseTransient(uri, text)
	if diags == nil {
		diags = []lsp.Diagnostic{}
	}
	if !scan.allow(len(diags)) {
		return false
	}
	h.notifyDiagnostics(uri, diags)
	return true
}

func (h *Handler) notifyDiagnostics(uri string, diagnostics []lsp.Diagnostic) {
	if diagnostics == nil {
		diagnostics = []lsp.Diagnostic{}
	}
	h.srv.Notify("textDocument/publishDiagnostics", lsp.PublishDiagnosticsParams{
		URI: uri, Diagnostics: diagnostics,
	})

	h.publishedDiagnosticsMu.Lock()
	defer h.publishedDiagnosticsMu.Unlock()
	if len(diagnostics) == 0 {
		delete(h.publishedDiagnostics, uri)
		return
	}
	h.publishedDiagnostics[uri] = struct{}{}
}

func (h *Handler) clearPublishedDiagnostics() {
	h.publishedDiagnosticsMu.Lock()
	uris := make([]string, 0, len(h.publishedDiagnostics))
	for uri := range h.publishedDiagnostics {
		uris = append(uris, uri)
	}
	h.publishedDiagnostics = make(map[string]struct{})
	h.publishedDiagnosticsMu.Unlock()

	for _, uri := range uris {
		h.srv.Notify("textDocument/publishDiagnostics", lsp.PublishDiagnosticsParams{
			URI: uri, Diagnostics: []lsp.Diagnostic{},
		})
	}
}

func (h *Handler) clearDiagnosticsOutsideWorkspace(seen map[string]struct{}) {
	h.publishedDiagnosticsMu.Lock()
	var stale []string
	for uri := range h.publishedDiagnostics {
		if _, ok := seen[uri]; ok {
			continue
		}
		stale = append(stale, uri)
		delete(h.publishedDiagnostics, uri)
	}
	h.publishedDiagnosticsMu.Unlock()

	for _, uri := range stale {
		h.srv.Notify("textDocument/publishDiagnostics", lsp.PublishDiagnosticsParams{
			URI: uri, Diagnostics: []lsp.Diagnostic{},
		})
	}
}

func (h *Handler) publishedDiagnosticsCount() int {
	h.publishedDiagnosticsMu.Lock()
	defer h.publishedDiagnosticsMu.Unlock()
	return len(h.publishedDiagnostics)
}

func (h *Handler) hasPublishedDiagnostics(uri string) bool {
	h.publishedDiagnosticsMu.Lock()
	defer h.publishedDiagnosticsMu.Unlock()
	_, ok := h.publishedDiagnostics[uri]
	return ok
}

type workspaceDiagnosticsScanState struct {
	totalDiagnostics atomic.Int64
	processedFiles   atomic.Int64
	totalFiles       int
	isCapped         atomic.Bool
	onProgress       func(done, total int)
}

func newWorkspaceDiagnosticsScanState(totalFiles int, onProgress func(done, total int)) *workspaceDiagnosticsScanState {
	return &workspaceDiagnosticsScanState{totalFiles: totalFiles, onProgress: onProgress}
}

func (s *workspaceDiagnosticsScanState) allow(count int) bool {
	if count <= 0 {
		return !s.capped()
	}
	for {
		current := s.totalDiagnostics.Load()
		if current >= workspaceDiagnosticsLimit {
			s.isCapped.Store(true)
			return false
		}
		if current+int64(count) > workspaceDiagnosticsLimit {
			s.isCapped.Store(true)
			return false
		}
		if s.totalDiagnostics.CompareAndSwap(current, current+int64(count)) {
			return true
		}
	}
}

func (s *workspaceDiagnosticsScanState) capped() bool {
	return s.isCapped.Load()
}

func (s *workspaceDiagnosticsScanState) total() int {
	return int(s.totalDiagnostics.Load())
}

func (h *Handler) readDocumentTextFromDisk(uri string) (string, error) {
	path, err := validatedDocumentPath(uri, h.idx.WorkspaceFolders())
	if err != nil {
		return "", err
	}
	h.runtimeMu.RLock()
	maxSize := h.cfg.Files.MaxSize
	h.runtimeMu.RUnlock()
	data, size, oversized, err := indexer.ReadFileWithinLimit(path, maxSize)
	if err != nil {
		return "", err
	}
	if oversized {
		return "", fmt.Errorf("document exceeds configured size limit: observed %d bytes, limit %d", size, h.cfg.Files.MaxSize)
	}
	return string(data), nil
}

func uriToPath(uri string) (string, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(parsed.Scheme, "file") || parsed.Opaque != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", os.ErrInvalid
	}

	path := filepath.FromSlash(parsed.Path)
	if runtime.GOOS == "windows" {
		if parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
			path = `\\` + parsed.Host + path
		} else if len(path) >= 3 && os.IsPathSeparator(path[0]) && path[2] == ':' {
			path = path[1:]
		}
	} else if parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
		return "", os.ErrInvalid
	}
	if path == "" || !filepath.IsAbs(path) {
		return "", os.ErrInvalid
	}
	return filepath.Clean(path), nil
}

func validatedDocumentPath(uri string, folders []indexer.WorkspaceFolder) (string, error) {
	path, err := uriToPath(uri)
	if err != nil {
		return "", err
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("document is not a regular file: %w", os.ErrInvalid)
	}
	if len(folders) == 0 {
		return resolvedPath, nil
	}

	for _, folder := range folders {
		root, err := uriToPath(folder.URI)
		if err != nil {
			continue
		}
		resolvedRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		if pathWithinRoot(resolvedPath, resolvedRoot) {
			return resolvedPath, nil
		}
	}
	return "", fmt.Errorf("document is outside the configured workspace: %w", os.ErrPermission)
}

func pathWithinRoot(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

// ─── Request helpers ──────────────────────────────────────────────────────────

func (h *Handler) completion(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.CompletionParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return []lsp.CompletionItem{}, nil
	}
	return h.prov.Completion.Provide(doc.URI, doc.Text, p.Position, p.Context), nil
}

func (h *Handler) completionResolve(raw json.RawMessage) (any, *lsp.ResponseError) {
	var item lsp.CompletionItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	return h.prov.Completion.Resolve(item), nil
}

func (h *Handler) hover(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.HoverParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	return h.prov.Hover.Provide(doc.URI, doc.Text, p.Position), nil
}

func (h *Handler) definition(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.DefinitionParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return []lsp.Location{}, nil
	}
	return h.prov.Definition.Provide(doc.URI, doc.Text, p.Position), nil
}

func (h *Handler) declaration(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.DefinitionParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return []lsp.Location{}, nil
	}
	return h.prov.Declaration.Provide(doc.URI, doc.Text, p.Position), nil
}

func (h *Handler) typeDefinition(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.DefinitionParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return []lsp.Location{}, nil
	}
	return h.prov.TypeDefinition.Provide(doc.URI, doc.Text, p.Position), nil
}

func (h *Handler) implementation(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.DefinitionParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return []lsp.Location{}, nil
	}
	return h.prov.Implementation.Provide(doc.URI, doc.Text, p.Position), nil
}

func (h *Handler) references(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.ReferenceParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return []lsp.Location{}, nil
	}
	return h.prov.References.Provide(doc.URI, doc.Text, p.Position, p.Context.IncludeDeclaration), nil
}

func (h *Handler) documentHighlight(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.DocumentHighlightParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return []lsp.DocumentHighlight{}, nil
	}
	return h.prov.Highlight.Provide(doc.URI, doc.Text, p.Position), nil
}

func (h *Handler) documentSymbol(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.DocumentSymbolParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return []lsp.DocumentSymbol{}, nil
	}
	return h.prov.Symbol.ProvideDocument(doc.URI, doc.Text), nil
}

func (h *Handler) workspaceSymbol(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.WorkspaceSymbolParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	return h.prov.Symbol.ProvideWorkspace(p.Query), nil
}

func (h *Handler) signatureHelp(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.SignatureHelpParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	return h.prov.SignatureHelp.Provide(doc.URI, doc.Text, p.Position), nil
}

func (h *Handler) formatting(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.DocumentFormattingParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return []lsp.TextEdit{}, nil
	}
	return h.prov.Formatting.Format(doc.URI, doc.Text, p.Options), nil
}

func (h *Handler) rangeFormatting(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.DocumentRangeFormattingParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return []lsp.TextEdit{}, nil
	}
	return h.prov.Formatting.FormatRange(doc.URI, doc.Text, p.Range, p.Options), nil
}

func (h *Handler) rename(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.RenameParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	return h.prov.Rename.Provide(doc.URI, doc.Text, p.Position, p.NewName), nil
}

func (h *Handler) prepareRename(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.PrepareRenameParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	return h.prov.Rename.Prepare(doc.URI, doc.Text, p.Position), nil
}

func (h *Handler) foldingRange(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.FoldingRangeParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return []lsp.FoldingRange{}, nil
	}
	return h.prov.Folding.Provide(doc.URI, doc.Text), nil
}

func (h *Handler) selectionRange(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.SelectionRangeParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return []lsp.SelectionRange{}, nil
	}
	return h.prov.SelectionRange.Provide(doc.URI, doc.Text, p.Positions), nil
}

func (h *Handler) codeAction(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.CodeActionParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return []lsp.CodeAction{}, nil
	}
	return h.prov.CodeAction.Provide(doc.URI, doc.Text, p.Range, p.Context), nil
}

func (h *Handler) codeActionResolve(raw json.RawMessage) (any, *lsp.ResponseError) {
	var a lsp.CodeAction
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	return h.prov.CodeAction.Resolve(a), nil
}

func (h *Handler) codeLens(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.CodeLensParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return []lsp.CodeLens{}, nil
	}
	return h.prov.CodeLens.Provide(doc.URI, doc.Text), nil
}

func (h *Handler) codeLensResolve(raw json.RawMessage) (any, *lsp.ResponseError) {
	var l lsp.CodeLens
	if err := json.Unmarshal(raw, &l); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	return h.prov.CodeLens.Resolve(l), nil
}

func (h *Handler) inlayHint(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.InlayHintParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return []lsp.InlayHint{}, nil
	}
	return h.prov.InlayHints.Provide(doc.URI, doc.Text, p.Range), nil
}

func (h *Handler) documentLink(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.DocumentLinkParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return []lsp.DocumentLink{}, nil
	}
	return h.prov.DocumentLinks.Provide(doc.URI, doc.Text), nil
}

func (h *Handler) inlineValue(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.InlineValueParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return []lsp.InlineValueVariableLookup{}, nil
	}
	return h.prov.InlineValues.Provide(doc.URI, doc.Text, p.Range), nil
}

func (h *Handler) typeHierarchyPrepare(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.TypeHierarchyPrepareParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	doc, ok := h.documents.Get(p.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	return h.prov.TypeHierarchy.Prepare(doc.URI, doc.Text, p.Position), nil
}

func (h *Handler) typeHierarchySupertypes(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.TypeHierarchySupertypesParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	return h.prov.TypeHierarchy.Supertypes(p.Item), nil
}

func (h *Handler) typeHierarchySubtypes(raw json.RawMessage) (any, *lsp.ResponseError) {
	var p lsp.TypeHierarchySubtypesParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	return h.prov.TypeHierarchy.Subtypes(p.Item), nil
}
