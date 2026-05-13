package parser

// ─── Position ────────────────────────────────────────────────────────────────

// Pos is a byte offset into the source file.
type Pos int

// Range describes a contiguous region of source text [Start, End).
type Range struct {
	Start Pos
	End   Pos
}

// ─── Base node ───────────────────────────────────────────────────────────────

// Node is the interface implemented by every AST node.
type Node interface {
	nodeRange() Range
	nodeErrors() []ParseError
}

// base embeds Range and optional parse errors.
type base struct {
	Pos    Range
	Errors []ParseError
}

func (b base) nodeRange() Range         { return b.Pos }
func (b base) nodeErrors() []ParseError { return b.Errors }

// ParseError records an error encountered during parsing, with the surrounding
// source range so the editor can underline it.
type ParseError struct {
	Message string
	Pos     Range
}

// ─── File ─────────────────────────────────────────────────────────────────────

type File struct {
	base
	Stmts []Stmt
}

// ─── Declarations ─────────────────────────────────────────────────────────────

type Namespace struct {
	base
	Name  string // "" = global namespace
	Stmts []Stmt // nil = bracket-less
}

type UseDeclaration struct {
	base
	Kind  UseKind
	Items []UseItem
}

type UseKind int

const (
	UseClass UseKind = iota
	UseFunction
	UseConst
)

type UseItem struct {
	base
	Name  string
	Alias string // "" = no alias
}

// ─── Class-like ───────────────────────────────────────────────────────────────

type ClassDecl struct {
	base
	DocComment string
	Attributes []Attribute
	Flags      ClassFlags
	Name       string
	Extends    string // FQN or ""
	Implements []string
	Members    []ClassMember
}

type InterfaceDecl struct {
	base
	DocComment string
	Attributes []Attribute
	Name       string
	Extends    []string
	Members    []ClassMember
}

type TraitDecl struct {
	base
	DocComment string
	Attributes []Attribute
	Name       string
	Members    []ClassMember
}

type EnumDecl struct {
	base
	DocComment string
	Attributes []Attribute
	Name       string
	BackedType string // "int" | "string" | ""
	Implements []string
	Members    []ClassMember
}

type ClassFlags int

const (
	FlagAbstract ClassFlags = 1 << iota
	FlagFinal
	FlagReadonly
	FlagAnonymous
)

// ClassMember is the interface for everything that can appear in a class body.
type ClassMember interface {
	Node
	classMember()
}

type MethodDecl struct {
	base
	DocComment string
	Attributes []Attribute
	Flags      MemberFlags
	Name       string
	Params     []Param
	ReturnType TypeNode
	Stmts      []Stmt // nil = abstract/interface
}

type PropertyDecl struct {
	base
	DocComment string
	Attributes []Attribute
	Flags      MemberFlags
	Type       TypeNode
	Items      []PropertyItem
}

type PropertyItem struct {
	base
	Name    string
	Default Expr // nil = no default
	Hooks   []PropertyHook
}

// PropertyHook represents PHP 8.4 property hooks: get/set { ... }
type PropertyHook struct {
	base
	Kind  string // "get" | "set"
	Short bool   // short = =>  expr  form
	Param *Param // set hook only: implicit $value or explicit (?string $val)
	Body  []Stmt
}

type ClassConstDecl struct {
	base
	DocComment string
	Attributes []Attribute
	Flags      MemberFlags
	Type       TypeNode
	Items      []ConstItem
}

type TraitUse struct {
	base
	Traits      []string
	Adaptations []TraitAdaptation
}

type TraitAdaptation interface {
	Node
	traitAdaptation()
}

type TraitAlias struct {
	base
	Trait  string // "" = any
	Method string
	Alias  string
	Flags  MemberFlags
}

type TraitPrecedence struct {
	base
	Trait     string
	Method    string
	InsteadOf []string
}

type EnumCase struct {
	base
	DocComment string
	Attributes []Attribute
	Name       string
	Value      Expr // nil = pure enum
}

func (m *MethodDecl) classMember()     {}
func (m *PropertyDecl) classMember()   {}
func (m *ClassConstDecl) classMember() {}
func (m *TraitUse) classMember()       {}
func (m *EnumCase) classMember()       {}

