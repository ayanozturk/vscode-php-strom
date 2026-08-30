package providers

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ayanozturk/go-php-parser/analyse"
	"github.com/ayanozturk/go-php-parser/overrides"
	"github.com/ayanozturk/go-php-parser/sharedcache"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

func TestDiagnosticsProvider_ParseError(t *testing.T) {
	p := &DiagnosticsProvider{}
	source := `<?php
class Foo {
	public function bar() {
		// missing closing brace
`
	snapshot := parseSemanticSnapshot(source)
	if len(snapshot.errors) == 0 {
		t.Fatal("expected go-php-parser to retain syntax errors with debug disabled")
	}

	diags := p.Analyse("file:///test.php", source)
	// Parser errors should surface as diagnostics
	if len(diags) == 0 {
		t.Fatal("expected at least one diagnostic for incomplete PHP, got none")
	}
	if !hasDiagnosticCode(diags, "Parser Errors") {
		t.Fatalf("expected parser diagnostics to use the Parser Errors group code, got %#v", diags)
	}
}

func TestDiagnosticsProviderAcceptsExpandedParserCompatibilitySyntax(t *testing.T) {
	source := `<?php
namespace Example;

interface Readable {}
interface Writable {}

function open_resource(): null|(Readable&Writable) {
    yield;
    return null;
}

final class Coordinates {
    public int $horizontal = 0, $vertical = 0;

    public function render(): void {
        $formatter = new readonly class {};
        array_walk([], fn(string $value) => print $value);
        namespace\prepare();
    }
}`
	diagnostics := (&DiagnosticsProvider{}).Analyse("file:///compatibility.php", source)
	if hasDiagnosticCode(diagnostics, "Parser Errors") {
		t.Fatalf("expected compatibility syntax to avoid parser diagnostics, got %#v", diagnostics)
	}
}

func TestDiagnosticsProvider_StyleIssue(t *testing.T) {
	p := &DiagnosticsProvider{}
	// PSR-12: method visibility must be declared; method name should be camelCase
	diags := p.Analyse("file:///test.php", `<?php
class Foo {
	function BAD_METHOD_NAME() {}
}
`)
	if len(diags) == 0 {
		t.Fatal("expected style diagnostics for visibility/naming issues, got none")
	}
}

func TestDiagnosticsProvider_DisablesConfiguredAnalysisCodes(t *testing.T) {
	cfg := Config{
		DisableUndefinedSymbols:   true,
		DisableUndefinedVariables: true,
		DisableTypeErrors:         true,
	}
	disabled := cfg.disabledAnalysisIssueCodes()
	for _, code := range []string{
		"PHPStan.Level0.Symbols",
		"PHPStan.Level0.Variables",
		"PHPStan.Level1.Variables",
		"A.RETURN.TYPE",
		"A.PROP.TYPE",
		"A.ARG.TYPE",
		"A.ARG.COUNT",
	} {
		if !disabled[code] {
			t.Fatalf("expected diagnostic code %s to be disabled, got %#v", code, disabled)
		}
	}
}

func TestDiagnosticsProvider_SuppressesDisabledUndefinedVariables(t *testing.T) {
	source := `<?php
function run(): void {
    echo $missing;
}
`
	enabled := (&DiagnosticsProvider{}).Analyse("file:///test.php", source)
	if !hasAnyDiagnosticCode(enabled, "PHPStan.Level0.Variables", "PHPStan.Level1.Variables") {
		t.Fatalf("expected undefined-variable diagnostic before disabling it, got %#v", enabled)
	}

	disabled := (&DiagnosticsProvider{cfg: Config{DisableUndefinedVariables: true}}).Analyse("file:///test.php", source)
	if hasAnyDiagnosticCode(disabled, "PHPStan.Level0.Variables", "PHPStan.Level1.Variables") {
		t.Fatalf("expected undefined-variable diagnostic to be disabled, got %#v", disabled)
	}
}

