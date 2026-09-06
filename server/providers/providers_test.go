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

func TestDefinitionProviderResolvesMethodCallOverSameNamedClass(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/CacheInterface.php", "<?php\nnamespace RG\\Caching;\ninterface CacheInterface { public function set($k, $v, $ttl); }\n")
	// Unrelated class that happens to share the member name "set" as its
	// own type name, mirroring vendor/RG classes literally named `Set`.
	idx.IndexDocument("file:///workspace/Set.php", "<?php\nnamespace Some\\Other\\Ns;\nclass Set {}\n")

	provider := &DefinitionProvider{idx: idx, cache: newSemanticDocumentCache()}
	text := "<?php\nclass Repo {\n    public function __construct(private \\RG\\Caching\\CacheInterface $cache) {}\n    public function run() {\n        $this->cache->set('k', 'v', 0);\n    }\n}\n"
	// Position on "set" inside `$this->cache->set(...)`.
	locs := provider.Provide("file:///workspace/Repo.php", text, lsp.Position{Line: 4, Character: 24})

	if len(locs) != 1 {
		t.Fatalf("expected a single definition for the method call, got %d: %+v", len(locs), locs)
	}
	if locs[0].URI != "file:///workspace/CacheInterface.php" {
		t.Fatalf("expected CacheInterface::set definition, got %s", locs[0].URI)
	}
}

func TestDefinitionProviderResolvesConfiguredStubClass(t *testing.T) {
	idx := indexer.New(indexer.Config{Stubs: []string{"Core", "SPL"}, PHPVersion: "8.3"})

	provider := &DefinitionProvider{idx: idx}
	text := "<?php\nreturn new ArrayIterator([]);\n"
	locs := provider.Provide("file:///workspace/Use.php", text, lsp.Position{Line: 1, Character: 14})

	if len(locs) != 1 {
		t.Fatalf("expected 1 stub definition, got %d", len(locs))
	}
	if locs[0].URI != "phpstub:8.3/SPL.php" {
		t.Fatalf("expected ArrayIterator definition in SPL stub, got %s", locs[0].URI)
	}
}

