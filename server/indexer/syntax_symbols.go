package indexer

import (
	"strconv"
	"strings"

	"github.com/ayanozturk/go-php-parser/analyse"
	"github.com/ayanozturk/go-php-parser/ast"
	"github.com/ayanozturk/go-php-parser/syntax"
	"github.com/ayanozturk/go-php-parser/token"
)

// extractSymbolsFromSyntax walks a declaration-tier (or full) syntax tree and
// records class-likes, functions, methods, properties, and constants.
// PHPDoc-synthesized members (@method) are not produced here — callers that
// still parse the legacy AST may merge those in.
func extractSymbolsFromSyntax(uri string, res *syntax.ParseResult) []*Symbol {
	if res == nil || res.File == nil || res.File.Root == nil {
		return nil
	}
	b := analyse.NewBinder("", map[string]string{})
	w := &syntaxSymbolWalk{
		b:    b,
		uri:  uri,
		file: res.File,
	}
	w.walkFile(res.File.Root)
	for _, sym := range w.syms {
		enrichLosslessFields(sym)
	}
	return w.syms
}

type syntaxSymbolWalk struct {
	b    *analyse.Binder
	uri  string
	file *syntax.File
	syms []*Symbol
}

type syntaxScopeSnap struct {
	ns      string
	aliases map[string]string
	owner   string
}

func (w *syntaxSymbolWalk) saveScope() syntaxScopeSnap {
	aliases := make(map[string]string, len(w.b.Aliases))
	for k, v := range w.b.Aliases {
		aliases[k] = v
	}
	return syntaxScopeSnap{ns: w.b.Namespace, aliases: aliases, owner: w.b.Owner}
}

func (w *syntaxSymbolWalk) restoreScope(s syntaxScopeSnap) {
	w.b.Namespace = s.ns
	w.b.Aliases = s.aliases
	w.b.Owner = s.owner
}

func (w *syntaxSymbolWalk) walkFile(n *syntax.RedNode) {
	if n == nil {
		return
	}
	if n.Kind() == syntax.KindStatementList {
		w.walkStatementList(n)
		return
	}
	for _, c := range n.Children() {
		if c.Kind() == syntax.KindStatementList {
			w.walkStatementList(c)
			continue
		}
		w.walk(c)
	}
}

func (w *syntaxSymbolWalk) walkStatementList(n *syntax.RedNode) {
	for _, c := range n.Children() {
		switch c.Kind() {
		case syntax.KindNamespaceDecl:
			w.walkNamespace(c)
		case syntax.KindUseDecl:
			syntax.AppendUseAliases(c, w.b.Aliases)
		default:
			w.walk(c)
		}
	}
}

