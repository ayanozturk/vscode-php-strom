package indexer

import (
	"testing"

	"github.com/ayanozturk/go-php-parser/analyse"
	"github.com/ayanozturk/go-php-parser/ast"
)

func TestIndexDocumentUsesLoweredProjectNodes(t *testing.T) {
	wi := New(Config{})
	uri := "file:///workspace/User.php"
	src := `<?php
namespace App;
use Vendor\Base;
class User extends Base {
    public string $name;
    public function id(): int {}
}
`
	wi.IndexDocument(uri, src)
	key := projectIndexKey(uri)
	wi.mu.RLock()
	nodes := wi.projectNodes[key]
	wi.mu.RUnlock()
	if len(nodes) == 0 {
		t.Fatal("expected project nodes from syntax lower")
	}
	idx := analyse.BuildProjectIndex(map[string][]ast.Node{"f.php": nodes})
	if _, ok := idx.ResolveClass(`App\User`); !ok {
		t.Fatal("ResolveClass App\\User failed on lowered nodes")
	}
	method, ok := idx.ResolveMethod(`App\User`, "id")
	if !ok || method.ReturnType != "int" {
		t.Fatalf("ResolveMethod id: ok=%v return=%q", ok, method.ReturnType)
	}
	syms := wi.index.GetByURI(uri)
	if len(syms) == 0 {
		t.Fatal("expected syntax symbols indexed")
	}
}
