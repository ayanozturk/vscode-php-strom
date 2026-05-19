package indexer

import (
	"context"
	"io/fs"
	"log"
	"os"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go-phpcs/ast"
	goplexer "go-phpcs/lexer"
	goparser "go-phpcs/parser"
)

// WorkspaceIndexer discovers and indexes PHP files in workspace folders.
type WorkspaceIndexer struct {
	cfg           Config
	index         *Index
	folders       []WorkspaceFolder
	workspaceURIs []string
	gitignores    []workspaceGitignore
	mu            sync.RWMutex
	onStart       func()
	onDone        func(int)
	onProgress    func(done, total int)
	indexedCount  int64
}

const perFileParseTimeout = 20 * time.Second

// ParsedFile contains the single-parse result used for both indexing and diagnostics.
type ParsedFile struct {
	URI     string
	Text    string
	Nodes   []ast.Node
	Errors  []string
	Symbols []*Symbol
}

// New creates a WorkspaceIndexer with the given configuration.
func New(cfg Config) *WorkspaceIndexer {
	return &WorkspaceIndexer{cfg: cfg, index: newIndex()}
}

// SetWorkspaceFolders updates the list of root folders to index.
func (wi *WorkspaceIndexer) SetWorkspaceFolders(folders []WorkspaceFolder) {
	wi.mu.Lock()
	wi.folders = folders
	wi.gitignores = loadWorkspaceGitignores(folders)
	wi.mu.Unlock()
}

// WorkspaceFolders returns the current workspace root folders.
func (wi *WorkspaceIndexer) WorkspaceFolders() []WorkspaceFolder {
	wi.mu.RLock()
	defer wi.mu.RUnlock()
	return append([]WorkspaceFolder(nil), wi.folders...)
}

// OnIndexingStart registers a callback called when workspace indexing begins.
func (wi *WorkspaceIndexer) OnIndexingStart(fn func()) { wi.onStart = fn }

// OnIndexingDone registers a callback called when workspace indexing finishes.
func (wi *WorkspaceIndexer) OnIndexingDone(fn func(int)) { wi.onDone = fn }

// OnIndexingProgress registers a callback called periodically during indexing.
// done is the number of files processed so far; total is the total file count.
func (wi *WorkspaceIndexer) OnIndexingProgress(fn func(done, total int)) { wi.onProgress = fn }

// IndexWorkspace scans all workspace folders and indexes every PHP file.
// It uses a goroutine pool sized to GOMAXPROCS for parallel parsing.
func (wi *WorkspaceIndexer) IndexWorkspace() {
	wi.indexWorkspace(nil)
}

// IndexWorkspaceParsed scans all workspace folders, updates the symbol index,
// and invokes visitor with the parsed representation for each file.
func (wi *WorkspaceIndexer) IndexWorkspaceParsed(visitor func(ParsedFile)) {
	wi.indexWorkspace(visitor)
}

func (wi *WorkspaceIndexer) indexWorkspace(visitor func(ParsedFile)) {
	if wi.onStart != nil {
		wi.onStart()
	}

	wi.mu.RLock()
	folders := wi.folders
	gitignores := append([]workspaceGitignore(nil), wi.gitignores...)
	wi.mu.RUnlock()

	paths := wi.collectWorkspaceFilePaths(folders, gitignores)
	uris := make([]string, 0, len(paths))
	for _, filePath := range paths {
		uris = append(uris, pathToURI(filePath))
	}

	wi.mu.Lock()
	wi.workspaceURIs = uris
	wi.mu.Unlock()

	total := len(paths)
	log.Printf("[indexer] discovered %d files across %d workspace folder(s)", total, len(folders))

	// Keep parser concurrency conservative until parser-level cancellation exists.
	// This prevents pathological files from saturating many CPU cores.
	numWorkers := runtime.GOMAXPROCS(0)
	if numWorkers < 1 {
		numWorkers = 1
	}
	log.Printf("[indexer] starting %d worker(s)", numWorkers)

	jobs := make(chan string, len(paths))
	for _, p := range paths {
		jobs <- p
	}
	close(jobs)

	var wg sync.WaitGroup
	atomic.StoreInt64(&wi.indexedCount, 0)

	// Report at most every 1% of files (min 10, max 200) to avoid flooding the client.
	reportEvery := total / 100
	if reportEvery < 10 {
		reportEvery = 10
	}
	if reportEvery > 200 {
		reportEvery = 200
	}

	for i := 0; i < numWorkers; i++ {
		workerID := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Printf("[indexer] worker %d started", workerID)
			for filePath := range jobs {
				wi.indexFile(filePath, visitor)
				done := int(atomic.AddInt64(&wi.indexedCount, 1))
				if done%reportEvery == 0 || done == total {
					log.Printf("[indexer] progress %d/%d", done, total)
					if wi.onProgress != nil {
						wi.onProgress(done, total)
					}
				}
			}
			log.Printf("[indexer] worker %d finished", workerID)
		}()
	}
	wg.Wait()

	count := int(atomic.LoadInt64(&wi.indexedCount))
	log.Printf("[indexer] finished — %d files indexed", count)
	if wi.onDone != nil {
		wi.onDone(len(wi.index.AllSymbols()))
	}
}

