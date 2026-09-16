package indexer

import (
	"os"
	"path/filepath"
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

func TestBatchIndexFileUsesLoweredProjectNodes(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "User.php")
	src := `<?php
namespace App;
use Vendor\Base;
class User extends Base {
    public string $name;
    public function id(): int {}
}
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	wi := New(Config{MaxSize: 1_000_000})
	uri := pathToURI(path)
	var collected ParsedFile
	_, _, indexed := wi.indexFile(path, true, nil, func(parsed ParsedFile) {
		collected = parsed
	})
	if !indexed {
		t.Fatal("expected batch indexFile to index fixture")
	}
	if len(collected.Nodes) == 0 {
		t.Fatal("expected lowered project nodes from batch indexFile")
	}
	idx := analyse.BuildProjectIndex(map[string][]ast.Node{"f.php": collected.Nodes})
	if _, ok := idx.ResolveClass(`App\User`); !ok {
		t.Fatal("ResolveClass App\\User failed on batch lowered nodes")
	}
	method, ok := idx.ResolveMethod(`App\User`, "id")
	if !ok || method.ReturnType != "int" {
		t.Fatalf("ResolveMethod id: ok=%v return=%q", ok, method.ReturnType)
	}
	syms := wi.index.GetByURI(uri)
	if len(syms) == 0 {
		t.Fatal("expected syntax symbols indexed from batch path")
	}
}
