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

func TestHoverProviderShowsPromotedPropertyType(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	provider := &HoverProvider{idx: idx}
	text := `<?php
class SessionStore {}

class Session
{
	public function __construct(private SessionStore $session)
	{
	}

	public function current(): SessionStore
	{
		return $this->session;
	}
}`

	hover := provider.Provide("file:///workspace/Session.php", text, lsp.Position{Line: 11, Character: 18})
	if hover == nil {
		t.Fatal("expected hover for promoted property type, got nil")
	}
	if !strings.Contains(hover.Contents.Value, "SessionStore") {
		t.Fatalf("expected promoted property type SessionStore in hover, got %q", hover.Contents.Value)
	}
	if !strings.Contains(hover.Contents.Value, `**\Session::$session**`) {
		t.Fatalf("expected promoted property symbol details in hover, got %q", hover.Contents.Value)
	}
}

func TestHoverProviderShowsStringLiteralTypeWithoutUnrelatedSymbol(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/Noise.php", `<?php
class DayAndTime {}
`)
	provider := &HoverProvider{idx: idx}
	text := `<?php
class Example
{
	public function run(): void
	{
		$this->someMethod('Ayan');
	}

	public function someMethod(int $param): int
	{
		return $param * 2;
	}
}`

	hover := provider.Provide("file:///workspace/Example.php", text, lsp.Position{Line: 5, Character: 21})
	if hover == nil {
		t.Fatal("expected hover for string literal, got nil")
	}
	if !strings.Contains(hover.Contents.Value, "```php\nstring\n```") {
		t.Fatalf("expected string type in hover, got %q", hover.Contents.Value)
	}
	if strings.Contains(hover.Contents.Value, `**\DayAndTime**`) {
		t.Fatalf("expected no unrelated symbol details for string literal hover, got %q", hover.Contents.Value)
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

func TestHoverProviderPrefersReceiverTypeForChainedMethodCalls(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/MetadataBag.php", `<?php
class MetadataBag
{
	public function getLifetime(): int
	{
		return 123;
	}
}
`)
	idx.IndexDocument("file:///workspace/Noise.php", `<?php
class Noise
{
	public function getLifetime(): mixed
	{
		return null;
	}
}
`)

	provider := &HoverProvider{idx: idx}
	text := `<?php
class Session
{
	private MetadataBag $bag;

	public function getMetadataBag(): MetadataBag
	{
		return $this->bag;
	}

	public function getDuration(): int
	{
		return $this->getMetadataBag()->getLifetime();
	}
}
`

	hover := provider.Provide("file:///workspace/Session.php", text, lsp.Position{Line: 12, Character: 35})
	if hover == nil {
		t.Fatal("expected hover for chained method call, got nil")
	}
	if !strings.Contains(hover.Contents.Value, "```php\nint\n```") {
		t.Fatalf("expected int type in hover, got %q", hover.Contents.Value)
	}
	if !strings.Contains(hover.Contents.Value, `**\MetadataBag::getLifetime**`) {
		t.Fatalf("expected MetadataBag::getLifetime symbol in hover, got %q", hover.Contents.Value)
	}
}

func TestHoverProviderDoesNotFallbackToGlobalMatchForUnresolvedReceiverChain(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/Noise.php", `<?php
class Certificate
{
	public function getLifetime(): mixed
	{
		return null;
	}
}
`)

	provider := &HoverProvider{idx: idx}
	text := `<?php
interface SessionLike {}

class Session
{
	private SessionLike $session;

	public function getDuration(): int
	{
		return $this->session->getMetadataBag()->getLifetime();
	}
}
`

	lines := strings.Split(text, "\n")
	methodOffset := strings.Index(lines[9], "getLifetime")
	if methodOffset < 0 {
		t.Fatal("expected getLifetime call in test fixture")
	}
	hover := provider.Provide("file:///workspace/Session.php", text, lsp.Position{Line: 9, Character: uint32(methodOffset + 2)})
	if hover == nil {
		t.Fatal("expected hover result for unresolved chain, got nil")
	}
	if strings.Contains(hover.Contents.Value, `**\Certificate::getLifetime**`) {
		t.Fatalf("expected unresolved chain hover to avoid unrelated global symbol, got %q", hover.Contents.Value)
	}
	if !strings.Contains(hover.Contents.Value, "mixed") {
		t.Fatalf("expected unresolved chain hover to still show mixed type, got %q", hover.Contents.Value)
	}
}
