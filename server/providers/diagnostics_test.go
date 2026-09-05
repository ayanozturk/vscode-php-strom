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
		DisabledAnalysis: DisabledAnalysis{
			UndefinedSymbols:      true,
			UndefinedVariables:    true,
			ClassModel:            true,
			InvalidCalls:          true,
			Language:              true,
			TypeErrors:            true,
			MethodVisibility:      true,
			ThrowTypes:            true,
			Deprecated:            true,
			UnreachableCode:       true,
			EmptyStatements:       true,
			AssignmentInCondition: true,
			SideEffects:           true,
		},
	}
	disabled := cfg.disabledAnalysisIssueCodes()
	for _, code := range []string{
		"Level0.Symbols",
		"Level2.MethodExistence",
		"Level7.MethodUnion",
		"Level1.Variables",
		"A.RETURN.TYPE",
		"A.RETURN.VOID",
		"A.RETURN.NEVER",
		"A.VOID.PURE",
		"A.PROP.TYPE",
		"A.ARG.TYPE",
		"A.ARG.COUNT",
		"Level0.PropertyCallableType",
		"A.ASSIGN.OP.INVALID",
		"A.BINARY.OP.INVALID",
		"Level2.PHPDocClass",
		"Level2.PHPDocParamName",
		"Level2.PHPDocParamType",
		"Level2.PHPDocPropertyType",
		"Level2.PHPDocReturnType",
		"Level2.PHPDocGenericLessTypes",
		"Level2.PHPDocGenericMoreTypes",
		"Level2.PHPDocNotGeneric",
		"Level2.PHPDocGenericNotSubtype",
		"Level6.MissingGenericType",
		"Level6.MissingIterableValueType",
		"Level6.MissingParameterType",
		"Level6.MissingReturnType",
		"Level6.MissingPropertyType",
		"Level2.MethodNonObject",
		"Level8.MethodNonObject",
		"Level0.ClassModel",
		"Level0.Invocation",
		"Level0.Language",
		"Level2.MethodVisibility",
		"Level3.ThrowType",
		"A.DEPRECATED.CALL",
		"Generic.CodeAnalysis.UnreachableCode",
		"Generic.CodeAnalysis.EmptyStatement",
		"Generic.CodeAnalysis.AssignmentInCondition",
		"PSR1.Files.SideEffects",
	} {
		if !disabled[code] {
			t.Fatalf("expected diagnostic code %s to be disabled, got %#v", code, disabled)
		}
	}
}

func TestDiagnosticsProvider_DisablesStyleWhenConfigured(t *testing.T) {
	source := `<?php
class bad_name {
	function BAD_METHOD_NAME() {}
}
`
	enabled := (&DiagnosticsProvider{}).Analyse("file:///test.php", source)
	if !hasStyleDiagnostic(enabled) {
		t.Fatalf("expected style diagnostics when style analysis is enabled, got %#v", enabled)
	}
	disabled := (&DiagnosticsProvider{cfg: Config{DisabledAnalysis: DisabledAnalysis{Style: true}}}).Analyse("file:///test.php", source)
	if hasStyleDiagnostic(disabled) {
		t.Fatalf("expected style diagnostics to be disabled, got %#v", disabled)
	}
}

func TestDiagnosticsProvider_SuppressesUndefinedMethodsWithUndefinedSymbols(t *testing.T) {
	source := `<?php
class Service {}

function run(Service $service): void {
    $service->missing();
}
`
	enabled := (&DiagnosticsProvider{}).Analyse("file:///test.php", source)
	if !hasDiagnosticCode(enabled, "Level2.MethodExistence") {
		t.Fatalf("expected undefined-method diagnostic before disabling undefined symbols, got %#v", enabled)
	}

	disabled := (&DiagnosticsProvider{cfg: Config{DisabledAnalysis: DisabledAnalysis{UndefinedSymbols: true}}}).Analyse("file:///test.php", source)
	if hasAnyDiagnosticCode(disabled, "Level2.MethodExistence", "Level7.MethodUnion") {
		t.Fatalf("expected undefined-method diagnostic to be disabled with undefined symbols, got %#v", disabled)
	}
}

func TestDiagnosticsProvider_ReportsSupportedUnknownMethodReceivers(t *testing.T) {
	source := `<?php
class FunctionService {}
class FirstBranch {}
class SecondBranch {}
class MissingLeft {}
class MissingRight {}
interface HasAvailableMethod { public function available(): void; }
interface NoAvailableMethod {}

function makeService(): FunctionService { return new FunctionService(); }

function run(bool $flag, MissingLeft|MissingRight $missing, HasAvailableMethod|NoAvailableMethod $union, HasAvailableMethod&NoAvailableMethod $intersection): void {
    $missing->absent();
    $union->available();
    $intersection->available();
    ($flag ? new FirstBranch() : new SecondBranch())->missing();
}

makeService()->missing();
`
	diagnostics := (&DiagnosticsProvider{}).Analyse("file:///receivers.php", source)

	var methodDiagnostics []lsp.Diagnostic
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "Level2.MethodExistence" {
			methodDiagnostics = append(methodDiagnostics, diagnostic)
		}
	}
	if len(methodDiagnostics) != 3 {
		t.Fatalf("expected three supported unknown-method diagnostics, got %d: %#v", len(methodDiagnostics), methodDiagnostics)
	}

	for _, expected := range []string{
		"FunctionService::missing()",
		"FirstBranch|SecondBranch::missing()",
		"MissingLeft|MissingRight::absent()",
	} {
		found := false
		for _, diagnostic := range methodDiagnostics {
			if strings.Contains(diagnostic.Message, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected unknown-method diagnostic containing %q, got %#v", expected, methodDiagnostics)
		}
	}
	if countDiagnosticMessage(diagnostics, "Level7.MethodUnion", "HasAvailableMethod|NoAvailableMethod::available()") != 1 {
		t.Fatalf("expected one partial-union level-seven diagnostic, got %#v", diagnostics)
	}
}

