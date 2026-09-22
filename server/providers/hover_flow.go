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
	if nodesContainVariableAtLine(nodes, targetLine, variableName) {
		result, _ = inferVariableTypeThroughStatements(nodes, nodes, targetLine, variableName, ctx)
	}
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
					result, _ = inferVariableTypeThroughStatements(nodes, n.Body, targetLine, variableName, ctx)
				}
			}
		}
	}
	walk(nodes)
	return result, result != ""
}

func inferVariableTypeThroughStatements(allNodes, statements []ast.Node, targetLine int, variableName string, ctx *analyse.AnalysisContext) (string, bool) {
	return inferVariableTypeThroughStatementsWithScope(allNodes, statements, targetLine, variableName, ctx, make(map[string]string))
}

func inferVariableTypeThroughStatementsWithScope(allNodes, statements []ast.Node, targetLine int, variableName string, ctx *analyse.AnalysisContext, variableTypes map[string]string) (string, bool) {
	currentType := ""
	for _, statement := range statements {
		if assignment := assignmentFromStatement(statement); assignment != nil {
			if variable, ok := assignment.Left.(*ast.VariableNode); ok {
				assignedType := inferAssignmentRightType(allNodes, assignment.Right, ctx, variableTypes)
				if assignedType != "" {
					variableTypes[variable.Name] = assignedType
				}
				if variable.Name == variableName {
					currentType = assignedType
				}
			}
		}

		if nodeContainsVariableAtLine(statement, targetLine, variableName) {
			if conditional, ok := statement.(*ast.IfNode); ok {
				if nodesContainVariableAtLine(conditional.Body, targetLine, variableName) {
					return inferVariableTypeThroughStatementsWithScope(allNodes, conditional.Body, targetLine, variableName, ctx, cloneHoverVariableTypes(variableTypes))
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

func cloneHoverVariableTypes(source map[string]string) map[string]string {
	cloned := make(map[string]string, len(source))
	for name, typeName := range source {
		cloned[name] = typeName
	}
	return cloned
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

func inferAssignmentRightType(allNodes []ast.Node, node ast.Node, ctx *analyse.AnalysisContext, variableTypes map[string]string) string {
	if instance, ok := node.(*ast.NewNode); ok {
		return instance.ClassName
	}
	call, ok := node.(*ast.MethodCallNode)
	if !ok {
		return ""
	}
	if receiver, ok := call.Object.(*ast.VariableNode); ok && ctx != nil && ctx.Resolver != nil {
		if receiverType := variableTypes[receiver.Name]; receiverType != "" {
			if serviceType := resolvedServiceCallType(call, receiverType, ctx); serviceType != "" {
				return serviceType
			}
			if className, single := analyse.ParseType(receiverType).SingleClassName(); single {
				if method, resolved := ctx.Resolver.ResolveMethod(className, call.Method); resolved {
					return method.ReturnType
				}
			}
		}
	}
	target, ok := analyse.InferHoverTargetAtPosition(allNodes, call.GetPos().Line, call.GetPos().Column, call.Method, ctx)
	if !ok {
		return ""
	}
	if (target.Type == "" || target.Type == "mixed") && target.ReceiverClass != "" && ctx != nil && ctx.Resolver != nil {
		if method, resolved := ctx.Resolver.ResolveMethod(target.ReceiverClass, call.Method); resolved {
			return method.ReturnType
		}
	}
	return target.Type
}

type serviceTypeResolver interface {
	ResolveServiceType(id string) (string, bool)
}

func resolvedServiceCallType(call *ast.MethodCallNode, receiverType string, ctx *analyse.AnalysisContext) string {
	if call == nil || !strings.EqualFold(call.Method, "get") || len(call.Args) == 0 || ctx == nil || ctx.Resolver == nil {
		return ""
	}
	serviceID := ""
	switch argument := call.Args[0].(type) {
	case *ast.StringNode:
		serviceID = argument.Value
	case *ast.StringLiteral:
		serviceID = argument.Value
	}
	if serviceID == "" || !isContainerResolverType(receiverType, ctx, make(map[string]struct{})) {
		return ""
	}
	resolver, ok := ctx.Resolver.(serviceTypeResolver)
	if !ok {
		return ""
	}
	serviceType, _ := resolver.ResolveServiceType(serviceID)
	return serviceType
}

func isContainerResolverType(typeName string, ctx *analyse.AnalysisContext, seen map[string]struct{}) bool {
	className, ok := analyse.ParseType(typeName).SingleClassName()
	if !ok {
		return false
	}
	className = strings.TrimPrefix(className, `\`)
	key := strings.ToLower(className)
	if key == "psr\\container\\containerinterface" || key == "symfony\\component\\dependencyinjection\\containerinterface" {
		return true
	}
	if _, duplicate := seen[key]; duplicate {
		return false
	}
	seen[key] = struct{}{}
	class, ok := ctx.Resolver.ResolveClass(className)
	if !ok {
		return false
	}
	for _, parent := range append(append([]string(nil), class.Extends...), class.Implements...) {
		if isContainerResolverType(parent, ctx, seen) {
			return true
		}
	}
	return false
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
	case *ast.FunctionCallNode:
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