func TestDiagnosticsProvider_AnalyseTransientDoesNotRetainSemanticCache(t *testing.T) {
	const uri = "file:///workspace/Transient.php"
	const source = `<?php
function run(): void
{
    if ($value = getValue()) {
        echo $value;
    }
}
`

	// Compare only the representative analysis diagnostic so style-rule
	// ordering or unrelated style changes cannot obscure the cache contract.
	relevant := func(diagnostics []lsp.Diagnostic) []lsp.Diagnostic {
		filtered := make([]lsp.Diagnostic, 0, len(diagnostics))
		for _, diagnostic := range diagnostics {
			if code, ok := diagnostic.Code.(string); ok && code == "Generic.CodeAnalysis.AssignmentInCondition" {
				filtered = append(filtered, diagnostic)
			}
		}
		return filtered
	}

	expected := (&DiagnosticsProvider{cache: newSemanticDocumentCache()}).Analyse(uri, source)
	cache := newSemanticDocumentCache()
	actual := (&DiagnosticsProvider{cache: cache}).AnalyseTransient(uri, source)
	expectedRelevant := relevant(expected)
	actualRelevant := relevant(actual)
	if len(expectedRelevant) == 0 {
		t.Fatalf("expected Analyse to produce the representative analysis diagnostic, got %#v", expected)
	}
	if !reflect.DeepEqual(actualRelevant, expectedRelevant) {
		t.Fatalf("expected AnalyseTransient analysis diagnostics to match Analyse, got %#v versus %#v", actualRelevant, expectedRelevant)
	}

	cache.mu.RLock()
	defer cache.mu.RUnlock()
	if len(cache.byURI) != 0 {
		t.Fatalf("expected AnalyseTransient not to retain parsed document snapshots, retained %d", len(cache.byURI))
	}
	if len(cache.analysis) != 0 {
		t.Fatalf("expected AnalyseTransient not to retain semantic snapshots, retained %d", len(cache.analysis))
	}
}

func hasAnyDiagnosticCode(diagnostics []lsp.Diagnostic, codes ...string) bool {
	for _, code := range codes {
		if hasDiagnosticCode(diagnostics, code) {
			return true
		}
	}
	return false
}

func hasDiagnosticCode(diagnostics []lsp.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func TestDiagnosticsProvider_StyleOverrideSuppressesMatchingClass(t *testing.T) {
	matcher, err := overrides.Compile(overrides.RuleOverrides{
		"PSR1.Classes.ClassDeclaration.PascalCase": overrides.RuleOverride{
			Classes: []string{"/^Legacy_/"},
		},
	})
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}

	p := &DiagnosticsProvider{cfg: Config{DiagnosticsOverrides: matcher}}
	diags := p.Analyse("file:///test.php", `<?php
class Legacy_service {}
`)
	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "PSR1.Classes.ClassDeclaration.PascalCase" {
			t.Fatalf("expected PascalCase diagnostic to be suppressed, got %+v", diag)
		}
	}
}

func TestDiagnosticsProvider_CleanFile(t *testing.T) {
	p := &DiagnosticsProvider{}
	diags := p.Analyse("file:///test.php", `<?php

class Foo
{
    public function bar(): void
    {
    }
}
`)
	// May still have style issues; just verify it doesn't panic
	_ = diags
}

func TestDiagnosticsProvider_UsesCurrentDocumentForSourceBackedRules(t *testing.T) {
	const uri = "file:///snapshot-boundary.php"
	const filename = "/snapshot-boundary.php"
	sharedcache.StoreCachedFileContent(filename, []byte("<?php\n;\n"))
	t.Cleanup(func() { sharedcache.DeleteCachedFileContent(filename) })

	source := `<?php
final class BuilderExample
{
    public function configure(object $builder): void
    {
        $builder
            ->add('field', [
                'rules' => [
                    new Constraint(
                        pattern: '/^[a-z]+$/',
                        message: 'Enter a valid value.',
                    ),
                ],
            ]);
    }
}
`

	diagnostics := (&DiagnosticsProvider{}).Analyse(uri, source)
	if hasDiagnosticCode(diagnostics, "Generic.CodeAnalysis.EmptyStatement") {
		t.Fatalf("expected source-backed rules to use the current document snapshot, got %+v", diagnostics)
	}
}

