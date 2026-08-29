package indexer

import (
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"maps"
	"math"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ayanozturk/go-php-parser/analyse"
	"github.com/ayanozturk/go-php-parser/ast"
	goplexer "github.com/ayanozturk/go-php-parser/lexer"
	goparser "github.com/ayanozturk/go-php-parser/parser"
)

// WorkspaceIndexer discovers and indexes PHP files in workspace folders.
type WorkspaceIndexer struct {
	cfg              Config
	index            *Index
	project          *analyse.ProjectIndex
	projectNodes     map[string][]ast.Node
	projectHashes    map[string]uint64
	projectRevision  uint64
	folders          []WorkspaceFolder
	workspaceURIs    []string
	gitignores       []workspaceGitignore
	compiledExcludes [][]string
	stubURIs         []string
	mu               sync.RWMutex
	onStart          func()
	onDone           func(IndexingSummary)
	onProgress       func(done, total int)
	processedCount   atomic.Int64
	indexedCount     atomic.Int64
	indexedLines     atomic.Int64
	indexedBytes     atomic.Int64
}

const perFileParseTimeout = 20 * time.Second

// IndexingSummary reports the aggregate work completed by a workspace index.
type IndexingSummary struct {
	FilesDiscovered int
	FilesIndexed    int
	SymbolsIndexed  int
	LinesScanned    int64
	BytesScanned    int64
	Duration        time.Duration
}

// ParsedFile contains the single-parse result used for both indexing and diagnostics.
type ParsedFile struct {
	URI     string
	Text    string
	Nodes   []ast.Node
	Errors  []string
	Symbols []*Symbol
	Lines   int
	Bytes   int
}

func splitPathSegments(p string) []string {
	p = filepath.ToSlash(p)
	p = strings.TrimPrefix(pathpkg.Clean(p), "./")
	p = strings.Trim(p, "/")
	if p == "" || p == "." {
		return nil
	}
	return strings.Split(p, "/")
}

// New creates a WorkspaceIndexer with the given configuration.
func New(cfg Config) *WorkspaceIndexer {
	wi := &WorkspaceIndexer{
		cfg:              cfg,
		index:            newIndex(),
		project:          analyse.NewProjectIndex(),
		projectNodes:     make(map[string][]ast.Node),
		projectHashes:    make(map[string]uint64),
		compiledExcludes: compileExcludePatterns(cfg.Exclude),
	}
	wi.loadConfiguredStubs()
	return wi
}

// UpdateConfig applies file discovery and size settings to future indexing work.
func (wi *WorkspaceIndexer) UpdateConfig(cfg Config) {
	wi.mu.Lock()
	wi.cfg = cfg
	wi.compiledExcludes = compileExcludePatterns(cfg.Exclude)
	wi.mu.Unlock()
}

