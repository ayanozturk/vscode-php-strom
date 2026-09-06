package indexer

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ayanozturk/go-php-parser/ast"
)

func TestReadAtMostStopsAfterLimitSentinel(t *testing.T) {
	source := bytes.NewReader(bytes.Repeat([]byte("x"), 128))

	data, oversized, err := readAtMost(source, 16)
	if err != nil {
		t.Fatalf("read bounded input: %v", err)
	}
	if !oversized {
		t.Fatal("expected input larger than the limit to be rejected")
	}
	if len(data) != 0 {
		t.Fatalf("expected oversized data to be discarded, got %d bytes", len(data))
	}
	if got, want := source.Len(), 111; got != want {
		t.Fatalf("expected only limit+1 bytes to be consumed, %d bytes remain; want %d", got, want)
	}
}

func TestReadFileWithinLimitRejectsOversizedRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.php")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 128), 0o644); err != nil {
		t.Fatalf("write oversized fixture: %v", err)
	}

	data, size, oversized, err := ReadFileWithinLimit(path, 16)
	if err != nil {
		t.Fatalf("read oversized file: %v", err)
	}
	if !oversized {
		t.Fatal("expected oversized regular file to be rejected")
	}
	if size != 128 {
		t.Fatalf("expected reported size 128, got %d", size)
	}
	if len(data) != 0 {
		t.Fatalf("expected oversized data to be discarded, got %d bytes", len(data))
	}
}

func TestReadFileWithinLimitAllowsFileAtLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bounded.php")
	want := []byte("<?php\n")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("write bounded fixture: %v", err)
	}

	data, size, oversized, err := ReadFileWithinLimit(path, int64(len(want)))
	if err != nil {
		t.Fatalf("read bounded file: %v", err)
	}
	if oversized {
		t.Fatal("did not expect file at the size limit to be rejected")
	}
	if size != int64(len(want)) {
		t.Fatalf("expected reported size %d, got %d", len(want), size)
	}
	if !bytes.Equal(data, want) {
		t.Fatalf("expected %q, got %q", want, data)
	}
}

func TestBuildLineOffsets(t *testing.T) {
	src := "<?php\nclass Foo\n{\n}\n"
	offsets := buildLineOffsets(src)
	// Line 0 starts at 0, line 1 starts after first \n, etc.
	expected := []int{0, 6, 16, 18, 20}
	if len(offsets) != len(expected) {
		t.Fatalf("expected %d offsets, got %d: %v", len(expected), len(offsets), offsets)
	}
	for i, want := range expected {
		if offsets[i] != want {
			t.Errorf("offsets[%d]: want %d, got %d", i, want, offsets[i])
		}
	}
}

func TestOffsetToLineChar(t *testing.T) {
	src := "<?php\nclass Foo\n{\n}\n"
	offsets := buildLineOffsets(src)

	tests := []struct {
		offset   int
		wantLine uint32
		wantChar uint32
	}{
		{0, 0, 0},  // start of file
		{5, 0, 5},  // end of first line before \n
		{6, 1, 0},  // start of "class Foo"
		{11, 1, 5}, // "F" in "Foo"
		{16, 2, 0}, // "{"
	}
	for _, tc := range tests {
		line, char := offsetToLineChar(offsets, tc.offset)
		if line != tc.wantLine || char != tc.wantChar {
			t.Errorf("offset %d: want (%d,%d), got (%d,%d)", tc.offset, tc.wantLine, tc.wantChar, line, char)
		}
	}
}

func TestExtractSymbolsHasLSPPositions(t *testing.T) {
	src := "<?php\nclass MyClass {}\n"
	syms := extractSymbols("file:///test.php", src)
	if len(syms) == 0 {
		t.Fatal("expected at least one symbol")
	}
	cls := syms[0]
	if cls.Name != "MyClass" {
		t.Fatalf("expected MyClass, got %s", cls.Name)
	}
	// "class" keyword is on line 1 (0-based), so StartLine should be 1
	if cls.StartLine != 1 {
		t.Errorf("expected StartLine=1 (0-based), got %d", cls.StartLine)
	}
}

func TestExtractSymbolsUsesGoParserForPropertyHooks(t *testing.T) {
	src := `<?php
class Example {
    public private(set) string $name {
        get => $this->name;
        set => $value;
    }
}
`

	syms := extractSymbols("file:///test.php", src)
	if len(syms) < 2 {
		t.Fatalf("expected class and property symbols, got %d", len(syms))
	}

	var property *Symbol
	for _, sym := range syms {
		if sym.Kind == KindProperty && sym.Name == "name" {
			property = sym
			break
		}
	}

	if property == nil {
		t.Fatal("expected property symbol for hook-backed property")
	}
	if property.Visibility != "public" {
		t.Fatalf("expected public visibility, got %q", property.Visibility)
	}
}