func TestDiagnosticsProvider_ReportsCallableClosureAndClassStringMethods(t *testing.T) {
	source := `<?php
class CallableService {}
class KnownCallableService { public function execute(): void {} }
class ClosureService {}
class KnownClosureService { public function execute(): void {} }
class DynamicService {}
class KnownDynamicService { public function execute(): void {} }

/** @param callable(): CallableService $factory */
function callableReceiver(callable $factory): void {
    $factory()->missing();
}

function closureReceiver(): void {
    $factory = static function (): ClosureService { return new ClosureService(); };
    $factory()->missing();
    $known = static function (): KnownClosureService { return new KnownClosureService(); };
    $known()->execute();
}

/** @param class-string<DynamicService> $class */
function dynamicReceiver(string $class): void {
    $value = new $class();
    $value->missing();
}

/** @param class-string<KnownDynamicService> $class */
function knownDynamicReceiver(string $class): void {
    $value = new $class();
    $value->execute();
}
`
	diagnostics := (&DiagnosticsProvider{}).Analyse("file:///callable-receivers.php", source)
	if countDiagnosticCode(diagnostics, "Level0.Symbols") != 0 {
		t.Fatalf("expected known receiver classes to avoid level-zero symbol diagnostics, got %#v", diagnostics)
	}
	if countDiagnosticCode(diagnostics, "Level2.MethodExistence") != 3 {
		t.Fatalf("expected three unknown-method diagnostics, got %#v", diagnostics)
	}
	for _, expected := range []string{
		"CallableService::missing()",
		"ClosureService::missing()",
		"DynamicService::missing()",
	} {
		if countDiagnosticMessage(diagnostics, "Level2.MethodExistence", expected) != 1 {
			t.Fatalf("expected one diagnostic containing %q, got %#v", expected, diagnostics)
		}
	}
	for _, unexpected := range []string{
		"KnownCallableService::execute()",
		"KnownClosureService::execute()",
		"KnownDynamicService::execute()",
	} {
		if countDiagnosticMessage(diagnostics, "Level2.MethodExistence", unexpected) != 0 {
			t.Fatalf("expected known method %q to remain clean, got %#v", unexpected, diagnostics)
		}
	}
}

func TestDiagnosticsProvider_DoesNotFlagCoreBuiltinsOrTraitMethods(t *testing.T) {
	source := `<?php
trait CreatedDateTrait {
    public function getCreatedDate(): mixed { return null; }
}
class InvitedUser {
    use CreatedDateTrait;
}
function run(InvitedUser $user, DateTime $now): string {
    $user->getCreatedDate();
    return $now->format('c');
}
function fail(): void {
    throw new RuntimeException('missing');
}
`
	diagnostics := (&DiagnosticsProvider{}).Analyse("file:///hr-builtins.php", source)
	for _, unexpected := range []string{
		"InvitedUser::getCreatedDate()",
		"DateTime::format()",
		"Instantiated class RuntimeException not found",
	} {
		for _, diagnostic := range diagnostics {
			if strings.Contains(diagnostic.Message, unexpected) {
				t.Fatalf("unexpected diagnostic %q: %#v", unexpected, diagnostics)
			}
		}
	}
}