// IndexDocument re-indexes a single open document from its text content.
func (wi *WorkspaceIndexer) IndexDocument(uri, text string) {
	syms := extractSymbols(uri, text)
	wi.index.PutFile(uri, syms)
	wi.trackWorkspaceURI(uri)
}

// RemoveDocument removes all symbols for a document URI from the index.
func (wi *WorkspaceIndexer) RemoveDocument(uri string) {
	wi.index.RemoveFile(uri)
	wi.untrackWorkspaceURI(uri)
}

// WorkspaceFileURIs returns every PHP file currently discovered under the workspace folders.
func (wi *WorkspaceIndexer) WorkspaceFileURIs() []string {
	wi.mu.RLock()
	if len(wi.workspaceURIs) > 0 {
		uris := append([]string(nil), wi.workspaceURIs...)
		wi.mu.RUnlock()
		return uris
	}
	folders := append([]WorkspaceFolder(nil), wi.folders...)
	wi.mu.RUnlock()

	gitignores := append([]workspaceGitignore(nil), wi.gitignores...)
	paths := wi.collectWorkspaceFilePaths(folders, gitignores)
	uris := make([]string, 0, len(paths))
	for _, filePath := range paths {
		uris = append(uris, pathToURI(filePath))
	}
	return uris
}

// GetIndex returns the underlying symbol index for provider use.
func (wi *WorkspaceIndexer) GetIndex() *Index { return wi.index }

// ─── Internal ─────────────────────────────────────────────────────────────────

func (wi *WorkspaceIndexer) collectWorkspaceFilePaths(folders []WorkspaceFolder, gitignores []workspaceGitignore) []string {
	var paths []string
	for _, folder := range folders {
		root := uriToPath(folder.URI)
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if wi.shouldExcludeDir(p) {
					return filepath.SkipDir
				}
				if ignoresPath(gitignores, p, true) {
					if canSkipIgnoredDir(gitignores, p) {
						return filepath.SkipDir
					}
					return nil
				}
				return nil
			}
			if ignoresPath(gitignores, p, false) {
				return nil
			}
			if wi.matchesAssociations(p) {
				paths = append(paths, p)
			}
			return nil
		})
	}
	return paths
}

func (wi *WorkspaceIndexer) trackWorkspaceURI(uri string) {
	wi.mu.Lock()
	defer wi.mu.Unlock()
	for _, existing := range wi.workspaceURIs {
		if existing == uri {
			return
		}
	}
	wi.workspaceURIs = append(wi.workspaceURIs, uri)
}

func (wi *WorkspaceIndexer) untrackWorkspaceURI(uri string) {
	wi.mu.Lock()
	defer wi.mu.Unlock()
	for index, existing := range wi.workspaceURIs {
		if existing != uri {
			continue
		}
		wi.workspaceURIs = append(wi.workspaceURIs[:index], wi.workspaceURIs[index+1:]...)
		return
	}
}

func (wi *WorkspaceIndexer) indexFile(path string, visitor func(ParsedFile)) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			log.Printf("[indexer] PANIC parsing %s: %v\n%s", path, r, buf[:n])
		}
	}()

	parsed, skipReason := wi.parseIndexableFile(path)
	if skipReason != "" {
		if strings.HasPrefix(skipReason, "timeout") {
			log.Printf("[indexer] %s: %s", skipReason, path)
		}
		return
	}

	wi.index.PutFile(parsed.URI, parsed.Symbols)
	if visitor != nil {
		visitor(parsed)
	}
}

