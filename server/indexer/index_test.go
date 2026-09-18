package indexer

import "testing"

func methodSymbol(uri, fqn, name string) *Symbol {
	return &Symbol{URI: uri, FQN: fqn, Name: name, Kind: KindMethod}
}

func TestMethodsByOwnerReflectsPutFile(t *testing.T) {
	idx := newIndex()
	idx.PutFile("file:///Foo.php", []*Symbol{
		{URI: "file:///Foo.php", FQN: `\Foo`, Name: "Foo", Kind: KindClass},
		methodSymbol("file:///Foo.php", `\Foo::bar`, "bar"),
		methodSymbol("file:///Foo.php", `\Foo::baz`, "baz"),
	})

	got := idx.MethodsByOwner(`\Foo`)
	if len(got) != 2 {
		t.Fatalf("expected 2 methods declared by \\Foo, got %d: %+v", len(got), got)
	}

	// Case-insensitive owner match, mirroring how classSym.FQN is looked up.
	if got := idx.MethodsByOwner(`\foo`); len(got) != 2 {
		t.Fatalf("expected case-insensitive owner match, got %d", len(got))
	}
}

func TestMethodsByOwnerClearedOnRemoveFile(t *testing.T) {
	idx := newIndex()
	idx.PutFile("file:///Foo.php", []*Symbol{
		methodSymbol("file:///Foo.php", `\Foo::bar`, "bar"),
		methodSymbol("file:///Foo.php", `\Foo::baz`, "baz"),
	})
	idx.RemoveFile("file:///Foo.php")

	if got := idx.MethodsByOwner(`\Foo`); len(got) != 0 {
		t.Fatalf("expected no methods after RemoveFile, got %d: %+v", len(got), got)
	}
}

func TestMethodsByOwnerStaysConsistentAcrossPutFileReplace(t *testing.T) {
	idx := newIndex()
	idx.PutFile("file:///Foo.php", []*Symbol{
		methodSymbol("file:///Foo.php", `\Foo::bar`, "bar"),
		methodSymbol("file:///Foo.php", `\Foo::baz`, "baz"),
	})
	// PutFile on the same URI must fully replace, not accumulate, the owner index.
	idx.PutFile("file:///Foo.php", []*Symbol{
		methodSymbol("file:///Foo.php", `\Foo::bar`, "bar"),
	})

	got := idx.MethodsByOwner(`\Foo`)
	if len(got) != 1 || got[0].Name != "bar" {
		t.Fatalf("expected only 'bar' to remain after replace, got %d: %+v", len(got), got)
	}
}

func TestMethodsByOwnerDoesNotAccumulateStaleEntriesAcrossCollidingFiles(t *testing.T) {
	// Regression test: distinct files can legitimately declare a member under
	// the exact same FQN (e.g. Symfony's per-scenario ProjectServiceContainer
	// test fixtures, each in its own file but sharing one namespace+class
	// name). MethodsByOwner must reflect only the FQN's current owner in
	// byFQN, not accumulate every file that ever declared it.
	idx := newIndex()
	idx.PutFile("file:///fixtureA.php", []*Symbol{
		methodSymbol("file:///fixtureA.php", `\Foo::bar`, "bar"),
	})
	// A second, unrelated file declares a member under the identical FQN.
	idx.PutFile("file:///fixtureB.php", []*Symbol{
		methodSymbol("file:///fixtureB.php", `\Foo::bar`, "bar"),
		methodSymbol("file:///fixtureB.php", `\Foo::baz`, "baz"),
	})

	got := idx.MethodsByOwner(`\Foo`)
	if len(got) != 2 {
		t.Fatalf("expected 2 members (bar overwritten by fixtureB, baz added), got %d: %+v", len(got), got)
	}
	for _, m := range got {
		if m.URI != "file:///fixtureB.php" {
			t.Fatalf("expected all members to belong to the current owner fixtureB, got member from %s", m.URI)
		}
	}
}

func TestMethodsByOwnerIsolatesDifferentOwnersInSameShard(t *testing.T) {
	idx := newIndex()
	// Multiple unrelated classes across multiple files must not bleed into
	// each other's owner bucket, including when two owners happen to land in
	// the same shard.
	idx.PutFile("file:///A.php", []*Symbol{methodSymbol("file:///A.php", `\A::run`, "run")})
	idx.PutFile("file:///B.php", []*Symbol{methodSymbol("file:///B.php", `\B::run`, "run")})

	a := idx.MethodsByOwner(`\A`)
	b := idx.MethodsByOwner(`\B`)
	if len(a) != 1 || a[0].FQN != `\A::run` {
		t.Fatalf("expected exactly \\A::run for owner \\A, got %+v", a)
	}
	if len(b) != 1 || b[0].FQN != `\B::run` {
		t.Fatalf("expected exactly \\B::run for owner \\B, got %+v", b)
	}
}