func TestDiagnosticsProvider_ReportsDeclaredCallableAndArrayShapeMethods(t *testing.T) {
	source := `<?php
class PropertyCallableService {}
class KnownPropertyCallableService { public function execute(): void {} }
class MethodCallableService {}
class KnownMethodCallableService { public function execute(): void {} }
class ShapeCallableService {}
class KnownShapeCallableService { public function execute(): void {} }
class TemplateService {}
class KnownTemplateService { public function execute(): void {} }

class Holder {
    /** @var callable(): PropertyCallableService */
    public $factory;
    /** @var callable(): KnownPropertyCallableService */
    public $knownFactory;
    /** @return callable(): MethodCallableService */
    public function make(): callable { return static fn (): MethodCallableService => new MethodCallableService(); }
    /** @return callable(): KnownMethodCallableService */
    public function knownMake(): callable { return static fn (): KnownMethodCallableService => new KnownMethodCallableService(); }
}

/** @param array{service: callable(): ShapeCallableService, known: callable(): KnownShapeCallableService} $factories */
function shapeReceiver(array $factories): void {
    $factory = $factories["service"];
    $factory()->missing();
    $factories["known"]()->execute();
}

/**
 * @template T of TemplateService
 * @param class-string<T> $class
 */
function templateReceiver(Holder $holder, string $class): void {
    ($holder->factory)()->missing();
    $holder->make()()->missing();
    ($holder->knownFactory)()->execute();
    $holder->knownMake()()->execute();
    $value = new $class();
    $value->missing();
}

/**
 * @template T of KnownTemplateService
 * @param class-string<T> $class
 */
function knownTemplateReceiver(string $class): void {
    $value = new $class();
    $value->execute();
}
`
	diagnostics := (&DiagnosticsProvider{}).Analyse("file:///declared-callables.php", source)
	if countDiagnosticCode(diagnostics, "Level0.Symbols") != 0 {
		t.Fatalf("expected known receiver classes to avoid level-zero symbol diagnostics, got %#v", diagnostics)
	}
	if countDiagnosticCode(diagnostics, "Level2.MethodExistence") != 4 {
		t.Fatalf("expected four unknown-method diagnostics, got %#v", diagnostics)
	}
	for _, expected := range []string{
		"PropertyCallableService::missing()",
		"MethodCallableService::missing()",
		"ShapeCallableService::missing()",
		"TemplateService::missing()",
	} {
		if countDiagnosticMessage(diagnostics, "Level2.MethodExistence", expected) != 1 {
			t.Fatalf("expected one diagnostic containing %q, got %#v", expected, diagnostics)
		}
	}
	for _, unexpected := range []string{
		"KnownPropertyCallableService::execute()",
		"KnownMethodCallableService::execute()",
		"KnownShapeCallableService::execute()",
		"KnownTemplateService::execute()",
	} {
		if countDiagnosticMessage(diagnostics, "Level2.MethodExistence", unexpected) != 0 {
			t.Fatalf("expected known method %q to remain clean, got %#v", unexpected, diagnostics)
		}
	}
}

func TestDiagnosticsProvider_ReportsNestedShapeAndExpressionReceivers(t *testing.T) {
	source := `<?php
class NestedShapeService {}
class KnownNestedShapeService { public function execute(): void {} }
class ListCallableService {}
class CloneService {}
class CoalesceService {}
class MatchLeft {}
class MatchRight {}
class NullsafeService {}

/**
 * @param array{inner: array{service: callable(): NestedShapeService, known: callable(): KnownNestedShapeService}} $nested
 * @param list{callable(): ListCallableService} $list
 */
function run(array $nested, array $list, CloneService $clone, ?CoalesceService $coalesce, bool $flag, ?NullsafeService $nullsafe): void {
    $nested["inner"]["service"]()->missing();
    $nested["inner"]["known"]()->execute();
    $list[0]()->missing();
    (clone $clone)->missing();
    ($coalesce ?? new CoalesceService())->missing();
    (match ($flag) { true => new MatchLeft(), false => new MatchRight() })->missing();
    $nullsafe?->missing();
}
`
	diagnostics := (&DiagnosticsProvider{}).Analyse("file:///nested-receivers.php", source)
	if countDiagnosticCode(diagnostics, "Level0.Symbols") != 0 {
		t.Fatalf("expected known receiver classes to avoid level-zero symbol diagnostics, got %#v", diagnostics)
	}
	if countDiagnosticCode(diagnostics, "Level2.MethodExistence") != 6 {
		t.Fatalf("expected six unknown-method diagnostics, got %#v", diagnostics)
	}
	for _, expected := range []string{
		"NestedShapeService::missing()",
		"ListCallableService::missing()",
		"CloneService::missing()",
		"CoalesceService::missing()",
		"MatchLeft|MatchRight::missing()",
		"NullsafeService::missing()",
	} {
		if countDiagnosticMessage(diagnostics, "Level2.MethodExistence", expected) != 1 {
			t.Fatalf("expected one diagnostic containing %q, got %#v", expected, diagnostics)
		}
	}
	if countDiagnosticMessage(diagnostics, "Level2.MethodExistence", "KnownNestedShapeService::execute()") != 0 {
		t.Fatalf("expected known nested shape method to remain clean, got %#v", diagnostics)
	}
}

