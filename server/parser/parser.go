package parser

import (
	"fmt"
	"strings"
)

// Parser is a recursive-descent, error-tolerant PHP 8.4 parser.
// On syntax errors it records a ParseError and tries to synchronise at the
// next statement boundary so that as much of the file as possible is parsed.
type Parser struct {
	lex    *Lexer
	cur    Token // current (already consumed) token
	peek   Token // one-token lookahead
	errors []ParseError
}

// Parse is the top-level entry point: parse src and return the File AST.
func Parse(src string) *File {
	p := &Parser{lex: NewLexer(src)}
	p.advance() // prime cur
	p.advance() // prime peek

	start := p.cur.StartPos
	stmts := p.parseStmtList(T_EOF)
	f := &File{
		base:  base{Pos: Range{Start: Pos(start), End: Pos(p.cur.EndPos)}, Errors: p.errors},
		Stmts: stmts,
	}
	return f
}

// ─── Token helpers ────────────────────────────────────────────────────────────

func (p *Parser) advance() Token {
	prev := p.cur
	p.cur = p.peek
	p.peek = p.lex.Next()
	return prev
}

func (p *Parser) peek1() Token { return p.peek }

func (p *Parser) is(tt TokenType) bool { return p.cur.Type == tt }

func (p *Parser) eat(tt TokenType) (Token, bool) {
	if p.cur.Type == tt {
		return p.advance(), true
	}
	return p.cur, false
}

func (p *Parser) expect(tt TokenType) Token {
	if p.cur.Type == tt {
		return p.advance()
	}
	p.errorf("expected %d, got %q", tt, p.cur.Value)
	return p.cur
}

func (p *Parser) errorf(format string, args ...interface{}) {
	p.errors = append(p.errors, ParseError{
		Message: fmt.Sprintf(format, args...),
		Pos:     Range{Start: Pos(p.cur.StartPos), End: Pos(p.cur.EndPos)},
	})
}

// sync advances until we find a token that can start a new statement or EOF,
// so parsing can continue after an error.
func (p *Parser) sync() {
	for {
		switch p.cur.Type {
		case T_EOF, T_SEMICOLON, T_RBRACE:
			return
		case T_CLASS, T_INTERFACE, T_TRAIT, T_ENUM,
			T_FUNCTION, T_NAMESPACE, T_USE, T_CONST,
			T_IF, T_WHILE, T_FOR, T_FOREACH, T_SWITCH,
			T_RETURN, T_ECHO, T_TRY, T_THROW:
			return
		}
		p.advance()
	}
}

func (p *Parser) startPos() Pos { return Pos(p.cur.StartPos) }
func (p *Parser) endPos() Pos   { return Pos(p.cur.EndPos) }

func (p *Parser) rng(start Pos) Range {
	return Range{Start: start, End: Pos(p.cur.StartPos)}
}

// ─── Statement list ───────────────────────────────────────────────────────────

func (p *Parser) parseStmtList(until TokenType) []Stmt {
	var stmts []Stmt
	for !p.is(until) && !p.is(T_EOF) {
		if p.is(T_CLOSE_TAG) || p.is(T_INLINE_HTML) || p.is(T_OPEN_TAG) || p.is(T_OPEN_TAG_WITH_ECHO) {
			p.advance()
			continue
		}
		s := p.parseStmt()
		if s != nil {
			stmts = append(stmts, s)
		}
	}
	return stmts
}

// ─── Statements ───────────────────────────────────────────────────────────────

func (p *Parser) parseStmt() Stmt {
	start := p.startPos()

	// Collect leading doc comment for declarations
	var docComment string
	if p.is(T_DOC_COMMENT) {
		docComment = p.cur.Value
		p.advance()
	}
	_ = docComment // passed into individual parse functions

	// Collect attributes #[...]
	var attrs []Attribute
	for p.is(T_ATTRIBUTE) {
		attrs = append(attrs, p.parseAttribute())
	}
	_ = attrs

	switch p.cur.Type {
	case T_NAMESPACE:
		return p.parseNamespace(start, docComment)
	case T_USE:
		return p.parseUse(start)
	case T_CONST:
		return p.parseConst(start)
	case T_ABSTRACT, T_FINAL, T_READONLY:
		return p.parseClassLike(start, docComment, attrs)
	case T_CLASS:
		return p.parseClassDecl(start, docComment, attrs, 0)
	case T_INTERFACE:
		return p.parseInterface(start, docComment, attrs)
	case T_TRAIT:
		return p.parseTrait(start, docComment, attrs)
	case T_ENUM:
		return p.parseEnum(start, docComment, attrs)
	case T_FUNCTION:
		return p.parseFunctionDecl(start, docComment, attrs)
	case T_IF:
		return p.parseIf(start)
	case T_WHILE:
		return p.parseWhile(start)
	case T_DO:
		return p.parseDoWhile(start)
	case T_FOR:
		return p.parseFor(start)
	case T_FOREACH:
		return p.parseForeach(start)
	case T_SWITCH:
		return p.parseSwitch(start)
	case T_BREAK:
		return p.parseBreak(start)
	case T_CONTINUE:
		return p.parseContinue(start)
	case T_RETURN:
		return p.parseReturn(start)
	case T_ECHO:
		return p.parseEcho(start)
	case T_TRY:
		return p.parseTryCatch(start)
	case T_THROW:
		return p.parseThrowStmt(start)
	case T_GLOBAL:
		return p.parseGlobal(start)
	case T_STATIC:
		// could be static function, static var, or static method modifier handled above
		if p.peek1().Type == T_VARIABLE {
			return p.parseStatic(start)
		}
		// fall through to expression
	case T_UNSET:
		return p.parseUnset(start)
	case T_DECLARE:
		return p.parseDeclare(start)
	case T_GOTO:
		p.advance()
		name := p.cur.Value
		p.advance()
		p.eat(T_SEMICOLON)
		return &GotoStmt{base: base{Pos: p.rng(start)}, Label: name}
	case T_LBRACE:
		p.advance()
		stmts := p.parseStmtList(T_RBRACE)
		p.expect(T_RBRACE)
		return &BlockStmt{base: base{Pos: p.rng(start)}, Stmts: stmts}
	case T_SEMICOLON:
		p.advance()
		return &NopStmt{base: base{Pos: p.rng(start)}}
	default:
		// Check label: IDENT :
		if p.is(T_STRING) && p.peek1().Type == T_COLON {
			name := p.cur.Value
			p.advance()
			p.advance()
			return &LabelStmt{base: base{Pos: p.rng(start)}, Name: name}
		}
	}

	// Expression statement
	e := p.parseExpr()
	if e == nil {
		p.errorf("unexpected token %q", p.cur.Value)
		p.advance()
		p.sync()
		return nil
	}
	p.eat(T_SEMICOLON)
	return &ExprStmt{base: base{Pos: p.rng(start)}, Expr: e}
}

func (p *Parser) parseNamespace(start Pos, _ string) Stmt {
	p.advance() // namespace
	var name string
	if p.is(T_STRING) || p.is(T_BACKSLASH) {
		name = p.parseName()
	}
	if p.is(T_LBRACE) {
		p.advance()
		stmts := p.parseStmtList(T_RBRACE)
		p.expect(T_RBRACE)
		return &NamespaceDeclStmt{
			base: base{Pos: p.rng(start)},
			Decl: &Namespace{base: base{Pos: p.rng(start)}, Name: name, Stmts: stmts},
		}
	}
	p.eat(T_SEMICOLON)
	return &NamespaceDeclStmt{
		base: base{Pos: p.rng(start)},
		Decl: &Namespace{base: base{Pos: p.rng(start)}, Name: name},
	}
}

