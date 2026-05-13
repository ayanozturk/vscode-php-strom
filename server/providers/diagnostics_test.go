package providers

import (
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