func TestDiagnosticsProvider_ReportsDynamicArrayShapeIndexes(t *testing.T) {
	source := `<?php
class AssignedIndexService {}
class KnownAssignedIndexService { public function execute(): void {} }
class StringIndexLeft {}
class StringIndexRight { public function execute(): void {} }
class IntListLeft {}
class IntListRight {}
class ConstIndexService {}
class Holder {
    public const KEY = 'service';
    /** @param array{service: callable(): ConstIndexService} $factories */
    public function run(array $factories): void {
        $factories[self::KEY]()->missing();
    }
}

/**
 * @param array{service: callable(): AssignedIndexService, known: callable(): KnownAssignedIndexService, left: callable(): StringIndexLeft, right: callable(): StringIndexRight} $factories
 * @param list{callable(): IntListLeft, callable(): IntListRight} $list
 */
function run(array $factories, array $list, string $name, int $i): void {
    $key = "service";
    $factories[$key]()->missing();
    $known = "known";
    $factories[$known]()->execute();
    $factories["serv" . "ice"]()->missing();
    $factories[$name]()->execute();
    $list[$i]()->missing();
}
`
	diagnostics := (&DiagnosticsProvider{}).Analyse("file:///dynamic-indexes.php", source)
	if countDiagnosticCode(diagnostics, "Level0.Symbols") != 0 {
		t.Fatalf("expected known receiver classes to avoid level-zero symbol diagnostics, got %#v", diagnostics)
	}
	if countDiagnosticCode(diagnostics, "Level2.MethodExistence") != 4 {
		t.Fatalf("expected four unknown-method diagnostics, got %#v", diagnostics)
	}
	got := map[string]int{}
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "Level2.MethodExistence" {
			got[diagnostic.Message]++
		}
	}
	if got["Call to an undefined method AssignedIndexService::missing()."] != 2 {
		t.Fatalf("expected assigned and concatenated AssignedIndexService diagnostics, got %#v", diagnostics)
	}
	if got["Call to an undefined method ConstIndexService::missing()."] != 1 {
		t.Fatalf("expected class-constant array-shape diagnostic, got %#v", diagnostics)
	}
	if got["Call to an undefined method IntListLeft|IntListRight::missing()."] != 1 {
		t.Fatalf("expected unknown int list-index diagnostic, got %#v", diagnostics)
	}
	if countDiagnosticMessage(diagnostics, "Level2.MethodExistence", "KnownAssignedIndexService::execute()") != 0 {
		t.Fatalf("expected known assigned and string-index methods to remain clean, got %#v", diagnostics)
	}
}

func TestDiagnosticsProvider_ReportsRemainingExpressionReceivers(t *testing.T) {
	source := `<?php
class GlobalConstShapeService {}
class ForeignConstShapeService {}
class MatchIndexLeft {}
class MatchIndexRight { public function execute(): void {} }
class PropertyShapeService {}
class KnownPropertyShapeService { public function execute(): void {} }
class ReturnedShapeService {}
class ListObjectService {}

const KEY = 'service';

class Other {
    public const KEY = 'service';
}

class Holder {
    /** @var array{service: callable(): PropertyShapeService, known: callable(): KnownPropertyShapeService} */
    public array $factories;
    /**
     * @return array{service: callable(): ReturnedShapeService}
     */
    public function factories(): array {
        return ['service' => static fn (): ReturnedShapeService => new ReturnedShapeService()];
    }
}

/**
 * @param array{service: callable(): GlobalConstShapeService} $global
 * @param array{service: callable(): ForeignConstShapeService} $foreign
 * @param array{service: callable(): MatchIndexLeft, known: callable(): MatchIndexRight} $matched
 * @param list{ListObjectService} $objects
 */
function run(array $global, array $foreign, array $matched, Holder $holder, array $objects, bool $flag): void {
    $global[KEY]()->missing();
    $foreign[Other::KEY]()->missing();
    $matched[match ($flag) { true => 'service', false => 'known' }]()->missing();
    $matched[match ($flag) { true => 'service', false => 'known' }]()->execute();
    $holder->factories["service"]()->missing();
    $holder->factories["known"]()->execute();
    $holder->factories()["service"]()->missing();
    $objects[0]->missing();
}
`
	diagnostics := (&DiagnosticsProvider{}).Analyse("file:///expression-receivers.php", source)
	if countDiagnosticCode(diagnostics, "Level0.Symbols") != 0 {
		t.Fatalf("expected known receiver classes to avoid level-zero symbol diagnostics, got %#v", diagnostics)
	}
	if countDiagnosticCode(diagnostics, "Level2.MethodExistence") != 6 {
		t.Fatalf("expected six unknown-method diagnostics, got %#v", diagnostics)
	}
	for _, expected := range []string{
		"GlobalConstShapeService::missing()",
		"ForeignConstShapeService::missing()",
		"MatchIndexLeft|MatchIndexRight::missing()",
		"PropertyShapeService::missing()",
		"ReturnedShapeService::missing()",
		"ListObjectService::missing()",
	} {
		if countDiagnosticMessage(diagnostics, "Level2.MethodExistence", expected) != 1 {
			t.Fatalf("expected one diagnostic containing %q, got %#v", expected, diagnostics)
		}
	}
	for _, unexpected := range []string{
		"MatchIndexRight::execute()",
		"KnownPropertyShapeService::execute()",
	} {
		if countDiagnosticMessage(diagnostics, "Level2.MethodExistence", unexpected) != 0 {
			t.Fatalf("expected known method %q to remain clean, got %#v", unexpected, diagnostics)
		}
	}
}

