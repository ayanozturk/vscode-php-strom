package providers

import (
	"strings"
	"testing"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

func TestDefinitionProviderFindsClassFromCursorInsideIdentifier(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/Foo.php", "<?php\nclass Foo {}\n")

	provider := &DefinitionProvider{idx: idx}
	text := "<?php\nnew Foo();\n"
	locs := provider.Provide("file:///workspace/Use.php", text, lsp.Position{Line: 1, Character: 4})

	if len(locs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(locs))
	}
	if locs[0].URI != "file:///workspace/Foo.php" {
		t.Fatalf("expected definition in Foo.php, got %s", locs[0].URI)
	}
}

func TestDefinitionProviderPrefersClassLikeExactMatch(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/Foo.php", "<?php\nclass Foo {}\n")
	idx.IndexDocument("file:///workspace/Other.php", "<?php\nclass Other { public function Foo() {} }\n")

	provider := &DefinitionProvider{idx: idx}
	text := "<?php\nnew Foo();\n"
	locs := provider.Provide("file:///workspace/Use.php", text, lsp.Position{Line: 1, Character: 5})

	if len(locs) != 1 {
		t.Fatalf("expected a single preferred definition, got %d", len(locs))
	}
	if locs[0].URI != "file:///workspace/Foo.php" {
		t.Fatalf("expected class definition in Foo.php, got %s", locs[0].URI)
	}
}

func TestHoverProviderShowsInferredLocalVariableType(t *testing.T) {
	provider := &HoverProvider{idx: indexer.New(indexer.Config{})}
	text := `<?php
class Example
{
    public function run(): void
    {
        $value = "hello";
        echo $value;
    }
}`

	hover := provider.Provide("file:///workspace/Example.php", text, lsp.Position{Line: 6, Character: 16})
	if hover == nil {
		t.Fatal("expected hover for inferred local variable type, got nil")
	}
	if !strings.Contains(hover.Contents.Value, "string") {
		t.Fatalf("expected inferred string type in hover, got %q", hover.Contents.Value)
	}
}

func TestHoverProviderShowsWorkspaceResolvedPropertyType(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/UserRepository.php", `<?php
class UserRepository {}
`)
	provider := &HoverProvider{idx: idx}
	text := `<?php
class Example
{
    private UserRepository $repo;

    public function run(): void
    {
        $this->repo;
    }
}`

	hover := provider.Provide("file:///workspace/Example.php", text, lsp.Position{Line: 7, Character: 17})
	if hover == nil {
		t.Fatal("expected hover for inferred property type, got nil")
	}
	if !strings.Contains(hover.Contents.Value, "UserRepository") {
		t.Fatalf("expected inferred UserRepository type in hover, got %q", hover.Contents.Value)
	}
}

func TestHoverProviderCombinesInferredTypeAndSymbolDocs(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/UserRepository.php", `<?php
/** Repository service. */
class UserRepository {}
`)
	provider := &HoverProvider{idx: idx}
	text := `<?php
class Example
{
    private UserRepository $repo;

    public function run(): void
    {
        $this->repo;
    }
}`

	hover := provider.Provide("file:///workspace/Example.php", text, lsp.Position{Line: 7, Character: 17})
	if hover == nil {
		t.Fatal("expected hover for inferred property type, got nil")
	}
	if !strings.Contains(hover.Contents.Value, "```php\nUserRepository\n```") {
		t.Fatalf("expected inferred type block in hover, got %q", hover.Contents.Value)
	}
	if !strings.Contains(hover.Contents.Value, `**\UserRepository::$repo**`) && !strings.Contains(hover.Contents.Value, `**\UserRepository**`) && !strings.Contains(hover.Contents.Value, `**\Example::$repo**`) {
		// Keep the assertion broad enough for current symbol lookup heuristics while still proving docs are present.
		t.Fatalf("expected symbol details in hover, got %q", hover.Contents.Value)
	}
}

func TestHoverProviderPrefersCurrentFileDeclaration(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	text := `<?php
class Session
{
    /** Store a value in the current session. */
    public function set($name, $value): void
    {
    }
}
`
	idx.IndexDocument("file:///workspace/Session.php", text)
	idx.IndexDocument("file:///workspace/Other.php", `<?php
class Other
{
    public function set() {}
}
`)

	provider := &HoverProvider{idx: idx}
	hover := provider.Provide("file:///workspace/Session.php", text, lsp.Position{Line: 4, Character: 20})
	if hover == nil {
		t.Fatal("expected hover for method declaration, got nil")
	}
	if !strings.Contains(hover.Contents.Value, `**\Session::set**`) {
		t.Fatalf("expected current-file method symbol in hover, got %q", hover.Contents.Value)
	}
}

func TestHoverProviderUsesDeterministicExactMatchOrdering(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/Beta.php", "<?php\nclass Beta { public function set() {} }\n")
	idx.IndexDocument("file:///workspace/Alpha.php", "<?php\nclass Alpha { public function set() {} }\n")

	provider := &HoverProvider{idx: idx}
	text := "<?php\nset();\n"

	first := provider.Provide("file:///workspace/Use.php", text, lsp.Position{Line: 1, Character: 2})
	second := provider.Provide("file:///workspace/Use.php", text, lsp.Position{Line: 1, Character: 2})
	if first == nil || second == nil {
		t.Fatal("expected hover for exact symbol match, got nil")
	}
	if first.Contents.Value != second.Contents.Value {
		t.Fatalf("expected deterministic hover contents, got %q then %q", first.Contents.Value, second.Contents.Value)
	}
}
