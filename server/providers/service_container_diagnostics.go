package providers

import (
	"regexp"
	"strings"

	"github.com/ayanozturk/go-php-parser/analyse"
	"github.com/ayanozturk/go-php-parser/ast"

	"github.com/ayanozturk/vscode-php-strom/lsp"
)

var methodNonObjectName = regexp.MustCompile(`^Cannot call method ([A-Za-z_][A-Za-z0-9_]*)\(\) on `)

func filterResolvedServiceMethodDiagnostics(filename string, nodes []ast.Node, ctx *analyse.AnalysisContext, diagnostics []lsp.Diagnostic) []lsp.Diagnostic {
	filtered := diagnostics[:0]
	for _, diagnostic := range diagnostics {
		code, _ := diagnostic.Code.(string)
		if code != "Level2.MethodNonObject" && code != "Level8.MethodNonObject" {
			filtered = append(filtered, diagnostic)
			continue
		}
		match := methodNonObjectName.FindStringSubmatch(diagnostic.Message)
		if len(match) != 2 {
			filtered = append(filtered, diagnostic)
			continue
		}
		variable := methodReceiverVariableAtRange(nodes, diagnostic.Range, match[1])
		if variable == "" {
			filtered = append(filtered, diagnostic)
			continue
		}
		serviceType, ok := resolvedServiceVariableTypeAtLine(nodes, int(diagnostic.Range.Start.Line)+1, variable, ctx)
		if !ok || ctx == nil || ctx.Resolver == nil {
			filtered = append(filtered, diagnostic)
			continue
		}
		className, single := analyse.ParseType(serviceType).SingleClassName()
		if !single {
			filtered = append(filtered, diagnostic)
			continue
		}
		if _, resolved := ctx.Resolver.ResolveMethod(strings.TrimPrefix(className, `\`), match[1]); !resolved {
			filtered = append(filtered, diagnostic)
		}
	}
	return filtered
}

func resolvedServiceVariableTypeAtLine(nodes []ast.Node, targetLine int, variableName string, ctx *analyse.AnalysisContext) (string, bool) {
	if nodesContainVariableAtLine(nodes, targetLine, variableName) {
		if serviceType := resolvedServiceVariableTypeThroughStatements(nodes, nodes, targetLine, variableName, ctx, make(map[string]string)); serviceType != "" {
			return serviceType, true
		}
	}
	var resolved string
	var walk func([]ast.Node)
	walk = func(current []ast.Node) {
		for _, node := range current {
			switch n := node.(type) {
			case *ast.NamespaceNode:
				walk(n.Body)
			case *ast.ClassNode:
				walk(n.Methods)
			case *ast.EnumNode:
				walk(n.Methods)
			case *ast.FunctionNode:
				if nodesContainVariableAtLine(n.Body, targetLine, variableName) {
					resolved = resolvedServiceVariableTypeThroughStatements(nodes, n.Body, targetLine, variableName, ctx, make(map[string]string))
				}
			}
		}
	}
	walk(nodes)
	return resolved, resolved != ""
}

func resolvedServiceVariableTypeThroughStatements(allNodes, statements []ast.Node, targetLine int, variableName string, ctx *analyse.AnalysisContext, variableTypes map[string]string) string {
	serviceType := ""
	for _, statement := range statements {
		if assignment := assignmentFromStatement(statement); assignment != nil {
			if variable, ok := assignment.Left.(*ast.VariableNode); ok {
				assignedType := inferAssignmentRightType(allNodes, assignment.Right, ctx, variableTypes)
				if assignedType != "" {
					variableTypes[variable.Name] = assignedType
				}
				if variable.Name == variableName {
					serviceType = assignedType
				}
			}
		}
		if nodeContainsVariableAtLine(statement, targetLine, variableName) {
			return serviceType
		}
	}
	return ""
}

func methodReceiverVariableAtRange(nodes []ast.Node, diagnosticRange lsp.Range, method string) string {
	var found string
	var walk func(ast.Node)
	walk = func(node ast.Node) {
		if node == nil || found != "" {
			return
		}
		switch n := node.(type) {
		case *ast.NamespaceNode:
			for _, child := range n.Body {
				walk(child)
			}
		case *ast.ClassNode:
			for _, child := range n.Methods {
				walk(child)
			}
		case *ast.FunctionNode:
			for _, child := range n.Body {
				walk(child)
			}
		case *ast.ExpressionStmt:
			walk(n.Expr)
		case *ast.AssignmentNode:
			walk(n.Right)
		case *ast.MethodCallNode:
			if astNodeMatchesRange(n, diagnosticRange) && strings.EqualFold(n.Method, method) {
				if variable, ok := n.Object.(*ast.VariableNode); ok {
					found = variable.Name
					return
				}
			}
			walk(n.Object)
			for _, argument := range n.Args {
				walk(argument)
			}
		case *ast.FunctionCallNode:
			for _, argument := range n.Args {
				walk(argument)
			}
		case *ast.ReturnNode:
			walk(n.Expr)
		case *ast.IfNode:
			walk(n.Condition)
			for _, child := range n.Body {
				walk(child)
			}
		}
	}
	for _, node := range nodes {
		walk(node)
	}
	return found
}

func astNodeMatchesRange(node ast.Node, diagnosticRange lsp.Range) bool {
	start, end := node.GetPos(), node.GetEndPos()
	return start.Line == int(diagnosticRange.Start.Line)+1 &&
		start.Column == int(diagnosticRange.Start.Character)+1 &&
		end.Line == int(diagnosticRange.End.Line)+1 &&
		end.Column == int(diagnosticRange.End.Character)+1
}