func (m *TraitAlias) traitAdaptation()      {}
func (m *TraitPrecedence) traitAdaptation() {}

type MemberFlags int

const (
	FlagPublic MemberFlags = 1 << iota
	FlagProtected
	FlagPrivate
	FlagStatic
	FlagAbstractMember
	FlagFinalMember
	FlagReadonlyMember
	FlagPublicSet // PHP 8.4 asymmetric visibility: public(set)
	FlagProtectedSet
	FlagPrivateSet
)

// ─── Function ─────────────────────────────────────────────────────────────────

type FunctionDecl struct {
	base
	DocComment string
	Attributes []Attribute
	ByRef      bool
	Name       string
	Params     []Param
	ReturnType TypeNode
	Stmts      []Stmt
}

// ─── Parameter ────────────────────────────────────────────────────────────────

type Param struct {
	base
	Attributes []Attribute
	Flags      MemberFlags // constructor promotion
	Type       TypeNode
	ByRef      bool
	Variadic   bool
	Name       string // without $
	Default    Expr   // nil = required
}

// ─── Constants ────────────────────────────────────────────────────────────────

type ConstDecl struct {
	base
	Items []ConstItem
}

type ConstItem struct {
	base
	Name  string
	Value Expr
}

// ─── Attributes (PHP 8.0) ─────────────────────────────────────────────────────

type Attribute struct {
	base
	Name string
	Args []Arg
}

// ─── Statements ───────────────────────────────────────────────────────────────

type Stmt interface {
	Node
	stmtNode()
}

type ExprStmt struct {
	base
	Expr Expr
}

type ReturnStmt struct {
	base
	Expr Expr // nil = void return
}

type EchoStmt struct {
	base
	Exprs []Expr
}

type IfStmt struct {
	base
	Cond    Expr
	Body    []Stmt
	Elseifs []ElseifClause
	Else    []Stmt
}

type ElseifClause struct {
	base
	Cond Expr
	Body []Stmt
}

type WhileStmt struct {
	base
	Cond Expr
	Body []Stmt
}

type DoWhileStmt struct {
	base
	Body []Stmt
	Cond Expr
}

type ForStmt struct {
	base
	Init []Expr
	Cond []Expr
	Loop []Expr
	Body []Stmt
}

type ForeachStmt struct {
	base
	Expr  Expr
	Key   Expr // nil = no key
	Value Expr
	ByRef bool
	Body  []Stmt
}

type SwitchStmt struct {
	base
	Cond  Expr
	Cases []SwitchCase
}

type SwitchCase struct {
	base
	Value Expr // nil = default
	Stmts []Stmt
}

type BreakStmt struct {
	base
	Num Expr // nil = no level
}

type ContinueStmt struct {
	base
	Num Expr
}

type GotoStmt struct {
	base
	Label string
}

type LabelStmt struct {
	base
	Name string
}

type TryCatchStmt struct {
	base
	Body    []Stmt
	Catches []CatchClause
	Finally []Stmt
}

type CatchClause struct {
	base
	Types []string // catch (A|B $e)
	Var   string   // "$e" or "" (anonymous catch)
	Body  []Stmt
}

type ThrowStmt struct {
	base
	Expr Expr
}

type GlobalStmt struct {
	base
	Vars []Expr
}

type StaticStmt struct {
	base
	Vars []StaticVar
}

type StaticVar struct {
	base
	Name    string
	Default Expr
}

type UnsetStmt struct {
	base
	Vars []Expr
}

type DeclareStmt struct {
	base
	Directives []DeclareDirective
	Body       []Stmt
}

type DeclareDirective struct {
	base
	Name  string
	Value Expr
}

type BlockStmt struct {
	base
	Stmts []Stmt
}

type NopStmt struct{ base }

// Embed class/function declarations as statements
type ClassDeclStmt struct {
	base
	Decl *ClassDecl
}

type InterfaceDeclStmt struct {
	base
	Decl *InterfaceDecl
}

type TraitDeclStmt struct {
	base
	Decl *TraitDecl
}

type EnumDeclStmt struct {
	base
	Decl *EnumDecl
}

type FunctionDeclStmt struct {
	base
	Decl *FunctionDecl
}

