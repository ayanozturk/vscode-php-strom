package indexer

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ayanozturk/vscode-php-strom/parser"
)

// WorkspaceIndexer discovers and indexes PHP files in workspace folders.
type WorkspaceIndexer struct {
	cfg          Config
	index        *Index
	folders      []WorkspaceFolder
	mu           sync.RWMutex
	onStart      func()
	onDone       func(int)
	onProgress   func(done, total int)
	indexedCount int64
}

// New creates a WorkspaceIndexer with the given configuration.
func New(cfg Config) *WorkspaceIndexer {
	return &WorkspaceIndexer{cfg: cfg, index: newIndex()}
}

// SetWorkspaceFolders updates the list of root folders to index.
func (wi *WorkspaceIndexer) SetWorkspaceFolders(folders []WorkspaceFolder) {
	wi.mu.Lock()
	wi.folders = folders
	wi.mu.Unlock()
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
	if wi.onStart != nil {
		wi.onStart()
	}

	wi.mu.RLock()
	folders := wi.folders
	wi.mu.RUnlock()

	// Collect all PHP file paths
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
				return nil
			}
			if wi.matchesAssociations(p) {
				paths = append(paths, p)
			}
			return nil
		})
	}

	total := len(paths)
	log.Printf("[indexer] discovered %d files across %d workspace folder(s)", total, len(folders))

	// Parallel parse using goroutine pool
	numWorkers := runtime.GOMAXPROCS(0)
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
				wi.indexFileWithTimeout(filePath)
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
}

// GetIndex returns the underlying symbol index for provider use.
func (wi *WorkspaceIndexer) GetIndex() *Index { return wi.index }

// ─── Internal ─────────────────────────────────────────────────────────────────

// indexFileWithTimeout parses a file inside a goroutine with a 5s deadline.
// If parsing hangs (e.g. infinite loop in the parser), the file is skipped and
// a warning is logged. The timed-out goroutine leaks but does not block others.
func (wi *WorkspaceIndexer) indexFileWithTimeout(path string) {
	const timeout = 5 * time.Second

	type result struct {
		syms []*Symbol
	}
	done := make(chan result, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 4096)
				n := runtime.Stack(buf, false)
				log.Printf("[indexer] PANIC parsing %s: %v\n%s", path, r, buf[:n])
				done <- result{}
			}
		}()
		info, err := os.Stat(path)
		if err != nil {
			done <- result{}
			return
		}
		if info.Size() > wi.cfg.MaxSize {
			log.Printf("[indexer] skipping oversized file (%d bytes): %s", info.Size(), path)
			done <- result{}
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			done <- result{}
			return
		}
		uri := pathToURI(path)
		syms := extractSymbols(uri, string(data))
		done <- result{syms: syms}
	}()

	select {
	case res := <-done:
		if len(res.syms) > 0 {
			uri := pathToURI(path)
			wi.index.PutFile(uri, res.syms)
		}
	case <-time.After(timeout):
		log.Printf("[indexer] TIMEOUT (>%s) parsing %s — skipping", timeout, path)
	}
}

