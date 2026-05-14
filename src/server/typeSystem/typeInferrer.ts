/**
 * TypeInferrer
 *
 * Computes the PHP type of any expression node in the AST using:
 *   - Declared type annotations (parameter types, return types, property types)
 *   - PHPDoc type annotations (@param, @var, @return, @template, etc.)
 *   - Control-flow type narrowing (instanceof, is_string, assert, etc.)
 *   - Type evolving for mutable variables and array mutations
 *   - Generic type resolution (@template / @extends / @implements / @use)
 *   - Conditional return types (TSubject is TCompare ? TTrue : TFalse)
 *   - Intersection and union types
 *   - Special types: self, static, $this, parent
 *
 * The TypeInferrer is the analytical heart of phpstrom. All other providers
 * (completion, diagnostics, hover, inlay hints) delegate to it.
 */

import { PhpNode, PhpType, PhpFileAst } from '../parser/phpParser.js';

export type PhpTypeResolved =
  | { kind: 'named'; fqn: string }
  | { kind: 'union'; types: PhpTypeResolved[] }
  | { kind: 'intersection'; types: PhpTypeResolved[] }
  | { kind: 'array'; key: PhpTypeResolved; value: PhpTypeResolved }
  | { kind: 'callable'; params: PhpTypeResolved[]; returnType: PhpTypeResolved }
  | { kind: 'generic'; base: PhpTypeResolved; args: PhpTypeResolved[] }
  | { kind: 'literal'; literalType: 'string' | 'int' | 'bool'; value: string | number | boolean }
  | { kind: 'never' }
  | { kind: 'mixed' }
  | { kind: 'void' };

export class TypeInferrer {
  /**
   * Infer the type of an expression node.
   *
   * @param node     The AST node whose type to infer.
   * @param ast      The parsed file AST providing context.
   * @param scope    Optional variable scope for variable type lookups.
   */
  infer(node: PhpNode, ast: PhpFileAst, scope?: VariableScope): PhpTypeResolved {
    // TODO: implement per-node-kind inference:
    //
    //   PhpLiteralNode         → literal type
    //   PhpVariableNode        → scope.get(name) or mixed
    //   PhpPropertyAccessNode  → infer LHS type → look up property → its type
    //   PhpMethodCallNode      → infer LHS type → look up method → return type
    //   PhpNewExpression       → NamedType of the instantiated class
    //   PhpFunctionCallNode    → look up function → return type
    //   PhpStaticPropertyAccess → resolve class → property type
    //   PhpStaticMethodCall    → resolve class → method return type
    //   PhpArrowFunctionNode   → callable type with inferred param/return types
    //   PhpMatchExpression     → union of all arm return types
    //   PhpTernaryExpression   → union of both branches
    //   PhpNullCoalescing      → left type (minus null) | right type
    //   PhpCastExpression      → the cast target type
    //   PhpInstanceOf          → narrows to the checked type (boolean return)
    //   PhpArray               → array shape or TValue[] depending on content

    void node;
    void ast;
    void scope;
    return { kind: 'mixed' };
  }

  /**
   * Apply type narrowing based on a conditional expression.
   * Returns the narrowed type for the truthy branch.
   */
  narrow(
    type: PhpTypeResolved,
    condition: PhpNode,
    ast: PhpFileAst,
  ): PhpTypeResolved {
    // TODO: Handle:
    //   instanceof checks
    //   is_string / is_int / is_array / is_null / etc.
    //   @assert annotations
    //   Equality comparisons (=== null, === false)
    //   Custom @assert-if-true / @assert-if-false annotations

    void condition;
    void ast;
    return type;
  }

  /**
   * Resolve a PhpType annotation string to a PhpTypeResolved.
   * Handles FQN resolution using the current namespace and use declarations.
   */
  resolve(type: PhpType, ast: PhpFileAst): PhpTypeResolved {
    // TODO: Parse the type string, apply use-declaration aliases, resolve FQN.
    void type;
    void ast;
    return { kind: 'mixed' };
  }
}

// ─── Variable Scope ──────────────────────────────────────────────────────────

export class VariableScope {
  private readonly vars = new Map<string, PhpTypeResolved>();
  private readonly parent?: VariableScope;

  constructor(parent?: VariableScope) {
    this.parent = parent;
  }

  set(name: string, type: PhpTypeResolved): void {
    this.vars.set(name, type);
  }

  get(name: string): PhpTypeResolved | undefined {
    return this.vars.get(name) ?? this.parent?.get(name);
  }

  child(): VariableScope {
    return new VariableScope(this);
  }
}
