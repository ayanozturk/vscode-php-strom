package providers

import (
	"testing"

	"github.com/ayanozturk/vscode-php-strom/indexer"
)

func TestFindCalleeAndParam_SimpleFunction(t *testing.T) {
	src := `<?php foo(`
	callee, param := findCalleeAndParam(src)
	if callee != "foo" {
		t.Errorf("expected callee=foo, got %q", callee)
	}
	if param != 0 {
		t.Errorf("expected param=0, got %d", param)
	}
}

func TestFindCalleeAndParam_SecondArg(t *testing.T) {
	src := `<?php foo($a, `
	callee, param := findCalleeAndParam(src)
	if callee != "foo" {
		t.Errorf("expected callee=foo, got %q", callee)
	}
	if param != 1 {
		t.Errorf("expected param=1, got %d", param)
	}
}

func TestFindCalleeAndParam_NestedCall(t *testing.T) {
	// Inside the outer call, cursor is on second arg.
	src := `<?php outer(inner(), `
	callee, param := findCalleeAndParam(src)
	if callee != "outer" {
		t.Errorf("expected callee=outer, got %q", callee)
	}
	if param != 1 {
		t.Errorf("expected param=1, got %d", param)
	}
}

func TestFindCalleeAndParam_NewExpression(t *testing.T) {
	src := `<?php new Foo(`
	callee, param := findCalleeAndParam(src)
	if callee != "__construct:Foo" {
		t.Errorf("expected callee=__construct:Foo, got %q", callee)
	}
	if param != 0 {
		t.Errorf("expected param=0, got %d", param)
	}
}

func TestFindCalleeAndParam_MethodCall(t *testing.T) {
	src := `<?php $obj->doSomething($x, `
	callee, param := findCalleeAndParam(src)
	if callee != "doSomething" {
		t.Errorf("expected callee=doSomething, got %q", callee)
	}
	if param != 1 {
		t.Errorf("expected param=1, got %d", param)
	}
}

func TestFindCalleeAndParam_Empty(t *testing.T) {
	callee, _ := findCalleeAndParam(`<?php `)
	if callee != "" {
		t.Errorf("expected empty callee, got %q", callee)
	}
}

func TestBuildSignatureHelp_NoParams(t *testing.T) {
	sym := &indexer.Symbol{Name: "foo", ReturnType: "void"}
	sh := buildSignatureHelp(sym, 0)
	if sh == nil {
		t.Fatal("expected non-nil SignatureHelp")
	}
	if sh.Signatures[0].Label != "foo(): void" {
		t.Errorf("unexpected label: %q", sh.Signatures[0].Label)
	}
	if sh.ActiveParameter != nil {
		t.Error("expected nil ActiveParameter for no-param function")
	}
}

func TestBuildSignatureHelp_WithParams(t *testing.T) {
	sym := &indexer.Symbol{
		Name:       "bar",
		ReturnType: "bool",
		Params: []indexer.SymbolParam{
			{Name: "a", Type: "string"},
			{Name: "b", Type: "int", HasDefault: true},
		},
	}
	sh := buildSignatureHelp(sym, 1)
	if sh == nil {
		t.Fatal("expected non-nil SignatureHelp")
	}
	sig := sh.Signatures[0]
	if len(sig.Parameters) != 2 {
		t.Fatalf("expected 2 parameters, got %d", len(sig.Parameters))
	}
	if sh.ActiveParameter == nil || *sh.ActiveParameter != 1 {
		t.Errorf("expected ActiveParameter=1")
	}
	if sig.ActiveParameter == nil || *sig.ActiveParameter != 1 {
		t.Errorf("expected sig.ActiveParameter=1")
	}
}

func TestBuildSignatureHelp_VariadicClamp(t *testing.T) {
	sym := &indexer.Symbol{
		Name: "variadic",
		Params: []indexer.SymbolParam{
			{Name: "a", Type: "string"},
			{Name: "rest", Type: "int", IsVariadic: true},
		},
	}
	// Active param index beyond the list — should clamp to last variadic param.
	sh := buildSignatureHelp(sym, 5)
	if sh == nil {
		t.Fatal("expected non-nil SignatureHelp")
	}
	if sh.ActiveParameter == nil || *sh.ActiveParameter != 1 {
		t.Errorf("expected ActiveParameter clamped to 1, got %v", sh.ActiveParameter)
	}
}

func TestRenderParam(t *testing.T) {
	tests := []struct {
		p    indexer.SymbolParam
		want string
	}{
		{indexer.SymbolParam{Name: "x", Type: "string"}, "string $x"},
		{indexer.SymbolParam{Name: "x"}, "$x"},
		{indexer.SymbolParam{Name: "x", Type: "int", HasDefault: true}, "int $x = <default>"},
		{indexer.SymbolParam{Name: "x", Type: "array", IsVariadic: true}, "array ...$x"},
		{indexer.SymbolParam{Name: "x", IsPassByRef: true}, "&$x"},
	}
	for _, tt := range tests {
		got := renderParam(tt.p)
		if got != tt.want {
			t.Errorf("renderParam(%+v) = %q, want %q", tt.p, got, tt.want)
		}
	}
}
