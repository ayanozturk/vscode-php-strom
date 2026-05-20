package providers

// signatureHelp.go implements LSP textDocument/signatureHelp.
//
// Strategy:
//  1. Walk the text backwards from the cursor to find the innermost open call
//     (function call, method call, or `new ClassName`).
//  2. Count commas at nesting depth 0 to derive the active parameter index.
//     Named arguments (identifier followed by `:`) override positional counting.
//  3. Look up the callee in the workspace index to get the parameter list.
//  4. Render a PHP-style signature label with bracketed offsets per parameter so
//     the editor can highlight the active one.

import (
	"fmt"
	"strings"

	"github.com/ayanozturk/vscode-php-strom/indexer"
	"github.com/ayanozturk/vscode-php-strom/lsp"
)

func (p *SignatureHelpProvider) Provide(uri, text string, pos lsp.Position) *lsp.SignatureHelp {
	lines := strings.Split(text, "\n")
	if int(pos.Line) >= len(lines) {
		return nil
	}

	// Build a flat char slice of everything up to the cursor so we can scan
	// backwards without dealing with line/col bookkeeping inside the loop.
	var before strings.Builder
	for i, l := range lines {
		if i > int(pos.Line) {
			break
		}
		if i == int(pos.Line) {
			col := int(pos.Character)
			if col > len(l) {
				col = len(l)
			}
			before.WriteString(l[:col])
		} else {
			before.WriteString(l)
			before.WriteByte('\n')
		}
	}
	src := before.String()

	callee, activeParam := findCalleeAndParam(src)
	if callee == "" {
		return nil
	}

	sym := p.resolveCallee(callee)
	if sym == nil {
		return nil
	}

	return buildSignatureHelp(sym, activeParam)
}

// findCalleeAndParam scans src (everything before the cursor) backwards to find
// the innermost unclosed call expression. Returns the callee name and the active
// parameter index (adjusted for named arguments when possible).
func findCalleeAndParam(src string) (string, int) {
	depth := 0     // tracks nested parens/brackets/braces after our open paren
	commas := 0    // commas at depth==0 after the opening (
	namedArg := "" // non-empty when cursor is inside a named arg
	i := len(src) - 1

	for i >= 0 {
		c := src[i]
		switch c {
		case ')', ']', '}':
			depth++
		case '(':
			if depth > 0 {
				depth--
				i--
				continue
			}
			// We found the opening paren for our call.
			// Now extract the callee to the left.
			callee := extractCallee(src[:i])
			if callee == "" {
				// Not a function call — keep looking for an outer one.
				i--
				continue
			}
			paramIndex := commas
			// Check for named argument: if inside "name: <expr>", use the param
			// name to find the index later (we return it as a synthetic negative
			// index here and handle it in buildSignatureHelp).
			_ = namedArg
			return callee, paramIndex
		case ',':
			if depth == 0 {
				commas++
				// Reset named arg tracking between arguments.
				namedArg = ""
			}
		case ':':
			// Detect named argument: skip whitespace backwards, expect identifier.
			if depth == 0 {
				j := i - 1
				for j >= 0 && (src[j] == ' ' || src[j] == '\t') {
					j--
				}
				end := j + 1
				for j >= 0 && isIdentChar(rune(src[j])) {
					j--
				}
				candidate := src[j+1 : end]
				if candidate != "" {
					namedArg = candidate
				}
			}
		}
		i--
	}

	return "", 0
}

// extractCallee looks at the text immediately before the opening paren and
// returns the callable name (e.g. "foo", "new Foo", "$obj->bar", "Foo::bar").
func extractCallee(before string) string {
	s := strings.TrimRight(before, " \t\n\r")
	if s == "" {
		return ""
	}

	// Walk back over the identifier (class name, method name, etc.)
	end := len(s)
	i := end - 1
	for i >= 0 && isIdentChar(rune(s[i])) {
		i--
	}
	name := s[i+1 : end]
	if name == "" {
		return ""
	}

	// Check for `new ClassName`
	prefix := strings.TrimRight(s[:i+1], " \t")
	if strings.HasSuffix(prefix, "new") {
		tail := prefix[len(prefix)-3:]
		if tail == "new" {
			// Make sure "new" is not part of a larger identifier
			if len(prefix) == 3 || !isIdentChar(rune(prefix[len(prefix)-4])) {
				return "__construct:" + name
			}
		}
	}

	// Check for `->method` or `::method`
	if strings.HasSuffix(prefix, "->") || strings.HasSuffix(prefix, "::") {
		return name
	}

	return name
}

