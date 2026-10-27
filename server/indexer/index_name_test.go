package indexer

import (
	"fmt"
	"testing"
)

func benchNameIndex() *Index {
	idx := newIndex()
	for f := 0; f < 2000; f++ {
		uri := fmt.Sprintf("file:///f%d.php", f)
		syms := make([]*Symbol, 0, 25)
		for s := 0; s < 25; s++ {
			name := fmt.Sprintf("Name%d", (f*25+s)%5000)
			syms = append(syms, &Symbol{Name: name, FQN: fmt.Sprintf(`\App\F%d\%s`, f, name), URI: uri, Kind: KindFunction})
		}
		idx.PutFile(uri, syms)
	}
	return idx
}

func BenchmarkGetByName(b *testing.B) {
	idx := benchNameIndex()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = idx.GetByName("name1234")
	}
}

func BenchmarkGetByNameParallel(b *testing.B) {
	idx := benchNameIndex()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = idx.GetByName("Name1234")
		}
	})
}

func BenchmarkPutFileReplace(b *testing.B) {
	idx := benchNameIndex()
	uri := "file:///f7.php"
	syms := make([]*Symbol, 0, 25)
	for s := 0; s < 25; s++ {
		name := fmt.Sprintf("Name%d", (7*25+s)%5000)
		syms = append(syms, &Symbol{Name: name, FQN: fmt.Sprintf(`\App\F7\%s`, name), URI: uri, Kind: KindFunction})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.PutFile(uri, syms)
	}
}

func TestGetByNameTracksPutAndRemove(t *testing.T) {
	idx := newIndex()
	a := &Symbol{Name: "Run", FQN: `\A\run`, URI: "file:///a.php", Kind: KindFunction}
	b := &Symbol{Name: "run", FQN: `\B\run`, URI: "file:///b.php", Kind: KindFunction}
	idx.PutFile("file:///a.php", []*Symbol{a})
	idx.PutFile("file:///b.php", []*Symbol{b})

	if got := idx.GetByName("RUN"); len(got) != 2 {
		t.Fatalf("case-insensitive lookup: got %d symbols, want 2", len(got))
	}

	// The returned slice is a copy: mutating it must not corrupt the index.
	got := idx.GetByName("run")
	got[0] = nil
	if again := idx.GetByName("run"); again[0] == nil || again[1] == nil {
		t.Fatal("GetByName result aliases index storage")
	}

	idx.RemoveFile("file:///a.php")
	got = idx.GetByName("run")
	if len(got) != 1 || got[0] != b {
		t.Fatalf("after removing a.php: got %v, want only b", got)
	}

	// Replacing a file drops its old names and adds the new ones.
	c := &Symbol{Name: "Other", FQN: `\B\other`, URI: "file:///b.php", Kind: KindFunction}
	idx.PutFile("file:///b.php", []*Symbol{c})
	if got := idx.GetByName("run"); got != nil {
		t.Fatalf("stale name after replace: %v", got)
	}
	if got := idx.GetByName("other"); len(got) != 1 || got[0] != c {
		t.Fatalf("new name missing after replace: %v", got)
	}

	idx.RemoveFile("file:///b.php")
	if got := idx.GetByName("other"); got != nil {
		t.Fatalf("expected empty index, got %v", got)
	}
}

func TestGetByNameConcurrentWithReindex(t *testing.T) {
	idx := newIndex()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 2000; i++ {
			uri := fmt.Sprintf("file:///f%d.php", i%20)
			idx.PutFile(uri, []*Symbol{{Name: "Shared", FQN: fmt.Sprintf(`\N%d\shared`, i%20), URI: uri, Kind: KindFunction}})
		}
	}()
	for i := 0; i < 2000; i++ {
		_ = idx.GetByName("shared")
	}
	<-done
}
