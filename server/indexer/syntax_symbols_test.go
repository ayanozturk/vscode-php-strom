package indexer

import (
	"testing"

	"github.com/ayanozturk/go-php-parser/analyse"
	"github.com/ayanozturk/go-php-parser/syntax"
)

func TestExtractSymbolsFromSyntaxClassMembers(t *testing.T) {
	src := `<?php
namespace App;

use Vendor\Helper;

class Example extends Base implements IFace {
    use Helper;

    public int $count = 0;
    public const FLAG = 1;

    public function run(Helper $h): void {}
}

function top(): int { return 1; }

const GLOBAL_C = 2;
`
	res := syntax.ParseForIndex([]byte(src))
	syms := extractSymbolsFromSyntax("file:///example.php", res)
	byFQN := map[string]*Symbol{}
	for _, s := range syms {
		byFQN[s.FQN] = s
	}
	cls := byFQN[`\App\Example`]
	if cls == nil || cls.Kind != KindClass {
		t.Fatalf("missing class: %#v", byFQN)
	}
	if len(cls.Traits) == 0 || cls.Traits[0] != `\Vendor\Helper` {
		t.Fatalf("expected trait Vendor\\Helper, got %#v", cls.Traits)
	}
	if prop := byFQN[`\App\Example::$count`]; prop == nil || prop.Name != "count" || prop.Type == "" {
		t.Fatalf("property: %#v", prop)
	}
	if m := byFQN[`\App\Example::run`]; m == nil || m.Kind != KindMethod {
		t.Fatalf("method: %#v", m)
	}
	if fn := byFQN[`\App\top`]; fn == nil || fn.Kind != KindFunction {
		t.Fatalf("function: %#v", fn)
	}
	if c := byFQN[`\App\GLOBAL_C`]; c == nil || c.Kind != KindConstant {
		t.Fatalf("const: %#v", c)
	}
}

func TestMergeSymbolsPreferSyntaxKeepsPHPDocAndTraits(t *testing.T) {
	syntaxSyms := []*Symbol{
		{FQN: `\App\Foo`, Name: "Foo", Kind: KindClass},
		{FQN: `\App\Foo::bar`, Name: "bar", Kind: KindMethod},
	}
	astSyms := []*Symbol{
		{FQN: `\App\Foo`, Name: "Foo", Kind: KindClass, Traits: []string{`\App\T`}, DocComment: "/** @template T */"},
		{FQN: `\App\Foo::fromDoc`, Name: "fromDoc", Kind: KindMethod}, // PHPDoc @method
	}
	merged := mergeSymbolsPreferSyntax(syntaxSyms, astSyms)
	byFQN := map[string]*Symbol{}
	for _, s := range merged {
		byFQN[s.FQN] = s
	}
	foo := byFQN[`\App\Foo`]
	if foo == nil || len(foo.Traits) != 1 || foo.DocComment == "" {
		t.Fatalf("expected AST enrichment on syntax class, got %#v", foo)
	}
	if byFQN[`\App\Foo::fromDoc`] == nil {
		t.Fatal("expected PHPDoc-only method retained from AST")
	}
	if byFQN[`\App\Foo::bar`] == nil {
		t.Fatal("expected syntax method retained")
	}
}

func TestPutDeclarationTierSharesParse(t *testing.T) {
	wi := &WorkspaceIndexer{
		index: newIndex(),
		usage: analyse.NewProjectUsageGraph(),
	}
	src := "<?php\nclass Shared {}\nfunction f(Shared $x) {}\n"
	syms := wi.putDeclarationTier("file:///shared.php", src)
	if len(syms) == 0 {
		t.Fatal("expected syntax symbols")
	}
	uses := wi.usageUses("file:///shared.php")
	if len(uses) == 0 {
		t.Fatal("expected usage graph uses from same parse")
	}
	foundClass := false
	for _, s := range syms {
		if s.Name == "Shared" && s.Kind == KindClass && s.SyntaxNodeID != 0 {
			foundClass = true
		}
	}
	if !foundClass {
		t.Fatalf("expected stamped class SyntaxNodeID, syms=%#v", syms)
	}
}