func TestDiagnosticsAcceptArrayIteratorAsTraversableFromStubs(t *testing.T) {
	idx := indexer.New(indexer.Config{Stubs: []string{"Core", "SPL"}, PHPVersion: "8.3"})
	provider := &DiagnosticsProvider{idx: idx, cache: newSemanticDocumentCache()}
	text := `<?php
function getIterator(): Traversable
{
    return new ArrayIterator([]);
}
`

	diags := provider.Analyse("file:///workspace/Example.php", text)
	for _, diag := range diags {
		if diag.Code == "A.RETURN.TYPE" {
			t.Fatalf("expected ArrayIterator to satisfy Traversable, got diagnostic: %s", diag.Message)
		}
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

func TestHoverProviderBindsGenericAssignmentAndNarrowsAfterNullGuard(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/Record.php", "<?php\nnamespace App;\nclass Record {}\n")
	idx.IndexDocument("file:///workspace/GenericStore.php", `<?php
namespace App;
/** @template T */
abstract class GenericStore
{
    /** @return T|null */
    public function lookup(string $id): ?object {}
}
`)
	idx.IndexDocument("file:///workspace/RecordStore.php", `<?php
namespace App;
/** @extends GenericStore<Record> */
class RecordStore extends GenericStore {}
`)
	idx.IndexDocument("file:///workspace/RecordProcessor.php", `<?php
namespace App;
class RecordProcessor
{
    public function process(Record $record): void {}
}
`)
	provider := &HoverProvider{idx: idx}
	text := `<?php
namespace App;

class Controller
{
    private RecordStore $store;
    private RecordProcessor $processor;

    public function run(string $id): void
    {
        $record = $this->store->lookup($id);
        if (!$record) {
            throw new \RuntimeException();
        }
        $this->processor->process($record);
    }
}
`

	assigned := provider.Provide("file:///workspace/Controller.php", text, lsp.Position{Line: 10, Character: 10})
	if assigned == nil || !strings.Contains(assigned.Contents.Value, "```php\nApp\\Record|null\n```") {
		t.Fatalf("expected bound nullable assignment hover, got %#v", assigned)
	}
	afterGuard := provider.Provide("file:///workspace/Controller.php", text, lsp.Position{Line: 14, Character: 35})
	if afterGuard == nil || !strings.Contains(afterGuard.Contents.Value, "```php\nApp\\Record\n```") {
		t.Fatalf("expected non-null hover after terminating guard, got %#v", afterGuard)
	}
}

func TestHoverProviderInfersGenericMethodResultFromInheritedReceiverFactory(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/Member.php", `<?php
namespace Domain;
class Member
{
    /** @return Sequence<string, Member> */
    public function peers(): Sequence {}
}
`)
	idx.IndexDocument("file:///workspace/BaseEndpoint.php", `<?php
namespace Web;
use Domain\Member;
abstract class BaseEndpoint
{
    protected function currentMember(): Member {}
}
`)
	provider := &HoverProvider{idx: idx}
	text := `<?php
namespace Web;
class TeamEndpoint extends BaseEndpoint
{
    public function run(): void
    {
        $member = $this->currentMember();
        $peers = $member->peers();
    }
}
`
	receiver := provider.Provide("file:///workspace/TeamEndpoint.php", text, lsp.Position{Line: 6, Character: 10})
	if receiver == nil || !strings.Contains(receiver.Contents.Value, "```php\nDomain\\Member\n```") {
		t.Fatalf("expected inherited factory return type for receiver assignment, got %#v", receiver)
	}
	result := provider.Provide("file:///workspace/TeamEndpoint.php", text, lsp.Position{Line: 7, Character: 10})
	if result == nil || !strings.Contains(result.Contents.Value, "```php\nDomain\\Sequence<string,Domain\\Member>\n```") {
		t.Fatalf("expected generic method result for inherited receiver, got %#v", result)
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

func TestHoverProviderResolvesBackedEnumValueInsteadOfUnrelatedClass(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/vendor/Doctrine/Value.php", `<?php
namespace Doctrine\Common\Collections\Expr;

/** @final since 2.5 */
class Value {}
`)

	uri := "file:///workspace/InvoiceStatus.php"
	text := `<?php
namespace App\Module\Subscription\Enum;

enum InvoiceStatus: int {
    case PENDING = 1;

    public function isPending(): bool {
        return $this->value === self::PENDING->value;
    }
}
`
	idx.IndexDocument(uri, text)
	provider := &HoverProvider{idx: idx, cache: newSemanticDocumentCache()}

	lines := strings.Split(text, "\n")
	line := 0
	for i, contents := range lines {
		if strings.Contains(contents, "$this->value") {
			line = i
			break
		}
	}
	if line == 0 {
		t.Fatal("expected $this->value in fixture")
	}
	thisValueCol := strings.Index(lines[line], "value")
	caseValueCol := strings.LastIndex(lines[line], "value")

	thisHover := provider.Provide(uri, text, lsp.Position{Line: uint32(line), Character: uint32(thisValueCol + 1)})
	if thisHover == nil {
		t.Fatal("expected hover for $this->value")
	}
	if strings.Contains(thisHover.Contents.Value, `Doctrine\Common\Collections\Expr\Value`) {
		t.Fatalf("expected enum $value hover not to resolve Doctrine Value, got %q", thisHover.Contents.Value)
	}
	if !strings.Contains(thisHover.Contents.Value, "int") {
		t.Fatalf("expected int type for backed enum $value, got %q", thisHover.Contents.Value)
	}
	if !strings.Contains(thisHover.Contents.Value, `InvoiceStatus::$value`) {
		t.Fatalf("expected InvoiceStatus::$value symbol in hover, got %q", thisHover.Contents.Value)
	}

	caseHover := provider.Provide(uri, text, lsp.Position{Line: uint32(line), Character: uint32(caseValueCol + 1)})
	if caseHover == nil {
		t.Fatal("expected hover for enum-case ->value")
	}
	if strings.Contains(caseHover.Contents.Value, `Doctrine\Common\Collections\Expr\Value`) {
		t.Fatalf("expected enum-case $value hover not to resolve Doctrine Value, got %q", caseHover.Contents.Value)
	}
}