func TestExtractSymbolsIndexesEveryGroupedProperty(t *testing.T) {
	src := `<?php
class Coordinates {
    public int $horizontal = 0, $vertical = 0;
}`
	syms := extractSymbols("file:///test.php", src)
	properties := map[string]*Symbol{}
	for _, sym := range syms {
		if sym.Kind == KindProperty {
			properties[sym.Name] = sym
		}
	}
	for _, name := range []string{"horizontal", "vertical"} {
		property := properties[name]
		if property == nil {
			t.Fatalf("expected grouped property %q to be indexed, got %#v", name, properties)
		}
		if property.Type != "int" || property.Visibility != "public" {
			t.Fatalf("unexpected grouped property symbol %q: %#v", name, property)
		}
	}
}

func TestExtractSymbolsCreatesPromotedPropertySymbols(t *testing.T) {
	src := `<?php
class SessionStore {}

class Session {
	public function __construct(private SessionStore $session) {}
}
`

	syms := extractSymbols("file:///test.php", src)
	for _, sym := range syms {
		if sym.Kind == KindProperty && sym.Name == "session" {
			if sym.Type != "SessionStore" {
				t.Fatalf("expected promoted property type SessionStore, got %q", sym.Type)
			}
			if sym.Visibility != "private" {
				t.Fatalf("expected promoted property visibility private, got %q", sym.Visibility)
			}
			return
		}
	}

	t.Fatal("expected promoted constructor param to be indexed as property symbol")
}

func TestExtractSymbolsResolvesAliasedMethodParameterTypes(t *testing.T) {
	src := `<?php
use Vendor\Package\SchemeFilter as SchemeFilter;

class Repository {
	public function getByFilterObject(SchemeFilter $filter): void {}
}
`

	syms := extractSymbols("file:///test.php", src)
	for _, sym := range syms {
		if sym.Kind == KindMethod && sym.Name == "getByFilterObject" {
			if len(sym.Params) != 1 {
				t.Fatalf("expected one parameter, got %#v", sym.Params)
			}
			if sym.Params[0].Type != "Vendor\\Package\\SchemeFilter" {
				t.Fatalf("expected aliased param type to resolve to Vendor\\Package\\SchemeFilter, got %q", sym.Params[0].Type)
			}
			return
		}
	}

	t.Fatal("expected method symbol")
}

func TestExtractSymbolsIndexesPHPDocMethodReturnTypes(t *testing.T) {
	src := `<?php
namespace App\Module\Shift\Repository;

use App\Module\Shift\Entity\Shift;

/**
 * @method Shift|null find($id, $lockMode = null, $lockVersion = null)
 * @method Shift[]    findAll()
 */
class ShiftRepository extends AbstractRepository
{
}
`

	syms := extractSymbols("file:///test.php", src)
	for _, sym := range syms {
		if sym.Kind != KindMethod || sym.FQN != `\App\Module\Shift\Repository\ShiftRepository::find` {
			continue
		}
		if sym.ReturnType != `App\Module\Shift\Entity\Shift|null` {
			t.Fatalf("expected PHPDoc method return type to resolve to Shift|null, got %q", sym.ReturnType)
		}
		if len(sym.Params) != 3 {
			t.Fatalf("expected three PHPDoc method params, got %#v", sym.Params)
		}
		if sym.Params[0].Name != "id" || sym.Params[0].HasDefault {
			t.Fatalf("expected required $id param, got %#v", sym.Params[0])
		}
		if sym.Params[1].Name != "lockMode" || !sym.Params[1].HasDefault {
			t.Fatalf("expected optional $lockMode param, got %#v", sym.Params[1])
		}
		return
	}

	t.Fatal("expected PHPDoc @method find symbol")
}