func TestDiagnosticsProvider_MethodClosingBraceOnOwnLine(t *testing.T) {
	p := &DiagnosticsProvider{}
	diags := p.Analyse("file:///test.php", `<?php

class SessionTest
{
    public function getIsNullable(): void
    {
        $result = $this->session->get(null);

        $this->assertNull($result); }
}
`)

	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "PSR12.Classes.ClosingBraceOnOwnLine" {
			return
		}
	}

	t.Fatalf("expected PSR12.Classes.ClosingBraceOnOwnLine diagnostic, got %+v", diags)
}

func TestDiagnosticsProvider_DoesNotReportClosingBraceForDocblockInlineTag(t *testing.T) {
	p := &DiagnosticsProvider{}
	diags := p.Analyse("file:///test.php", `<?php

class CKEditor5PluginManagerTest extends KernelTestBase {

  /**
   * {@inheritdoc}
   */
  protected static $modules = [
    'system',
  ];
}
`)

	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "PSR12.Classes.ClosingBraceOnOwnLine" {
			t.Fatalf("unexpected PSR12.Classes.ClosingBraceOnOwnLine diagnostic for docblock inline tag: %+v", diag)
		}
	}
}

func TestDiagnosticsProvider_DoesNotReportVisibilityForFunctionTextInString(t *testing.T) {
	p := &DiagnosticsProvider{}
	diags := p.Analyse("file:///test.php", `<?php

class UndefinedFunctionErrorEnhancerTest
{
    public static function provideUndefinedFunctionData()
    {
        return [
            [
                'Call to undefined function test_namespaced_function()',
                "Attempted to call undefined function \"test_namespaced_function\" from the global namespace.",
            ],
        ];
    }
}
`)

	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "PSR12.Methods.VisibilityDeclared" {
			t.Fatalf("unexpected PSR12.Methods.VisibilityDeclared diagnostic for function text inside string: %+v", diag)
		}
	}
}

func TestDiagnosticsProvider_DoesNotReportControlSpacingForDocblockText(t *testing.T) {
	p := &DiagnosticsProvider{}
	diags := p.Analyse("file:///test.php", `<?php
/**
 * --------------------------------------------------------------------------
 * DEFINE APPLICATION CONSTANTS
 * --------------------------------------------------------------------------
 *
 * SELF - The name of THIS file (typically "index.php")
 * Reads entire file into an array
 * file( string $filename [, int $flags = 0 ] ): array|false
 */
function bootstrap(): void
{
	file($path);
}
`)
	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "PSR12.ControlStructures.ControlStructureSpacing" {
			t.Fatalf("unexpected control structure spacing diagnostic for docblock text: %+v", diag)
		}
	}
}

func TestDiagnosticsProvider_DoesNotReportControlSpacingForMethodNamedDo(t *testing.T) {
	p := &DiagnosticsProvider{}
	diags := p.Analyse("file:///test.php", `<?php
class GenerateCoverageTodoCommand
{
    public static function do($container, $lazyLoad = true)
    {
        return self::do($container, $lazyLoad);
    }
}
`)
	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "PSR12.ControlStructures.ControlStructureSpacing" {
			t.Fatalf("unexpected control structure spacing diagnostic for method named do: %+v", diag)
		}
	}
}

func TestDiagnosticsProvider_DoesNotReportUnreachableAfterIfReturnWithNullsafeCall(t *testing.T) {
	p := &DiagnosticsProvider{}
	diags := p.Analyse("file:///test.php", `<?php
class Contact
{
    public function getContactEmail(): ?string
    {
        if ($this->email) {
            return $this->email;
        }

        $admin = $this->getAdministrator();
        return $admin?->getEmail();
    }
}
`)
	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "Generic.CodeAnalysis.UnreachableCode" {
			t.Fatalf("unexpected unreachable diagnostic: %+v", diag)
		}
	}
}