func TestDiagnosticsProvider_ReportsExpandedLevel0ClassModel(t *testing.T) {
	source := `<?php
class NotAnInterface {}
class ImplementsClass implements NotAnInterface {}
interface Contract {}
class ExtendsInterface extends Contract {}
class Demo {
    public function work(): void {}
}
Demo::work();
Demo::missing();
enum UnitWithValue {
    case A = 1;
}
`
	diagnostics := (&DiagnosticsProvider{}).Analyse("file:///level0-class-model.php", source)
	if countDiagnosticCode(diagnostics, "Level0.ClassModel") != 3 {
		t.Fatalf("expected three class-model diagnostics, got %#v", diagnostics)
	}
	if countDiagnosticCode(diagnostics, "Level0.Invocation") != 1 {
		t.Fatalf("expected one static-call-to-instance diagnostic, got %#v", diagnostics)
	}
	if countDiagnosticCode(diagnostics, "Level0.Symbols") != 1 {
		t.Fatalf("expected one unknown static method diagnostic, got %#v", diagnostics)
	}
	for _, expected := range []string{
		"implements class NotAnInterface",
		"extends interface Contract",
		"is not backed, but case A has value 1",
		"Static call to instance method Demo::work()",
		"undefined static method Demo::missing()",
	} {
		found := false
		for _, diagnostic := range diagnostics {
			if strings.Contains(diagnostic.Message, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected diagnostic containing %q, got %#v", expected, diagnostics)
		}
	}
}

func TestDiagnosticsProvider_ReportsLeftoverLevel0EnumAndCasts(t *testing.T) {
	source := `<?php
$x = (void) 1;
$y = (unset) 1;
enum BadMethods: string {
    case A = 'a';
    public function __construct() {}
}
`
	diagnostics := (&DiagnosticsProvider{}).Analyse("file:///level0-enum-casts.php", source)
	if countDiagnosticCode(diagnostics, "Level0.Language") != 2 {
		t.Fatalf("expected two invalid-cast diagnostics, got %#v", diagnostics)
	}
	if countDiagnosticCode(diagnostics, "Level0.ClassModel") != 1 {
		t.Fatalf("expected one enum constructor diagnostic, got %#v", diagnostics)
	}
	for _, expected := range []string{
		"Cannot cast to void.",
		"Cannot cast to unset.",
		"Enum BadMethods contains constructor.",
	} {
		found := false
		for _, diagnostic := range diagnostics {
			if strings.Contains(diagnostic.Message, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected diagnostic containing %q, got %#v", expected, diagnostics)
		}
	}
}

func TestDiagnosticsProvider_ReportsNonObjectMethodReceivers(t *testing.T) {
	source := `<?php
class UnionStringService {}
class KnownUnionStringService { public function execute(): void {} }
class UnionIntService {}

function run(
    int $int,
    array $array,
    callable $callable,
    iterable $iterable,
    object $object,
    UnionStringService|string $union,
    KnownUnionStringService|string $known,
    int|UnionIntService $intUnion
): void {
    $int->missing();
    $array->missing();
    $callable->missing();
    $iterable->missing();
    $object->missing();
    $union->missing();
    $known->execute();
    $intUnion->missing();
}
`
	diagnostics := (&DiagnosticsProvider{}).Analyse("file:///non-object-receivers.php", source)
	if countDiagnosticCode(diagnostics, "Level0.Symbols") != 0 {
		t.Fatalf("expected known receiver classes to avoid level-zero symbol diagnostics, got %#v", diagnostics)
	}
	if countDiagnosticCode(diagnostics, "Level2.MethodNonObject") != 6 {
		t.Fatalf("expected six non-object method diagnostics, got %#v", diagnostics)
	}
	for _, expected := range []string{
		"on int.",
		"on array.",
		"on callable.",
		"on iterable.",
	} {
		if countDiagnosticMessage(diagnostics, "Level2.MethodNonObject", expected) != 1 {
			t.Fatalf("expected one diagnostic containing %q, got %#v", expected, diagnostics)
		}
	}
	if countDiagnosticMessage(diagnostics, "Level2.MethodNonObject", "on object.") != 0 {
		t.Fatalf("expected object receivers to remain clean, got %#v", diagnostics)
	}
	if countDiagnosticMessage(diagnostics, "Level2.MethodNonObject", "execute() on") != 0 {
		t.Fatalf("expected known class-or-string methods to remain clean, got %#v", diagnostics)
	}
}

func TestDiagnosticsProvider_ReportsDNFAndNullableUnknownMethods(t *testing.T) {
	source := `<?php
interface DnfMissingLeft {}
interface DnfMissingTag {}
class DnfMissingRight {}
interface HasAvailableMethod { public function available(): void; }
interface DnfAvailableTag {}
class NullableMissing {}
class NullableKnown { public function available(): void {} }

function run(
    (DnfMissingLeft&DnfMissingTag)|DnfMissingRight $allMissing,
    (HasAvailableMethod&DnfAvailableTag)|DnfMissingRight $partiallyAvailable,
    ?NullableMissing $nullableMissing,
    NullableKnown|null $nullableKnown,
    bool $flag
): void {
    $allMissing->missing();
    $partiallyAvailable->available();
    $nullableMissing->missing();
    $nullableKnown->available();
    ($flag ? new NullableMissing() : null)->missing();
}
`
	diagnostics := (&DiagnosticsProvider{}).Analyse("file:///dnf-nullable.php", source)
	if countDiagnosticCode(diagnostics, "Level0.Symbols") != 0 {
		t.Fatalf("expected defined DNF members to avoid level-zero symbol diagnostics, got %#v", diagnostics)
	}

	methodDiagnostics := countDiagnosticCode(diagnostics, "Level2.MethodExistence")
	if methodDiagnostics != 3 {
		t.Fatalf("expected three DNF/nullable unknown-method diagnostics, got %d: %#v", methodDiagnostics, diagnostics)
	}

	if countDiagnosticMessage(diagnostics, "Level2.MethodExistence", "(DnfMissingLeft&DnfMissingTag)|DnfMissingRight::missing()") != 1 {
		t.Fatalf("expected one all-missing DNF diagnostic, got %#v", diagnostics)
	}
	if countDiagnosticMessage(diagnostics, "Level2.MethodExistence", "NullableMissing::missing()") != 2 {
		t.Fatalf("expected nullable parameter and ternary diagnostics, got %#v", diagnostics)
	}
	for _, unexpected := range []string{"available()", "NullableKnown::available()"} {
		if countDiagnosticMessage(diagnostics, "Level2.MethodExistence", unexpected) != 0 {
			t.Fatalf("expected supported DNF/known nullable method to remain clean for %q, got %#v", unexpected, diagnostics)
		}
	}
	if countDiagnosticMessage(diagnostics, "Level7.MethodUnion", "(DnfAvailableTag&HasAvailableMethod)|DnfMissingRight::available()") != 1 {
		t.Fatalf("expected one partial-DNF level-seven diagnostic, got %#v", diagnostics)
	}
	if countDiagnosticMessage(diagnostics, "Level8.MethodNonObject", "available() on NullableKnown|null.") != 1 {
		t.Fatalf("expected one known nullable level-eight diagnostic, got %#v", diagnostics)
	}
}

func TestDiagnosticsProvider_SuppressesDisabledUndefinedVariables(t *testing.T) {
	source := `<?php
function run(): void {
    echo $missing;
}
`
	enabled := (&DiagnosticsProvider{}).Analyse("file:///test.php", source)
	if !hasDiagnosticCode(enabled, "Level1.Variables") {
		t.Fatalf("expected undefined-variable diagnostic before disabling it, got %#v", enabled)
	}

	disabled := (&DiagnosticsProvider{cfg: Config{DisabledAnalysis: DisabledAnalysis{UndefinedVariables: true}}}).Analyse("file:///test.php", source)
	if hasDiagnosticCode(disabled, "Level1.Variables") {
		t.Fatalf("expected undefined-variable diagnostic to be disabled, got %#v", disabled)
	}
}

func TestDiagnosticsProviderPreservesAnalysisLevelBoundaries(t *testing.T) {
	source := `<?php
class VisibilityExample {
    protected function hidden(): void {}
    public static function allowed(): void {}
}
(new VisibilityExample())->hidden();
(new VisibilityExample())->allowed();
throw new DateTime();
`
	diagnostics := (&DiagnosticsProvider{}).Analyse("file:///levels.php", source)

	if !hasDiagnosticCode(diagnostics, "Level2.MethodVisibility") {
		t.Fatalf("expected protected visibility at level two, got %#v", diagnostics)
	}
	if !hasDiagnosticCode(diagnostics, "Level3.ThrowType") {
		t.Fatalf("expected non-throwable object at level three, got %#v", diagnostics)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "Level0.Invocation" && strings.Contains(diagnostic.Message, "static method") {
			t.Fatalf("instance syntax for a static method should remain clean, got %#v", diagnostics)
		}
	}
}

func TestDiagnosticsProviderReportsBinaryAndInferredReturnTypes(t *testing.T) {
	source := `<?php
function invalidOperation(): void {
    echo 1 + "bad";
}

function invalidReturn(): string {
    return 1 < 2;
}

function validReturn(): bool {
    return 1 < 2;
}
`
	diagnostics := (&DiagnosticsProvider{}).Analyse("file:///expressions.php", source)
	if countDiagnosticCode(diagnostics, "A.BINARY.OP.INVALID") != 1 {
		t.Fatalf("expected one invalid binary-operation diagnostic, got %#v", diagnostics)
	}
	if countDiagnosticCode(diagnostics, "A.RETURN.TYPE") != 1 {
		t.Fatalf("expected one inferred return-type diagnostic, got %#v", diagnostics)
	}
}

func TestDiagnosticsProviderReportsPHPDocDeclarationErrors(t *testing.T) {
	source := `<?php
class KnownService {}

/** @param MissingService $service */
function unknownClass($service): void { echo $service; }

/** @param string $value */
function incompatibleType(int $value): void { echo $value; }

/** @param string $missing */
function unknownName(int $value): void { echo $value; }

/** @return string */
function incompatibleReturn(): int { return 1; }

class Holder {
    /** @var string */
    public int $count = 0;
}

/** @return KnownService */
function clean(KnownService $service): KnownService { return $service; }
`
	diagnostics := (&DiagnosticsProvider{}).Analyse("file:///phpdoc.php", source)
	if countDiagnosticCode(diagnostics, "Level2.PHPDocClass") != 1 {
		t.Fatalf("expected one unknown PHPDoc-class diagnostic, got %#v", diagnostics)
	}
	if countDiagnosticCode(diagnostics, "Level2.PHPDocParamType") != 1 {
		t.Fatalf("expected one PHPDoc parameter-type diagnostic, got %#v", diagnostics)
	}
	if countDiagnosticCode(diagnostics, "Level2.PHPDocParamName") != 1 {
		t.Fatalf("expected one PHPDoc parameter-name diagnostic, got %#v", diagnostics)
	}
	if countDiagnosticCode(diagnostics, "Level2.PHPDocReturnType") != 1 {
		t.Fatalf("expected one PHPDoc return-type diagnostic, got %#v", diagnostics)
	}
	if countDiagnosticCode(diagnostics, "Level2.PHPDocPropertyType") != 1 {
		t.Fatalf("expected one PHPDoc property-type diagnostic, got %#v", diagnostics)
	}
}

func TestDiagnosticsProviderReportsAndSuppressesPHPDocGenericErrors(t *testing.T) {
	source := `<?php
/** @template TValue */
final class SingleValueContainer {}

/**
 * @template TKey
 * @template TValue
 */
final class PairValueContainer {}

final class PlainValueContainer {}

class Animal {}

class Vehicle {}

/** @template TAnimal of Animal */
final class AnimalContainer {}

/** @param SingleValueContainer<int, string> $box */
function inspectTooMany(SingleValueContainer $box): void {}

/** @param PairValueContainer<int> $box */
function inspectTooFew(PairValueContainer $box): void {}

/** @param PlainValueContainer<int> $box */
function inspectNonGeneric(PlainValueContainer $box): void {}

/** @param AnimalContainer<Vehicle> $box */
function inspectInvalidBound(AnimalContainer $box): void {}

/** @param AnimalContainer<Animal> $box */
function inspectValidBound(AnimalContainer $box): void {}
`

	enabled := (&DiagnosticsProvider{}).Analyse("file:///phpdoc-generics.php", source)
	for _, code := range []string{
		"Level2.PHPDocGenericMoreTypes",
		"Level2.PHPDocGenericLessTypes",
		"Level2.PHPDocNotGeneric",
		"Level2.PHPDocGenericNotSubtype",
	} {
		if countDiagnosticCode(enabled, code) != 1 {
			t.Fatalf("expected one %s diagnostic at level two, got %#v", code, enabled)
		}
	}

	disabled := (&DiagnosticsProvider{cfg: Config{DisabledAnalysis: DisabledAnalysis{TypeErrors: true}}}).Analyse("file:///phpdoc-generics.php", source)
	for _, code := range []string{
		"Level2.PHPDocGenericMoreTypes",
		"Level2.PHPDocGenericLessTypes",
		"Level2.PHPDocNotGeneric",
		"Level2.PHPDocGenericNotSubtype",
	} {
		if hasDiagnosticCode(disabled, code) {
			t.Fatalf("expected %s to be suppressed by type-error toggle, got %#v", code, disabled)
		}
	}
}

func TestDiagnosticsProviderReportsAndSuppressesLevel6MissingTypes(t *testing.T) {
	source := `<?php
class Animal {}

class Vehicle {}

/** @template TAnimal of Animal */
final class AnimalContainer {}

function inspectMissingGeneric(AnimalContainer $box): void {}

/** @param AnimalContainer<Animal> $box */
function inspectKnownGeneric(AnimalContainer $box): void {}

function inspectMissingIterable(array $items): void {}

/** @param array<int, string> $items */
function inspectKnownIterable(array $items): void {}
`

	enabled := (&DiagnosticsProvider{}).Analyse("file:///level6-missing-types.php", source)
	if countDiagnosticCode(enabled, "Level6.MissingGenericType") != 1 {
		t.Fatalf("expected one missing-generic-type diagnostic at level six, got %#v", enabled)
	}
	if countDiagnosticCode(enabled, "Level6.MissingIterableValueType") != 1 {
		t.Fatalf("expected one missing-iterable-value-type diagnostic at level six, got %#v", enabled)
	}

	disabled := (&DiagnosticsProvider{cfg: Config{DisabledAnalysis: DisabledAnalysis{TypeErrors: true}}}).Analyse("file:///level6-missing-types.php", source)
	for _, code := range []string{
		"Level6.MissingGenericType",
		"Level6.MissingIterableValueType",
	} {
		if hasDiagnosticCode(disabled, code) {
			t.Fatalf("expected %s to be suppressed by type-error toggle, got %#v", code, disabled)
		}
	}
}

func TestDiagnosticsProviderReportsLevel6MissingDeclarationTypesAndHonorsControls(t *testing.T) {
	source := `<?php
final class DeclarationTypes
{
    public $missingProperty;

    /** @var string */
    public $documentedProperty;

    public function missingMethod($value)
    {
        return $value;
    }

    /**
     * @param string $value
     * @return string
     */
    public function documentedMethod($value)
    {
        return $value;
    }

    public function explicitMethod(mixed $value): void
    {
    }
}

function missingFunction($value)
{
    return $value;
}

function explicitFunction(mixed $value): void
{
}
`

	enabled := (&DiagnosticsProvider{}).Analyse("file:///level6-missing-declaration-types.php", source)
	want := map[string]int{
		"Level6.MissingParameterType": 2,
		"Level6.MissingReturnType":    2,
		"Level6.MissingPropertyType":  1,
	}
	for code, count := range want {
		if got := countDiagnosticCode(enabled, code); got != count {
			t.Fatalf("expected %d %s diagnostics, got %d: %#v", count, code, got, enabled)
		}
	}

	disabled := (&DiagnosticsProvider{cfg: Config{DisabledAnalysis: DisabledAnalysis{TypeErrors: true}}}).Analyse("file:///level6-missing-declaration-types.php", source)
	for code := range want {
		if hasDiagnosticCode(disabled, code) {
			t.Fatalf("expected %s to be suppressed by type-error toggle, got %#v", code, disabled)
		}
	}
}

func TestDiagnosticsProviderReportsAndSuppressesNamedFunctionArgumentTypes(t *testing.T) {
	source := `<?php
function acceptCount(int $count): void {}
function run(): void { acceptCount('wrong'); }
`

	enabled := (&DiagnosticsProvider{}).Analyse("file:///level5-function-argument.php", source)
	if countDiagnosticCode(enabled, "A.ARG.TYPE") != 1 {
		t.Fatalf("expected one named-function argument-type diagnostic, got %#v", enabled)
	}

	disabled := (&DiagnosticsProvider{cfg: Config{DisabledAnalysis: DisabledAnalysis{TypeErrors: true}}}).Analyse("file:///level5-function-argument.php", source)
	if hasDiagnosticCode(disabled, "A.ARG.TYPE") {
		t.Fatalf("expected named-function argument-type diagnostic to be suppressed, got %#v", disabled)
	}
}

func TestDiagnosticsProviderReportsAndSuppressesCallablePropertyTypes(t *testing.T) {
	source := `<?php
final class CallbackHolder {
    public callable $native;
    /** @var callable */
    public $documented;
    public Closure $closure;
}
`

	enabled := (&DiagnosticsProvider{}).Analyse("file:///callable-property.php", source)
	if countDiagnosticCode(enabled, "Level0.PropertyCallableType") != 1 {
		t.Fatalf("expected one native callable-property diagnostic, got %#v", enabled)
	}

	disabled := (&DiagnosticsProvider{cfg: Config{DisabledAnalysis: DisabledAnalysis{TypeErrors: true}}}).Analyse("file:///callable-property.php", source)
	if hasDiagnosticCode(disabled, "Level0.PropertyCallableType") {
		t.Fatalf("expected callable-property diagnostic to be suppressed, got %#v", disabled)
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

func hasStyleDiagnostic(diagnostics []lsp.Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		code, _ := diagnostic.Code.(string)
		if strings.HasPrefix(code, "PSR") || strings.HasPrefix(code, "Generic.Functions") || strings.HasPrefix(code, "Generic.Arrays") {
			return true
		}
	}
	return false
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

func countDiagnosticCode(diagnostics []lsp.Diagnostic, code string) int {
	count := 0
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			count++
		}
	}
	return count
}

func countDiagnosticMessage(diagnostics []lsp.Diagnostic, code, message string) int {
	count := 0
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code && strings.Contains(diagnostic.Message, message) {
			count++
		}
	}
	return count
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
		if code, ok := diag.Code.(string); ok && code == "Level0.Invocation" && strings.Contains(diag.Message, "assertSame") {
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
		if code, ok := diag.Code.(string); ok && code == "Level0.ClassModel" && strings.Contains(diag.Message, "JsonException") {
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
		if code, ok := diag.Code.(string); ok && code == "Level0.Symbols" && strings.Contains(diag.Message, "helper_id") {
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
		if code, ok := diag.Code.(string); ok && code == "Level0.Symbols" && strings.Contains(diag.Message, "local_helper") {
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
			if code, ok := diag.Code.(string); ok && code == "Level0.ClassModel" && strings.Contains(diag.Message, "DuplicateName") {
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
		if diagnostic.Code != "Level0.Symbols" {
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
	t.Fatalf("expected Level0.Symbols diagnostic, got %#v", diagnostics)
}

func TestDiagnosticsProvider_TernaryNarrowingIsBranchLocal(t *testing.T) {
	p := &DiagnosticsProvider{}
	diags := p.Analyse("file:///ternary.php", `<?php
class Lantern {
    public function glow(): string { return 'lit'; }
}
function illuminate(Lantern $lamp): string { return $lamp->glow(); }
function guarded(?Lantern $lamp): string {
    return $lamp ? illuminate($lamp) : '';
}
function reversed(?Lantern $lamp): string {
    return $lamp === null ? '' : $lamp->glow();
}
function fallback(?Lantern $lamp): Lantern {
    return $lamp ?: new Lantern();
}
function outside(?Lantern $lamp): string {
    $text = $lamp ? $lamp->glow() : '';
    return illuminate($lamp);
}
`)
	var typeIssues int
	for _, diag := range diags {
		code, _ := diag.Code.(string)
		if code == "A.ARG.TYPE" {
			typeIssues++
			if diag.Range.Start.Line != 16 {
				t.Fatalf("unexpected argument error inside a narrowed branch: %+v", diag)
			}
		}
		if code == "A.RETURN.TYPE" || code == "Level8.MethodNonObject" {
			t.Fatalf("unexpected guarded expression error: %+v", diag)
		}
	}
	if typeIssues != 1 {
		t.Fatalf("expected one unguarded argument error, got %+v", diags)
	}
}