func TestExtractSymbolsKeepsGenericPHPDocMethodParametersIntact(t *testing.T) {
	src := `<?php
namespace App;

/**
 * @method Record|null lookup(array<string, mixed> $criteria, ?array<string, mixed> $orderBy = null)
 */
class RecordStore
{
}
`

	syms := extractSymbols("file:///test.php", src)
	for _, sym := range syms {
		if sym.Kind != KindMethod || sym.FQN != `\App\RecordStore::lookup` {
			continue
		}
		if len(sym.Params) != 2 {
			t.Fatalf("expected two PHPDoc method params, got %#v", sym.Params)
		}
		if sym.Params[0].Name != "criteria" || sym.Params[0].Type != "array<string,mixed>" || sym.Params[0].HasDefault {
			t.Fatalf("expected required generic criteria param, got %#v", sym.Params[0])
		}
		if sym.Params[1].Name != "orderBy" || sym.Params[1].Type != "?array<string,mixed>" || !sym.Params[1].HasDefault {
			t.Fatalf("expected optional generic orderBy param, got %#v", sym.Params[1])
		}
		return
	}

	t.Fatal("expected generic PHPDoc @method symbol")
}

func TestExtractSymbolsIndexesGenericClassInheritance(t *testing.T) {
	syms := extractSymbols("file:///workspace/Repositories.php", `<?php
namespace App;

/**
 * @template T
 */
abstract class GenericStore {
    /**
     * @return T|null
     */
    public function lookup(string $id): ?object {}
}

/**
 * @extends GenericStore<Record>
 */
class RecordStore extends GenericStore {}
`)
	for _, sym := range syms {
		if sym.FQN == `\App\GenericStore::lookup` && sym.ReturnType != "T|null" {
			t.Fatalf("expected template method return type to be preserved, got %q", sym.ReturnType)
		}
		if sym.Name != "RecordStore" {
			continue
		}
		if len(sym.GenericParents) != 1 || sym.GenericParents[0].FQN != `\App\GenericStore` || len(sym.GenericParents[0].TypeArguments) != 1 || sym.GenericParents[0].TypeArguments[0] != `App\Record` {
			t.Fatalf("unexpected generic parent metadata: %#v", sym.GenericParents)
		}
		return
	}
	t.Fatal("expected RecordStore symbol")
}

func TestExtractSymbolsResolvesImplementedInterfacesToFQNs(t *testing.T) {
	src := `<?php
namespace Doctrine\Common\Collections;

interface Collection {}

class ArrayCollection implements Collection {}
`

	syms := extractSymbols("file:///test.php", src)
	for _, sym := range syms {
		if sym.Kind == KindClass && sym.Name == "ArrayCollection" {
			if len(sym.Implements) != 1 || sym.Implements[0] != `\Doctrine\Common\Collections\Collection` {
				t.Fatalf("expected fully-qualified implements entry, got %#v", sym.Implements)
			}
			return
		}
	}

	t.Fatal("expected ArrayCollection symbol")
}

func TestExtractSymbolsIndexesTraitUsesOnClassesAndTraits(t *testing.T) {
	src := `<?php
namespace Symfony\Bundle\FrameworkBundle\Test;

trait BrowserKitAssertionsTrait {}

trait WebTestAssertionsTrait
{
    use BrowserKitAssertionsTrait;
}

class WebTestCase
{
    use WebTestAssertionsTrait;
}
`

	syms := extractSymbols("file:///test.php", src)
	var webTestCase, assertionsTrait *Symbol
	for _, sym := range syms {
		switch {
		case sym.Kind == KindClass && sym.Name == "WebTestCase":
			webTestCase = sym
		case sym.Kind == KindModule && sym.Name == "WebTestAssertionsTrait":
			assertionsTrait = sym
		}
	}
	if webTestCase == nil || len(webTestCase.Traits) != 1 || webTestCase.Traits[0] != `\Symfony\Bundle\FrameworkBundle\Test\WebTestAssertionsTrait` {
		t.Fatalf("expected WebTestCase trait use, got %#v", webTestCase)
	}
	if assertionsTrait == nil || len(assertionsTrait.Traits) != 1 || assertionsTrait.Traits[0] != `\Symfony\Bundle\FrameworkBundle\Test\BrowserKitAssertionsTrait` {
		t.Fatalf("expected composed trait use, got %#v", assertionsTrait)
	}
}

func TestConfiguredStubsAreIndexed(t *testing.T) {
	wi := New(Config{Stubs: []string{"Core", "SPL"}, PHPVersion: "8.3"})
	sym := wi.GetIndex().GetByFQN(`\ArrayIterator`)
	if sym == nil {
		t.Fatal("expected ArrayIterator stub symbol to be indexed")
	}
	if len(sym.Implements) == 0 {
		t.Fatalf("expected ArrayIterator implements metadata, got %#v", sym)
	}
}