func (wi *WorkspaceIndexer) parseIndexableFile(path string) (ParsedFile, string) {
	ctx, cancel := context.WithTimeout(context.Background(), perFileParseTimeout)
	defer cancel()

	info, err := os.Stat(path)
	if err != nil {
		return ParsedFile{}, "stat-error"
	}
	if !info.Mode().IsRegular() {
		return ParsedFile{}, "non-regular-file"
	}
	if info.Size() > wi.cfg.MaxSize {
		log.Printf("[indexer] skipping oversized file (%d bytes): %s", info.Size(), path)
		return ParsedFile{}, "oversized-file"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ParsedFile{}, "read-error"
	}

	uri := pathToURI(path)
	parsed := ParseSourceWithContext(ctx, uri, string(data))

	if ctx.Err() != nil {
		return ParsedFile{}, "timeout>20s"
	}

	return parsed, ""
}

func (wi *WorkspaceIndexer) shouldExcludeDir(path string) bool {
	for _, pat := range wi.cfg.Exclude {
		if matchSimple(pat, path) {
			return true
		}
	}
	return false
}

func (wi *WorkspaceIndexer) matchesAssociations(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".php" || ext == ".phtml" || ext == ".phar"
}

// ─── Symbol extraction ────────────────────────────────────────────────────────

// ParseSource parses PHP source once and derives both AST-backed diagnostics input
// and the symbol index representation from the same node list.
func ParseSource(uri, src string) ParsedFile {
	return ParseSourceWithContext(context.Background(), uri, src)
}

// ParseSourceWithContext parses PHP source using a cooperative context to abort early if requested.
func ParseSourceWithContext(ctx context.Context, uri, src string) ParsedFile {
	l := goplexer.New(src)
	p := goparser.New(l, false)
	p.Ctx = ctx
	nodes := p.Parse()
	errs := append([]string(nil), p.Errors()...)
	return ParsedFile{
		URI:     uri,
		Text:    src,
		Nodes:   nodes,
		Errors:  errs,
		Symbols: extractSymbolsFromNodes(uri, nodes),
	}
}

// extractSymbols parses PHP source and extracts top-level declarations.
func extractSymbols(uri, src string) []*Symbol {
	return ParseSource(uri, src).Symbols
}

func extractSymbolsFromNodes(uri string, nodes []ast.Node) []*Symbol {
	var syms []*Symbol
	extractFromNodes(nodes, uri, extractionContext{aliases: make(map[string]string)}, &syms)
	for _, sym := range syms {
		populateLSPRange(sym)
	}
	return syms
}

type extractionContext struct {
	namespace string
	aliases   map[string]string
}

// buildLineOffsets returns a slice where element i is the byte offset of line i (0-based).
func buildLineOffsets(src string) []int {
	offsets := []int{0}
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}