func TestDiagnosticsProvider_DoesNotReportUnreachableAfterShortTernaryInPreviousMethod(t *testing.T) {
	p := &DiagnosticsProvider{}
	diags := p.Analyse("file:///test.php", `<?php
class Company
{
    public function getAdministrator(): ?User
    {
        return $this->users->first() ?: null;
    }

    public function getContactEmail(): ?string
    {
        if ($this->email) {
            return $this->email;
        }

        $admin = $this->getAdministrator();
        return $admin?->getEmail();
    }
}
`)
	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "Generic.CodeAnalysis.UnreachableCode" {
			t.Fatalf("unexpected unreachable diagnostic after short ternary: %+v", diag)
		}
	}
}

func TestDiagnosticsProvider_DoesNotReportUnreachableAfterConditionalExit(t *testing.T) {
	p := &DiagnosticsProvider{}
	diags := p.Analyse("file:///test.php", `<?php
class AuthService
{
    public function logout($redirectURL): void
    {
        if ($redirectURL) {
            redirect($redirectURL);
            exit();
        }

        $authenticationState = $this->getState();
        $error = 'Your account is not set up';
    }
}
`)
	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "Generic.CodeAnalysis.UnreachableCode" {
			t.Fatalf("unexpected unreachable diagnostic after conditional exit: %+v", diag)
		}
	}
}

func TestDiagnosticsProvider_DoesNotReportUnreachableAfterConditionalThrowInPreviousMethod(t *testing.T) {
	p := &DiagnosticsProvider{}
	diags := p.Analyse("file:///workspace/AbstractArrayCollection.php", `<?php
abstract class AbstractArrayCollection
{
    protected function assertItemType(object $item): void
    {
        $expected = $this->getItemClass();
        $given = get_class($item);
        if (!$item instanceof $expected) {
            throw new InvalidArgumentException("Item of type {$given} is not an instance of expected {$expected}.");
        }
    }

    /**
     * @param list<T> $items
     * @return static
     */
    /**
     * @param list<T> $items
     * @return static
     */
    protected function createFromArray(array $items): static
    {
        return new static($items);
    }
}
`)
	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "Generic.CodeAnalysis.UnreachableCode" {
			t.Fatalf("unexpected unreachable diagnostic after conditional throw: %+v", diag)
		}
	}
}

func TestDiagnosticsProvider_HonorsPhpstanIgnoreLine(t *testing.T) {
	p := &DiagnosticsProvider{}
	diags := p.Analyse("file:///test.php", `<?php
function value(): int
{
    return 1;
    $x = 2; // @phpstan-ignore-line
}
`)
	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "Generic.CodeAnalysis.UnreachableCode" {
			t.Fatalf("expected @phpstan-ignore-line to suppress unreachable diagnostic, got %+v", diag)
		}
	}
}

func TestDiagnosticsProvider_DoesNotReportClassInstantiationForVariableNamesContainingNew(t *testing.T) {
	p := &DiagnosticsProvider{}
	diags := p.Analyse("file:///test.php", `<?php
class AuditTrail
{
	public function capture(LoggerInterface $logger): void
    {
		$newToken = build_identifier();
		$logger->record($newToken);
    }
}
`)
	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "PSR1.Classes.ClassInstantiation" {
			t.Fatalf("unexpected class instantiation diagnostic: %+v", diag)
		}
	}
}

