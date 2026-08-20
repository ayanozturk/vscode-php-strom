package providers

import (
	"strings"

	"github.com/ayanozturk/go-php-parser/analyse"
	"github.com/ayanozturk/go-php-parser/ast"
)

// inferVariableFlowHoverType bridges flow-sensitive hover inference for the
// parser version currently pinned by the extension. It can be removed once
// that dependency includes the equivalent analyser traversal.
func inferVariableFlowHoverType(nodes []ast.Node, targetLine int, variableName string, ctx *analyse.AnalysisContext) (string, bool) {
	if variableName == "" {
		return "", false
	}
	var result string
	var walk func([]ast.Node)
	walk = func(current []ast.Node) {
		for _, node := range current {
			switch n := node.(type) {
			case *ast.NamespaceNode:
				walk(n.Body)
			case *ast.ClassNode:
				walk(n.Methods)
			case *ast.FunctionNode:
				if nodesContainVariableAtLine(n.Body, targetLine, variableName) {
					result, _ = inferVariableTypeThroughStatements(nodes, n.Body, targetLine, variableName, ctx)
				}
			}
		}
	}
	walk(nodes)
	return result, result != ""
}

func inferVariableTypeThroughStatements(allNodes, statements []ast.Node, targetLine int, variableName string, ctx *analyse.AnalysisContext) (string, bool) {
	currentType := ""
	for _, statement := range statements {
		if assignment := assignmentFromStatement(statement); assignment != nil {
			if variable, ok := assignment.Left.(*ast.VariableNode); ok && variable.Name == variableName {
				currentType = inferAssignmentRightType(allNodes, assignment.Right, ctx)
			}
		}

		if nodeContainsVariableAtLine(statement, targetLine, variableName) {
			if conditional, ok := statement.(*ast.IfNode); ok {
				if nodesContainVariableAtLine(conditional.Body, targetLine, variableName) {
					return inferVariableTypeThroughStatements(allNodes, conditional.Body, targetLine, variableName, ctx)
				}
			}
			return currentType, currentType != ""
		}

		if conditional, ok := statement.(*ast.IfNode); ok && conditional.GetPos().Line < targetLine && guardRejectsNull(conditional, variableName) {
			currentType = removeNullFromType(currentType)
		}
	}
	return currentType, currentType != ""
}

func assignmentFromStatement(node ast.Node) *ast.AssignmentNode {
	switch n := node.(type) {
	case *ast.AssignmentNode:
		return n
	case *ast.ExpressionStmt:
		assignment, _ := n.Expr.(*ast.AssignmentNode)
		return assignment
	default:
		return nil
	}
}

func inferAssignmentRightType(allNodes []ast.Node, node ast.Node, ctx *analyse.AnalysisContext) string {
	call, ok := node.(*ast.MethodCallNode)
	if !ok {
		return ""
	}
	target, ok := analyse.InferHoverTargetAtPosition(allNodes, call.GetPos().Line, call.GetPos().Column, call.Method, ctx)
	if !ok {
		return ""
	}
	return target.Type
}

func guardRejectsNull(node *ast.IfNode, variableName string) bool {
	if node == nil || node.Else != nil || len(node.ElseIfs) != 0 || !statementsTerminateForHover(node.Body) {
		return false
	}
	switch condition := node.Condition.(type) {
	case *ast.UnaryExpr:
		variable, ok := condition.Operand.(*ast.VariableNode)
		return ok && condition.Operator == "!" && variable.Name == variableName
	case *ast.BinaryExpr:
		if condition.Operator != "==" && condition.Operator != "===" {
			return false
		}
		return variableComparedWithNull(condition.Left, condition.Right, variableName)
	}
	return false
}

func variableComparedWithNull(left, right ast.Node, variableName string) bool {
	leftVariable, leftIsVariable := left.(*ast.VariableNode)
	rightVariable, rightIsVariable := right.(*ast.VariableNode)
	return leftIsVariable && leftVariable.Name == variableName && isHoverNull(right) || rightIsVariable && rightVariable.Name == variableName && isHoverNull(left)
}

func isHoverNull(node ast.Node) bool {
	switch node.(type) {
	case *ast.NullLiteral, *ast.NullNode:
		return true
	default:
		return false
	}
}

func statementsTerminateForHover(statements []ast.Node) bool {
	if len(statements) == 0 {
		return false
	}
	switch last := statements[len(statements)-1].(type) {
	case *ast.ReturnNode, *ast.ThrowNode:
		return true
	case *ast.ExpressionStmt:
		_, ok := last.Expr.(*ast.ThrowNode)
		return ok
	default:
		return false
	}
}

func removeNullFromType(raw string) string {
	if strings.HasPrefix(raw, "?") {
		return strings.TrimPrefix(raw, "?")
	}
	parts := strings.Split(raw, "|")
	filtered := parts[:0]
	for _, part := range parts {
		if !strings.EqualFold(strings.TrimSpace(part), "null") {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, "|")
}

func nodesContainVariableAtLine(nodes []ast.Node, line int, variableName string) bool {
	for _, node := range nodes {
		if nodeContainsVariableAtLine(node, line, variableName) {
			return true
		}
	}
	return false
}

func nodeContainsVariableAtLine(node ast.Node, line int, variableName string) bool {
	if node == nil {
		return false
	}
	switch n := node.(type) {
	case *ast.VariableNode:
		return n.Name == variableName && n.GetPos().Line == line
	case *ast.ExpressionStmt:
		return nodeContainsVariableAtLine(n.Expr, line, variableName)
	case *ast.AssignmentNode:
		return nodeContainsVariableAtLine(n.Left, line, variableName) || nodeContainsVariableAtLine(n.Right, line, variableName)
	case *ast.IfNode:
		return nodeContainsVariableAtLine(n.Condition, line, variableName) || nodesContainVariableAtLine(n.Body, line, variableName)
	case *ast.UnaryExpr:
		return nodeContainsVariableAtLine(n.Operand, line, variableName)
	case *ast.BinaryExpr:
		return nodeContainsVariableAtLine(n.Left, line, variableName) || nodeContainsVariableAtLine(n.Right, line, variableName)
	case *ast.MethodCallNode:
		if nodeContainsVariableAtLine(n.Object, line, variableName) {
			return true
		}
		for _, argument := range n.Args {
			if nodeContainsVariableAtLine(argument, line, variableName) {
				return true
			}
		}
	case *ast.PropertyFetchNode:
		return nodeContainsVariableAtLine(n.Object, line, variableName)
	case *ast.ReturnNode:
		return nodeContainsVariableAtLine(n.Expr, line, variableName)
	case *ast.ThrowNode:
		return nodeContainsVariableAtLine(n.Expr, line, variableName)
	}
	return false
}