func (wi *WorkspaceIndexer) shouldExcludeDir(path string) bool {
	base := filepath.Base(path)
	for _, pat := range wi.cfg.Exclude {
		if matchSimple(pat, base) {
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

// extractSymbols parses PHP source and extracts top-level declarations.
func extractSymbols(uri, src string) []*Symbol {
	file := parser.Parse(src)
	var syms []*Symbol
	var ns string
	extractFromStmts(file.Stmts, uri, ns, &syms)
	return syms
}

func extractFromStmts(stmts []parser.Stmt, uri, ns string, syms *[]*Symbol) {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *parser.NamespaceDeclStmt:
			newNS := s.Decl.Name
			if s.Decl.Stmts != nil {
				extractFromStmts(s.Decl.Stmts, uri, newNS, syms)
			} else {
				// bracket-less: rest of file uses this namespace
				ns = newNS
			}

		case *parser.ClassDeclStmt:
			fqn := fqn(ns, s.Decl.Name)
			sym := &Symbol{
				FQN: fqn, Name: s.Decl.Name, Kind: KindClass,
				Namespace: ns, URI: uri, Range: s.Decl.Pos,
				DocComment: s.Decl.DocComment,
				IsFinal:    s.Decl.Flags&parser.FlagFinal != 0,
				IsAbstract: s.Decl.Flags&parser.FlagAbstract != 0,
				IsReadonly: s.Decl.Flags&parser.FlagReadonly != 0,
				Visibility: "public",
			}
			if s.Decl.Extends != "" {
				sym.Extends = []string{s.Decl.Extends}
			}
			sym.Implements = s.Decl.Implements
			*syms = append(*syms, sym)
			extractMembers(s.Decl.Members, uri, fqn, syms)

		case *parser.InterfaceDeclStmt:
			fqn := fqn(ns, s.Decl.Name)
			sym := &Symbol{
				FQN: fqn, Name: s.Decl.Name, Kind: KindInterface,
				Namespace: ns, URI: uri, Range: s.Decl.Pos,
				DocComment: s.Decl.DocComment,
				Visibility: "public",
			}
			sym.Extends = s.Decl.Extends
			*syms = append(*syms, sym)
			extractMembers(s.Decl.Members, uri, fqn, syms)

		case *parser.TraitDeclStmt:
			fqn := fqn(ns, s.Decl.Name)
			*syms = append(*syms, &Symbol{
				FQN: fqn, Name: s.Decl.Name, Kind: KindModule,
				Namespace: ns, URI: uri, Range: s.Decl.Pos,
				DocComment: s.Decl.DocComment, Visibility: "public",
			})
			extractMembers(s.Decl.Members, uri, fqn, syms)

		case *parser.EnumDeclStmt:
			fqn := fqn(ns, s.Decl.Name)
			*syms = append(*syms, &Symbol{
				FQN: fqn, Name: s.Decl.Name, Kind: KindEnum,
				Namespace: ns, URI: uri, Range: s.Decl.Pos,
				DocComment: s.Decl.DocComment, Visibility: "public",
			})
			extractMembers(s.Decl.Members, uri, fqn, syms)

		case *parser.FunctionDeclStmt:
			fqn := fqn(ns, s.Decl.Name)
			*syms = append(*syms, &Symbol{
				FQN: fqn, Name: s.Decl.Name, Kind: KindFunction,
				Namespace: ns, URI: uri, Range: s.Decl.Pos,
				DocComment: s.Decl.DocComment, Visibility: "public",
				Params: extractParams(s.Decl.Params),
			})

		case *parser.ConstDeclStmt:
			for _, item := range s.Decl.Items {
				fqn := fqn(ns, item.Name)
				*syms = append(*syms, &Symbol{
					FQN: fqn, Name: item.Name, Kind: KindConstant,
					Namespace: ns, URI: uri, Range: item.Pos, Visibility: "public",
				})
			}
		}
	}
}

func extractMembers(members []parser.ClassMember, uri, classFQN string, syms *[]*Symbol) {
	for _, m := range members {
		switch mem := m.(type) {
		case *parser.MethodDecl:
			fqn := classFQN + "::" + mem.Name
			*syms = append(*syms, &Symbol{
				FQN: fqn, Name: mem.Name,
				Kind:       KindMethod,
				URI:        uri,
				Range:      mem.Pos,
				DocComment: mem.DocComment,
				IsStatic:   mem.Flags&parser.FlagStatic != 0,
				IsAbstract: mem.Flags&parser.FlagAbstractMember != 0,
				IsFinal:    mem.Flags&parser.FlagFinalMember != 0,
				Visibility: visibilityFromFlags(mem.Flags),
				Params:     extractParams(mem.Params),
			})
		case *parser.PropertyDecl:
			for _, item := range mem.Items {
				fqn := classFQN + "::$" + item.Name
				*syms = append(*syms, &Symbol{
					FQN: fqn, Name: item.Name,
					Kind:       KindProperty,
					URI:        uri,
					Range:      item.Pos,
					DocComment: mem.DocComment,
					IsStatic:   mem.Flags&parser.FlagStatic != 0,
					IsReadonly: mem.Flags&parser.FlagReadonlyMember != 0,
					Visibility: visibilityFromFlags(mem.Flags),
				})
			}
		case *parser.ClassConstDecl:
			for _, item := range mem.Items {
				fqn := classFQN + "::" + item.Name
				*syms = append(*syms, &Symbol{
					FQN: fqn, Name: item.Name,
					Kind:       KindConstant,
					URI:        uri,
					Range:      item.Pos,
					DocComment: mem.DocComment,
					Visibility: visibilityFromFlags(mem.Flags),
				})
			}
		case *parser.EnumCase:
			fqn := classFQN + "::" + mem.Name
			*syms = append(*syms, &Symbol{
				FQN: fqn, Name: mem.Name,
				Kind:       KindEnumMember,
				URI:        uri,
				Range:      mem.Pos,
				DocComment: mem.DocComment,
				Visibility: "public",
			})
		}
	}
}

func extractParams(params []parser.Param) []SymbolParam {
	sp := make([]SymbolParam, len(params))
	for i, p := range params {
		sp[i] = SymbolParam{
			Name:        p.Name,
			HasDefault:  p.Default != nil,
			IsVariadic:  p.Variadic,
			IsPassByRef: p.ByRef,
		}
		if p.Type != nil {
			sp[i].Type = typeNodeToString(p.Type)
		}
	}
	return sp
}

func typeNodeToString(t parser.TypeNode) string {
	if t == nil {
		return ""
	}
	switch n := t.(type) {
	case *parser.NamedType:
		return n.Name
	case *parser.NullableType:
		return "?" + typeNodeToString(n.Inner)
	case *parser.UnionType:
		parts := make([]string, len(n.Types))
		for i, t := range n.Types {
			parts[i] = typeNodeToString(t)
		}
		return strings.Join(parts, "|")
	case *parser.IntersectionType:
		parts := make([]string, len(n.Types))
		for i, t := range n.Types {
			parts[i] = typeNodeToString(t)
		}
		return strings.Join(parts, "&")
	}
	return ""
}

func visibilityFromFlags(f parser.MemberFlags) string {
	if f&parser.FlagPrivate != 0 {
		return "private"
	}
	if f&parser.FlagProtected != 0 {
		return "protected"
	}
	return "public"
}

func fqn(ns, name string) string {
	if ns == "" {
		return `\` + name
	}
	return `\` + ns + `\` + name
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
	// Simple glob: only support leading/trailing *
	pattern = strings.TrimPrefix(pattern, "**/")
	pattern = strings.TrimSuffix(pattern, "/**")
	return strings.Contains(name, strings.Trim(pattern, "*"))
}
