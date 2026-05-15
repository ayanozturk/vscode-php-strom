package indexer

import "go-phpcs/ast"

type Range struct {
	Start ast.Position
	End   ast.Position
}

// SymbolKind mirrors the LSP symbol kind values.
type SymbolKind int

const (
	KindFile          SymbolKind = 1
	KindModule        SymbolKind = 2
	KindNamespace     SymbolKind = 3
	KindPackage       SymbolKind = 4
	KindClass         SymbolKind = 5
	KindMethod        SymbolKind = 6
	KindProperty      SymbolKind = 7
	KindField         SymbolKind = 8
	KindConstructor   SymbolKind = 9
	KindEnum          SymbolKind = 10
	KindInterface     SymbolKind = 11
	KindFunction      SymbolKind = 12
	KindVariable      SymbolKind = 13
	KindConstant      SymbolKind = 14
	KindString        SymbolKind = 15
	KindNumber        SymbolKind = 16
	KindBoolean       SymbolKind = 17
	KindArray         SymbolKind = 18
	KindObject        SymbolKind = 19
	KindKey           SymbolKind = 20
	KindNull          SymbolKind = 21
	KindEnumMember    SymbolKind = 22
	KindStruct        SymbolKind = 23
	KindEvent         SymbolKind = 24
	KindOperator      SymbolKind = 25
	KindTypeParameter SymbolKind = 26
)

// Symbol represents a named PHP declaration extracted from source.
type Symbol struct {
	// Identity
	FQN       string // fully-qualified name: \Foo\Bar\Baz
	Name      string // short name without namespace
	Kind      SymbolKind
	Namespace string

	// Location
	URI   string // file:// URI
	Range Range

	// LSP line/character positions (0-based), populated at extraction time.
	StartLine uint32
	StartChar uint32
	EndLine   uint32
	EndChar   uint32

	// Relationships
	Extends    []string // FQNs of parent classes/interfaces
	Implements []string // FQNs of implemented interfaces
	DocComment string

	// For methods/functions: return type and parameter names
	ReturnType string
	Type       string
	Params     []SymbolParam

	// Visibility & modifiers
	IsStatic   bool
	IsAbstract bool
	IsFinal    bool
	IsReadonly bool
	Visibility string // "public" | "protected" | "private"
}

// SymbolParam captures a parameter's name and type for signature help.
type SymbolParam struct {
	Name        string
	Type        string
	HasDefault  bool
	IsVariadic  bool
	IsPassByRef bool
}
