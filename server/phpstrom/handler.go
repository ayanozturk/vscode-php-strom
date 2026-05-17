package phpstrom

import (
	"encoding/json"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

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

	workspaceDiagnosticsMu sync.Mutex
	publishedDiagnosticsMu sync.Mutex
	publishedDiagnostics   map[string]struct{}
}

const workspaceDiagnosticsLimit = 10_000

type saveAnalysisFinishedParams struct {
	URI       string `json:"uri"`
	Published bool   `json:"published"`
}

type workspaceDiagnosticsFinishedParams struct {
	FilesWithDiagnostics int `json:"filesWithDiagnostics"`
	TotalDiagnostics     int `json:"totalDiagnostics"`
	Capped               bool `json:"capped"`
}

func NewHandler(srv *Server) *Handler {
	cfg := DefaultConfig()
	docs := NewDocumentStore()
	idx := indexer.New(cfg.toIndexerConfig())
	prov := providers.NewRegistry(idx, cfg.toProviderConfig())

	h := &Handler{
		srv:                  srv,
		cfg:                  cfg,
		documents:            docs,
		idx:                  idx,
		prov:                 prov,
		publishedDiagnostics: make(map[string]struct{}),
	}

	idx.OnIndexingStart(func() { srv.Notify("phpstrom/indexingStarted", nil) })
	idx.OnIndexingProgress(func(done, total int) {
		srv.Notify("phpstrom/indexingProgress", map[string]int{"done": done, "total": total})
	})
	idx.OnIndexingDone(func(count int) {
		srv.Notify("phpstrom/indexingFinished", map[string]int{"symbolCount": count})
	})

	return h
}