func compileExcludePatterns(patterns []string) [][]string {
	var compiled [][]string
	for _, pattern := range patterns {
		for _, expanded := range expandBracePattern(filepath.ToSlash(pattern)) {
			compiled = append(compiled, splitPathSegments(expanded))
		}
	}
	return compiled
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
func (wi *WorkspaceIndexer) OnIndexingDone(fn func(IndexingSummary)) { wi.onDone = fn }

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
	started := time.Now()
	skipFunctionBodies := visitor == nil
	if wi.onStart != nil {
		wi.onStart()
	}

	log.Printf("[indexer] scanning workspace folders for PHP files...")

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

	numWorkers := WorkerCountFor(total)
	log.Printf("[indexer] starting %d worker(s) to overlap I/O and parsing", numWorkers)

	jobs := make(chan string, len(paths))
	for _, p := range paths {
		jobs <- p
	}
	close(jobs)

	var wg sync.WaitGroup
	parsedProjectNodes := make(map[string][]ast.Node, len(paths))
	parsedProjectHashes := make(map[string]uint64, len(paths))
	var parsedProjectMu sync.Mutex
	wi.processedCount.Store(0)
	wi.indexedCount.Store(0)
	wi.indexedLines.Store(0)
	wi.indexedBytes.Store(0)

	// Report at most every 1% of files (min 10, max 200) to avoid flooding the client.
	reportEvery := max(total/100, 10)
	if reportEvery > 200 {
		reportEvery = 200
	}

	for workerID := range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Printf("[indexer] worker %d started", workerID)
			for filePath := range jobs {
				lines, bytesScanned, indexed := wi.indexFile(filePath, skipFunctionBodies, visitor, func(parsed ParsedFile) {
					parsedProjectMu.Lock()
					key := projectIndexKey(parsed.URI)
					parsedProjectNodes[key] = parsed.Nodes
					parsedProjectHashes[key] = sourceHash(parsed.Text)
					parsedProjectMu.Unlock()
				})
				if indexed {
					wi.indexedCount.Add(1)
					wi.indexedLines.Add(int64(lines))
					wi.indexedBytes.Add(int64(bytesScanned))
				}
				done := int(wi.processedCount.Add(1))
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

	wi.replaceWorkspaceProjectNodes(parsedProjectNodes, parsedProjectHashes)

	count := int(wi.indexedCount.Load())
	summary := IndexingSummary{
		FilesDiscovered: total,
		FilesIndexed:    count,
		SymbolsIndexed:  wi.index.Size(),
		LinesScanned:    wi.indexedLines.Load(),
		BytesScanned:    wi.indexedBytes.Load(),
		Duration:        time.Since(started),
	}
	log.Printf(
		"[indexer] finished — %d/%d files indexed, %d LOC scanned, %d symbols in %s",
		summary.FilesIndexed,
		summary.FilesDiscovered,
		summary.LinesScanned,
		summary.SymbolsIndexed,
		summary.Duration.Round(time.Millisecond),
	)
	if wi.onDone != nil {
		wi.onDone(summary)
	}
}

// WorkerCountFor returns a conservative worker count for CPU-heavy PHP parsing.
func WorkerCountFor(total int) int {
	if total <= 0 {
		return 0
	}
	workers := max(runtime.GOMAXPROCS(0), 1)
	if workers > 4 {
		workers = 4
	}
	if workers > total {
		workers = total
	}
	return workers
}

// DiagnosticWorkerCountFor scales diagnostics workers with file count while
// preserving interactive responsiveness. Large workspaces get more workers.
func DiagnosticWorkerCountFor(total int) int {
	base := WorkerCountFor(total)
	if total < 1000 {
		if base > 2 {
			return 2
		}
	} else if total < 10000 {
		if base > 3 {
			return 3
		}
	} else {
		if base > 4 {
			return 4
		}
	}
	return base
}

// IndexDocument re-indexes a single open document from its text content.
func (wi *WorkspaceIndexer) IndexDocument(uri, text string) {
	key := projectIndexKey(uri)
	hash := sourceHash(text)
	wi.mu.RLock()
	lastHash, seen := wi.projectHashes[key]
	unchanged := seen && lastHash == hash
	wi.mu.RUnlock()
	if unchanged {
		wi.trackWorkspaceURI(uri)
		return
	}

	parsed := ParseSource(uri, text)
	wi.index.PutFile(uri, parsed.Symbols)
	wi.putProjectNodes(uri, parsed.Nodes, hash)
	wi.trackWorkspaceURI(uri)
}

// RemoveDocument removes all symbols for a document URI from the index.
func (wi *WorkspaceIndexer) RemoveDocument(uri string) {
	wi.index.RemoveFile(uri)
	wi.removeProjectNodes(uri)
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

// ProjectIndex returns the parser-native project index used by analysis rules.
func (wi *WorkspaceIndexer) ProjectIndex() *analyse.ProjectIndex {
	wi.mu.RLock()
	defer wi.mu.RUnlock()
	return wi.project
}

// ProjectIndexForFile returns a project index suitable for analysing the given
// source. If the workspace index already has the same content, it reuses the
// cached project index; otherwise it overlays the current file without mutating
// workspace state.
func (wi *WorkspaceIndexer) ProjectIndexForFile(filename, text string, nodes []ast.Node) *analyse.ProjectIndex {
	project, _ := wi.ProjectIndexSnapshotForFile(filename, text, nodes)
	return project
}

// ProjectIndexSnapshotForFile returns an immutable project-index view and the
// workspace revision it was derived from. The revision changes whenever the
// workspace project graph is rebuilt, allowing semantic consumers to retain a
// snapshot only while its cross-file symbols remain current.
func (wi *WorkspaceIndexer) ProjectIndexSnapshotForFile(filename, text string, nodes []ast.Node) (*analyse.ProjectIndex, uint64) {
	hash := sourceHash(text)
	wi.mu.RLock()
	revision := wi.projectRevision
	if lastHash, seen := wi.projectHashes[filename]; seen && lastHash == hash {
		project := wi.project
		wi.mu.RUnlock()
		return project, revision
	}
	parsed := make(map[string][]ast.Node, len(wi.projectNodes)+1)
	maps.Copy(parsed, wi.projectNodes)
	wi.mu.RUnlock()

	parsed[filename] = nodes
	return analyse.BuildProjectIndex(parsed), revision
}

func (wi *WorkspaceIndexer) SetStubs(stubsPath string, stubs []string, phpVersion string) {
	wi.mu.Lock()
	for _, uri := range wi.stubURIs {
		wi.index.RemoveFile(uri)
		key := projectIndexKey(uri)
		delete(wi.projectNodes, key)
		delete(wi.projectHashes, key)
	}
	wi.stubURIs = nil
	wi.cfg.StubsPath = stubsPath
	wi.cfg.Stubs = append([]string(nil), stubs...)
	wi.cfg.PHPVersion = phpVersion
	wi.rebuildProjectIndexLocked()
	wi.mu.Unlock()

	wi.loadConfiguredStubs()
}

func (wi *WorkspaceIndexer) putProjectNodes(uri string, nodes []ast.Node, hash uint64) {
	wi.mu.Lock()
	key := projectIndexKey(uri)
	wi.projectNodes[key] = nodes
	wi.projectHashes[key] = hash
	wi.rebuildProjectIndexLocked()
	wi.mu.Unlock()
}

func (wi *WorkspaceIndexer) removeProjectNodes(uri string) {
	wi.mu.Lock()
	key := projectIndexKey(uri)
	delete(wi.projectNodes, key)
	delete(wi.projectHashes, key)
	wi.rebuildProjectIndexLocked()
	wi.mu.Unlock()
}

func (wi *WorkspaceIndexer) replaceWorkspaceProjectNodes(nodes map[string][]ast.Node, hashes map[string]uint64) {
	wi.mu.Lock()
	next := make(map[string][]ast.Node, len(nodes)+len(wi.stubURIs))
	nextHashes := make(map[string]uint64, len(hashes)+len(wi.stubURIs))
	for _, uri := range wi.stubURIs {
		key := projectIndexKey(uri)
		if stubNodes, ok := wi.projectNodes[key]; ok {
			next[key] = stubNodes
			nextHashes[key] = wi.projectHashes[key]
		}
	}
	for uri, parsedNodes := range nodes {
		next[uri] = parsedNodes
		nextHashes[uri] = hashes[uri]
	}
	wi.projectNodes = next
	wi.projectHashes = nextHashes
	wi.rebuildProjectIndexLocked()
	wi.mu.Unlock()
}

func (wi *WorkspaceIndexer) rebuildProjectIndexLocked() {
	parsed := make(map[string][]ast.Node, len(wi.projectNodes))
	maps.Copy(parsed, wi.projectNodes)
	wi.project = analyse.BuildProjectIndex(parsed)
	wi.projectRevision++
}

func projectIndexKey(uri string) string {
	return uriToPath(uri)
}

func sourceHash(text string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(text))
	return h.Sum64()
}

// ─── Internal ─────────────────────────────────────────────────────────────────

func (wi *WorkspaceIndexer) collectWorkspaceFilePaths(folders []WorkspaceFolder, gitignores []workspaceGitignore) []string {
	return wi.collectWorkspaceFilePathsParallel(folders, gitignores)
}

func (wi *WorkspaceIndexer) collectWorkspaceFilePathsParallel(folders []WorkspaceFolder, gitignores []workspaceGitignore) []string {
	var paths []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Keep discovery I/O bounded so activation does not monopolise the disk.
	sem := make(chan struct{}, 8)

	var walk func(string)
	walk = func(path string) {
		entries, err := os.ReadDir(path)
		if err != nil {
			return
		}

		var subdirs []string
		for _, d := range entries {
			p := filepath.Join(path, d.Name())
			entryType := d.Type()
			if entryType&os.ModeSymlink != 0 {
				continue
			}
			if d.IsDir() {
				if wi.shouldExcludeDir(p) {
					continue
				}
				if ignoresPath(gitignores, p, true) && !shouldIndexGitignoredPath(p) {
					if canSkipIgnoredDir(gitignores, p) {
						continue
					}
				}
				subdirs = append(subdirs, p)
			} else {
				if !entryType.IsRegular() {
					continue
				}
				if ignoresPath(gitignores, p, false) && !shouldIndexGitignoredPath(p) {
					continue
				}
				if wi.matchesAssociations(p) {
					mu.Lock()
					paths = append(paths, p)
					mu.Unlock()
				}
			}
		}

		for _, subdir := range subdirs {
			select {
			case sem <- struct{}{}:
				// Spawn a concurrent worker for this subdirectory
				wg.Add(1)
				go func(dir string) {
					defer func() {
						<-sem
						wg.Done()
					}()
					walk(dir)
				}(subdir)
			default:
				// Fall back to walking sequentially in the current goroutine
				walk(subdir)
			}
		}
	}

	for _, folder := range folders {
		root := uriToPath(folder.URI)
		if root != "" {
			walk(root)
		}
	}

	wg.Wait()
	return paths
}

func shouldIndexGitignoredPath(path string) bool {
	return slices.Contains(splitPathSegments(path), "vendor")
}

func (wi *WorkspaceIndexer) trackWorkspaceURI(uri string) {
	wi.mu.Lock()
	defer wi.mu.Unlock()
	if slices.Contains(wi.workspaceURIs, uri) {
		return
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

func (wi *WorkspaceIndexer) indexFile(path string, skipFunctionBodies bool, visitor func(ParsedFile), projectCollector func(ParsedFile)) (int, int, bool) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			log.Printf("[indexer] PANIC parsing %s: %v\n%s", path, r, buf[:n])
		}
	}()

	parsed, skipReason := wi.parseIndexableFile(path, skipFunctionBodies)
	if skipReason != "" {
		if strings.HasPrefix(skipReason, "timeout") {
			log.Printf("[indexer] %s: %s", skipReason, path)
		}
		return 0, 0, false
	}

	wi.index.PutFile(parsed.URI, parsed.Symbols)
	if projectCollector != nil {
		projectCollector(parsed)
	}
	if visitor != nil {
		visitor(parsed)
	}
	return parsed.Lines, parsed.Bytes, true
}