// offsetToLineChar converts a byte offset to 0-based LSP line and character.
func offsetToLineChar(lineOffsets []int, offset int) (line, char uint32) {
	lo, hi := 0, len(lineOffsets)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if lineOffsets[mid] <= offset {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return uint32(lo), uint32(offset - lineOffsets[lo])
}

func extractFromNodes(nodes []ast.Node, uri string, ctx extractionContext, syms *[]*Symbol) {
	for _, node := range nodes {
		switch n := node.(type) {
		case *ast.NamespaceNode:
			namespaceCtx := extractionContext{namespace: n.Name, aliases: make(map[string]string)}
			if len(n.Body) > 0 {
				extractFromNodes(n.Body, uri, namespaceCtx, syms)
			} else {
				// Bracket-less namespace: subsequent top-level declarations use this namespace.
				ctx = namespaceCtx
			}

		case *ast.UseNode:
			if n.Type != "" && n.Type != "class" {
				continue
			}
			alias := n.Alias
			if alias == "" {
				alias = unqualifiedTypeName(n.Path)
			}
			ctx.aliases[strings.ToLower(alias)] = strings.TrimPrefix(n.Path, `\`)

		case *ast.ClassNode:
			classFQN := fqn(ctx.namespace, n.Name)
			sym := &Symbol{
				FQN:        classFQN,
				Name:       n.Name,
				Kind:       KindClass,
				Namespace:  ctx.namespace,
				URI:        uri,
				Range:      positionRange(n.GetPos()),
				DocComment: docRaw(n.PHPDoc),
				IsFinal:    hasModifier(n.Modifier, "final"),
				IsAbstract: hasModifier(n.Modifier, "abstract"),
				Visibility: "public",
			}
			if n.Extends != "" {
				sym.Extends = []string{resolveClassLike(ctx, n.Extends)}
			}
			for _, implemented := range n.Implements {
				sym.Implements = append(sym.Implements, resolveClassLike(ctx, implemented))
			}
			*syms = append(*syms, sym)
			extractClassMembers(n, uri, classFQN, ctx, syms)

		case *ast.InterfaceNode:
			interfaceFQN := fqn(ctx.namespace, n.Name)
			extends := make([]string, 0, len(n.Extends))
			for _, parent := range n.Extends {
				extends = append(extends, resolveClassLike(ctx, parent))
			}
			*syms = append(*syms, &Symbol{
				FQN:        interfaceFQN,
				Name:       n.Name,
				Kind:       KindInterface,
				Namespace:  ctx.namespace,
				URI:        uri,
				Range:      positionRange(n.GetPos()),
				DocComment: docRaw(n.PHPDoc),
				Extends:    extends,
				Visibility: "public",
			})
			extractInterfaceMembers(n.Members, uri, interfaceFQN, ctx, syms)

		case *ast.TraitNode:
			traitName := ""
			if n.Name != nil {
				traitName = n.Name.Name
			}
			traitFQN := fqn(ctx.namespace, traitName)
			*syms = append(*syms, &Symbol{
				FQN:        traitFQN,
				Name:       traitName,
				Kind:       KindModule,
				Namespace:  ctx.namespace,
				URI:        uri,
				Range:      positionRange(n.GetPos()),
				Visibility: "public",
			})
			extractTraitMembers(n.Body, uri, traitFQN, ctx, syms)

		case *ast.EnumNode:
			enumFQN := fqn(ctx.namespace, n.Name)
			*syms = append(*syms, &Symbol{
				FQN:        enumFQN,
				Name:       n.Name,
				Kind:       KindEnum,
				Namespace:  ctx.namespace,
				URI:        uri,
				Range:      positionRange(n.GetPos()),
				Visibility: "public",
			})
			extractEnumCases(n.Cases, uri, enumFQN, syms)

		case *ast.FunctionNode:
			functionFQN := fqn(ctx.namespace, n.Name)
			*syms = append(*syms, &Symbol{
				FQN:        functionFQN,
				Name:       n.Name,
				Kind:       KindFunction,
				Namespace:  ctx.namespace,
				URI:        uri,
				Range:      positionRange(n.GetPos()),
				DocComment: docRaw(n.PHPDoc),
				ReturnType: resolveTypeHint(ctx, n.ReturnType),
				Visibility: "public",
				Params:     extractParams(ctx, n.Params),
			})

		case *ast.ConstantNode:
			constFQN := fqn(ctx.namespace, n.Name)
			*syms = append(*syms, &Symbol{
				FQN:        constFQN,
				Name:       n.Name,
				Kind:       KindConstant,
				Namespace:  ctx.namespace,
				URI:        uri,
				Range:      positionRange(n.GetPos()),
				Visibility: defaultVisibility(n.Visibility),
			})
		}
	}
}

func resolveClassLike(ctx extractionContext, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if strings.HasPrefix(name, `\`) {
		return ensureLeadingSlash(name)
	}

	firstSegment := name
	remainder := ""
	if idx := strings.Index(name, `\`); idx >= 0 {
		firstSegment = name[:idx]
		remainder = name[idx+1:]
	}
	if target, ok := ctx.aliases[strings.ToLower(firstSegment)]; ok {
		if remainder != "" {
			return ensureLeadingSlash(target + `\` + remainder)
		}
		return ensureLeadingSlash(target)
	}
	if ctx.namespace != "" {
		return ensureLeadingSlash(ctx.namespace + `\` + name)
	}
	return ensureLeadingSlash(name)
}

func resolveTypeHint(ctx extractionContext, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	prefix := ""
	if strings.HasPrefix(raw, "?") {
		prefix = "?"
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "?"))
	}

	parts := strings.Split(raw, "|")
	for idx, part := range parts {
		intersections := strings.Split(part, "&")
		for innerIdx, atom := range intersections {
			atom = strings.TrimSpace(atom)
			if atom == "" || !isResolvableClassLikeType(atom) {
				intersections[innerIdx] = atom
				continue
			}
			intersections[innerIdx] = strings.TrimPrefix(resolveClassLike(ctx, atom), `\`)
		}
		parts[idx] = strings.Join(intersections, "&")
	}

	return prefix + strings.Join(parts, "|")
}

func isResolvableClassLikeType(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "array", "bool", "callable", "false", "float", "int", "iterable", "mixed", "never", "null", "object", "resource", "string", "true", "void", "self", "static", "parent":
		return false
	default:
		return true
	}
}

func unqualifiedTypeName(name string) string {
	name = strings.TrimPrefix(strings.TrimSpace(name), `\`)
	if idx := strings.LastIndex(name, `\`); idx >= 0 {
		return name[idx+1:]
	}
	return name
}

func extractClassMembers(class *ast.ClassNode, uri, classFQN string, ctx extractionContext, syms *[]*Symbol) {
	for _, property := range promotedPropertySymbols(class, uri, classFQN, ctx) {
		*syms = append(*syms, property)
	}

	for _, methodNode := range class.Methods {
		method, ok := methodNode.(*ast.FunctionNode)
		if !ok {
			continue
		}
		visibility := visibilityFromModifiers(method.Visibility, method.Modifiers)
		*syms = append(*syms, &Symbol{
			FQN:        classFQN + "::" + method.Name,
			Name:       method.Name,
			Kind:       KindMethod,
			URI:        uri,
			Range:      positionRange(method.GetPos()),
			DocComment: docRaw(method.PHPDoc),
			ReturnType: resolveTypeHint(ctx, method.ReturnType),
			IsStatic:   hasModifierList(method.Modifiers, "static"),
			IsAbstract: hasModifierList(method.Modifiers, "abstract"),
			IsFinal:    hasModifierList(method.Modifiers, "final"),
			Visibility: visibility,
			Params:     extractParams(ctx, method.Params),
		})
	}

	for _, propertyNode := range class.Properties {
		property, ok := propertyNode.(*ast.PropertyNode)
		if !ok {
			continue
		}
		*syms = append(*syms, &Symbol{
			FQN:        classFQN + "::$" + property.Name,
			Name:       property.Name,
			Kind:       KindProperty,
			URI:        uri,
			Range:      positionRange(property.GetPos()),
			Type:       resolveTypeHint(ctx, property.TypeHint),
			IsStatic:   property.IsStatic,
			IsReadonly: property.IsReadonly,
			Visibility: defaultVisibility(property.Visibility),
		})
	}

	for _, constantNode := range class.Constants {
		constant, ok := constantNode.(*ast.ConstantNode)
		if !ok {
			continue
		}
		*syms = append(*syms, &Symbol{
			FQN:        classFQN + "::" + constant.Name,
			Name:       constant.Name,
			Kind:       KindConstant,
			URI:        uri,
			Range:      positionRange(constant.GetPos()),
			Visibility: defaultVisibility(constant.Visibility),
		})
	}
}

func promotedPropertySymbols(class *ast.ClassNode, uri, classFQN string, ctx extractionContext) []*Symbol {
	if class == nil {
		return nil
	}

	var symbols []*Symbol
	for _, methodNode := range class.Methods {
		method, ok := methodNode.(*ast.FunctionNode)
		if !ok || method == nil || !strings.EqualFold(method.Name, "__construct") {
			continue
		}
		for _, paramNode := range method.Params {
			param, ok := paramNode.(*ast.ParamNode)
			if !ok || !param.IsPromoted {
				continue
			}
			typeHint := param.TypeHint
			if typeHint == "" && param.UnionType != nil {
				typeHint = param.UnionType.TokenLiteral()
			}
			symbols = append(symbols, &Symbol{
				FQN:        classFQN + "::$" + param.Name,
				Name:       param.Name,
				Kind:       KindProperty,
				URI:        uri,
				Range:      positionRange(param.GetPos()),
				Type:       resolveTypeHint(ctx, typeHint),
				Visibility: defaultVisibility(param.Visibility),
			})
		}
	}
	return symbols
}

func extractTraitMembers(members []ast.Node, uri, traitFQN string, ctx extractionContext, syms *[]*Symbol) {
	for _, member := range members {
		switch n := member.(type) {
		case *ast.FunctionNode:
			visibility := visibilityFromModifiers(n.Visibility, n.Modifiers)
			*syms = append(*syms, &Symbol{
				FQN:        traitFQN + "::" + n.Name,
				Name:       n.Name,
				Kind:       KindMethod,
				URI:        uri,
				Range:      positionRange(n.GetPos()),
				DocComment: docRaw(n.PHPDoc),
				ReturnType: resolveTypeHint(ctx, n.ReturnType),
				IsStatic:   hasModifierList(n.Modifiers, "static"),
				IsAbstract: hasModifierList(n.Modifiers, "abstract"),
				IsFinal:    hasModifierList(n.Modifiers, "final"),
				Visibility: visibility,
				Params:     extractParams(ctx, n.Params),
			})
		case *ast.ConstantNode:
			*syms = append(*syms, &Symbol{
				FQN:        traitFQN + "::" + n.Name,
				Name:       n.Name,
				Kind:       KindConstant,
				URI:        uri,
				Range:      positionRange(n.GetPos()),
				Visibility: defaultVisibility(n.Visibility),
			})
		}
	}
}

func extractInterfaceMembers(members []ast.Node, uri, interfaceFQN string, ctx extractionContext, syms *[]*Symbol) {
	for _, member := range members {
		switch n := member.(type) {
		case *ast.InterfaceMethodNode:
			*syms = append(*syms, &Symbol{
				FQN:        interfaceFQN + "::" + n.Name,
				Name:       n.Name,
				Kind:       KindMethod,
				URI:        uri,
				Range:      positionRange(n.GetPos()),
				DocComment: docRaw(n.PHPDoc),
				ReturnType: resolveTypeHint(ctx, typeNodeToString(n.ReturnType)),
				Visibility: defaultVisibility(n.Visibility),
				Params:     extractParams(ctx, n.Params),
			})
		case *ast.ConstantNode:
			*syms = append(*syms, &Symbol{
				FQN:        interfaceFQN + "::" + n.Name,
				Name:       n.Name,
				Kind:       KindConstant,
				URI:        uri,
				Range:      positionRange(n.GetPos()),
				Visibility: defaultVisibility(n.Visibility),
			})
		}
	}
}

func extractEnumCases(cases []*ast.EnumCaseNode, uri, enumFQN string, syms *[]*Symbol) {
	for _, enumCase := range cases {
		*syms = append(*syms, &Symbol{
			FQN:        enumFQN + "::" + enumCase.Name,
			Name:       enumCase.Name,
			Kind:       KindEnumMember,
			URI:        uri,
			Range:      positionRange(enumCase.GetPos()),
			Visibility: "public",
		})
	}
}

func extractParams(ctx extractionContext, params []ast.Node) []SymbolParam {
	sp := make([]SymbolParam, 0, len(params))
	for _, param := range params {
		p, ok := param.(*ast.ParamNode)
		if !ok {
			continue
		}
		sp = append(sp, SymbolParam{
			Name:        p.Name,
			Type:        resolveTypeHint(ctx, paramTypeToString(p)),
			HasDefault:  p.DefaultValue != nil,
			IsVariadic:  p.IsVariadic,
			IsPassByRef: p.IsByRef,
		})
	}
	return sp
}

func paramTypeToString(param *ast.ParamNode) string {
	if param == nil {
		return ""
	}
	if param.TypeHint != "" {
		return param.TypeHint
	}
	if param.UnionType != nil {
		return param.UnionType.TokenLiteral()
	}
	return ""
}

func typeNodeToString(node ast.Node) string {
	if node == nil {
		return ""
	}
	switch n := node.(type) {
	case *ast.IdentifierNode:
		return n.Value
	case *ast.UnionTypeNode:
		return strings.Join(n.Types, "|")
	case *ast.IntersectionTypeNode:
		return strings.Join(n.Types, "&")
	default:
		return node.TokenLiteral()
	}
}

func positionRange(pos ast.Position) Range {
	return Range{
		Start: pos,
		End: ast.Position{
			Line:   pos.Line,
			Column: pos.Column + 1,
			Offset: pos.Offset,
		},
	}
}

func populateLSPRange(sym *Symbol) {
	startLine := clampToZeroBased(sym.Range.Start.Line)
	startChar := clampToZeroBased(sym.Range.Start.Column)
	endLine := clampToZeroBased(sym.Range.End.Line)
	endChar := clampToZeroBased(sym.Range.End.Column)

	sym.StartLine = uint32(startLine)
	sym.StartChar = uint32(startChar)
	sym.EndLine = uint32(endLine)
	sym.EndChar = uint32(endChar)
}

func clampToZeroBased(v int) int {
	if v <= 0 {
		return 0
	}
	return v - 1
}

func docRaw(doc *ast.PHPDocNode) string {
	if doc == nil {
		return ""
	}
	return doc.RawContent
}

func hasModifier(modifier, want string) bool {
	return modifier == want
}

func hasModifierList(modifiers []string, want string) bool {
	for _, modifier := range modifiers {
		if modifier == want {
			return true
		}
	}
	return false
}

func visibilityFromModifiers(legacy string, modifiers []string) string {
	if legacy != "" {
		return legacy
	}
	for _, modifier := range modifiers {
		switch modifier {
		case "private", "protected", "public":
			return modifier
		}
	}
	return "public"
}

func defaultVisibility(visibility string) string {
	if visibility == "" {
		return "public"
	}
	return visibility
}

func fqn(ns, name string) string {
	if ns == "" {
		return `\` + name
	}
	return `\` + ns + `\` + name
}

func ensureLeadingSlash(name string) string {
	if name == "" || strings.HasPrefix(name, `\`) {
		return name
	}
	return `\` + name
}

// ─── Path utilities ────────────────────────────────────────────────────────────

func pathToURI(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return "file://" + path
}

func uriToPath(uri string) string {
	p := strings.TrimPrefix(uri, "file://")
	return filepath.FromSlash(p)
}

func matchSimple(pattern, name string) bool {
	variants := expandBracePattern(filepath.ToSlash(pattern))
	normalizedName := filepath.ToSlash(name)
	for _, variant := range variants {
		if matchPathSegments(variant, normalizedName) {
			return true
		}
	}
	return false
}

func expandBracePattern(pattern string) []string {
	start := strings.IndexByte(pattern, '{')
	if start == -1 {
		return []string{pattern}
	}
	end := strings.IndexByte(pattern[start:], '}')
	if end == -1 {
		return []string{pattern}
	}
	end += start
	parts := strings.Split(pattern[start+1:end], ",")
	variants := make([]string, 0, len(parts))
	for _, part := range parts {
		variants = append(variants, expandBracePattern(pattern[:start]+part+pattern[end+1:])...)
	}
	return variants
}

func matchPathSegments(pattern, candidate string) bool {
	pattern = strings.TrimPrefix(pathpkg.Clean(pattern), "./")
	candidate = strings.TrimPrefix(pathpkg.Clean(candidate), "./")

	patternSegments := strings.Split(strings.Trim(pattern, "/"), "/")
	candidateSegments := strings.Split(strings.Trim(candidate, "/"), "/")
	if len(patternSegments) == 1 && patternSegments[0] == "." {
		patternSegments = nil
	}
	if len(candidateSegments) == 1 && candidateSegments[0] == "." {
		candidateSegments = nil
	}
	return matchSegmentSequence(patternSegments, candidateSegments)
}

func matchSegmentSequence(patternSegments, candidateSegments []string) bool {
	if len(patternSegments) == 0 {
		return len(candidateSegments) == 0
	}

	segment := patternSegments[0]
	if segment == "**" {
		if len(patternSegments) == 1 {
			return true
		}
		for idx := 0; idx <= len(candidateSegments); idx++ {
			if matchSegmentSequence(patternSegments[1:], candidateSegments[idx:]) {
				return true
			}
		}
		return false
	}

	if len(candidateSegments) == 0 {
		return false
	}
	matched, err := pathpkg.Match(segment, candidateSegments[0])
	if err != nil || !matched {
		return false
	}
	return matchSegmentSequence(patternSegments[1:], candidateSegments[1:])
}

type workspaceGitignore struct {
	root  string
	rules []gitignoreRule
}

type gitignoreRule struct {
	pattern  string
	negated  bool
	dirOnly  bool
	anchored bool
	hasSlash bool
}

func loadWorkspaceGitignores(folders []WorkspaceFolder) []workspaceGitignore {
	matchers := make([]workspaceGitignore, 0, len(folders))
	for _, folder := range folders {
		matcher, ok := loadWorkspaceGitignore(folder)
		if ok {
			matchers = append(matchers, matcher)
		}
	}
	return matchers
}

func loadWorkspaceGitignore(folder WorkspaceFolder) (workspaceGitignore, bool) {
	root := uriToPath(folder.URI)
	if root == "" {
		return workspaceGitignore{}, false
	}

	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return workspaceGitignore{}, false
	}

	rules := parseGitignoreRules(string(data))
	if len(rules) == 0 {
		return workspaceGitignore{}, false
	}

	return workspaceGitignore{root: filepath.Clean(root), rules: rules}, true
}

func ignoresPath(matchers []workspaceGitignore, filename string, isDir bool) bool {
	for _, matcher := range matchers {
		if matcher.ignores(filename, isDir) {
			return true
		}
	}
	return false
}

func canSkipIgnoredDir(matchers []workspaceGitignore, dir string) bool {
	for _, matcher := range matchers {
		if matcher.mayReincludeDescendant() && matcher.contains(dir) {
			return false
		}
	}
	return true
}

func (m workspaceGitignore) ignores(filename string, isDir bool) bool {
	rel, ok := relativeWorkspacePath(m.root, filename)
	if !ok {
		return false
	}

	ignored := false
	for _, rule := range m.rules {
		if !rule.matches(rel, isDir) {
			continue
		}
		ignored = !rule.negated
	}
	return ignored
}

func (m workspaceGitignore) contains(filename string) bool {
	_, ok := relativeWorkspacePath(m.root, filename)
	return ok
}

func (m workspaceGitignore) mayReincludeDescendant() bool {
	for _, rule := range m.rules {
		if rule.negated {
			return true
		}
	}
	return false
}

func parseGitignoreRules(content string) []gitignoreRule {
	lines := strings.Split(content, "\n")
	rules := make([]gitignoreRule, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, `\#`) || strings.HasPrefix(line, `\!`) {
			line = line[1:]
		}

		rule := gitignoreRule{negated: strings.HasPrefix(line, "!")}
		if rule.negated {
			line = strings.TrimPrefix(line, "!")
		}
		line = strings.TrimPrefix(line, "./")
		rule.anchored = strings.HasPrefix(line, "/")
		line = strings.TrimPrefix(line, "/")
		rule.dirOnly = strings.HasSuffix(line, "/")
		line = strings.TrimSuffix(line, "/")
		line = filepath.ToSlash(line)
		line = strings.TrimPrefix(line, "./")
		if line == "" {
			continue
		}
		rule.pattern = line
		rule.hasSlash = strings.Contains(line, "/")
		rules = append(rules, rule)
	}
	return rules
}

func (r gitignoreRule) matches(rel string, isDir bool) bool {
	rel = filepath.ToSlash(rel)
	if rel == "" || rel == "." {
		return false
	}

	if r.dirOnly {
		if isDir && r.matchesPath(rel) {
			return true
		}
		for _, dir := range parentDirectories(rel) {
			if r.matchesPath(dir) {
				return true
			}
		}
		return false
	}

	if r.matchesPath(rel) {
		return true
	}

	if !r.hasSlash {
		return matchSimple(r.pattern, pathpkg.Base(rel))
	}

	return false
}

func (r gitignoreRule) matchesPath(candidate string) bool {
	for _, pattern := range r.globVariants() {
		if matchSimple(pattern, candidate) {
			return true
		}
	}
	return false
}

func (r gitignoreRule) globVariants() []string {
	if r.pattern == "" {
		return nil
	}
	if r.anchored {
		return []string{r.pattern}
	}
	return []string{r.pattern, "**/" + r.pattern}
}

func parentDirectories(rel string) []string {
	dir := pathpkg.Dir(filepath.ToSlash(rel))
	if dir == "." || dir == "" {
		return nil
	}
	parts := strings.Split(dir, "/")
	dirs := make([]string, 0, len(parts))
	for i := range parts {
		dirs = append(dirs, strings.Join(parts[:i+1], "/"))
	}
	return dirs
}

func relativeWorkspacePath(root, filename string) (string, bool) {
	root = filepath.Clean(root)
	filename = filepath.Clean(filename)
	rel, err := filepath.Rel(root, filename)
	if err != nil {
		return "", false
	}
	if rel == "." || rel == "" {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}