type NamespaceDeclStmt struct {
	base
	Decl *Namespace
}

type UseDeclStmt struct {
	base
	Decl *UseDeclaration
}

type ConstDeclStmt struct {
	base
	Decl *ConstDecl
}

// Implement stmtNode() for all statement types
func (s *ExprStmt) stmtNode()          {}
func (s *ReturnStmt) stmtNode()        {}
func (s *EchoStmt) stmtNode()          {}
func (s *IfStmt) stmtNode()            {}
func (s *WhileStmt) stmtNode()         {}
func (s *DoWhileStmt) stmtNode()       {}
func (s *ForStmt) stmtNode()           {}
func (s *ForeachStmt) stmtNode()       {}
func (s *SwitchStmt) stmtNode()        {}
func (s *BreakStmt) stmtNode()         {}
func (s *ContinueStmt) stmtNode()      {}
func (s *GotoStmt) stmtNode()          {}
func (s *LabelStmt) stmtNode()         {}
func (s *TryCatchStmt) stmtNode()      {}
func (s *ThrowStmt) stmtNode()         {}
func (s *GlobalStmt) stmtNode()        {}
func (s *StaticStmt) stmtNode()        {}
func (s *UnsetStmt) stmtNode()         {}
func (s *DeclareStmt) stmtNode()       {}
func (s *BlockStmt) stmtNode()         {}
func (s *NopStmt) stmtNode()           {}
func (s *ClassDeclStmt) stmtNode()     {}
func (s *InterfaceDeclStmt) stmtNode() {}
func (s *TraitDeclStmt) stmtNode()     {}
func (s *EnumDeclStmt) stmtNode()      {}
func (s *FunctionDeclStmt) stmtNode()  {}
func (s *NamespaceDeclStmt) stmtNode() {}
func (s *UseDeclStmt) stmtNode()       {}
func (s *ConstDeclStmt) stmtNode()     {}

// ─── Expressions ──────────────────────────────────────────────────────────────

type Expr interface {
	Node
	exprNode()
}

type IntLit struct {
	base
	Value string
}

type FloatLit struct {
	base
	Value string
}

type StringLit struct {
	base
	Value string
}

type NullLit struct{ base }
type TrueLit struct{ base }
type FalseLit struct{ base }

type VarExpr struct {
	base
	Name string // without $
}

type ConstExpr struct {
	base
	Name string // FQN
}

type AssignExpr struct {
	base
	Op    string
	Left  Expr
	Right Expr
}

type BinaryExpr struct {
	base
	Op    string
	Left  Expr
	Right Expr
}

type UnaryExpr struct {
	base
	Op      string
	Operand Expr
	Postfix bool // true = $i++
}

type CastExpr struct {
	base
	Kind    string
	Operand Expr
}

type TernaryExpr struct {
	base
	Cond Expr
	Then Expr // nil = Elvis ?:
	Else Expr
}

type NullCoalesceExpr struct {
	base
	Left  Expr
	Right Expr
}

type PropFetchExpr struct {
	base
	Object   Expr
	Property string
	Nullsafe bool
}

type StaticPropFetchExpr struct {
	base
	Class    Expr
	Property string // with $
}

type MethodCallExpr struct {
	base
	Object   Expr
	Method   string
	Args     []Arg
	Nullsafe bool
}

type StaticCallExpr struct {
	base
	Class  Expr
	Method string
	Args   []Arg
}

type FuncCallExpr struct {
	base
	Func Expr
	Args []Arg
}

type NewExpr struct {
	base
	Class Expr
	Args  []Arg
}

type CloneExpr struct {
	base
	Expr Expr
}

type ArrayExpr struct {
	base
	Items []ArrayItem
}

type ArrayItem struct {
	base
	Key    Expr // nil = no key
	Value  Expr
	ByRef  bool
	Unpack bool // ...
}

type ArrayAccessExpr struct {
	base
	Array Expr
	Index Expr // nil = []
}

type ClosureExpr struct {
	base
	Static     bool
	ByRef      bool
	Params     []Param
	Uses       []ClosureUse
	ReturnType TypeNode
	Stmts      []Stmt
}

type ClosureUse struct {
	base
	ByRef bool
	Name  string
}

