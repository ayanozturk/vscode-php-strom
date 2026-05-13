/**
 * PhpParser
 *
 * Facade for parsing PHP source text into an AST. The actual parsing strategy
 * can be swapped out; the interface remains stable.
 *
 * Recommended backend (add as a dependency when implementing):
 *   - tree-sitter + tree-sitter-php  (fastest, incremental, WASM-friendly)
 *   - php-parser (pure JS, good PHP 8 support)
 *
 * The types below represent a minimal, language-server-friendly AST that all
 * consumers in this codebase depend on.  A real implementation would map the
 * chosen parser's native AST to these types.
 */

// ─── Position / Range ────────────────────────────────────────────────────────

export interface PhpPosition {
  line: number;   // 0-based
  column: number; // 0-based
  offset: number; // byte offset into source
}

export interface PhpRange {
  start: PhpPosition;
  end: PhpPosition;
}

// ─── Base node ───────────────────────────────────────────────────────────────

export interface PhpNode {
  kind: string;
  range: PhpRange;
  parent?: PhpNode;
}

// ─── Names & types ───────────────────────────────────────────────────────────

export interface PhpName extends PhpNode {
  kind: 'name' | 'qualified_name' | 'fully_qualified_name' | 'relative_name';
  name: string;
  /** Resolved fully-qualified name (populated during analysis). */
  resolved?: string;
}

export interface PhpType {
  raw: string;          // the exact text as written
  resolved: string;     // fully-qualified representation
  nullable: boolean;
  union: PhpType[];     // for A|B|C
  intersection: PhpType[]; // for A&B&C
}

// ─── PHPDoc ──────────────────────────────────────────────────────────────────

export interface PhpDocTag {
  tag: string;        // e.g. "@param", "@return"
  name?: string;      // e.g. "$myParam"
  type?: string;
  description?: string;
}

export interface PhpDoc {
  range: PhpRange;
  summary?: string;
  tags: PhpDocTag[];
}

// ─── Declarations ────────────────────────────────────────────────────────────

export interface PhpParam extends PhpNode {
  kind: 'parameter';
  name: string;
  type?: PhpType;
  nullable: boolean;
  variadic: boolean;
  byRef: boolean;
  defaultValue?: PhpNode;
  doc?: PhpDoc;
}

export interface PhpProperty extends PhpNode {
  kind: 'property';
  name: string;
  type?: PhpType;
  visibility: 'public' | 'protected' | 'private';
  static: boolean;
  readonly: boolean;
  doc?: PhpDoc;
}

export interface PhpConstant extends PhpNode {
  kind: 'constant' | 'class_constant';
  name: string;
  type?: PhpType;
  value?: PhpNode;
  visibility?: 'public' | 'protected' | 'private';
  doc?: PhpDoc;
}

export interface PhpMethod extends PhpNode {
  kind: 'method';
  name: string;
  params: PhpParam[];
  returnType?: PhpType;
  visibility: 'public' | 'protected' | 'private';
  static: boolean;
  abstract: boolean;
  final: boolean;
  body?: PhpNode;
  doc?: PhpDoc;
}

export interface PhpFunction extends PhpNode {
  kind: 'function';
  name: string;
  namespace?: string;
  params: PhpParam[];
  returnType?: PhpType;
  body?: PhpNode;
  doc?: PhpDoc;
}

export interface PhpClassLike extends PhpNode {
  kind: 'class' | 'interface' | 'trait' | 'enum';
  name: string;
  namespace?: string;
  fqn: string;
  extends?: string[];
  implements?: string[];
  uses?: string[];   // trait uses
  properties: PhpProperty[];
  constants: PhpConstant[];
  methods: PhpMethod[];
  abstract: boolean;
  final: boolean;
  readonly: boolean;
  doc?: PhpDoc;
}

export interface PhpUseDeclaration extends PhpNode {
  kind: 'use';
  name: string;   // fully-qualified
  alias?: string;
  useType: 'class' | 'function' | 'const';
}

export interface PhpNamespace extends PhpNode {
  kind: 'namespace';
  name: string;
  body: PhpNode[];
}

// ─── File AST ────────────────────────────────────────────────────────────────

export interface PhpFileAst {
  uri: string;
  version: number;
  errors: ParseError[];
  namespaces: PhpNamespace[];
  /** Top-level (global namespace) declarations */
  classes: PhpClassLike[];
  functions: PhpFunction[];
  constants: PhpConstant[];
  useDeclarations: PhpUseDeclaration[];
}

export interface ParseError {
  message: string;
  range: PhpRange;
}

// ─── Parser interface ────────────────────────────────────────────────────────

export interface IPhpParser {
  /**
   * Parse a PHP source string into a file AST.
   * @param uri   Document URI (for error messages and caching).
   * @param text  Full source text.
   * @param version  Document version for incremental parsing support.
   */
  parse(uri: string, text: string, version: number): PhpFileAst;

  /**
   * Apply incremental edits and re-parse only the changed ranges.
   * Falls back to a full parse if incremental is not supported.
   */
  parseIncremental(
    previous: PhpFileAst,
    text: string,
    version: number,
  ): PhpFileAst;

  /** Return the node at the given offset within the last parsed result. */
  nodeAt(ast: PhpFileAst, offset: number): PhpNode | undefined;

  /** Dispose of resources (e.g. WASM module) */
  dispose(): void;
}

// ─── Stub implementation (replace with real parser) ─────────────────────────

/**
 * StubPhpParser is a placeholder that returns an empty AST.
 * Replace with a real tree-sitter or php-parser based implementation.
 */
export class StubPhpParser implements IPhpParser {
  parse(uri: string, _text: string, version: number): PhpFileAst {
    return {
      uri,
      version,
      errors: [],
      namespaces: [],
      classes: [],
      functions: [],
      constants: [],
      useDeclarations: [],
    };
  }

  parseIncremental(previous: PhpFileAst, text: string, version: number): PhpFileAst {
    return this.parse(previous.uri, text, version);
  }

  nodeAt(_ast: PhpFileAst, _offset: number): PhpNode | undefined {
    return undefined;
  }

  dispose(): void {
    // no-op
  }
}

export const phpParser: IPhpParser = new StubPhpParser();