func (p *Parser) parseUse(start Pos) Stmt {
	p.advance() // use
	kind := UseClass
	if p.is(T_FUNCTION) {
		kind = UseFunction
		p.advance()
	} else if p.is(T_CONST) {
		kind = UseConst
		p.advance()
	}
	items := p.parseUseItems(kind)
	p.eat(T_SEMICOLON)
	return &UseDeclStmt{
		base: base{Pos: p.rng(start)},
		Decl: &UseDeclaration{Kind: kind, Items: items},
	}
}

func (p *Parser) parseUseItems(kind UseKind) []UseItem {
	var items []UseItem
	for {
		prefix := ""
		if p.is(T_STRING) {
			prefix = p.parseName()
		}
		if p.is(T_BACKSLASH) && p.peek1().Type == T_LBRACE {
			p.advance()
			p.advance() // \{
			for !p.is(T_RBRACE) && !p.is(T_EOF) {
				subKind := kind
				if p.is(T_FUNCTION) {
					subKind = UseFunction
					p.advance()
				} else if p.is(T_CONST) {
					subKind = UseConst
					p.advance()
				}
				_ = subKind
				name := prefix + `\` + p.parseName()
				alias := ""
				if p.is(T_AS) {
					p.advance()
					alias = p.cur.Value
					p.advance()
				}
				items = append(items, UseItem{Name: name, Alias: alias})
				if !p.is(T_COMMA) {
					break
				}
				p.advance()
			}
			p.expect(T_RBRACE)
		} else {
			alias := ""
			if p.is(T_AS) {
				p.advance()
				alias = p.cur.Value
				p.advance()
			}
			items = append(items, UseItem{Name: prefix, Alias: alias})
		}
		if !p.is(T_COMMA) {
			break
		}
		p.advance()
	}
	return items
}

func (p *Parser) parseConst(start Pos) Stmt {
	p.advance() // const
	var items []ConstItem
	for {
		iStart := p.startPos()
		name := p.cur.Value
		p.advance()
		p.expect(T_ASSIGN)
		val := p.parseExpr()
		items = append(items, ConstItem{base: base{Pos: p.rng(iStart)}, Name: name, Value: val})
		if !p.is(T_COMMA) {
			break
		}
		p.advance()
	}
	p.eat(T_SEMICOLON)
	return &ConstDeclStmt{
		base: base{Pos: p.rng(start)},
		Decl: &ConstDecl{base: base{Pos: p.rng(start)}, Items: items},
	}
}

func (p *Parser) parseClassLike(start Pos, doc string, attrs []Attribute) Stmt {
	var flags ClassFlags
	for p.is(T_ABSTRACT) || p.is(T_FINAL) || p.is(T_READONLY) {
		switch p.cur.Type {
		case T_ABSTRACT:
			flags |= FlagAbstract
		case T_FINAL:
			flags |= FlagFinal
		case T_READONLY:
			flags |= FlagReadonly
		}
		p.advance()
	}
	if p.is(T_CLASS) {
		return p.parseClassDecl(start, doc, attrs, flags)
	}
	p.errorf("expected class after modifiers")
	return nil
}

func (p *Parser) parseClassDecl(start Pos, doc string, attrs []Attribute, flags ClassFlags) Stmt {
	p.advance() // class
	name := p.cur.Value
	p.advance()
	var extends string
	if p.is(T_EXTENDS) {
		p.advance()
		extends = p.parseName()
	}
	var implements []string
	if p.is(T_IMPLEMENTS) {
		p.advance()
		implements = p.parseNameList()
	}
	members := p.parseClassBody()
	return &ClassDeclStmt{
		base: base{Pos: p.rng(start)},
		Decl: &ClassDecl{
			base: base{Pos: p.rng(start)}, DocComment: doc, Attributes: attrs,
			Flags: flags, Name: name, Extends: extends, Implements: implements, Members: members,
		},
	}
}

func (p *Parser) parseInterface(start Pos, doc string, attrs []Attribute) Stmt {
	p.advance() // interface
	name := p.cur.Value
	p.advance()
	var extends []string
	if p.is(T_EXTENDS) {
		p.advance()
		extends = p.parseNameList()
	}
	members := p.parseClassBody()
	return &InterfaceDeclStmt{
		base: base{Pos: p.rng(start)},
		Decl: &InterfaceDecl{
			base: base{Pos: p.rng(start)}, DocComment: doc, Attributes: attrs,
			Name: name, Extends: extends, Members: members,
		},
	}
}

func (p *Parser) parseTrait(start Pos, doc string, attrs []Attribute) Stmt {
	p.advance() // trait
	name := p.cur.Value
	p.advance()
	members := p.parseClassBody()
	return &TraitDeclStmt{
		base: base{Pos: p.rng(start)},
		Decl: &TraitDecl{
			base: base{Pos: p.rng(start)}, DocComment: doc, Attributes: attrs,
			Name: name, Members: members,
		},
	}
}

func (p *Parser) parseEnum(start Pos, doc string, attrs []Attribute) Stmt {
	p.advance() // enum
	name := p.cur.Value
	p.advance()
	var backedType string
	if p.is(T_COLON) {
		p.advance()
		backedType = p.cur.Value
		p.advance()
	}
	var implements []string
	if p.is(T_IMPLEMENTS) {
		p.advance()
		implements = p.parseNameList()
	}
	members := p.parseClassBody()
	return &EnumDeclStmt{
		base: base{Pos: p.rng(start)},
		Decl: &EnumDecl{
			base: base{Pos: p.rng(start)}, DocComment: doc, Attributes: attrs,
			Name: name, BackedType: backedType, Implements: implements, Members: members,
		},
	}
}

func (p *Parser) parseClassBody() []ClassMember {
	p.expect(T_LBRACE)
	var members []ClassMember
	for !p.is(T_RBRACE) && !p.is(T_EOF) {
		m := p.parseClassMember()
		if m != nil {
			members = append(members, m)
		}
	}
	p.expect(T_RBRACE)
	return members
}

func (p *Parser) parseClassMember() ClassMember {
	// doc comment
	var doc string
	if p.is(T_DOC_COMMENT) {
		doc = p.cur.Value
		p.advance()
	}
	// attributes
	var attrs []Attribute
	for p.is(T_ATTRIBUTE) {
		attrs = append(attrs, p.parseAttribute())
	}

	if p.is(T_USE) {
		return p.parseTraitUse()
	}
	if p.is(T_CASE) {
		return p.parseEnumCase(doc, attrs)
	}
	if p.is(T_CONST) {
		return p.parseClassConst(doc, attrs)
	}

	// collect modifiers
	var flags MemberFlags
	for {
		switch p.cur.Type {
		case T_PUBLIC:
			flags |= FlagPublic
			p.advance()
			if p.is(T_LPAREN) {
				// asymmetric visibility: public(set)
				p.advance()
				if p.is(T_STATIC) {
					// public(set) — just ignore
				}
				// parse set keyword
				p.advance() // set
				p.expect(T_RPAREN)
				flags |= FlagPublicSet
			}
			continue
		case T_PROTECTED:
			flags |= FlagProtected
			p.advance()
			continue
		case T_PRIVATE:
			flags |= FlagPrivate
			p.advance()
			continue
		case T_STATIC:
			flags |= FlagStatic
			p.advance()
			continue
		case T_ABSTRACT:
			flags |= FlagAbstractMember
			p.advance()
			continue
		case T_FINAL:
			flags |= FlagFinalMember
			p.advance()
			continue
		case T_READONLY:
			flags |= FlagReadonlyMember
			p.advance()
			continue
		}
		break
	}

	// function declaration
	if p.is(T_FUNCTION) {
		return p.parseMethod(doc, attrs, flags)
	}

	// property declaration
	typeNode := p.tryParseTypeNode()
	if p.is(T_VARIABLE) {
		return p.parseProperty(doc, attrs, flags, typeNode)
	}

	// unknown — skip
	p.errorf("unexpected %q in class body", p.cur.Value)
	p.advance()
	return nil
}

func (p *Parser) parseMethod(doc string, attrs []Attribute, flags MemberFlags) ClassMember {
	start := p.startPos()
	p.advance() // function
	byRef := false
	if p.is(T_AMPERSAND) {
		byRef = true
		p.advance()
	}
	_ = byRef
	name := p.cur.Value
	p.advance()
	params := p.parseParamList()
	var returnType TypeNode
	if p.is(T_COLON) {
		p.advance()
		returnType = p.parseTypeNode()
	}
	var stmts []Stmt
	if p.is(T_LBRACE) {
		p.advance()
		stmts = p.parseStmtList(T_RBRACE)
		p.expect(T_RBRACE)
	} else {
		p.eat(T_SEMICOLON)
	}
	return &MethodDecl{
		base: base{Pos: p.rng(start)}, DocComment: doc, Attributes: attrs,
		Flags: flags, Name: name, Params: params, ReturnType: returnType, Stmts: stmts,
	}
}

func (p *Parser) parseProperty(doc string, attrs []Attribute, flags MemberFlags, typeNode TypeNode) ClassMember {
	start := p.startPos()
	var items []PropertyItem
	for p.is(T_VARIABLE) {
		iStart := p.startPos()
		name := p.cur.Value[1:] // strip $
		p.advance()
		var def Expr
		if p.is(T_ASSIGN) {
			p.advance()
			def = p.parseExpr()
		}
		var hooks []PropertyHook
		if p.is(T_LBRACE) {
			hooks = p.parsePropertyHooks()
		}
		items = append(items, PropertyItem{
			base: base{Pos: p.rng(iStart)}, Name: name, Default: def, Hooks: hooks,
		})
		if !p.is(T_COMMA) {
			break
		}
		p.advance()
	}
	p.eat(T_SEMICOLON)
	return &PropertyDecl{
		base: base{Pos: p.rng(start)}, DocComment: doc, Attributes: attrs,
		Flags: flags, Type: typeNode, Items: items,
	}
}

func (p *Parser) parsePropertyHooks() []PropertyHook {
	p.advance() // {
	var hooks []PropertyHook
	for !p.is(T_RBRACE) && !p.is(T_EOF) {
		hStart := p.startPos()
		var flags MemberFlags
		if p.is(T_FINAL) {
			flags |= FlagFinalMember
			p.advance()
		}
		kind := p.cur.Value // "get" or "set"
		p.advance()
		var param *Param
		if kind == "set" && p.is(T_LPAREN) {
			p.advance()
			pr := p.parseParam()
			param = &pr
			p.expect(T_RPAREN)
		}
		_ = flags
		if p.is(T_DOUBLE_ARROW) {
			p.advance()
			expr := p.parseExpr()
			p.eat(T_SEMICOLON)
			hooks = append(hooks, PropertyHook{
				base: base{Pos: p.rng(hStart)}, Kind: kind, Short: true, Param: param,
				Body: []Stmt{&ReturnStmt{base: base{}, Expr: expr}},
			})
		} else if p.is(T_LBRACE) {
			p.advance()
			stmts := p.parseStmtList(T_RBRACE)
			p.expect(T_RBRACE)
			hooks = append(hooks, PropertyHook{
				base: base{Pos: p.rng(hStart)}, Kind: kind, Param: param, Body: stmts,
			})
		} else if p.is(T_SEMICOLON) {
			p.advance()
		}
	}
	p.expect(T_RBRACE)
	return hooks
}

func (p *Parser) parseClassConst(doc string, attrs []Attribute) ClassMember {
	start := p.startPos()
	p.advance() // const
	var flags MemberFlags
	for p.is(T_PUBLIC) || p.is(T_PROTECTED) || p.is(T_PRIVATE) || p.is(T_FINAL) {
		switch p.cur.Type {
		case T_PUBLIC:
			flags |= FlagPublic
		case T_PROTECTED:
			flags |= FlagProtected
		case T_PRIVATE:
			flags |= FlagPrivate
		case T_FINAL:
			flags |= FlagFinalMember
		}
		p.advance()
	}
	typeNode := p.tryParseTypeNode()
	var items []ConstItem
	for {
		iStart := p.startPos()
		name := p.cur.Value
		p.advance()
		p.expect(T_ASSIGN)
		val := p.parseExpr()
		items = append(items, ConstItem{base: base{Pos: p.rng(iStart)}, Name: name, Value: val})
		if !p.is(T_COMMA) {
			break
		}
		p.advance()
	}
	p.eat(T_SEMICOLON)
	return &ClassConstDecl{
		base: base{Pos: p.rng(start)}, DocComment: doc, Attributes: attrs,
		Flags: flags, Type: typeNode, Items: items,
	}
}

func (p *Parser) parseTraitUse() ClassMember {
	start := p.startPos()
	p.advance() // use
	traits := p.parseNameList()
	if p.is(T_SEMICOLON) {
		p.advance()
		return &TraitUse{base: base{Pos: p.rng(start)}, Traits: traits}
	}
	p.expect(T_LBRACE)
	var adaptations []TraitAdaptation
	for !p.is(T_RBRACE) && !p.is(T_EOF) {
		adaptations = append(adaptations, p.parseTraitAdaptation())
	}
	p.expect(T_RBRACE)
	return &TraitUse{base: base{Pos: p.rng(start)}, Traits: traits, Adaptations: adaptations}
}

func (p *Parser) parseTraitAdaptation() TraitAdaptation {
	start := p.startPos()
	name := p.parseName()
	var trait, method string
	if p.is(T_DOUBLE_COLON) {
		trait = name
		p.advance()
		method = p.cur.Value
		p.advance()
	} else {
		method = name
	}
	if p.is(T_INSTEADOF) {
		p.advance()
		insteadOf := p.parseNameList()
		p.eat(T_SEMICOLON)
		return &TraitPrecedence{base: base{Pos: p.rng(start)}, Trait: trait, Method: method, InsteadOf: insteadOf}
	}
	// alias
	p.expect(T_AS)
	var flags MemberFlags
	for p.is(T_PUBLIC) || p.is(T_PROTECTED) || p.is(T_PRIVATE) {
		switch p.cur.Type {
		case T_PUBLIC:
			flags |= FlagPublic
		case T_PROTECTED:
			flags |= FlagProtected
		case T_PRIVATE:
			flags |= FlagPrivate
		}
		p.advance()
	}
	alias := ""
	if p.is(T_STRING) {
		alias = p.cur.Value
		p.advance()
	}
	p.eat(T_SEMICOLON)
	return &TraitAlias{base: base{Pos: p.rng(start)}, Trait: trait, Method: method, Alias: alias, Flags: flags}
}

func (p *Parser) parseEnumCase(doc string, attrs []Attribute) ClassMember {
	start := p.startPos()
	p.advance() // case
	name := p.cur.Value
	p.advance()
	var value Expr
	if p.is(T_ASSIGN) {
		p.advance()
		value = p.parseExpr()
	}
	p.eat(T_SEMICOLON)
	return &EnumCase{base: base{Pos: p.rng(start)}, DocComment: doc, Attributes: attrs, Name: name, Value: value}
}

// ─── Function declaration ─────────────────────────────────────────────────────

func (p *Parser) parseFunctionDecl(start Pos, doc string, attrs []Attribute) Stmt {
	p.advance() // function
	byRef := false
	if p.is(T_AMPERSAND) {
		byRef = true
		p.advance()
	}
	_ = byRef
	name := p.cur.Value
	p.advance()
	params := p.parseParamList()
	var returnType TypeNode
	if p.is(T_COLON) {
		p.advance()
		returnType = p.parseTypeNode()
	}
	p.expect(T_LBRACE)
	stmts := p.parseStmtList(T_RBRACE)
	p.expect(T_RBRACE)
	return &FunctionDeclStmt{
		base: base{Pos: p.rng(start)},
		Decl: &FunctionDecl{
			base: base{Pos: p.rng(start)}, DocComment: doc, Attributes: attrs,
			Name: name, Params: params, ReturnType: returnType, Stmts: stmts,
		},
	}
}

// ─── Parameters ───────────────────────────────────────────────────────────────

func (p *Parser) parseParamList() []Param {
	p.expect(T_LPAREN)
	var params []Param
	for !p.is(T_RPAREN) && !p.is(T_EOF) {
		params = append(params, p.parseParam())
		if !p.is(T_COMMA) {
			break
		}
		p.advance()
	}
	p.expect(T_RPAREN)
	return params
}

func (p *Parser) parseParam() Param {
	start := p.startPos()
	var attrs []Attribute
	for p.is(T_ATTRIBUTE) {
		attrs = append(attrs, p.parseAttribute())
	}
	// constructor promotion / modifiers
	var flags MemberFlags
	for p.is(T_PUBLIC) || p.is(T_PROTECTED) || p.is(T_PRIVATE) || p.is(T_READONLY) {
		switch p.cur.Type {
		case T_PUBLIC:
			flags |= FlagPublic
		case T_PROTECTED:
			flags |= FlagProtected
		case T_PRIVATE:
			flags |= FlagPrivate
		case T_READONLY:
			flags |= FlagReadonlyMember
		}
		p.advance()
	}
	typeNode := p.tryParseTypeNode()
	byRef := false
	if p.is(T_AMPERSAND) {
		byRef = true
		p.advance()
	}
	variadic := false
	if p.is(T_ELLIPSIS) {
		variadic = true
		p.advance()
	}
	name := ""
	if p.is(T_VARIABLE) {
		name = p.cur.Value[1:] // strip $
		p.advance()
	}
	var def Expr
	if p.is(T_ASSIGN) {
		p.advance()
		def = p.parseExpr()
	}
	return Param{
		base: base{Pos: p.rng(start)}, Attributes: attrs, Flags: flags, Type: typeNode,
		ByRef: byRef, Variadic: variadic, Name: name, Default: def,
	}
}

// ─── Type nodes ───────────────────────────────────────────────────────────────

func (p *Parser) parseTypeNode() TypeNode {
	start := p.startPos()
	if p.is(T_QUESTION) {
		p.advance()
		inner := p.parseSingleType()
		return &NullableType{base: base{Pos: p.rng(start)}, Inner: inner}
	}
	first := p.parseSingleType()
	if p.is(T_PIPE) {
		// union
		types := []TypeNode{first}
		for p.is(T_PIPE) {
			p.advance()
			types = append(types, p.parseSingleType())
		}
		return &UnionType{base: base{Pos: p.rng(start)}, Types: types}
	}
	if p.is(T_AMPERSAND) {
		// intersection
		types := []TypeNode{first}
		for p.is(T_AMPERSAND) {
			p.advance()
			types = append(types, p.parseSingleType())
		}
		return &IntersectionType{base: base{Pos: p.rng(start)}, Types: types}
	}
	return first
}

func (p *Parser) parseSingleType() TypeNode {
	start := p.startPos()
	name := p.parseName()
	return &NamedType{base: base{Pos: p.rng(start)}, Name: name}
}

// tryParseTypeNode returns nil when the current token does not look like a
// type node (e.g. it is directly a $variable).
func (p *Parser) tryParseTypeNode() TypeNode {
	switch p.cur.Type {
	case T_VARIABLE, T_ASSIGN, T_COMMA, T_RPAREN, T_SEMICOLON, T_RBRACE, T_EOF:
		return nil
	case T_ELLIPSIS, T_AMPERSAND:
		return nil
	case T_QUESTION:
		// nullable type
		return p.parseTypeNode()
	}
	// Only parse as type if the next token after the potential name is not an
	// operator (to avoid eating expressions).
	// Use lookahead: after name/backslash-chain, if we see ( it's a function call.
	if p.is(T_STRING) || p.isTypeKeyword() || p.is(T_BACKSLASH) {
		// lookahead: after a name chain, what follows?
		return p.parseTypeNode()
	}
	return nil
}

func (p *Parser) isTypeKeyword() bool {
	switch p.cur.Type {
	case T_INT_TYPE, T_FLOAT_TYPE, T_STRING_TYPE, T_BOOL_TYPE,
		T_VOID_TYPE, T_NEVER_TYPE, T_MIXED_TYPE, T_OBJECT_TYPE,
		T_ITERABLE_TYPE, T_SELF_TYPE, T_STATIC_TYPE, T_PARENT_TYPE,
		T_NULL_TYPE, T_TRUE_TYPE, T_FALSE_TYPE, T_ARRAY:
		return true
	}
	return false
}

// ─── Simple statement parsers ─────────────────────────────────────────────────

func (p *Parser) parseIf(start Pos) Stmt {
	p.advance() // if
	p.expect(T_LPAREN)
	cond := p.parseExpr()
	p.expect(T_RPAREN)
	body := p.parseBlock()
	var elseifs []ElseifClause
	var els []Stmt
	for p.is(T_ELSEIF) {
		p.advance()
		p.expect(T_LPAREN)
		ec := p.parseExpr()
		p.expect(T_RPAREN)
		eb := p.parseBlock()
		elseifs = append(elseifs, ElseifClause{Cond: ec, Body: eb})
	}
	if p.is(T_ELSE) {
		p.advance()
		els = p.parseBlock()
	}
	return &IfStmt{base: base{Pos: p.rng(start)}, Cond: cond, Body: body, Elseifs: elseifs, Else: els}
}

func (p *Parser) parseBlock() []Stmt {
	if p.is(T_LBRACE) {
		p.advance()
		stmts := p.parseStmtList(T_RBRACE)
		p.expect(T_RBRACE)
		return stmts
	}
	s := p.parseStmt()
	if s == nil {
		return nil
	}
	return []Stmt{s}
}

func (p *Parser) parseWhile(start Pos) Stmt {
	p.advance()
	p.expect(T_LPAREN)
	cond := p.parseExpr()
	p.expect(T_RPAREN)
	body := p.parseBlock()
	return &WhileStmt{base: base{Pos: p.rng(start)}, Cond: cond, Body: body}
}

func (p *Parser) parseDoWhile(start Pos) Stmt {
	p.advance()
	body := p.parseBlock()
	p.expect(T_WHILE)
	p.expect(T_LPAREN)
	cond := p.parseExpr()
	p.expect(T_RPAREN)
	p.eat(T_SEMICOLON)
	return &DoWhileStmt{base: base{Pos: p.rng(start)}, Body: body, Cond: cond}
}

func (p *Parser) parseFor(start Pos) Stmt {
	p.advance()
	p.expect(T_LPAREN)
	init := p.parseExprList(T_SEMICOLON)
	p.expect(T_SEMICOLON)
	cond := p.parseExprList(T_SEMICOLON)
	p.expect(T_SEMICOLON)
	loop := p.parseExprList(T_RPAREN)
	p.expect(T_RPAREN)
	body := p.parseBlock()
	return &ForStmt{base: base{Pos: p.rng(start)}, Init: init, Cond: cond, Loop: loop, Body: body}
}

func (p *Parser) parseForeach(start Pos) Stmt {
	p.advance()
	p.expect(T_LPAREN)
	subject := p.parseExpr()
	p.expect(T_AS)
	byRef := false
	if p.is(T_AMPERSAND) {
		byRef = true
		p.advance()
	}
	first := p.parseExpr()
	var key, value Expr
	if p.is(T_DOUBLE_ARROW) {
		p.advance()
		if p.is(T_AMPERSAND) {
			byRef = true
			p.advance()
		}
		key = first
		value = p.parseExpr()
	} else {
		value = first
	}
	p.expect(T_RPAREN)
	body := p.parseBlock()
	return &ForeachStmt{
		base: base{Pos: p.rng(start)}, Expr: subject,
		Key: key, Value: value, ByRef: byRef, Body: body,
	}
}

func (p *Parser) parseSwitch(start Pos) Stmt {
	p.advance()
	p.expect(T_LPAREN)
	cond := p.parseExpr()
	p.expect(T_RPAREN)
	p.expect(T_LBRACE)
	var cases []SwitchCase
	for !p.is(T_RBRACE) && !p.is(T_EOF) {
		cStart := p.startPos()
		var val Expr
		if p.is(T_CASE) {
			p.advance()
			val = p.parseExpr()
		} else if p.is(T_DEFAULT) {
			p.advance()
		}
		if p.is(T_COLON) || p.is(T_SEMICOLON) {
			p.advance()
		}
		var stmts []Stmt
		for !p.is(T_CASE) && !p.is(T_DEFAULT) && !p.is(T_RBRACE) && !p.is(T_EOF) {
			s := p.parseStmt()
			if s != nil {
				stmts = append(stmts, s)
			}
		}
		cases = append(cases, SwitchCase{base: base{Pos: p.rng(cStart)}, Value: val, Stmts: stmts})
	}
	p.expect(T_RBRACE)
	return &SwitchStmt{base: base{Pos: p.rng(start)}, Cond: cond, Cases: cases}
}

func (p *Parser) parseBreak(start Pos) Stmt {
	p.advance()
	var num Expr
	if !p.is(T_SEMICOLON) {
		num = p.parseExpr()
	}
	p.eat(T_SEMICOLON)
	return &BreakStmt{base: base{Pos: p.rng(start)}, Num: num}
}

func (p *Parser) parseContinue(start Pos) Stmt {
	p.advance()
	var num Expr
	if !p.is(T_SEMICOLON) {
		num = p.parseExpr()
	}
	p.eat(T_SEMICOLON)
	return &ContinueStmt{base: base{Pos: p.rng(start)}, Num: num}
}

func (p *Parser) parseReturn(start Pos) Stmt {
	p.advance()
	var e Expr
	if !p.is(T_SEMICOLON) && !p.is(T_CLOSE_TAG) {
		e = p.parseExpr()
	}
	p.eat(T_SEMICOLON)
	return &ReturnStmt{base: base{Pos: p.rng(start)}, Expr: e}
}

func (p *Parser) parseEcho(start Pos) Stmt {
	p.advance()
	exprs := p.parseExprList(T_SEMICOLON)
	p.eat(T_SEMICOLON)
	return &EchoStmt{base: base{Pos: p.rng(start)}, Exprs: exprs}
}

func (p *Parser) parseTryCatch(start Pos) Stmt {
	p.advance()
	p.expect(T_LBRACE)
	body := p.parseStmtList(T_RBRACE)
	p.expect(T_RBRACE)
	var catches []CatchClause
	for p.is(T_CATCH) {
		cStart := p.startPos()
		p.advance()
		p.expect(T_LPAREN)
		var types []string
		types = append(types, p.parseName())
		for p.is(T_PIPE) {
			p.advance()
			types = append(types, p.parseName())
		}
		varName := ""
		if p.is(T_VARIABLE) {
			varName = p.cur.Value[1:]
			p.advance()
		}
		p.expect(T_RPAREN)
		p.expect(T_LBRACE)
		stmts := p.parseStmtList(T_RBRACE)
		p.expect(T_RBRACE)
		catches = append(catches, CatchClause{base: base{Pos: p.rng(cStart)}, Types: types, Var: varName, Body: stmts})
	}
	var finally []Stmt
	if p.is(T_FINALLY) {
		p.advance()
		p.expect(T_LBRACE)
		finally = p.parseStmtList(T_RBRACE)
		p.expect(T_RBRACE)
	}
	return &TryCatchStmt{base: base{Pos: p.rng(start)}, Body: body, Catches: catches, Finally: finally}
}

func (p *Parser) parseThrowStmt(start Pos) Stmt {
	p.advance()
	e := p.parseExpr()
	p.eat(T_SEMICOLON)
	return &ThrowStmt{base: base{Pos: p.rng(start)}, Expr: e}
}

func (p *Parser) parseGlobal(start Pos) Stmt {
	p.advance()
	var vars []Expr
	for p.is(T_VARIABLE) {
		vars = append(vars, &VarExpr{base: base{Pos: p.rng(p.startPos())}, Name: p.cur.Value[1:]})
		p.advance()
		if !p.is(T_COMMA) {
			break
		}
		p.advance()
	}
	p.eat(T_SEMICOLON)
	return &GlobalStmt{base: base{Pos: p.rng(start)}, Vars: vars}
}

func (p *Parser) parseStatic(start Pos) Stmt {
	p.advance() // static
	var vars []StaticVar
	for p.is(T_VARIABLE) {
		vStart := p.startPos()
		name := p.cur.Value[1:]
		p.advance()
		var def Expr
		if p.is(T_ASSIGN) {
			p.advance()
			def = p.parseExpr()
		}
		vars = append(vars, StaticVar{base: base{Pos: p.rng(vStart)}, Name: name, Default: def})
		if !p.is(T_COMMA) {
			break
		}
		p.advance()
	}
	p.eat(T_SEMICOLON)
	return &StaticStmt{base: base{Pos: p.rng(start)}, Vars: vars}
}

func (p *Parser) parseUnset(start Pos) Stmt {
	p.advance()
	p.expect(T_LPAREN)
	vars := p.parseExprList(T_RPAREN)
	p.expect(T_RPAREN)
	p.eat(T_SEMICOLON)
	return &UnsetStmt{base: base{Pos: p.rng(start)}, Vars: vars}
}

func (p *Parser) parseDeclare(start Pos) Stmt {
	p.advance()
	p.expect(T_LPAREN)
	var directives []DeclareDirective
	for !p.is(T_RPAREN) && !p.is(T_EOF) {
		dStart := p.startPos()
		name := p.cur.Value
		p.advance()
		p.expect(T_ASSIGN)
		val := p.parseExpr()
		directives = append(directives, DeclareDirective{base: base{Pos: p.rng(dStart)}, Name: name, Value: val})
		if !p.is(T_COMMA) {
			break
		}
		p.advance()
	}
	p.expect(T_RPAREN)
	var body []Stmt
	if p.is(T_LBRACE) {
		p.advance()
		body = p.parseStmtList(T_RBRACE)
		p.expect(T_RBRACE)
	} else {
		p.eat(T_SEMICOLON)
	}
	return &DeclareStmt{base: base{Pos: p.rng(start)}, Directives: directives, Body: body}
}

// ─── Expression parser (Pratt) ────────────────────────────────────────────────

func (p *Parser) parseExpr() Expr {
	return p.parseAssignment()
}

func (p *Parser) parseExprList(until TokenType) []Expr {
	if p.is(until) || p.is(T_EOF) {
		return nil
	}
	var exprs []Expr
	for {
		e := p.parseExpr()
		if e != nil {
			exprs = append(exprs, e)
		}
		if !p.is(T_COMMA) {
			break
		}
		p.advance()
		if p.is(until) {
			break
		}
	}
	return exprs
}

func (p *Parser) parseAssignment() Expr {
	left := p.parseTernary()
	if left == nil {
		return nil
	}
	op := ""
	switch p.cur.Type {
	case T_ASSIGN:
		op = "="
	case T_PLUS_ASSIGN:
		op = "+="
	case T_MINUS_ASSIGN:
		op = "-="
	case T_STAR_ASSIGN:
		op = "*="
	case T_SLASH_ASSIGN:
		op = "/="
	case T_PERCENT_ASSIGN:
		op = "%="
	case T_STARSTAR_ASSIGN:
		op = "**="
	case T_AMPERSAND_ASSIGN:
		op = "&="
	case T_PIPE_ASSIGN:
		op = "|="
	case T_CARET_ASSIGN:
		op = "^="
	case T_LSHIFT_ASSIGN:
		op = "<<="
	case T_RSHIFT_ASSIGN:
		op = ">>="
	case T_DOT_ASSIGN:
		op = ".="
	case T_QUESTIONQUESTION_ASSIGN:
		op = "??="
	}
	if op != "" {
		start := Pos(left.nodeRange().Start)
		p.advance()
		right := p.parseAssignment()
		return &AssignExpr{base: base{Pos: p.rng(start)}, Op: op, Left: left, Right: right}
	}
	return left
}

func (p *Parser) parseTernary() Expr {
	cond := p.parseNullCoalesce()
	if cond == nil {
		return nil
	}
	if p.is(T_QUESTION) {
		start := Pos(cond.nodeRange().Start)
		p.advance()
		var then Expr
		if !p.is(T_COLON) {
			then = p.parseExpr()
		}
		p.expect(T_COLON)
		els := p.parseExpr()
		return &TernaryExpr{base: base{Pos: p.rng(start)}, Cond: cond, Then: then, Else: els}
	}
	return cond
}

func (p *Parser) parseNullCoalesce() Expr {
	left := p.parseBinary(0)
	if left == nil {
		return nil
	}
	if p.is(T_QUESTIONQUESTION) {
		start := Pos(left.nodeRange().Start)
		p.advance()
		right := p.parseNullCoalesce()
		return &NullCoalesceExpr{base: base{Pos: p.rng(start)}, Left: left, Right: right}
	}
	return left
}

type binop struct {
	prec  int
	right bool
}

var binops = map[TokenType]binop{
	T_LOGICAL_OR:    {1, false},
	T_LOGICAL_XOR:   {2, false},
	T_LOGICAL_AND:   {3, false},
	T_BOOL_OR:       {4, false},
	T_BOOL_AND:      {5, false},
	T_PIPE:          {6, false},
	T_CARET:         {7, false},
	T_AMPERSAND:     {8, false},
	T_EQ:            {9, false},
	T_IDENTICAL:     {9, false},
	T_NEQ:           {9, false},
	T_NOT_IDENTICAL: {9, false},
	T_SPACESHIP:     {9, false},
	T_LT:            {10, false},
	T_LTE:           {10, false},
	T_GT:            {10, false},
	T_GTE:           {10, false},
	T_INSTANCEOF:    {10, false},
	T_LSHIFT:        {11, false},
	T_RSHIFT:        {11, false},
	T_PLUS:          {12, false},
	T_MINUS:         {12, false},
	T_DOT:           {12, false},
	T_STAR:          {13, false},
	T_SLASH:         {13, false},
	T_PERCENT:       {13, false},
	T_STARSTAR:      {14, true},
}

func (p *Parser) parseBinary(minPrec int) Expr {
	left := p.parseUnary()
	if left == nil {
		return nil
	}
	for {
		bo, ok := binops[p.cur.Type]
		if !ok || bo.prec < minPrec {
			break
		}
		start := Pos(left.nodeRange().Start)
		op := p.cur.Value
		tt := p.cur.Type
		p.advance()
		var right Expr
		if bo.right {
			right = p.parseBinary(bo.prec)
		} else {
			right = p.parseBinary(bo.prec + 1)
		}
		if tt == T_INSTANCEOF {
			className := p.tokenToName(p.cur)
			_ = className
			return &InstanceofExpr{base: base{Pos: p.rng(start)}, Expr: left, Class: op}
		}
		left = &BinaryExpr{base: base{Pos: p.rng(start)}, Op: op, Left: left, Right: right}
	}
	return left
}

func (p *Parser) tokenToName(t Token) string { return t.Value }

func (p *Parser) parseUnary() Expr {
	start := p.startPos()
	switch p.cur.Type {
	case T_BANG:
		p.advance()
		return &UnaryExpr{base: base{Pos: p.rng(start)}, Op: "!", Operand: p.parseUnary()}
	case T_TILDE:
		p.advance()
		return &UnaryExpr{base: base{Pos: p.rng(start)}, Op: "~", Operand: p.parseUnary()}
	case T_MINUS:
		p.advance()
		return &UnaryExpr{base: base{Pos: p.rng(start)}, Op: "-", Operand: p.parseUnary()}
	case T_PLUS:
		p.advance()
		return &UnaryExpr{base: base{Pos: p.rng(start)}, Op: "+", Operand: p.parseUnary()}
	case T_AT:
		p.advance()
		return p.parseUnary()
	case T_INC:
		p.advance()
		return &UnaryExpr{base: base{Pos: p.rng(start)}, Op: "++", Operand: p.parsePrimary()}
	case T_DEC:
		p.advance()
		return &UnaryExpr{base: base{Pos: p.rng(start)}, Op: "--", Operand: p.parsePrimary()}
	case T_INT_CAST:
		p.advance()
		return &CastExpr{base: base{Pos: p.rng(start)}, Kind: "int", Operand: p.parseUnary()}
	case T_DOUBLE_CAST:
		p.advance()
		return &CastExpr{base: base{Pos: p.rng(start)}, Kind: "float", Operand: p.parseUnary()}
	case T_STRING_CAST:
		p.advance()
		return &CastExpr{base: base{Pos: p.rng(start)}, Kind: "string", Operand: p.parseUnary()}
	case T_ARRAY_CAST:
		p.advance()
		return &CastExpr{base: base{Pos: p.rng(start)}, Kind: "array", Operand: p.parseUnary()}
	case T_OBJECT_CAST:
		p.advance()
		return &CastExpr{base: base{Pos: p.rng(start)}, Kind: "object", Operand: p.parseUnary()}
	case T_BOOL_CAST:
		p.advance()
		return &CastExpr{base: base{Pos: p.rng(start)}, Kind: "bool", Operand: p.parseUnary()}
	case T_UNSET_CAST:
		p.advance()
		return &CastExpr{base: base{Pos: p.rng(start)}, Kind: "unset", Operand: p.parseUnary()}
	case T_THROW:
		p.advance()
		return &ThrowExpr{base: base{Pos: p.rng(start)}, Expr: p.parseExpr()}
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() Expr {
	e := p.parsePrimary()
	if e == nil {
		return nil
	}
	for {
		start := Pos(e.nodeRange().Start)
		switch p.cur.Type {
		case T_OBJECT_OPERATOR, T_NULLSAFE_OBJECT_OPERATOR:
			nullsafe := p.cur.Type == T_NULLSAFE_OBJECT_OPERATOR
			p.advance()
			name := p.cur.Value
			p.advance()
			if p.is(T_LPAREN) {
				args := p.parseArgList()
				e = &MethodCallExpr{base: base{Pos: p.rng(start)}, Object: e, Method: name, Args: args, Nullsafe: nullsafe}
			} else {
				e = &PropFetchExpr{base: base{Pos: p.rng(start)}, Object: e, Property: name, Nullsafe: nullsafe}
			}
		case T_DOUBLE_COLON:
			p.advance()
			name := p.cur.Value
			p.advance()
			if p.is(T_LPAREN) {
				args := p.parseArgList()
				e = &StaticCallExpr{base: base{Pos: p.rng(start)}, Class: e, Method: name, Args: args}
			} else if strings.HasPrefix(name, "$") {
				e = &StaticPropFetchExpr{base: base{Pos: p.rng(start)}, Class: e, Property: name}
			} else {
				e = &ClassConstFetchExpr{base: base{Pos: p.rng(start)}, Class: e, Name: name}
			}
		case T_LBRACKET:
			p.advance()
			var idx Expr
			if !p.is(T_RBRACKET) {
				idx = p.parseExpr()
			}
			p.expect(T_RBRACKET)
			e = &ArrayAccessExpr{base: base{Pos: p.rng(start)}, Array: e, Index: idx}
		case T_LBRACE:
			// {expr} access — same as []
			p.advance()
			idx := p.parseExpr()
			p.expect(T_RBRACE)
			e = &ArrayAccessExpr{base: base{Pos: p.rng(start)}, Array: e, Index: idx}
		case T_INC:
			p.advance()
			e = &UnaryExpr{base: base{Pos: p.rng(start)}, Op: "++", Operand: e, Postfix: true}
		case T_DEC:
			p.advance()
			e = &UnaryExpr{base: base{Pos: p.rng(start)}, Op: "--", Operand: e, Postfix: true}
		default:
			return e
		}
	}
}

func (p *Parser) parsePrimary() Expr {
	start := p.startPos()

	switch p.cur.Type {
	case T_LNUMBER:
		v := p.cur.Value
		p.advance()
		return &IntLit{base: base{Pos: p.rng(start)}, Value: v}
	case T_DNUMBER:
		v := p.cur.Value
		p.advance()
		return &FloatLit{base: base{Pos: p.rng(start)}, Value: v}
	case T_CONSTANT_ENCAPSED_STRING:
		v := p.cur.Value
		p.advance()
		return &StringLit{base: base{Pos: p.rng(start)}, Value: v}
	case T_ENCAPSED_AND_WHITESPACE:
		v := p.cur.Value
		p.advance()
		return &StringLit{base: base{Pos: p.rng(start)}, Value: v}
	case T_HEREDOC, T_NOWDOC:
		v := p.cur.Value
		p.advance()
		return &StringLit{base: base{Pos: p.rng(start)}, Value: v}
	case T_NULL_TYPE:
		p.advance()
		return &NullLit{base: base{Pos: p.rng(start)}}
	case T_TRUE_TYPE:
		p.advance()
		return &TrueLit{base: base{Pos: p.rng(start)}}
	case T_FALSE_TYPE:
		p.advance()
		return &FalseLit{base: base{Pos: p.rng(start)}}
	case T_VARIABLE:
		name := p.cur.Value[1:]
		p.advance()
		return &VarExpr{base: base{Pos: p.rng(start)}, Name: name}
	case T_LPAREN:
		p.advance()
		e := p.parseExpr()
		p.expect(T_RPAREN)
		return e
	case T_LBRACKET:
		return p.parseArray(start)
	case T_ARRAY:
		if p.peek1().Type == T_LPAREN {
			return p.parseOldArray(start)
		}
	case T_NEW:
		return p.parseNew(start)
	case T_CLONE:
		p.advance()
		return &CloneExpr{base: base{Pos: p.rng(start)}, Expr: p.parsePrimary()}
	case T_FUNCTION:
		return p.parseClosure(start)
	case T_FN:
		return p.parseArrowFunc(start)
	case T_STATIC:
		if p.peek1().Type == T_FUNCTION {
			p.advance()
			return p.parseClosure(start)
		}
		if p.peek1().Type == T_FN {
			p.advance()
			return p.parseArrowFunc(start)
		}
	case T_MATCH:
		return p.parseMatch(start)
	case T_LIST:
		return p.parseList(start)
	case T_ISSET:
		return p.parseIsset(start)
	case T_EMPTY:
		p.advance()
		p.expect(T_LPAREN)
		e := p.parseExpr()
		p.expect(T_RPAREN)
		return &EmptyExpr{base: base{Pos: p.rng(start)}, Expr: e}
	case T_EVAL:
		p.advance()
		p.expect(T_LPAREN)
		e := p.parseExpr()
		p.expect(T_RPAREN)
		return &EvalExpr{base: base{Pos: p.rng(start)}, Expr: e}
	case T_INCLUDE:
		p.advance()
		return &IncludeExpr{base: base{Pos: p.rng(start)}, Kind: "include", Expr: p.parseExpr()}
	case T_INCLUDE_ONCE:
		p.advance()
		return &IncludeExpr{base: base{Pos: p.rng(start)}, Kind: "include_once", Expr: p.parseExpr()}
	case T_REQUIRE:
		p.advance()
		return &IncludeExpr{base: base{Pos: p.rng(start)}, Kind: "require", Expr: p.parseExpr()}
	case T_REQUIRE_ONCE:
		p.advance()
		return &IncludeExpr{base: base{Pos: p.rng(start)}, Kind: "require_once", Expr: p.parseExpr()}
	case T_PRINT:
		p.advance()
		return &PrintExpr{base: base{Pos: p.rng(start)}, Expr: p.parseExpr()}
	case T_YIELD:
		p.advance()
		return &UnaryExpr{base: base{Pos: p.rng(start)}, Op: "yield", Operand: p.parseExpr()}
	case T_YIELD_FROM:
		p.advance()
		return &UnaryExpr{base: base{Pos: p.rng(start)}, Op: "yield from", Operand: p.parseExpr()}
	case T_ATTRIBUTE:
		// Can't happen as expression primary but skip gracefully
	}

	// Identifier / name (constants, class names, function calls)
	if p.is(T_STRING) || p.is(T_BACKSLASH) || p.isTypeKeyword() ||
		p.is(T_STATIC) || p.is(T_SELF_TYPE) || p.is(T_PARENT_TYPE) {
		name := p.parseName()
		// Named argument or function call
		if p.is(T_LPAREN) {
			args := p.parseArgList()
			return &FuncCallExpr{
				base: base{Pos: p.rng(start)},
				Func: &ConstExpr{base: base{Pos: p.rng(start)}, Name: name},
				Args: args,
			}
		}
		return &ConstExpr{base: base{Pos: p.rng(start)}, Name: name}
	}

	return nil
}

func (p *Parser) parseArray(start Pos) Expr {
	p.advance() // [
	var items []ArrayItem
	for !p.is(T_RBRACKET) && !p.is(T_EOF) {
		items = append(items, p.parseArrayItem())
		if !p.is(T_COMMA) {
			break
		}
		p.advance()
		if p.is(T_RBRACKET) {
			break
		}
	}
	p.expect(T_RBRACKET)
	return &ArrayExpr{base: base{Pos: p.rng(start)}, Items: items}
}

func (p *Parser) parseOldArray(start Pos) Expr {
	p.advance() // array
	p.advance() // (
	var items []ArrayItem
	for !p.is(T_RPAREN) && !p.is(T_EOF) {
		items = append(items, p.parseArrayItem())
		if !p.is(T_COMMA) {
			break
		}
		p.advance()
		if p.is(T_RPAREN) {
			break
		}
	}
	p.expect(T_RPAREN)
	return &ArrayExpr{base: base{Pos: p.rng(start)}, Items: items}
}

func (p *Parser) parseArrayItem() ArrayItem {
	start := p.startPos()
	unpack := false
	if p.is(T_ELLIPSIS) {
		unpack = true
		p.advance()
	}
	first := p.parseExpr()
	if p.is(T_DOUBLE_ARROW) {
		p.advance()
		byRef := false
		if p.is(T_AMPERSAND) {
			byRef = true
			p.advance()
		}
		value := p.parseExpr()
		return ArrayItem{base: base{Pos: p.rng(start)}, Key: first, Value: value, ByRef: byRef, Unpack: unpack}
	}
	return ArrayItem{base: base{Pos: p.rng(start)}, Value: first, Unpack: unpack}
}

func (p *Parser) parseNew(start Pos) Expr {
	p.advance() // new
	var class Expr
	if p.is(T_CLASS) {
		// anonymous class
		p.advance()
		class = &ConstExpr{Name: "class"}
	} else {
		name := p.parseName()
		class = &ConstExpr{base: base{Pos: p.rng(start)}, Name: name}
	}
	var args []Arg
	if p.is(T_LPAREN) {
		args = p.parseArgList()
	}
	return &NewExpr{base: base{Pos: p.rng(start)}, Class: class, Args: args}
}

func (p *Parser) parseClosure(start Pos) Expr {
	static := false
	if p.is(T_STATIC) {
		static = true
		p.advance()
	}
	p.advance() // function
	byRef := false
	if p.is(T_AMPERSAND) {
		byRef = true
		p.advance()
	}
	params := p.parseParamList()
	var uses []ClosureUse
	if p.is(T_USE) {
		p.advance()
		p.expect(T_LPAREN)
		for !p.is(T_RPAREN) && !p.is(T_EOF) {
			uByRef := false
			if p.is(T_AMPERSAND) {
				uByRef = true
				p.advance()
			}
			name := p.cur.Value[1:]
			p.advance()
			uses = append(uses, ClosureUse{ByRef: uByRef, Name: name})
			if !p.is(T_COMMA) {
				break
			}
			p.advance()
		}
		p.expect(T_RPAREN)
	}
	var returnType TypeNode
	if p.is(T_COLON) {
		p.advance()
		returnType = p.parseTypeNode()
	}
	p.expect(T_LBRACE)
	stmts := p.parseStmtList(T_RBRACE)
	p.expect(T_RBRACE)
	return &ClosureExpr{
		base: base{Pos: p.rng(start)}, Static: static, ByRef: byRef,
		Params: params, Uses: uses, ReturnType: returnType, Stmts: stmts,
	}
}

func (p *Parser) parseArrowFunc(start Pos) Expr {
	static := false
	if p.is(T_STATIC) {
		static = true
		p.advance()
	}
	p.advance() // fn
	byRef := false
	if p.is(T_AMPERSAND) {
		byRef = true
		p.advance()
	}
	params := p.parseParamList()
	var returnType TypeNode
	if p.is(T_COLON) {
		p.advance()
		returnType = p.parseTypeNode()
	}
	p.expect(T_DOUBLE_ARROW)
	expr := p.parseExpr()
	return &ArrowFuncExpr{
		base: base{Pos: p.rng(start)}, Static: static, ByRef: byRef,
		Params: params, ReturnType: returnType, Expr: expr,
	}
}

func (p *Parser) parseMatch(start Pos) Expr {
	p.advance() // match
	p.expect(T_LPAREN)
	cond := p.parseExpr()
	p.expect(T_RPAREN)
	p.expect(T_LBRACE)
	var arms []MatchArm
	for !p.is(T_RBRACE) && !p.is(T_EOF) {
		aStart := p.startPos()
		var conds []Expr
		if p.is(T_DEFAULT) {
			p.advance()
		} else {
			conds = append(conds, p.parseExpr())
			for p.is(T_COMMA) {
				p.advance()
				if p.is(T_DOUBLE_ARROW) {
					break
				}
				conds = append(conds, p.parseExpr())
			}
		}
		p.expect(T_DOUBLE_ARROW)
		val := p.parseExpr()
		arms = append(arms, MatchArm{base: base{Pos: p.rng(aStart)}, Conds: conds, Value: val})
		if !p.is(T_COMMA) {
			break
		}
		p.advance()
		if p.is(T_RBRACE) {
			break
		}
	}
	p.expect(T_RBRACE)
	return &MatchExpr{base: base{Pos: p.rng(start)}, Cond: cond, Arms: arms}
}

func (p *Parser) parseList(start Pos) Expr {
	p.advance() // list
	p.expect(T_LPAREN)
	var items []ArrayItem
	for !p.is(T_RPAREN) && !p.is(T_EOF) {
		items = append(items, p.parseArrayItem())
		if !p.is(T_COMMA) {
			break
		}
		p.advance()
	}
	p.expect(T_RPAREN)
	return &ListExpr{base: base{Pos: p.rng(start)}, Items: items}
}

func (p *Parser) parseIsset(start Pos) Expr {
	p.advance() // isset
	p.expect(T_LPAREN)
	vars := p.parseExprList(T_RPAREN)
	p.expect(T_RPAREN)
	return &IssetExpr{base: base{Pos: p.rng(start)}, Vars: vars}
}

// ─── Argument list ────────────────────────────────────────────────────────────

func (p *Parser) parseArgList() []Arg {
	p.expect(T_LPAREN)
	// first-class callable: f(...)
	if p.is(T_ELLIPSIS) && p.peek1().Type == T_RPAREN {
		p.advance()
		p.advance()
		return nil // handled by caller as FCC
	}
	var args []Arg
	for !p.is(T_RPAREN) && !p.is(T_EOF) {
		args = append(args, p.parseArg())
		if !p.is(T_COMMA) {
			break
		}
		p.advance()
		if p.is(T_RPAREN) {
			break
		}
	}
	p.expect(T_RPAREN)
	return args
}

func (p *Parser) parseArg() Arg {
	start := p.startPos()
	// named argument?
	if p.is(T_STRING) && p.peek1().Type == T_COLON {
		name := p.cur.Value
		p.advance()
		p.advance()
		return Arg{base: base{Pos: p.rng(start)}, Name: name, Value: p.parseExpr()}
	}
	byRef := false
	if p.is(T_AMPERSAND) {
		byRef = true
		p.advance()
	}
	unpack := false
	if p.is(T_ELLIPSIS) {
		unpack = true
		p.advance()
	}
	val := p.parseExpr()
	return Arg{base: base{Pos: p.rng(start)}, ByRef: byRef, Unpack: unpack, Value: val}
}

// ─── Attribute parsing ────────────────────────────────────────────────────────

func (p *Parser) parseAttribute() Attribute {
	start := p.startPos()
	p.advance() // #[
	name := p.parseName()
	var args []Arg
	if p.is(T_LPAREN) {
		args = p.parseArgList()
	}
	// attributes can be comma-separated in one #[...]; parse remaining
	for p.is(T_COMMA) {
		p.advance()
		// only return first; caller collects repeated attributes
		break
	}
	p.eat(T_RBRACKET)
	return Attribute{base: base{Pos: p.rng(start)}, Name: name, Args: args}
}

// ─── Name helpers ─────────────────────────────────────────────────────────────

// parseName reads a (possibly qualified) PHP name: Foo\Bar\Baz
func (p *Parser) parseName() string {
	var parts []string
	leading := ""
	if p.is(T_BACKSLASH) {
		leading = `\`
		p.advance()
	}
	for p.is(T_STRING) || p.isTypeKeyword() || p.is(T_STATIC) ||
		p.is(T_SELF_TYPE) || p.is(T_PARENT_TYPE) || p.is(T_NULL_TYPE) ||
		p.is(T_TRUE_TYPE) || p.is(T_FALSE_TYPE) {
		parts = append(parts, p.cur.Value)
		p.advance()
		if p.is(T_BACKSLASH) {
			p.advance()
		} else {
			break
		}
	}
	return leading + strings.Join(parts, `\`)
}

func (p *Parser) parseNameList() []string {
	var names []string
	names = append(names, p.parseName())
	for p.is(T_COMMA) {
		p.advance()
		names = append(names, p.parseName())
	}
	return names
}