func (w *syntaxSymbolWalk) walkNamespace(n *syntax.RedNode) {
	nsName := ""
	var body *syntax.RedNode
	for _, c := range n.Children() {
		switch c.Kind() {
		case syntax.KindUnqualifiedName, syntax.KindQualifiedName,
			syntax.KindFullyQualifiedName, syntax.KindRelativeName:
			nsName = strings.TrimPrefix(syntax.NameText(c), `\`)
		case syntax.KindStatementList:
			body = c
		}
	}
	if body != nil {
		prev := w.saveScope()
		w.b.Namespace = nsName
		w.b.Aliases = map[string]string{}
		w.b.Owner = ""
		w.walkStatementList(body)
		w.restoreScope(prev)
		return
	}
	w.b.Namespace = nsName
	w.b.Aliases = map[string]string{}
	w.b.Owner = ""
}

func (w *syntaxSymbolWalk) walk(n *syntax.RedNode) {
	if n == nil {
		return
	}
	switch n.Kind() {
	case syntax.KindNamespaceDecl:
		w.walkNamespace(n)
		return
	case syntax.KindUseDecl:
		syntax.AppendUseAliases(n, w.b.Aliases)
		return
	case syntax.KindStatementList:
		w.walkStatementList(n)
		return
	case syntax.KindClassDecl, syntax.KindInterfaceDecl, syntax.KindTraitDecl, syntax.KindEnumDecl:
		w.walkClassLike(n)
		return
	case syntax.KindFunctionDecl:
		w.walkFunction(n, false)
		return
	case syntax.KindConstDecl:
		w.walkGlobalConst(n)
		return
	}
	for _, c := range n.Children() {
		w.walk(c)
	}
}

func (w *syntaxSymbolWalk) walkClassLike(n *syntax.RedNode) {
	kind := KindClass
	switch n.Kind() {
	case syntax.KindInterfaceDecl:
		kind = KindInterface
	case syntax.KindTraitDecl:
		kind = KindModule // match legacy AST extract for traits
	case syntax.KindEnumDecl:
		kind = KindEnum
	}
	mods := syntaxModifiers(n)
	var nameNode *syntax.RedNode
	var extends, implements []string
	var members *syntax.RedNode
	for _, c := range n.Children() {
		switch c.Kind() {
		case syntax.KindUnqualifiedName, syntax.KindQualifiedName,
			syntax.KindFullyQualifiedName, syntax.KindRelativeName:
			if nameNode == nil {
				nameNode = c
			}
		case syntax.KindExtendsClause:
			extends = append(extends, w.clauseNames(c)...)
		case syntax.KindImplementsClause:
			implements = append(implements, w.clauseNames(c)...)
		case syntax.KindMemberList, syntax.KindStatementList:
			members = c
		}
	}
	if nameNode == nil {
		return
	}
	name := strings.TrimPrefix(syntax.NameText(nameNode), `\`)
	if i := strings.LastIndexByte(name, '\\'); i >= 0 {
		name = name[i+1:]
	}
	classFQN := fqn(w.b.Namespace, name)
	sym := &Symbol{
		FQN:        classFQN,
		Name:       name,
		Kind:       kind,
		Namespace:  w.b.Namespace,
		URI:        w.uri,
		Range:      syntaxSpanToRange(w.file, nameNode.Span()),
		Extends:    extends,
		Implements: implements,
		IsFinal:    mods.final,
		IsAbstract: mods.abstract,
		IsReadonly: mods.readonly,
		Visibility: "public",
	}
	w.applyLSPRange(sym, nameNode.Span())
	w.syms = append(w.syms, sym)

	prevOwner := w.b.Owner
	w.b.Owner = strings.TrimPrefix(classFQN, `\`)
	if members != nil {
		for _, m := range members.Children() {
			w.walkMember(m, classFQN, sym)
		}
	}
	w.b.Owner = prevOwner
}

func (w *syntaxSymbolWalk) clauseNames(clause *syntax.RedNode) []string {
	var out []string
	for _, c := range clause.Children() {
		switch c.Kind() {
		case syntax.KindUnqualifiedName, syntax.KindQualifiedName,
			syntax.KindFullyQualifiedName, syntax.KindRelativeName:
			resolved := w.b.BindName(c, "class")
			if resolved == "" {
				continue
			}
			out = append(out, ensureLeadingSlash(resolved))
		}
	}
	return out
}

func (w *syntaxSymbolWalk) walkMember(n *syntax.RedNode, ownerFQN string, classSym *Symbol) {
	if n == nil {
		return
	}
	switch n.Kind() {
	case syntax.KindFunctionDecl, syntax.KindMethodDecl:
		w.walkFunction(n, true)
	case syntax.KindPropertyDecl:
		w.walkProperty(n, ownerFQN)
	case syntax.KindClassConstDecl:
		w.walkClassConst(n, ownerFQN)
	case syntax.KindEnumCase:
		w.walkEnumCase(n, ownerFQN)
	case syntax.KindUseTraitClause:
		if classSym == nil {
			return
		}
		for _, name := range w.clauseNames(n) {
			classSym.Traits = append(classSym.Traits, name)
		}
	}
}

func (w *syntaxSymbolWalk) walkFunction(n *syntax.RedNode, isMethod bool) {
	mods := syntaxModifiers(n)
	var nameNode *syntax.RedNode
	var params *syntax.RedNode
	var returnType *syntax.RedNode
	seenColon := false
	for _, c := range n.Children() {
		switch c.Kind() {
		case syntax.KindUnqualifiedName:
			if nameNode == nil {
				nameNode = c
			}
		case syntax.KindParamList:
			params = c
		case syntax.KindNamedType, syntax.KindNullableType, syntax.KindUnionType,
			syntax.KindIntersectionType, syntax.KindParenthesizedType,
			syntax.KindPrimitiveType, syntax.KindCallableType:
			if seenColon {
				returnType = c
			}
		case syntax.KindToken:
			if c.Green != nil && c.Green.IsToken() {
				if tok, ok := c.Green.Token(); ok && tok.Type == token.T_COLON {
					seenColon = true
				}
			}
		}
	}
	if nameNode == nil {
		return
	}
	name := syntax.NameText(nameNode)
	kind := KindFunction
	symFQN := fqn(w.b.Namespace, name)
	if isMethod || w.b.Owner != "" {
		kind = KindMethod
		if strings.EqualFold(name, "__construct") {
			kind = KindConstructor
		}
		owner := w.b.Owner
		if owner == "" {
			owner = strings.TrimPrefix(symFQN, `\`)
		}
		symFQN = `\` + owner + `::` + name
	}
	ret := ""
	if returnType != nil {
		ret = w.resolveTypeDisplay(returnType)
	}
	sym := &Symbol{
		FQN:        symFQN,
		Name:       name,
		Kind:       kind,
		Namespace:  w.b.Namespace,
		URI:        w.uri,
		Range:      syntaxSpanToRange(w.file, nameNode.Span()),
		ReturnType: ret,
		Params:     w.extractParams(params),
		IsStatic:   mods.static,
		IsAbstract: mods.abstract,
		IsFinal:    mods.final,
		Visibility: mods.visibility,
	}
	if sym.Visibility == "" {
		sym.Visibility = "public"
	}
	w.applyLSPRange(sym, nameNode.Span())
	w.syms = append(w.syms, sym)
}

func (w *syntaxSymbolWalk) walkProperty(n *syntax.RedNode, ownerFQN string) {
	mods := syntaxModifiers(n)
	var typ *syntax.RedNode
	for _, c := range n.Children() {
		switch c.Kind() {
		case syntax.KindNamedType, syntax.KindNullableType, syntax.KindUnionType,
			syntax.KindIntersectionType, syntax.KindParenthesizedType,
			syntax.KindPrimitiveType, syntax.KindCallableType:
			if typ == nil {
				typ = c
			}
		case syntax.KindToken:
			if c.Green == nil || !c.Green.IsToken() {
				continue
			}
			tok, ok := c.Green.Token()
			if !ok || tok.Type != token.T_VARIABLE {
				continue
			}
			name := strings.TrimPrefix(tokenText(c), "$")
			if name == "" {
				continue
			}
			owner := strings.TrimPrefix(ownerFQN, `\`)
			sym := &Symbol{
				FQN:        `\` + owner + `::$` + name,
				Name:       name, // match legacy AST extract (no leading $)
				Kind:       KindProperty,
				Namespace:  w.b.Namespace,
				URI:        w.uri,
				Range:      syntaxSpanToRange(w.file, c.Span()),
				Type:       w.resolveTypeDisplay(typ),
				IsStatic:   mods.static,
				IsReadonly: mods.readonly,
				Visibility: mods.visibility,
			}
			if sym.Visibility == "" {
				sym.Visibility = "public"
			}
			w.applyLSPRange(sym, c.Span())
			w.syms = append(w.syms, sym)
		}
	}
}

func (w *syntaxSymbolWalk) walkClassConst(n *syntax.RedNode, ownerFQN string) {
	mods := syntaxModifiers(n)
	owner := strings.TrimPrefix(ownerFQN, `\`)
	for _, c := range n.Children() {
		if c.Kind() != syntax.KindUnqualifiedName {
			continue
		}
		name := syntax.NameText(c)
		if name == "" {
			continue
		}
		sym := &Symbol{
			FQN:        `\` + owner + `::` + name,
			Name:       name,
			Kind:       KindConstant,
			Namespace:  w.b.Namespace,
			URI:        w.uri,
			Range:      syntaxSpanToRange(w.file, c.Span()),
			Visibility: mods.visibility,
		}
		if sym.Visibility == "" {
			sym.Visibility = "public"
		}
		w.applyLSPRange(sym, c.Span())
		w.syms = append(w.syms, sym)
	}
}

func (w *syntaxSymbolWalk) walkEnumCase(n *syntax.RedNode, ownerFQN string) {
	owner := strings.TrimPrefix(ownerFQN, `\`)
	for _, c := range n.Children() {
		if c.Kind() != syntax.KindUnqualifiedName {
			continue
		}
		name := syntax.NameText(c)
		if name == "" {
			continue
		}
		sym := &Symbol{
			FQN:        `\` + owner + `::` + name,
			Name:       name,
			Kind:       KindEnumMember,
			Namespace:  w.b.Namespace,
			URI:        w.uri,
			Range:      syntaxSpanToRange(w.file, c.Span()),
			Visibility: "public",
		}
		w.applyLSPRange(sym, c.Span())
		w.syms = append(w.syms, sym)
		return
	}
}

func (w *syntaxSymbolWalk) walkGlobalConst(n *syntax.RedNode) {
	for _, c := range n.Children() {
		if c.Kind() != syntax.KindUnqualifiedName {
			continue
		}
		name := syntax.NameText(c)
		if name == "" {
			continue
		}
		sym := &Symbol{
			FQN:        fqn(w.b.Namespace, name),
			Name:       name,
			Kind:       KindConstant,
			Namespace:  w.b.Namespace,
			URI:        w.uri,
			Range:      syntaxSpanToRange(w.file, c.Span()),
			Visibility: "public",
		}
		w.applyLSPRange(sym, c.Span())
		w.syms = append(w.syms, sym)
	}
}

func (w *syntaxSymbolWalk) extractParams(list *syntax.RedNode) []SymbolParam {
	if list == nil {
		return nil
	}
	var out []SymbolParam
	for _, c := range list.Children() {
		if c.Kind() != syntax.KindParam {
			continue
		}
		p := SymbolParam{}
		var typ *syntax.RedNode
		for _, part := range c.Children() {
			switch part.Kind() {
			case syntax.KindNamedType, syntax.KindNullableType, syntax.KindUnionType,
				syntax.KindIntersectionType, syntax.KindParenthesizedType,
				syntax.KindPrimitiveType, syntax.KindCallableType:
				typ = part
			case syntax.KindToken:
				if part.Green == nil || !part.Green.IsToken() {
					continue
				}
				tok, ok := part.Green.Token()
				if !ok {
					continue
				}
				switch tok.Type {
				case token.T_VARIABLE:
					p.Name = tokenText(part)
				case token.T_AMPERSAND:
					p.IsPassByRef = true
				case token.T_ELLIPSIS:
					p.IsVariadic = true
				case token.T_ASSIGN:
					p.HasDefault = true
				}
			}
		}
		if typ != nil {
			p.Type = w.resolveTypeDisplay(typ)
			p.TypeFQN = simpleClassFQN(p.Type)
		}
		if p.Name != "" {
			out = append(out, p)
		}
	}
	return out
}

func (w *syntaxSymbolWalk) resolveTypeDisplay(n *syntax.RedNode) string {
	if n == nil {
		return ""
	}
	t := w.b.TypeFromSyntax(n)
	if s := t.String(); s != "" {
		return s
	}
	return syntax.TypeText(n)
}

func (w *syntaxSymbolWalk) applyLSPRange(sym *Symbol, span syntax.Span) {
	if w.file == nil || len(w.file.Lines) == 0 {
		populateLSPRange(sym)
		return
	}
	sl, sc := syntaxOffsetLineCol(w.file.Lines, span.Start)
	el, ec := syntaxOffsetLineCol(w.file.Lines, span.End)
	sym.StartLine = uint32(sl)
	sym.StartChar = uint32(sc)
	sym.EndLine = uint32(el)
	sym.EndChar = uint32(ec)
}

type syntaxMods struct {
	visibility string
	static     bool
	abstract   bool
	final      bool
	readonly   bool
}

func syntaxModifiers(n *syntax.RedNode) syntaxMods {
	var m syntaxMods
	if n == nil {
		return m
	}
	for _, c := range n.Children() {
		if c.Kind() != syntax.KindModifierList {
			continue
		}
		for _, tokNode := range c.Children() {
			lit := strings.ToLower(tokenText(tokNode))
			switch lit {
			case "public", "protected", "private":
				m.visibility = lit
			case "static":
				m.static = true
			case "abstract":
				m.abstract = true
			case "final":
				m.final = true
			case "readonly":
				m.readonly = true
			}
		}
	}
	return m
}

func tokenText(n *syntax.RedNode) string {
	if n == nil {
		return ""
	}
	if n.Green != nil && n.Green.IsToken() {
		tok, ok := n.Green.Token()
		if ok && tok.Literal != "" {
			return tok.Literal
		}
	}
	return strings.TrimSpace(n.Text())
}

func syntaxSpanToRange(file *syntax.File, span syntax.Span) Range {
	if file == nil || len(file.Lines) == 0 {
		return Range{
			Start: ast.Position{Offset: span.Start},
			End:   ast.Position{Offset: span.End},
		}
	}
	sl, sc := syntaxOffsetLineCol(file.Lines, span.Start)
	el, ec := syntaxOffsetLineCol(file.Lines, span.End)
	return Range{
		Start: ast.Position{Line: sl + 1, Column: sc + 1, Offset: span.Start},
		End:   ast.Position{Line: el + 1, Column: ec + 1, Offset: span.End},
	}
}

func syntaxOffsetLineCol(lines token.LineTable, offset int) (line, col int) {
	if len(lines) == 0 {
		return 0, offset
	}
	if offset < 0 {
		offset = 0
	}
	lo, hi := 0, len(lines)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		if lines[mid] <= offset {
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	idx := hi
	if idx < 0 {
		idx = 0
	}
	return idx, offset - lines[idx]
}

// mergeSymbolsPreferSyntax keeps syntax-extracted declarations and appends AST
// symbols whose FQN+kind are not already present (PHPDoc @method, promoted
// props, etc.). When both sides have the same key, copy AST-only enrichment
// (Traits, DocComment, Templates) onto the syntax symbol.
func mergeSymbolsPreferSyntax(syntaxSyms, astSyms []*Symbol) []*Symbol {
	if len(astSyms) == 0 {
		return syntaxSyms
	}
	if len(syntaxSyms) == 0 {
		return astSyms
	}
	byKey := make(map[string]*Symbol, len(syntaxSyms))
	out := make([]*Symbol, 0, len(syntaxSyms)+len(astSyms)/8)
	for _, s := range syntaxSyms {
		byKey[symbolMergeKey(s)] = s
		out = append(out, s)
	}
	for _, s := range astSyms {
		key := symbolMergeKey(s)
		if existing, ok := byKey[key]; ok {
			enrichSymbolFromAST(existing, s)
			continue
		}
		byKey[key] = s
		out = append(out, s)
	}
	return out
}

// enrichSymbolFromAST fills fields the syntax walk does not yet synthesize.
func enrichSymbolFromAST(dst, src *Symbol) {
	if dst == nil || src == nil {
		return
	}
	if len(dst.Traits) == 0 && len(src.Traits) > 0 {
		dst.Traits = append([]string(nil), src.Traits...)
	}
	if dst.DocComment == "" && src.DocComment != "" {
		dst.DocComment = src.DocComment
	}
	if len(dst.Templates) == 0 && len(src.Templates) > 0 {
		dst.Templates = append([]string(nil), src.Templates...)
	}
	if len(dst.GenericParents) == 0 && len(src.GenericParents) > 0 {
		dst.GenericParents = append([]GenericParent(nil), src.GenericParents...)
	}
}

func symbolMergeKey(s *Symbol) string {
	if s == nil {
		return ""
	}
	return strings.ToLower(s.FQN) + "\x00" + strconv.Itoa(int(s.Kind))
}
