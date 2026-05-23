package providers

import (
	"go-phpcs/overrides"
	"testing"

	"github.com/ayanozturk/vscode-php-strom/indexer"
)

func TestDiagnosticsProvider_ParseError(t *testing.T) {
	p := &DiagnosticsProvider{}
	diags := p.Analyse("file:///test.php", `<?php
class Foo {
	public function bar() {
		// missing closing brace
`)
	// Parser errors should surface as diagnostics
	if len(diags) == 0 {
		t.Fatal("expected at least one diagnostic for incomplete PHP, got none")
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