func TestDiagnosticsProvider_UsesWorkspaceResolverForMethodReturnTypes(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/User.php", `<?php
class User {}
`)
	idx.IndexDocument("file:///workspace/UserRepository.php", `<?php
class UserRepository
{
    public function current(): User
    {
        return new User();
    }
}
`)

	p := &DiagnosticsProvider{idx: idx}
	diags := p.Analyse("file:///workspace/Controller.php", `<?php
class Controller
{
    private UserRepository $repo;

    public function show(): User
    {
        return $this->repo->current();
    }
}
`)

	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "A.RETURN.TYPE" {
			t.Fatalf("unexpected return type diagnostic with workspace resolver: %+v", diag)
		}
	}
}

func TestDiagnosticsProvider_UsesIndexedVendorParentMethodsForInvocation(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/vendor/phpunit/TestCase.php", `<?php
namespace PHPUnit\Framework;

class TestCase
{
    public function assertSame(mixed $expected, mixed $actual, string $message = ''): void {}
}
`)

	p := &DiagnosticsProvider{idx: idx}
	diags := p.Analyse("file:///workspace/tests/DemoTest.php", `<?php
namespace App\Tests;

use PHPUnit\Framework\TestCase;

final class DemoTest extends TestCase
{
    public function testIt(): void
    {
        $this->assertSame(1, 1);
    }
}
`)

	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "PHPStan.Level0.Invocation" && strings.Contains(diag.Message, "assertSame") {
			t.Fatalf("expected indexed vendor parent method signature to satisfy invocation, got %+v", diag)
		}
	}
}

func TestDiagnosticsProvider_UsesIndexedVendorThrowableHierarchy(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/vendor/nette/JsonException.php", `<?php
namespace Nette\Utils;

class JsonException extends \JsonException {}
`)

	p := &DiagnosticsProvider{idx: idx}
	diags := p.Analyse("file:///workspace/src/Model.php", `<?php
namespace App;

use Nette\Utils\JsonException;

function fail(): void
{
    throw new JsonException('bad');
}
`)

	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "PHPStan.Level0.ClassModel" && strings.Contains(diag.Message, "JsonException") {
			t.Fatalf("expected indexed vendor throwable hierarchy to satisfy throw check, got %+v", diag)
		}
	}
}

func TestDiagnosticsProvider_UsesRepositoryPHPDocMethodReturnTypeForArgTypes(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/src/Shift.php", `<?php
namespace App\Module\Shift\Entity;

class Shift {}
`)
	idx.IndexDocument("file:///workspace/src/ShiftRepository.php", `<?php
namespace App\Module\Shift\Repository;

use App\Module\Shift\Entity\Shift;

/**
 * @method Shift|null find($id, $lockMode = null, $lockVersion = null)
 */
class ShiftRepository {}
`)

	p := &DiagnosticsProvider{idx: idx}
	diags := p.Analyse("file:///workspace/src/Controller.php", `<?php
namespace App\Module\Shift\Controller;

use App\Module\Shift\Entity\Shift;
use App\Module\Shift\Repository\ShiftRepository;

class Controller
{
    private ShiftRepository $shiftRepository;

    public function run(string $id): void
    {
        $shift = $this->shiftRepository->find($id);
        if ($shift && $this->isShiftAccessibleToCompany($shift)) {}
    }

    private function isShiftAccessibleToCompany(Shift $shift): bool
    {
        return true;
    }
}
`)

	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "A.ARG.TYPE" && strings.Contains(diag.Message, "isShiftAccessibleToCompany") {
			t.Fatalf("expected PHPDoc repository return type to satisfy argument type, got %+v", diag)
		}
	}
}

func TestDiagnosticsProvider_AcceptsRequiredGenericPHPDocMethodArgument(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/src/RecordStore.php", `<?php
namespace App;

/**
 * @method object|null lookup(array<string, mixed> $criteria, ?array<string, mixed> $orderBy = null)
 */
class RecordStore {}
`)

	p := &DiagnosticsProvider{idx: idx}
	diags := p.Analyse("file:///workspace/src/RecordService.php", `<?php
namespace App;

class RecordService
{
    public function __construct(private RecordStore $store) {}

    public function find(string $name): ?object
    {
        return $this->store->lookup(['name' => $name]);
    }
}
`)

	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "A.ARG.COUNT" {
			t.Fatalf("expected one required generic PHPDoc method argument to be accepted, got %+v", diag)
		}
	}
}

func TestDiagnosticsProvider_BindsInheritedGenericMethodReturnType(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/Record.php", `<?php
namespace App;
class Record {}
`)
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

	p := &DiagnosticsProvider{idx: idx}
	diags := p.Analyse("file:///workspace/Controller.php", `<?php
namespace App;

class Controller
{
    private RecordStore $store;
    private RecordProcessor $processor;

    public function run(string $id): void
    {
        $record = $this->store->lookup($id);
        if (!$record) {
            return;
        }
        $this->processor->process($record);
    }
}
`)

	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "A.ARG.TYPE" && strings.Contains(diag.Message, "process") {
			t.Fatalf("expected inherited generic return type to satisfy argument type, got %+v", diag)
		}
	}
}

func TestDiagnosticsProvider_UsesProjectIndexForWorkspaceFunctions(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/helpers.php", `<?php
function helper_id(int $id): string
{
    return (string) $id;
}
`)

	p := &DiagnosticsProvider{idx: idx}
	diags := p.Analyse("file:///workspace/consumer.php", `<?php
function consume(): string
{
    return helper_id(1);
}
`)

	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "PHPStan.Level0.Symbols" && strings.Contains(diag.Message, "helper_id") {
			t.Fatalf("unexpected missing workspace function diagnostic: %+v", diag)
		}
	}
}

func TestProjectFallbackResolverUsesWorkspaceSymbolsWhenProjectMissesClass(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/vendor/phpunit/TestCase.php", `<?php
namespace PHPUnit\Framework;

class TestCase {}
`)

	resolver := projectFallbackResolver{
		project:  analyse.NewProjectIndex(),
		fallback: workspaceSymbolResolver{idx: idx},
	}

	if _, ok := resolver.ResolveClass(`PHPUnit\Framework\TestCase`); !ok {
		t.Fatal("expected resolver to fall back to workspace symbols for PHPUnit\\Framework\\TestCase")
	}
	if !resolver.ClassExists(`PHPUnit\Framework\TestCase`) {
		t.Fatal("expected ClassExists to use workspace symbol fallback")
	}
}

func TestWorkspaceSymbolResolverSupportsClassModelQueries(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/Contract.php", `<?php
interface Contract {
    public const string FORMAT = 'json';
    public function execute(string $value): void;
}
`)

	resolver := workspaceSymbolResolver{idx: idx}
	method, ok := resolver.ResolveOwnMethod("Contract", "execute")
	if !ok || method.Name != "execute" || !method.Abstract {
		t.Fatalf("expected own abstract method, got %#v, %v", method, ok)
	}
	methods := resolver.MethodsDeclaredBy("Contract")
	if len(methods) != 1 || methods[0].Name != "execute" {
		t.Fatalf("expected declared method, got %#v", methods)
	}
	constant, ok := resolver.ResolveOwnConstant("Contract", "FORMAT")
	if !ok || constant.Name != "FORMAT" || constant.Visibility != "public" {
		t.Fatalf("expected own class constant, got %#v, %v", constant, ok)
	}
}

func TestDiagnosticsProvider_OverlaysCurrentFileIntoProjectIndex(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	p := &DiagnosticsProvider{idx: idx}

	diags := p.Analyse("file:///workspace/current.php", `<?php
function local_helper(): string
{
    return 'ok';
}

function consume(): string
{
    return local_helper();
}
`)

	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "PHPStan.Level0.Symbols" && strings.Contains(diag.Message, "local_helper") {
			t.Fatalf("unexpected missing current-file function diagnostic: %+v", diag)
		}
	}
}

func TestDiagnosticsProvider_UsesProjectIndexForDuplicateClasses(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/First.php", `<?php
class DuplicateName {}
`)
	idx.IndexDocument("file:///workspace/Second.php", `<?php
class DuplicateName {}
`)

	p := &DiagnosticsProvider{idx: idx}
	for _, uri := range []string{"file:///workspace/First.php", "file:///workspace/Second.php"} {
		diags := p.Analyse(uri, `<?php
class DuplicateName {}
`)

		for _, diag := range diags {
			if code, ok := diag.Code.(string); ok && code == "PHPStan.Level0.ClassModel" && strings.Contains(diag.Message, "DuplicateName") {
				return
			}
		}
	}

	t.Fatal("expected duplicate class diagnostic from project index")
}

func TestDiagnosticsProvider_UsesWorkspaceResolverForMethodArgumentTypes(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/UserRepository.php", `<?php
class UserRepository
{
    public function findById(int $id): User
    {
        return new User();
    }
}
`)
	idx.IndexDocument("file:///workspace/User.php", `<?php
class User {}
`)

	p := &DiagnosticsProvider{idx: idx}
	diags := p.Analyse("file:///workspace/Controller.php", `<?php
class Controller
{
    private UserRepository $repo;

    public function show(): User
    {
        return $this->repo->findById("bad");
    }
}
`)

	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "A.ARG.TYPE" {
			return
		}
	}

	t.Fatalf("expected A.ARG.TYPE diagnostic for workspace-resolved method argument mismatch, got %+v", diags)
}

func TestDiagnosticsProvider_UsesWorkspaceResolverForConstructorArgumentCount(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/Auth2Service.php", `<?php
class Auth2Service
{
	public function __construct(
		string $clientId,
		string $clientSecret,
		string $issuer,
		string $redirectUri,
		string $scope,
		string $grantType,
		string $state,
		string $nonce,
		string $audience,
		string $tenant,
		string $region,
		string $mode
	) {
	}
}
`)

	p := &DiagnosticsProvider{idx: idx}
	diags := p.Analyse("file:///workspace/Factory.php", `<?php
class Factory
{
	public function make(): Auth2Service
	{
		return new Auth2Service(
			"a",
			"b",
			"c",
			"d",
			"e",
			"f",
			"g",
			"h",
			"i",
			"j",
			"k",
			"l",
			"m"
		);
	}
}
`)

	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "A.ARG.COUNT" {
			return
		}
	}

	t.Fatalf("expected A.ARG.COUNT diagnostic for workspace-resolved constructor arg mismatch, got %+v", diags)
}

func TestDiagnosticsProvider_DoesNotReportAliasedWorkspaceMethodArgumentTypes(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/RG_Scheme_SchemeFilter.php", `<?php
class RG_Scheme_SchemeFilter {}
`)
	idx.IndexDocument("file:///workspace/Repository.php", `<?php
use RG_Scheme_SchemeFilter as SchemeFilter;

class Repository
{
	public function getByFilterObject(SchemeFilter $filter): void
	{
	}
}
`)

	p := &DiagnosticsProvider{idx: idx}
	diags := p.Analyse("file:///workspace/Controller.php", `<?php
class Controller
{
	private Repository $repository;

	public function run(): void
	{
		$filter = new RG_Scheme_SchemeFilter();
		$this->repository->getByFilterObject($filter);
	}
}
`)

	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "A.ARG.TYPE" {
			t.Fatalf("expected no A.ARG.TYPE diagnostic for aliased workspace parameter type, got %+v", diag)
		}
	}
}

func TestDiagnosticsProvider_UsesWorkspaceResolverForConstructorNamedArgumentTypes(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/Response.php", `<?php
class Response
{
	public function __construct(
		int $status = 200,
		array $headers = [],
		?string $body = null
	) {
	}
}
`)

	p := &DiagnosticsProvider{idx: idx}
	good := p.Analyse("file:///workspace/ControllerGood.php", `<?php
class ControllerGood
{
	public function run(): void
	{
		new Response(body: "ok");
	}
}
`)

	for _, diag := range good {
		if code, ok := diag.Code.(string); ok && (code == "A.ARG.COUNT" || code == "A.ARG.TYPE") {
			t.Fatalf("expected no constructor arg diagnostics for valid named arg usage, got %+v", diag)
		}
	}

	bad := p.Analyse("file:///workspace/ControllerBad.php", `<?php
class ControllerBad
{
	public function run(): void
	{
		new Response(body: []);
	}
}
`)

	for _, diag := range bad {
		if code, ok := diag.Code.(string); ok && code == "A.ARG.TYPE" {
			return
		}
	}

	t.Fatalf("expected A.ARG.TYPE diagnostic for workspace-resolved constructor named arg mismatch, got %+v", bad)
}

func TestDiagnosticsProvider_UsesWorkspaceResolverForPropertyAssignments(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/UserRepository.php", `<?php
class UserRepository {}
`)

	p := &DiagnosticsProvider{idx: idx}
	diags := p.Analyse("file:///workspace/Controller.php", `<?php
class Controller
{
    private UserRepository $repo;

    public function replace(): void
    {
        $this->repo = "bad";
    }
}
`)

	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "A.PROP.TYPE" {
			return
		}
	}

	t.Fatalf("expected A.PROP.TYPE diagnostic for typed property assignment mismatch, got %+v", diags)
}

func TestDiagnosticsProvider_AllowsInterfaceImplementationsForPropertyAssignments(t *testing.T) {
	idx := indexer.New(indexer.Config{})
	idx.IndexDocument("file:///workspace/Collection.php", `<?php
namespace Doctrine\Common\Collections;

interface Collection {}
`)
	idx.IndexDocument("file:///workspace/ArrayCollection.php", `<?php
namespace Doctrine\Common\Collections;

class ArrayCollection implements Collection {}
`)

	p := &DiagnosticsProvider{idx: idx}
	diags := p.Analyse("file:///workspace/Entity.php", `<?php
namespace App;

use Doctrine\Common\Collections\ArrayCollection;
use Doctrine\Common\Collections\Collection;

class Entity
{
    /** @var Collection<string, Policy> */
    private Collection $users;

    public function __construct()
    {
        $this->users = new ArrayCollection();
    }
}
`)

	for _, diag := range diags {
		if code, ok := diag.Code.(string); ok && code == "A.PROP.TYPE" {
			t.Fatalf("expected no A.PROP.TYPE diagnostic for interface implementation assignment, got %+v", diags)
		}
	}
}

func TestLineColToRange(t *testing.T) {
	r := lineColToRange(5, 10)
	if r.Start.Line != 4 || r.Start.Character != 9 {
		t.Errorf("expected line=4 char=9, got line=%d char=%d", r.Start.Line, r.Start.Character)
	}
}

func TestLineColToRange_ZeroValues(t *testing.T) {
	r := lineColToRange(0, 0)
	if r.Start.Line != 0 || r.Start.Character != 0 {
		t.Errorf("expected 0,0 got %d,%d", r.Start.Line, r.Start.Character)
	}
}

func TestParseErrorRange(t *testing.T) {
	source := "zero zero\none one\ntwo two\nthree three\nfour four five"
	r := parseErrorRange(newSourcePositionMapper(source), "line 5:10: unexpected token")
	if r.Start.Line != 4 || r.Start.Character != 9 {
		t.Errorf("expected line=4 char=9, got line=%d char=%d", r.Start.Line, r.Start.Character)
	}
}

func TestParseErrorRange_UnstructuredMessage(t *testing.T) {
	r := parseErrorRange(newSourcePositionMapper("<?php\n"), "parser panic recovered")
	if r.Start.Line != 0 || r.Start.Character != 0 {
		t.Errorf("expected fallback 0,0 got %d,%d", r.Start.Line, r.Start.Character)
	}
}

func TestParseErrorRange_MapsUTF16Column(t *testing.T) {
	r := parseErrorRange(newSourcePositionMapper("<?php\nx🙂"), "line 2:3: unexpected token")
	if r.Start != (lsp.Position{Line: 1, Character: 3}) || r.End != r.Start {
		t.Fatalf("expected parser error at UTF-16 position (1,3), got %+v", r)
	}
}

func TestDiagnosticsProvider_MapsStructuredAnalysisSpanAfterEmoji(t *testing.T) {
	source := "<?php\n\"🙂\"; missing_call();\n"
	diagnostics := (&DiagnosticsProvider{}).Analyse("file:///structured-span.php", source)

	for _, diagnostic := range diagnostics {
		if diagnostic.Code != "PHPStan.Level0.Symbols" {
			continue
		}
		want := lsp.Range{
			Start: lsp.Position{Line: 1, Character: 6},
			End:   lsp.Position{Line: 1, Character: 20},
		}
		if diagnostic.Range != want {
			t.Fatalf("expected undefined function span %+v, got %+v", want, diagnostic.Range)
		}
		if diagnostic.Range.Start == diagnostic.Range.End {
			t.Fatal("expected structured analysis issue to retain a non-point range")
		}
		return
	}
	t.Fatalf("expected PHPStan.Level0.Symbols diagnostic, got %#v", diagnostics)
}
