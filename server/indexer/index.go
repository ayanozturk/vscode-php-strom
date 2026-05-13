package indexer

import (
	"strings"
	"sync"
)

const shards = 64

// Index is a concurrent, sharded in-memory symbol store.
type Index struct {
	shards [shards]shard
}

type shard struct {
	mu    sync.RWMutex
	byFQN map[string]*Symbol   // FQN → symbol
	byURI map[string][]*Symbol // file URI → symbols in that file
}

func newIndex() *Index {
	idx := &Index{}
	for i := range idx.shards {
		idx.shards[i].byFQN = make(map[string]*Symbol)
		idx.shards[i].byURI = make(map[string][]*Symbol)
	}
	return idx
}

func (idx *Index) shardFor(key string) *shard {
	h := fnv32(key)
	return &idx.shards[h%shards]
}

// PutFile replaces all symbols for a given URI atomically.
func (idx *Index) PutFile(uri string, symbols []*Symbol) {
	// First remove all old symbols for this URI
	idx.RemoveFile(uri)

	for _, sym := range symbols {
		s := idx.shardFor(sym.FQN)
		s.mu.Lock()
		s.byFQN[sym.FQN] = sym
		s.byURI[uri] = append(s.byURI[uri], sym)
		s.mu.Unlock()
	}
}

// RemoveFile removes all symbols for a URI.
func (idx *Index) RemoveFile(uri string) {
	// collect old FQNs for this URI across all shards
	for i := range idx.shards {
		s := &idx.shards[i]
		s.mu.Lock()
		old := s.byURI[uri]
		for _, sym := range old {
			delete(s.byFQN, sym.FQN)
		}
		delete(s.byURI, uri)
		s.mu.Unlock()
	}
}

// GetByFQN returns the symbol with the given fully-qualified name, or nil.
func (idx *Index) GetByFQN(fqn string) *Symbol {
	s := idx.shardFor(fqn)
	s.mu.RLock()
	sym := s.byFQN[fqn]
	s.mu.RUnlock()
	return sym
}

// GetByURI returns all symbols declared in a file.
func (idx *Index) GetByURI(uri string) []*Symbol {
	var all []*Symbol
	for i := range idx.shards {
		s := &idx.shards[i]
		s.mu.RLock()
		all = append(all, s.byURI[uri]...)
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