func TestExpandedSPLStubsAreIndexed(t *testing.T) {
	wi := New(Config{Stubs: []string{"Core", "SPL"}, PHPVersion: "8.4"})
	for _, fqn := range []string{
		`\RecursiveIteratorIterator`,
		`\SplFileObject`,
		`\SplPriorityQueue`,
		`\SplObjectStorage`,
		`\InvalidArgumentException`,
	} {
		if sym := wi.GetIndex().GetByFQN(fqn); sym == nil {
			t.Fatalf("expected %s stub symbol to be indexed", fqn)
		}
	}
}

func TestMatchSimpleMatchesVendorRootPattern(t *testing.T) {
	if !matchSimple("**/vendor/**", "/workspace/project/vendor") {
		t.Fatal("expected vendor root to match exclude pattern")
	}
	if !matchSimple("**/vendor/**", "/workspace/project/vendor/symfony/http-foundation") {
		t.Fatal("expected nested vendor path to match exclude pattern")
	}
}

func TestExtractSymbolsIndexesEnumNativePropertiesAndMethods(t *testing.T) {
	src := `<?php
namespace App\Module\Subscription\Enum;

enum InvoiceStatus: int {
    case PENDING = 1;

    public function isPending(): bool {
        return $this->value === self::PENDING->value;
    }
}

enum UnitStatus {
    case Ready;
}
`
	syms := extractSymbols("file:///test.php", src)
	byFQN := map[string]*Symbol{}
	for _, sym := range syms {
		byFQN[sym.FQN] = sym
	}

	value, ok := byFQN[`\App\Module\Subscription\Enum\InvoiceStatus::$value`]
	if !ok || value.Kind != KindProperty || value.Type != "int" || !value.IsReadonly {
		t.Fatalf("expected backed enum $value property, got %#v, %v", value, ok)
	}
	name, ok := byFQN[`\App\Module\Subscription\Enum\InvoiceStatus::$name`]
	if !ok || name.Kind != KindProperty || name.Type != "string" {
		t.Fatalf("expected enum $name property, got %#v, %v", name, ok)
	}
	method, ok := byFQN[`\App\Module\Subscription\Enum\InvoiceStatus::isPending`]
	if !ok || method.Kind != KindMethod {
		t.Fatalf("expected enum instance method, got %#v, %v", method, ok)
	}
	if _, ok := byFQN[`\App\Module\Subscription\Enum\UnitStatus::$name`]; !ok {
		t.Fatal("expected unit enum $name property")
	}
	if _, ok := byFQN[`\App\Module\Subscription\Enum\UnitStatus::$value`]; ok {
		t.Fatal("did not expect unit enum $value property")
	}
}

func TestMatchSimpleMatchesVendorTestsPattern(t *testing.T) {
	if !matchSimple("**/vendor/**/{Tests,tests}/**", "/workspace/project/vendor/symfony/http-foundation/Tests") {
		t.Fatal("expected Tests directory under vendor to match exclude pattern")
	}
	if !matchSimple("**/vendor/**/{Tests,tests}/**", "/workspace/project/vendor/symfony/http-foundation/tests/Unit") {
		t.Fatal("expected tests subtree under vendor to match exclude pattern")
	}
	if matchSimple("**/vendor/**/{Tests,tests}/**", "/workspace/project/vendor/symfony/http-foundation") {
		t.Fatal("did not expect non-test vendor directory to match vendor test exclude pattern")
	}
}

