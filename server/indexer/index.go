package indexer

import (
	"strings"
	"sync"
)

const shards = 64

// Index is a concurrent, sharded in-memory symbol store.
type Index struct {
	shards    [shards]shard
	muURI     sync.Mutex
	uriToFQNs map[string][]string
}

type shard struct {
	mu          sync.RWMutex
	byFQN       map[string]*Symbol   // FQN → symbol
	byNameLower map[string][]*Symbol // lowercase unqualified name → symbols
}

func newIndex() *Index {
	idx := &Index{
		uriToFQNs: make(map[string][]string),
	}
	for i := range idx.shards {
		idx.shards[i].byFQN = make(map[string]*Symbol)
		idx.shards[i].byNameLower = make(map[string][]*Symbol)
	}
	return idx
}

func (idx *Index) shardFor(key string) *shard {
	h := fnv32(key)
	return &idx.shards[h%shards]
}

// PutFile replaces all symbols for a given URI atomically.
func (idx *Index) PutFile(uri string, symbols []*Symbol) {
	// First remove all old symbols for this URI (only locks active shards containing the symbols)
	idx.RemoveFile(uri)

	var fqns []string
	if len(symbols) > 0 {
		fqns = make([]string, 0, len(symbols))
		grouped := make(map[*shard][]*Symbol, min(len(symbols), shards))
		for _, sym := range symbols {
			s := idx.shardFor(sym.FQN)
			grouped[s] = append(grouped[s], sym)
			fqns = append(fqns, sym.FQN)
		}
		for s, shardSymbols := range grouped {
			s.mu.Lock()
			for _, sym := range shardSymbols {
				nameLower := strings.ToLower(sym.Name)
				s.byFQN[sym.FQN] = sym
				s.byNameLower[nameLower] = append(s.byNameLower[nameLower], sym)
			}
			s.mu.Unlock()
		}
	}

	idx.muURI.Lock()
	if len(fqns) > 0 {
		idx.uriToFQNs[uri] = fqns
	} else {
		delete(idx.uriToFQNs, uri)
	}
	idx.muURI.Unlock()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// RemoveFile removes all symbols for a URI.
func (idx *Index) RemoveFile(uri string) {
	idx.muURI.Lock()
	fqns, ok := idx.uriToFQNs[uri]
	if !ok {
		idx.muURI.Unlock()
		return
	}
	delete(idx.uriToFQNs, uri)
	idx.muURI.Unlock()

	grouped := make(map[*shard][]string, min(len(fqns), shards))
	for _, fqn := range fqns {
		s := idx.shardFor(fqn)
		grouped[s] = append(grouped[s], fqn)
	}

	for s, shardFQNs := range grouped {
		s.mu.Lock()
		for _, fqn := range shardFQNs {
			sym, exists := s.byFQN[fqn]
			if !exists {
				continue
			}
			nameLower := strings.ToLower(sym.Name)
			current := s.byNameLower[nameLower]
			updated := current[:0]
			for _, existing := range current {
				if existing.URI != uri {
					updated = append(updated, existing)
				}
			}
			if len(updated) == 0 {
				delete(s.byNameLower, nameLower)
			} else {
				s.byNameLower[nameLower] = updated
			}
			delete(s.byFQN, fqn)
		}
		s.mu.Unlock()
	}
}

// Size returns the number of indexed symbols without materialising a full slice.
func (idx *Index) Size() int {
	total := 0
	for i := range idx.shards {
		s := &idx.shards[i]
		s.mu.RLock()
		total += len(s.byFQN)
		s.mu.RUnlock()
	}
	return total
}

// GetByFQN returns the symbol with the given fully-qualified name, or nil.
func (idx *Index) GetByFQN(fqn string) *Symbol {
	s := idx.shardFor(fqn)
	s.mu.RLock()
	sym := s.byFQN[fqn]
	s.mu.RUnlock()
	return sym
}

// GetByName returns all symbols whose unqualified name matches (case-insensitive).
// This is an O(1) lookup backed by the byNameLower secondary index.
func (idx *Index) GetByName(name string) []*Symbol {
	nameLower := strings.ToLower(name)
	var results []*Symbol
	for i := range idx.shards {
		s := &idx.shards[i]
		s.mu.RLock()
		results = append(results, s.byNameLower[nameLower]...)
		s.mu.RUnlock()
	}
	return results
}

// GetByURI returns all symbols declared in a file.
// Uses the uriToFQNs lookup to avoid scanning all shards.
func (idx *Index) GetByURI(uri string) []*Symbol {
	idx.muURI.Lock()
	fqns := idx.uriToFQNs[uri]
	if len(fqns) == 0 {
		idx.muURI.Unlock()
		return nil
	}
	// Snapshot to avoid holding muURI while acquiring shard locks.
	fqnsCopy := append([]string(nil), fqns...)
	idx.muURI.Unlock()

	all := make([]*Symbol, 0, len(fqnsCopy))
	for _, fqn := range fqnsCopy {
		s := idx.shardFor(fqn)
		s.mu.RLock()
		if sym := s.byFQN[fqn]; sym != nil {
			all = append(all, sym)
		}
		s.mu.RUnlock()
	}
	return all
}

// Search does a case-insensitive substring match against symbol names.
// Caller should limit results.
func (idx *Index) Search(query string) []*Symbol {
	lower := strings.ToLower(query)
	var results []*Symbol
	for i := range idx.shards {
		s := &idx.shards[i]
		s.mu.RLock()
		for _, sym := range s.byFQN {
			if strings.Contains(strings.ToLower(sym.Name), lower) {
				results = append(results, sym)
			}
		}
		s.mu.RUnlock()
	}
	return results
}

// FuzzySearch matches acronyms / camelCase abbreviations.
// E.g. "HM" matches "HttpManager".
func (idx *Index) FuzzySearch(query string) []*Symbol {
	var results []*Symbol
	for i := range idx.shards {
		s := &idx.shards[i]
		s.mu.RLock()
		for _, sym := range s.byFQN {
			if camelMatch(sym.Name, query) {
				results = append(results, sym)
			}
		}
		s.mu.RUnlock()
	}
	return results
}

// AllSymbols returns a flat copy of every symbol in the index.
func (idx *Index) AllSymbols() []*Symbol {
	var all []*Symbol
	for i := range idx.shards {
		s := &idx.shards[i]
		s.mu.RLock()
		for _, sym := range s.byFQN {
			all = append(all, sym)
		}
		s.mu.RUnlock()
	}
	return all
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func fnv32(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// camelMatch returns true if query is a prefix sequence of uppercase initials
// from name, e.g. camelMatch("HttpResponseFactory", "HRF") → true.
func camelMatch(name, query string) bool {
	qi := 0
	q := strings.ToUpper(query)
	for _, ch := range name {
		if qi >= len(q) {
			return true
		}
		if strings.ToUpper(string(ch)) == string(q[qi]) {
			qi++
		}
	}
	return qi >= len(q)
}
