package providers

import (
	"go-phpcs/overrides"
	"testing"
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
		"PSR1.Classes.ClassDeclaration.PascalCase": {
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