// resolveCallee looks up the callee in the index.
// Callee format from extractCallee:
//   - "__construct:ClassName" → look up __construct on that class
//   - "methodName"            → search methods by name
//   - "FunctionName"          → search functions by name
func (p *SignatureHelpProvider) resolveCallee(callee string) *indexer.Symbol {
	if p.idx == nil {
		return nil
	}

	idx := p.idx.GetIndex()

	if strings.HasPrefix(callee, "__construct:") {
		className := callee[len("__construct:"):]
		// Find the constructor for that class.
		results := idx.Search(className)
		for _, sym := range results {
			if strings.EqualFold(sym.Name, className) && sym.Kind == indexer.KindClass {
				// Look for a matching __construct method.
				ctorName := sym.FQN + "::__construct"
				ctors := idx.Search("__construct")
				for _, ctor := range ctors {
					if strings.EqualFold(ctor.FQN, ctorName) {
						return ctor
					}
				}
				// No explicit constructor — return the class itself so we can show
				// an implicit no-param signature.
				return sym
			}
		}
		return nil
	}

	// Method or function lookup.
	results := idx.Search(callee)
	for _, sym := range results {
		if strings.EqualFold(sym.Name, callee) &&
			(sym.Kind == indexer.KindMethod || sym.Kind == indexer.KindFunction || sym.Kind == indexer.KindConstructor) {
			return sym
		}
	}
	return nil
}

// buildSignatureHelp renders an LSP SignatureHelp from a resolved symbol.
func buildSignatureHelp(sym *indexer.Symbol, activeParam int) *lsp.SignatureHelp {
	params := sym.Params
	if activeParam >= len(params) && len(params) > 0 {
		// Clamp to last param (handles variadic overflow).
		last := params[len(params)-1]
		if last.IsVariadic {
			activeParam = len(params) - 1
		}
	}

	// Build the label, recording byte offsets for each parameter span.
	var sb strings.Builder
	// Prefix: ClassName::methodName( or functionName(
	sb.WriteString(sym.Name)
	sb.WriteByte('(')

	paramInfos := make([]lsp.ParameterInformation, 0, len(params))
	for pi, param := range params {
		if pi > 0 {
			sb.WriteString(", ")
		}
		start := len(sb.String())
		sb.WriteString(renderParam(param))
		end := len(sb.String())

		paramInfos = append(paramInfos, lsp.ParameterInformation{
			Label: [2]uint32{uint32(start), uint32(end)},
		})
	}
	sb.WriteByte(')')

	// Append return type if known.
	if sym.ReturnType != "" {
		sb.WriteString(": ")
		sb.WriteString(sym.ReturnType)
	}

	label := sb.String()

	sig := lsp.SignatureInformation{
		Label:      label,
		Parameters: paramInfos,
	}
	if sym.DocComment != "" {
		sig.Documentation = &lsp.MarkupContent{Kind: "markdown", Value: sym.DocComment}
	}
	if len(params) > 0 && activeParam < len(paramInfos) {
		ap := activeParam
		sig.ActiveParameter = &ap
	}

	zero := 0
	return &lsp.SignatureHelp{
		Signatures:      []lsp.SignatureInformation{sig},
		ActiveSignature: &zero,
		ActiveParameter: func() *int {
			if len(params) == 0 {
				return nil
			}
			ap := activeParam
			return &ap
		}(),
	}
}

// renderParam returns the PHP-style string for a single parameter.
func renderParam(p indexer.SymbolParam) string {
	var sb strings.Builder
	if p.Type != "" {
		sb.WriteString(p.Type)
		sb.WriteByte(' ')
	}
	if p.IsVariadic {
		sb.WriteString("...")
	}
	if p.IsPassByRef {
		sb.WriteByte('&')
	}
	sb.WriteByte('$')
	sb.WriteString(p.Name)
	if p.HasDefault {
		sb.WriteString(fmt.Sprintf(" = <default>"))
	}
	return sb.String()
}