func TestCollectWorkspaceFilePathsRespectsGitignore(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte("generated/\nignored.php\n!generated/keep.php\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "generated"), 0o755); err != nil {
		t.Fatalf("mkdir generated: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "kept.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatalf("write kept.php: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "ignored.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatalf("write ignored.php: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "generated", "skip.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatalf("write generated/skip.php: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "generated", "keep.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatalf("write generated/keep.php: %v", err)
	}

	wi := New(Config{Associations: []string{"**/*.php"}})
	folders := []WorkspaceFolder{{URI: pathToURI(tmpDir), Name: "tmp"}}
	wi.SetWorkspaceFolders(folders)

	paths := wi.collectWorkspaceFilePaths(folders, wi.gitignores)
	seen := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		seen[filepath.Base(p)] = struct{}{}
	}

	if _, ok := seen["kept.php"]; !ok {
		t.Fatal("expected kept.php to be indexed")
	}
	if _, ok := seen["ignored.php"]; ok {
		t.Fatal("expected ignored.php to be excluded by .gitignore")
	}
	if _, ok := seen["skip.php"]; ok {
		t.Fatal("expected generated/skip.php to be excluded by .gitignore")
	}
	if _, ok := seen["keep.php"]; !ok {
		t.Fatal("expected negated .gitignore rule to include generated/keep.php")
	}
}

func TestCollectWorkspaceFilePathsIndexesGitignoredVendorDependencies(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte("/vendor/\n"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	vendorFile := filepath.Join(tmpDir, "vendor", "phpunit", "phpunit", "src", "Framework", "TestCase.php")
	if err := os.MkdirAll(filepath.Dir(vendorFile), 0o755); err != nil {
		t.Fatalf("mkdir vendor tree: %v", err)
	}
	if err := os.WriteFile(vendorFile, []byte("<?php\nnamespace PHPUnit\\Framework;\nclass TestCase {}\n"), 0o644); err != nil {
		t.Fatalf("write vendor file: %v", err)
	}

	wi := New(Config{Associations: []string{"**/*.php"}})
	folders := []WorkspaceFolder{{URI: pathToURI(tmpDir), Name: "tmp"}}
	wi.SetWorkspaceFolders(folders)

	paths := wi.collectWorkspaceFilePaths(folders, wi.gitignores)
	for _, path := range paths {
		if path == vendorFile {
			return
		}
	}

	t.Fatalf("expected gitignored Composer dependency to be indexed, got %#v", paths)
}

func TestReplaceWorkspaceProjectNodesDropsVendorASTsAndKeepsSymbols(t *testing.T) {
	wi := New(Config{})
	vendorURI := "file:///workspace/vendor/symfony/framework-bundle/Test/WebTestCase.php"
	appURI := "file:///workspace/src/App.php"
	vendor := ParseSourceForIndex(vendorURI, `<?php
namespace Symfony\Bundle\FrameworkBundle\Test;
class WebTestCase {
    public static function assertResponseIsSuccessful(): void {}
}
`)
	app := ParseSourceForIndex(appURI, `<?php
class App {
    public function run(): void {}
}
`)
	wi.replaceWorkspaceProjectNodes(map[string][]ast.Node{
		vendorURI: vendor.Nodes,
		appURI:    app.Nodes,
	}, map[string]uint64{
		vendorURI: sourceHash(vendor.Text),
		appURI:    sourceHash(app.Text),
	})

	if _, ok := wi.projectNodes[vendorURI]; ok {
		t.Fatal("expected vendor AST to be dropped from the live node map")
	}
	if _, ok := wi.projectNodes[appURI]; !ok {
		t.Fatal("expected application AST to remain")
	}
	if _, ok := wi.ProjectIndex().ResolveMethod(`Symfony\Bundle\FrameworkBundle\Test\WebTestCase`, "assertResponseIsSuccessful"); !ok {
		t.Fatal("expected vendor method symbols to remain after dropping ASTs")
	}
}

func TestUpdateConfigAppliesWorkspaceExcludes(t *testing.T) {
	tmpDir := t.TempDir()
	excludedFile := filepath.Join(tmpDir, "generated", "Generated.php")
	if err := os.MkdirAll(filepath.Dir(excludedFile), 0o755); err != nil {
		t.Fatalf("mkdir generated tree: %v", err)
	}
	if err := os.WriteFile(excludedFile, []byte("<?php\nclass Generated {}\n"), 0o644); err != nil {
		t.Fatalf("write generated file: %v", err)
	}

	wi := New(Config{Associations: []string{"**/*.php"}, MaxSize: 1_000_000})
	folders := []WorkspaceFolder{{URI: pathToURI(tmpDir), Name: "tmp"}}
	wi.SetWorkspaceFolders(folders)
	wi.UpdateConfig(Config{
		Associations: []string{"**/*.php"},
		Exclude:      []string{"**/generated/**"},
		MaxSize:      1_000_000,
	})

	paths := wi.collectWorkspaceFilePaths(folders, wi.gitignores)
	if len(paths) != 0 {
		t.Fatalf("expected updated excludes to omit generated files, got %#v", paths)
	}
}

func TestDiagnosticWorkerCountLeavesInteractiveCapacity(t *testing.T) {
	if got := DiagnosticWorkerCountFor(999); got > 2 {
		t.Fatalf("expected at most two full-analysis workers for small workspaces, got %d", got)
	}
	if got := DiagnosticWorkerCountFor(1_000); got > 3 {
		t.Fatalf("expected at most three full-analysis workers for medium workspaces, got %d", got)
	}
	if got := DiagnosticWorkerCountFor(10_000); got > 4 {
		t.Fatalf("expected at most four full-analysis workers for large workspaces, got %d", got)
	}
}
