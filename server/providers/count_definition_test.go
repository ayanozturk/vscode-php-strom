package providers

import (
	"strings"
	"testing"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

func TestDefinitionProviderPrefersFunctionOverTraitForBareCountCall(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/CountTrait.php", `<?php
namespace Predis\Command\Traits;
trait Count {}
`)
	idx.IndexDocument("file:///workspace/CountFn.php", `<?php
function count($value) {}
`)

	provider := &DefinitionProvider{idx: idx, cache: newSemanticDocumentCache()}
	text := `<?php
use Predis\Command\Traits\Count;

function run(array $xs): int
{
    return count($xs);
}
`
	lines := strings.Split(text, "\n")
	line := 0
	col := 0
	for i, contents := range lines {
		if idx := strings.Index(contents, "count($xs)"); idx >= 0 {
			line = i
			col = idx + 2 // cursor inside "count"
			break
		}
	}
	locs := provider.Provide("file:///workspace/Caller.php", text, lsp.Position{Line: uint32(line), Character: uint32(col)})

	if len(locs) == 0 {
		t.Fatal("expected a definition for bare count() call")
	}
	if !strings.Contains(locs[0].URI, "CountFn.php") {
		t.Fatalf("expected function definition in CountFn.php, got %+v", locs)
	}
	for _, loc := range locs {
		if strings.Contains(loc.URI, "CountTrait.php") {
			t.Fatalf("expected function definition, not trait Count: %+v", locs)
		}
	}
}

func TestWorkspaceSymbolResolverResolveFunctionDoesNotShortNameFallbackForFQN(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/Other.php", `<?php
function helper() {}
`)
	resolver := workspaceSymbolResolver{idx: idx}
	if fn, ok := resolver.ResolveFunction("Other\\helper"); ok {
		t.Fatalf("qualified miss should not fall back to unqualified helper, got %#v", fn)
	}
}