// HandleRequest processes a request with an ID and returns a result.
func (h *Handler) HandleRequest(method string, raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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
		go h.indexAndPublishWorkspaceDiagnostics()

	case "textDocument/didOpen":
		var p lsp.DidOpenTextDocumentParams
		if err := json.Unmarshal(raw, &p); err != nil {
			log.Printf("didOpen: %v", err)
			return
		}
		doc := h.documents.Open(p.TextDocument)
		h.idx.IndexDocument(doc.URI, doc.Text)
		if h.cfg.Diagnostics.Run == "onType" {
			go h.publishDiagnostics(doc.URI, doc.Text, doc.Version)
		}

	case "textDocument/didChange":
		var p lsp.DidChangeTextDocumentParams
		if err := json.Unmarshal(raw, &p); err != nil {
			log.Printf("didChange: %v", err)
			return
		}
		doc := h.documents.Change(p.TextDocument.URI, p.TextDocument.Version, p.ContentChanges)
		h.idx.IndexDocument(doc.URI, doc.Text)
		if h.cfg.Diagnostics.Run == "onType" {
			go h.publishDiagnostics(doc.URI, doc.Text, doc.Version)
		}

	case "textDocument/didSave":
		var p lsp.DidSaveTextDocumentParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return
		}
		if doc, ok := h.documents.Get(p.TextDocument.URI); ok {
			if text, err := readDocumentTextFromDisk(p.TextDocument.URI); err == nil {
				if updated, ok := h.documents.SetText(p.TextDocument.URI, text); ok {
					doc = updated
				}
			}
			h.idx.IndexDocument(doc.URI, doc.Text)
			go func(uri, text string, version int) {
				published := h.publishDiagnostics(uri, text, version)
				h.srv.Notify("phpstrom/saveAnalysisFinished", saveAnalysisFinishedParams{
					URI:       uri,
					Published: published,
				})
			}(doc.URI, doc.Text, doc.Version)
		}

	case "textDocument/didClose":
		var p lsp.DidCloseTextDocumentParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return
		}
		h.documents.Close(p.TextDocument.URI)
		if text, err := readDocumentTextFromDisk(p.TextDocument.URI); err == nil {
			h.idx.IndexDocument(p.TextDocument.URI, text)
			h.publishWorkspaceDocumentDiagnostics(p.TextDocument.URI, text)
			return
		}
		h.idx.RemoveDocument(p.TextDocument.URI)
		h.notifyDiagnostics(p.TextDocument.URI, []lsp.Diagnostic{})

	case "workspace/didChangeConfiguration":
		var p struct {
			Settings map[string]interface{} `json:"settings"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return
		}
		h.cfg.Update(p.Settings)
		h.prov = providers.NewRegistry(h.idx, h.cfg.toProviderConfig())
		go h.runWorkspaceDiagnosticsScan(false)

	case "phpstrom/indexWorkspace":
		go h.indexAndPublishWorkspaceDiagnostics()

	case "exit":
	}
}

// ─── initialize ───────────────────────────────────────────────────────────────

func (h *Handler) initialize(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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
	h.prov = providers.NewRegistry(h.idx, h.cfg.toProviderConfig())

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
	diags := h.prov.Diagnostics.Analyse(uri, text)
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

func (h *Handler) indexAndPublishWorkspaceDiagnostics() {
	h.srv.Notify("phpstrom/workspaceDiagnosticsStarted", nil)

	h.workspaceDiagnosticsMu.Lock()
	defer h.workspaceDiagnosticsMu.Unlock()
	scan := newWorkspaceDiagnosticsScanState()
	defer h.srv.Notify("phpstrom/workspaceDiagnosticsFinished", workspaceDiagnosticsFinishedParams{
		FilesWithDiagnostics: h.publishedDiagnosticsCount(),
		TotalDiagnostics:     scan.total(),
		Capped:               scan.capped(),
	})

	if !h.cfg.Diagnostics.Enable {
		h.idx.IndexWorkspace()
		h.clearPublishedDiagnostics()
		return
	}

	seen := make(map[string]struct{})
	var seenMu sync.Mutex
	h.idx.IndexWorkspaceParsed(func(parsed indexer.ParsedFile) {
		if scan.capped() {
			return
		}

		seenMu.Lock()
		seen[parsed.URI] = struct{}{}
		seenMu.Unlock()

		if doc, ok := h.documents.Snapshot(parsed.URI); ok {
			h.publishDiagnosticsForScan(parsed.URI, doc.Text, doc.Version, scan)
			return
		}

		h.publishWorkspaceParsedDiagnosticsForScan(parsed, scan)
	})
	h.clearDiagnosticsOutsideWorkspace(seen)
}

func (h *Handler) runWorkspaceDiagnosticsScan(indexWorkspace bool) {
	h.srv.Notify("phpstrom/workspaceDiagnosticsStarted", nil)
	scan := newWorkspaceDiagnosticsScanState()
	defer h.srv.Notify("phpstrom/workspaceDiagnosticsFinished", workspaceDiagnosticsFinishedParams{
		FilesWithDiagnostics: h.publishedDiagnosticsCount(),
		TotalDiagnostics:     scan.total(),
		Capped:               scan.capped(),
	})

	if indexWorkspace {
		h.idx.IndexWorkspace()
	}
	h.runWorkspaceDiagnostics(scan)
}

func (h *Handler) runWorkspaceDiagnostics(scan *workspaceDiagnosticsScanState) {
	h.workspaceDiagnosticsMu.Lock()
	defer h.workspaceDiagnosticsMu.Unlock()

	if !h.cfg.Diagnostics.Enable {
		h.clearPublishedDiagnostics()
		return
	}

	workspaceURIs := h.idx.WorkspaceFileURIs()
	seen := make(map[string]struct{}, len(workspaceURIs))
	jobs := make(chan string, len(workspaceURIs))
	workerCount := runtime.GOMAXPROCS(0)
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > len(workspaceURIs) && len(workspaceURIs) > 0 {
		workerCount = len(workspaceURIs)
	}
	var wg sync.WaitGroup

	for _, uri := range workspaceURIs {
		seen[uri] = struct{}{}
		jobs <- uri
	}
	close(jobs)

	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for uri := range jobs {
				if scan.capped() {
					return
				}

				if doc, ok := h.documents.Snapshot(uri); ok {
					h.publishDiagnosticsForScan(uri, doc.Text, doc.Version, scan)
					continue
				}

				text, err := readDocumentTextFromDisk(uri)
				if err != nil {
					continue
				}
				h.publishWorkspaceDocumentDiagnosticsForScan(uri, text, scan)
			}
		}()
	}

	wg.Wait()

	h.clearDiagnosticsOutsideWorkspace(seen)
}

func (h *Handler) publishWorkspaceDocumentDiagnostics(uri, text string) bool {
	diags := h.prov.Diagnostics.Analyse(uri, text)
	if diags == nil {
		diags = []lsp.Diagnostic{}
	}
	h.notifyDiagnostics(uri, diags)
	return true
}

func (h *Handler) publishWorkspaceDocumentDiagnosticsForScan(uri, text string, scan *workspaceDiagnosticsScanState) bool {
	diags := h.prov.Diagnostics.Analyse(uri, text)
	if diags == nil {
		diags = []lsp.Diagnostic{}
	}
	if !scan.allow(len(diags)) {
		return false
	}
	h.notifyDiagnostics(uri, diags)
	return true
}

func (h *Handler) publishWorkspaceParsedDiagnostics(parsed indexer.ParsedFile) bool {
	diags := h.prov.Diagnostics.AnalyseParsed(parsed.URI, parsed.Text, parsed.Nodes, parsed.Errors)
	if diags == nil {
		diags = []lsp.Diagnostic{}
	}
	h.notifyDiagnostics(parsed.URI, diags)
	return true
}

func (h *Handler) publishWorkspaceParsedDiagnosticsForScan(parsed indexer.ParsedFile, scan *workspaceDiagnosticsScanState) bool {
	diags := h.prov.Diagnostics.AnalyseParsed(parsed.URI, parsed.Text, parsed.Nodes, parsed.Errors)
	if diags == nil {
		diags = []lsp.Diagnostic{}
	}
	if !scan.allow(len(diags)) {
		return false
	}
	h.notifyDiagnostics(parsed.URI, diags)
	return true
}

func (h *Handler) publishDiagnosticsForScan(uri, text string, version int, scan *workspaceDiagnosticsScanState) bool {
	diags := h.prov.Diagnostics.Analyse(uri, text)
	if diags == nil {
		diags = []lsp.Diagnostic{}
	}
	if !scan.allow(len(diags)) {
		return false
	}
	current, ok := h.documents.Snapshot(uri)
	if !ok || current.Version != version || current.Text != text {
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

type workspaceDiagnosticsScanState struct {
	totalDiagnostics atomic.Int64
	isCapped         atomic.Bool
}

func newWorkspaceDiagnosticsScanState() *workspaceDiagnosticsScanState {
	return &workspaceDiagnosticsScanState{}
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

func readDocumentTextFromDisk(uri string) (string, error) {
	path, err := uriToPath(uri)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func uriToPath(uri string) (string, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Scheme == "file" {
		path := parsed.Path
		if path == "" {
			path = strings.TrimPrefix(uri, "file://")
		}
		if unescaped, err := url.PathUnescape(path); err == nil {
			path = unescaped
		}
		return filepath.FromSlash(path), nil
	}
	return "", os.ErrInvalid
}

// ─── Request helpers ──────────────────────────────────────────────────────────

func (h *Handler) completion(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) completionResolve(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
	var item lsp.CompletionItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	return h.prov.Completion.Resolve(item), nil
}

func (h *Handler) hover(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) definition(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) declaration(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) typeDefinition(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) implementation(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) references(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) documentHighlight(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) documentSymbol(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) workspaceSymbol(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
	var p lsp.WorkspaceSymbolParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	return h.prov.Symbol.ProvideWorkspace(p.Query), nil
}

func (h *Handler) signatureHelp(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) formatting(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) rangeFormatting(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) rename(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) prepareRename(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) foldingRange(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) selectionRange(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) codeAction(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) codeActionResolve(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
	var a lsp.CodeAction
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	return h.prov.CodeAction.Resolve(a), nil
}

func (h *Handler) codeLens(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) codeLensResolve(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
	var l lsp.CodeLens
	if err := json.Unmarshal(raw, &l); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	return h.prov.CodeLens.Resolve(l), nil
}

func (h *Handler) inlayHint(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) documentLink(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) inlineValue(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) typeHierarchyPrepare(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
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

func (h *Handler) typeHierarchySupertypes(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
	var p lsp.TypeHierarchySupertypesParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	return h.prov.TypeHierarchy.Supertypes(p.Item), nil
}

func (h *Handler) typeHierarchySubtypes(raw json.RawMessage) (interface{}, *lsp.ResponseError) {
	var p lsp.TypeHierarchySubtypesParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &lsp.ResponseError{Code: lsp.InvalidParams, Message: err.Error()}
	}
	return h.prov.TypeHierarchy.Subtypes(p.Item), nil
}