func (wi *WorkspaceIndexer) parseIndexableFile(path string, skipFunctionBodies bool) (ParsedFile, string) {
	ctx, cancel := context.WithTimeout(context.Background(), perFileParseTimeout)
	defer cancel()

	wi.mu.RLock()
	maxSize := wi.cfg.MaxSize
	wi.mu.RUnlock()
	data, size, oversized, err := ReadFileWithinLimit(path, maxSize)
	if err != nil {
		return ParsedFile{}, "read-error"
	}
	if oversized {
		log.Printf("[indexer] skipping oversized file (observed %d bytes, limit %d): %s", size, maxSize, path)
		return ParsedFile{}, "oversized-file"
	}

	uri := pathToURI(path)
	var parsed ParsedFile
	if skipFunctionBodies {
		parsed = ParseSourceForIndexWithContext(ctx, uri, string(data))
	} else {
		parsed = ParseSourceWithContext(ctx, uri, string(data))
	}
	parsed.Lines = countLines(data)
	parsed.Bytes = len(data)

	if ctx.Err() != nil {
		return ParsedFile{}, "timeout>20s"
	}

	return parsed, ""
}

// ReadFileWithinLimit reads at most maxSize+1 bytes so callers can reject
// oversized or concurrently growing files without allocating their full contents.
func ReadFileWithinLimit(path string, maxSize int64) ([]byte, int64, bool, error) {
	file, err := openFileForBoundedRead(path)
	if err != nil {
		return nil, 0, false, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, 0, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, false, fmt.Errorf("read %s: not a regular file: %w", path, os.ErrInvalid)
	}
	if info.Size() > maxSize {
		return nil, info.Size(), true, nil
	}

	data, oversized, err := readAtMostWithSizeHint(file, maxSize, info.Size())
	if err != nil {
		return nil, 0, false, err
	}
	if oversized {
		return nil, maxSize + 1, true, nil
	}
	return data, int64(len(data)), false, nil
}