type ArrowFuncExpr struct {
	base
	Static     bool
	ByRef      bool
	Params     []Param
	ReturnType TypeNode
	Expr       Expr
}

// Match expression (PHP 8)
type MatchExpr struct {
	base
	Cond Expr
	Arms []MatchArm
}

type MatchArm struct {
	base
	Conds []Expr // empty = default
	Value Expr
}

// Throw as expression (PHP 8)
type ThrowExpr struct {
	base
	Expr Expr
}

// First-class callable f(...) (PHP 8.1)
type FirstClassCallableExpr struct {
	base
	Callable Expr
}

// Named argument
type Arg struct {
	base
	Name   string // "" = positional
	ByRef  bool
	Unpack bool
	Value  Expr
}

type InstanceofExpr struct {
	base
	Expr  Expr
	Class string
}

type IssetExpr struct {
	base
	Vars []Expr
}

type EmptyExpr struct {
	base
	Expr Expr
}

type EvalExpr struct {
	base
	Expr Expr
}

type IncludeExpr struct {
	base
	Kind string // "include" "include_once" "require" "require_once"
	Expr Expr
}

type PrintExpr struct {
	base
	Expr Expr
}

type ListExpr struct {
	base
	Items []ArrayItem
}

type ShellExecExpr struct {
	base
	Parts []Expr
}

type ClassConstFetchExpr struct {
	base
	Class Expr
	Name  string
}

type InterpolatedStringExpr struct {
	base
	Parts []Expr
}

// Implement exprNode() for all expression types
func (e *IntLit) exprNode()                 {}
func (e *FloatLit) exprNode()               {}
func (e *StringLit) exprNode()              {}
func (e *NullLit) exprNode()                {}
func (e *TrueLit) exprNode()                {}
func (e *FalseLit) exprNode()               {}
func (e *VarExpr) exprNode()                {}
func (e *ConstExpr) exprNode()              {}
func (e *AssignExpr) exprNode()             {}
func (e *BinaryExpr) exprNode()             {}
func (e *UnaryExpr) exprNode()              {}
func (e *CastExpr) exprNode()               {}
func (e *TernaryExpr) exprNode()            {}
func (e *NullCoalesceExpr) exprNode()       {}
func (e *PropFetchExpr) exprNode()          {}
func (e *StaticPropFetchExpr) exprNode()    {}
func (e *MethodCallExpr) exprNode()         {}
func (e *StaticCallExpr) exprNode()         {}
func (e *FuncCallExpr) exprNode()           {}
func (e *NewExpr) exprNode()                {}
func (e *CloneExpr) exprNode()              {}
func (e *ArrayExpr) exprNode()              {}
func (e *ArrayAccessExpr) exprNode()        {}
func (e *ClosureExpr) exprNode()            {}
func (e *ArrowFuncExpr) exprNode()          {}
func (e *MatchExpr) exprNode()              {}
func (e *ThrowExpr) exprNode()              {}
func (e *FirstClassCallableExpr) exprNode() {}
func (e *InstanceofExpr) exprNode()         {}
func (e *IssetExpr) exprNode()              {}
func (e *EmptyExpr) exprNode()              {}
func (e *EvalExpr) exprNode()               {}
func (e *IncludeExpr) exprNode()            {}
func (e *PrintExpr) exprNode()              {}
func (e *ListExpr) exprNode()               {}
func (e *ShellExecExpr) exprNode()          {}
func (e *ClassConstFetchExpr) exprNode()    {}
func (e *InterpolatedStringExpr) exprNode() {}

// ─── Type nodes ───────────────────────────────────────────────────────────────

type TypeNode interface {
	Node
	typeNode()
}

// NamedType: int, string, Foo\Bar
type NamedType struct {
	base
	Name string
}

// NullableType: ?Foo
type NullableType struct {
	base
	Inner TypeNode
}

// UnionType: A|B|C (PHP 8.0)
type UnionType struct {
	base
	Types []TypeNode
}

// IntersectionType: A&B (PHP 8.1)
type IntersectionType struct {
	base
	Types []TypeNode
}

func (t *NamedType) typeNode()        {}
func (t *NullableType) typeNode()     {}
func (t *UnionType) typeNode()        {}
func (t *IntersectionType) typeNode() {}