func readAtMost(reader io.Reader, maxSize int64) ([]byte, bool, error) {
	return readAtMostWithSizeHint(reader, maxSize, 0)
}

func readAtMostWithSizeHint(reader io.Reader, maxSize, sizeHint int64) ([]byte, bool, error) {
	readLimit := maxSize
	if readLimit < math.MaxInt64 {
		readLimit++
	}

	var buffer bytes.Buffer
	maxInt := int64(^uint(0) >> 1)
	if sizeHint > 0 && sizeHint <= maxInt-int64(bytes.MinRead) {
		buffer.Grow(int(sizeHint) + bytes.MinRead)
	}
	_, err := buffer.ReadFrom(io.LimitReader(reader, readLimit))
	if err != nil {
		return nil, false, err
	}
	data := buffer.Bytes()
	if int64(len(data)) > maxSize {
		return nil, true, nil
	}
	return data, false, nil
}

func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	lines := bytes.Count(data, []byte{'\n'})
	if data[len(data)-1] != '\n' {
		lines++
	}
	return lines
}

func (wi *WorkspaceIndexer) shouldExcludeDir(path string) bool {
	wi.mu.RLock()
	compiledExcludes := append([][]string(nil), wi.compiledExcludes...)
	wi.mu.RUnlock()
	if len(compiledExcludes) == 0 {
		return false
	}
	segments := splitPathSegments(path)
	for _, patternSegments := range compiledExcludes {
		if matchSegmentSequence(patternSegments, segments) {
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

// ParseSourceForIndex parses PHP source with function/method bodies skipped.
// It is intended for symbol indexing, where declarations and signatures matter
// but statement bodies are not needed.
func ParseSourceForIndex(uri, src string) ParsedFile {
	return ParseSourceForIndexWithContext(context.Background(), uri, src)
}

// ParseSourceForIndexWithContext is the cancellable variant of ParseSourceForIndex.
func ParseSourceForIndexWithContext(ctx context.Context, uri, src string) ParsedFile {
	return parseSource(ctx, uri, src, true)
}

// ParseSourceWithContext parses PHP source using a cooperative context to abort early if requested.
func ParseSourceWithContext(ctx context.Context, uri, src string) ParsedFile {
	return parseSource(ctx, uri, src, false)
}

func parseSource(ctx context.Context, uri, src string, skipFunctionBodies bool) ParsedFile {
	l := goplexer.New(src)
	p := goparser.New(l, false)
	p.Ctx = ctx
	p.SkipFunctionBodies = skipFunctionBodies
	nodes := p.Parse()
	recoverMissingMemberPHPDocs(nodes, src)
	errs := append([]string(nil), p.Errors()...)
	return ParsedFile{
		URI:     uri,
		Text:    src,
		Nodes:   nodes,
		Errors:  errs,
		Symbols: extractSymbolsFromNodes(uri, nodes),
		Lines:   countLines([]byte(src)),
		Bytes:   len(src),
	}
}

func recoverMissingMemberPHPDocs(nodes []ast.Node, src string) {
	for _, node := range nodes {
		switch n := node.(type) {
		case *ast.NamespaceNode:
			recoverMissingMemberPHPDocs(n.Body, src)
		case *ast.ClassNode:
			for _, methodNode := range n.Methods {
				if method, ok := methodNode.(*ast.FunctionNode); ok {
					if method.PHPDoc == nil {
						method.PHPDoc = phpDocImmediatelyBefore(src, method.GetPos().Offset)
					}
					repairGenericPHPDocTypes(method.PHPDoc)
				}
			}
		case *ast.InterfaceNode:
			for _, member := range n.Members {
				if method, ok := member.(*ast.InterfaceMethodNode); ok {
					if method.PHPDoc == nil {
						method.PHPDoc = phpDocImmediatelyBefore(src, method.GetPos().Offset)
					}
					repairGenericPHPDocTypes(method.PHPDoc)
				}
			}
		}
	}
}

func repairGenericPHPDocTypes(doc *ast.PHPDocNode) {
	if doc == nil {
		return
	}
	for _, rawLine := range strings.Split(doc.RawContent, "\n") {
		line := strings.TrimSpace(rawLine)
		line = strings.TrimSpace(strings.TrimPrefix(line, "/**"))
		line = strings.TrimSpace(strings.TrimSuffix(line, "*/"))
		line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		switch {
		case strings.HasPrefix(line, "@return"):
			typeName, _ := phpDocTypeAndRest(strings.TrimSpace(strings.TrimPrefix(line, "@return")))
			doc.ReturnType = typeName
		case strings.HasPrefix(line, "@var"):
			typeName, _ := phpDocTypeAndRest(strings.TrimSpace(strings.TrimPrefix(line, "@var")))
			doc.VarType = typeName
		case strings.HasPrefix(line, "@param"):
			typeName, remainder := phpDocTypeAndRest(strings.TrimSpace(strings.TrimPrefix(line, "@param")))
			fields := strings.Fields(remainder)
			if typeName == "" || len(fields) == 0 {
				continue
			}
			name := strings.TrimPrefix(fields[0], "$")
			for paramIdx := range doc.Params {
				if doc.Params[paramIdx].Name == name {
					doc.Params[paramIdx].Type = typeName
					break
				}
			}
		}
	}
}

func phpDocTypeAndRest(value string) (string, string) {
	value = strings.TrimSpace(value)
	depth := 0
	for idx, r := range value {
		switch r {
		case '<', '(', '{', '[':
			depth++
		case '>', ')', '}', ']':
			if depth > 0 {
				depth--
			}
		case ' ', '\t':
			if depth == 0 {
				return strings.TrimSpace(value[:idx]), strings.TrimSpace(value[idx:])
			}
		}
	}
	return strings.TrimSpace(value), ""
}

func phpDocImmediatelyBefore(src string, offset int) *ast.PHPDocNode {
	if offset <= 0 || offset > len(src) {
		return nil
	}
	prefix := src[:offset]
	end := strings.LastIndex(prefix, "*/")
	if end < 0 {
		return nil
	}
	start := strings.LastIndex(prefix[:end], "/**")
	if start < 0 {
		return nil
	}
	if strings.Contains(prefix[start:end], "*/") {
		return nil
	}
	for _, field := range strings.Fields(prefix[end+2:]) {
		switch strings.ToLower(field) {
		case "public", "protected", "private", "static", "final", "abstract", "readonly", "&":
		default:
			return nil
		}
	}
	return ast.ExtractPHPDocFromComment(prefix[start : end+2])
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
			templates, genericParents := extractGenericMetadata(n.PHPDoc, ctx)
			sym := &Symbol{
				FQN:            classFQN,
				Name:           n.Name,
				Kind:           KindClass,
				Namespace:      ctx.namespace,
				URI:            uri,
				Range:          positionRange(n.GetPos()),
				DocComment:     docRaw(n.PHPDoc),
				Templates:      templates,
				GenericParents: genericParents,
				IsFinal:        hasModifier(n.Modifier, "final"),
				IsAbstract:     hasModifier(n.Modifier, "abstract"),
				Visibility:     "public",
			}
			if n.Extends != "" {
				sym.Extends = []string{resolveClassLike(ctx, n.Extends)}
			}
			for _, implemented := range n.Implements {
				sym.Implements = append(sym.Implements, resolveClassLike(ctx, implemented))
			}
			*syms = append(*syms, sym)
			extractPHPDocMethods(n.PHPDoc, uri, classFQN, ctx, syms)
			extractClassMembers(n, uri, classFQN, ctx, templates, syms)

		case *ast.InterfaceNode:
			interfaceFQN := fqn(ctx.namespace, n.Name)
			templates, genericParents := extractGenericMetadata(n.PHPDoc, ctx)
			extends := make([]string, 0, len(n.Extends))
			for _, parent := range n.Extends {
				extends = append(extends, resolveClassLike(ctx, parent))
			}
			*syms = append(*syms, &Symbol{
				FQN:            interfaceFQN,
				Name:           n.Name,
				Kind:           KindInterface,
				Namespace:      ctx.namespace,
				URI:            uri,
				Range:          positionRange(n.GetPos()),
				DocComment:     docRaw(n.PHPDoc),
				Extends:        extends,
				Templates:      templates,
				GenericParents: genericParents,
				Visibility:     "public",
			})
			extractInterfaceMembers(n.Members, uri, interfaceFQN, ctx, templates, syms)

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

	firstSegment, remainder, _ := strings.Cut(name, `\`)
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
	return resolveTypeHintWithTemplates(ctx, raw, nil)
}

func resolveTypeHintWithTemplates(ctx extractionContext, raw string, templates []string) string {
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
	templateSet := make(map[string]struct{}, len(templates))
	for _, template := range templates {
		templateSet[template] = struct{}{}
	}
	for idx, part := range parts {
		intersections := strings.Split(part, "&")
		for innerIdx, atom := range intersections {
			atom = strings.TrimSpace(atom)
			if _, ok := templateSet[atom]; ok {
				intersections[innerIdx] = atom
				continue
			}
			if name, arguments, ok := parseGenericTypeReference(atom); ok {
				for argumentIdx := range arguments {
					arguments[argumentIdx] = resolveGenericArgument(ctx, arguments[argumentIdx], templates)
				}
				base := name
				if isResolvableClassLikeType(name) {
					base = strings.TrimPrefix(resolveClassLike(ctx, name), `\`)
				}
				intersections[innerIdx] = base + "<" + strings.Join(arguments, ",") + ">"
				continue
			}
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

func extractGenericMetadata(doc *ast.PHPDocNode, ctx extractionContext) ([]string, []GenericParent) {
	if doc == nil {
		return nil, nil
	}
	var templates []string
	var parentExpressions []string
	for _, rawLine := range strings.Split(doc.RawContent, "\n") {
		line := strings.TrimSpace(rawLine)
		line = strings.TrimSpace(strings.TrimPrefix(line, "/**"))
		line = strings.TrimSpace(strings.TrimSuffix(line, "*/"))
		line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		tag := strings.ToLower(strings.TrimPrefix(fields[0], "@"))
		switch tag {
		case "template", "template-covariant", "template-contravariant", "phpstan-template", "psalm-template":
			templates = append(templates, fields[1])
		case "extends", "template-extends", "phpstan-extends", "psalm-extends", "implements", "template-implements", "phpstan-implements", "psalm-implements":
			parentExpressions = append(parentExpressions, strings.Join(fields[1:], " "))
		}
	}

	parents := make([]GenericParent, 0, len(parentExpressions))
	for _, expression := range parentExpressions {
		name, arguments, ok := parseGenericTypeReference(expression)
		if !ok || len(arguments) == 0 {
			continue
		}
		parent := GenericParent{FQN: resolveClassLike(ctx, name)}
		for _, argument := range arguments {
			parent.TypeArguments = append(parent.TypeArguments, resolveGenericArgument(ctx, argument, templates))
		}
		parents = append(parents, parent)
	}
	return templates, parents
}

func parseGenericTypeReference(raw string) (string, []string, bool) {
	raw = strings.TrimSpace(raw)
	open := strings.Index(raw, "<")
	if open <= 0 {
		return "", nil, false
	}
	depth, close := 0, -1
	for idx, r := range raw[open:] {
		switch r {
		case '<':
			depth++
		case '>':
			depth--
			if depth == 0 {
				close = open + idx
			}
		}
		if close >= 0 {
			break
		}
	}
	if close < 0 {
		return "", nil, false
	}
	return strings.TrimSpace(raw[:open]), splitGenericArguments(raw[open+1 : close]), true
}

func splitGenericArguments(raw string) []string {
	start, depth := 0, 0
	var parts []string
	for idx, r := range raw {
		switch r {
		case '<', '(', '{', '[':
			depth++
		case '>', ')', '}', ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(raw[start:idx]))
				start = idx + 1
			}
		}
	}
	return append(parts, strings.TrimSpace(raw[start:]))
}

func resolveGenericArgument(ctx extractionContext, raw string, templates []string) string {
	raw = strings.TrimSpace(raw)
	if slices.Contains(templates, raw) {
		return raw
	}
	if name, arguments, ok := parseGenericTypeReference(raw); ok {
		for i := range arguments {
			arguments[i] = resolveGenericArgument(ctx, arguments[i], templates)
		}
		base := name
		if isResolvableClassLikeType(name) {
			base = strings.TrimPrefix(resolveClassLike(ctx, name), `\`)
		}
		return base + "<" + strings.Join(arguments, ",") + ">"
	}
	return resolveTypeHintWithTemplates(ctx, raw, templates)
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

func extractClassMembers(class *ast.ClassNode, uri, classFQN string, ctx extractionContext, templates []string, syms *[]*Symbol) {
	*syms = append(*syms, promotedPropertySymbols(class, uri, classFQN, ctx)...)

	for _, methodNode := range class.Methods {
		method, ok := methodNode.(*ast.FunctionNode)
		if !ok {
			continue
		}
		visibility := visibilityFromModifiers(method.Visibility, method.Modifiers)
		returnType := method.ReturnType
		if method.PHPDoc != nil && method.PHPDoc.ReturnType != "" {
			returnType = method.PHPDoc.ReturnType
		}
		*syms = append(*syms, &Symbol{
			FQN:        classFQN + "::" + method.Name,
			Name:       method.Name,
			Kind:       KindMethod,
			URI:        uri,
			Range:      positionRange(method.GetPos()),
			DocComment: docRaw(method.PHPDoc),
			ReturnType: resolveTypeHintWithTemplates(ctx, returnType, templates),
			IsStatic:   hasModifierList(method.Modifiers, "static"),
			IsAbstract: hasModifierList(method.Modifiers, "abstract"),
			IsFinal:    hasModifierList(method.Modifiers, "final"),
			Visibility: visibility,
			Params:     extractParamsWithPHPDoc(ctx, method.Params, method.PHPDoc, templates),
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

var phpdocMethodPattern = regexp.MustCompile(`^@method\s+(?:(static)\s+)?([^\s]+)\s+([A-Za-z_][A-Za-z0-9_]*)\s*\((.*)\)`)

func extractPHPDocMethods(doc *ast.PHPDocNode, uri, classFQN string, ctx extractionContext, syms *[]*Symbol) {
	if doc == nil {
		return
	}
	for _, line := range phpdocLines(doc.RawContent) {
		matches := phpdocMethodPattern.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		*syms = append(*syms, &Symbol{
			FQN:        classFQN + "::" + matches[3],
			Name:       matches[3],
			Kind:       KindMethod,
			URI:        uri,
			Range:      positionRange(doc.GetPos()),
			DocComment: doc.RawContent,
			ReturnType: resolveTypeHint(ctx, matches[2]),
			IsStatic:   matches[1] == "static",
			Visibility: "public",
			Params:     phpdocMethodParams(ctx, matches[4]),
		})
	}
}

func phpdocLines(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "/**")
	raw = strings.TrimSuffix(raw, "*/")

	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "*")
		lines[i] = strings.TrimSpace(line)
	}
	return lines
}

func phpdocMethodParams(ctx extractionContext, raw string) []SymbolParam {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	parts := splitGenericArguments(raw)
	params := make([]SymbolParam, 0, len(parts))
	for _, part := range parts {
		param, ok := phpdocMethodParam(ctx, part)
		if ok {
			params = append(params, param)
		}
	}
	return params
}

func phpdocMethodParam(ctx extractionContext, raw string) (SymbolParam, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return SymbolParam{}, false
	}
	hasDefault := strings.Contains(raw, "=")
	if beforeDefault, _, ok := strings.Cut(raw, "="); ok {
		raw = strings.TrimSpace(beforeDefault)
	}

	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return SymbolParam{}, false
	}
	nameToken := fields[len(fields)-1]
	isVariadic := strings.Contains(nameToken, "...")
	isPassByRef := strings.Contains(nameToken, "&")
	name := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(nameToken, "..."), "&"), "$")
	if name == "" {
		return SymbolParam{}, false
	}

	typeHint := ""
	if len(fields) > 1 {
		typeHint = strings.Join(fields[:len(fields)-1], " ")
	}
	return SymbolParam{
		Name:        name,
		Type:        resolveTypeHint(ctx, typeHint),
		HasDefault:  hasDefault,
		IsVariadic:  isVariadic,
		IsPassByRef: isPassByRef,
	}, true
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

func extractInterfaceMembers(members []ast.Node, uri, interfaceFQN string, ctx extractionContext, templates []string, syms *[]*Symbol) {
	for _, member := range members {
		switch n := member.(type) {
		case *ast.InterfaceMethodNode:
			returnType := typeNodeToString(n.ReturnType)
			if n.PHPDoc != nil && n.PHPDoc.ReturnType != "" {
				returnType = n.PHPDoc.ReturnType
			}
			*syms = append(*syms, &Symbol{
				FQN:        interfaceFQN + "::" + n.Name,
				Name:       n.Name,
				Kind:       KindMethod,
				URI:        uri,
				Range:      positionRange(n.GetPos()),
				DocComment: docRaw(n.PHPDoc),
				ReturnType: resolveTypeHintWithTemplates(ctx, returnType, templates),
				Visibility: defaultVisibility(n.Visibility),
				Params:     extractParamsWithPHPDoc(ctx, n.Params, n.PHPDoc, templates),
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
	return extractParamsWithPHPDoc(ctx, params, nil, nil)
}

func extractParamsWithPHPDoc(ctx extractionContext, params []ast.Node, doc *ast.PHPDocNode, templates []string) []SymbolParam {
	sp := make([]SymbolParam, 0, len(params))
	for _, param := range params {
		p, ok := param.(*ast.ParamNode)
		if !ok {
			continue
		}
		typeHint := paramTypeToString(p)
		if doc != nil {
			if documented := doc.GetParamTypeFromPHPDoc(p.Name); documented != "" {
				typeHint = documented
			}
		}
		sp = append(sp, SymbolParam{
			Name:        p.Name,
			Type:        resolveTypeHintWithTemplates(ctx, typeHint, templates),
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
	return slices.Contains(modifiers, want)
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
	root         string
	rootSegments []string
	rules        []gitignoreRule
}

type gitignoreRule struct {
	pattern          string
	negated          bool
	dirOnly          bool
	anchored         bool
	hasSlash         bool
	expandedPatterns [][]string
	simpleGlobs      []string
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

	return workspaceGitignore{
		root:         filepath.Clean(root),
		rootSegments: splitPathSegments(root),
		rules:        rules,
	}, true
}

func ignoresPath(matchers []workspaceGitignore, filename string, isDir bool) bool {
	segments := splitPathSegments(filename)
	for _, matcher := range matchers {
		if matcher.ignoresWithSegments(segments, isDir) {
			return true
		}
	}
	return false
}

func canSkipIgnoredDir(matchers []workspaceGitignore, dir string) bool {
	segments := splitPathSegments(dir)
	for _, matcher := range matchers {
		if matcher.mayReincludeDescendant() && matcher.containsWithSegments(segments) {
			return false
		}
	}
	return true
}

func (m workspaceGitignore) ignoresWithSegments(segments []string, isDir bool) bool {
	relSegments, ok := m.relativeWorkspaceSegments(segments)
	if !ok {
		return false
	}

	ignored := false
	for _, rule := range m.rules {
		if !rule.matchesWithSegments(relSegments, isDir) {
			continue
		}
		ignored = !rule.negated
	}
	return ignored
}

func (m workspaceGitignore) containsWithSegments(segments []string) bool {
	_, ok := m.relativeWorkspaceSegments(segments)
	return ok
}

func (m workspaceGitignore) relativeWorkspaceSegments(segments []string) ([]string, bool) {
	if len(segments) < len(m.rootSegments) {
		return nil, false
	}
	for i, seg := range m.rootSegments {
		if segments[i] != seg {
			return nil, false
		}
	}
	return segments[len(m.rootSegments):], true
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

		// Pre-compute expanded patterns and simple globs
		if rule.pattern != "" {
			var rawVariants []string
			if rule.anchored {
				rawVariants = []string{rule.pattern}
			} else {
				rawVariants = []string{rule.pattern, "**/" + rule.pattern}
			}
			for _, v := range rawVariants {
				for _, expanded := range expandBracePattern(filepath.ToSlash(v)) {
					segs := splitPathSegments(expanded)
					rule.expandedPatterns = append(rule.expandedPatterns, segs)
				}
			}
			if !rule.hasSlash {
				rule.simpleGlobs = expandBracePattern(filepath.ToSlash(rule.pattern))
			}
		}

		rules = append(rules, rule)
	}
	return rules
}

func (r gitignoreRule) matchesWithSegments(candidateSegments []string, isDir bool) bool {
	if len(candidateSegments) == 0 {
		return false
	}

	if r.dirOnly {
		if isDir && r.matchesPathWithSegments(candidateSegments) {
			return true
		}
		// Check all parent directories.
		for i := 1; i < len(candidateSegments); i++ {
			if r.matchesPathWithSegments(candidateSegments[:i]) {
				return true
			}
		}
		return false
	}

	if r.matchesPathWithSegments(candidateSegments) {
		return true
	}

	if !r.hasSlash {
		base := candidateSegments[len(candidateSegments)-1]
		for _, pat := range r.simpleGlobs {
			matched, err := pathpkg.Match(pat, base)
			if err == nil && matched {
				return true
			}
		}
		return false
	}

	return false
}

func (r gitignoreRule) matchesPathWithSegments(candidateSegments []string) bool {
	for _, patternSegments := range r.expandedPatterns {
		if matchSegmentSequence(patternSegments, candidateSegments) {
			return true
		}
	}
	return false
}
