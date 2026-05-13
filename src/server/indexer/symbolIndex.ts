/**
 * Symbol Index
 *
 * Central in-memory store for all PHP symbols discovered during indexing.
 * Provides fast lookup by name, FQN, URI, and location.
 */

import { PhpClassLike, PhpConstant, PhpFunction, PhpFileAst } from '../parser/phpParser.js';

export type SymbolKind = 'class' | 'interface' | 'trait' | 'enum' | 'function' | 'constant' | 'method' | 'property';

export interface IndexedSymbol {
  kind: SymbolKind;
  name: string;         // unqualified
  fqn: string;          // fully-qualified name (with leading \)
  uri: string;          // file URI
  range: { start: { line: number; character: number }; end: { line: number; character: number } };
  containerFqn?: string; // for methods/properties: the containing class FQN
  doc?: string;         // extracted PHPDoc summary
}

export class SymbolIndex {
  /** Map from FQN → symbol */
  private readonly byFqn = new Map<string, IndexedSymbol>();

  /** Map from unqualified name (lower) → symbols (multiple overloads / namespaces) */
  private readonly byName = new Map<string, IndexedSymbol[]>();

  /** Map from URI → symbols declared in that file */
  private readonly byUri = new Map<string, IndexedSymbol[]>();

  // ─── Mutations ─────────────────────────────────────────────────────────────

  addFile(ast: PhpFileAst): void {
    this.removeFile(ast.uri);
    const symbols: IndexedSymbol[] = [];

    for (const cls of ast.classes) {
      const sym = classLikeToSymbol(cls, ast.uri);
      symbols.push(sym);
      for (const method of cls.methods) {
        symbols.push({
          kind: 'method',
          name: method.name,
          fqn: `${cls.fqn}::${method.name}`,
          uri: ast.uri,
          range: lspRange(method.range),
          containerFqn: cls.fqn,
          doc: method.doc?.summary,
        });
      }
      for (const prop of cls.properties) {
        symbols.push({
          kind: 'property',
          name: prop.name,
          fqn: `${cls.fqn}::$${prop.name}`,
          uri: ast.uri,
          range: lspRange(prop.range),
          containerFqn: cls.fqn,
          doc: prop.doc?.summary,
        });
      }
    }

    for (const fn of ast.functions) {
      symbols.push({
        kind: 'function',
        name: fn.name,
        fqn: fn.namespace ? `\\${fn.namespace}\\${fn.name}` : `\\${fn.name}`,
        uri: ast.uri,
        range: lspRange(fn.range),
        doc: fn.doc?.summary,
      });
    }

    for (const c of ast.constants) {
      symbols.push({
        kind: 'constant',
        name: c.name,
        fqn: `\\${c.name}`,
        uri: ast.uri,
        range: lspRange(c.range),
        doc: c.doc?.summary,
      });
    }

    for (const sym of symbols) {
      this.byFqn.set(sym.fqn, sym);
      const key = sym.name.toLowerCase();
      if (!this.byName.has(key)) this.byName.set(key, []);
      this.byName.get(key)!.push(sym);
    }
    this.byUri.set(ast.uri, symbols);
  }

  removeFile(uri: string): void {
    const existing = this.byUri.get(uri);
    if (!existing) return;
    for (const sym of existing) {
      this.byFqn.delete(sym.fqn);
      const key = sym.name.toLowerCase();
      const arr = this.byName.get(key);
      if (arr) {
        const filtered = arr.filter((s) => s.uri !== uri);
        if (filtered.length === 0) this.byName.delete(key);
        else this.byName.set(key, filtered);
      }
    }
    this.byUri.delete(uri);
  }

  clear(): void {
    this.byFqn.clear();
    this.byName.clear();
    this.byUri.clear();
  }

  // ─── Lookups ────────────────────────────────────────────────────────────────

  getByFqn(fqn: string): IndexedSymbol | undefined {
    return this.byFqn.get(fqn);
  }

  getByName(name: string): IndexedSymbol[] {
    return this.byName.get(name.toLowerCase()) ?? [];
  }

  getByUri(uri: string): IndexedSymbol[] {
    return this.byUri.get(uri) ?? [];
  }

  /**
   * Fuzzy camelCase / underscored prefix search across all symbols.
   * Returns up to `limit` results.
   */
  search(query: string, limit = 100): IndexedSymbol[] {
    if (!query) return [...this.byFqn.values()].slice(0, limit);
    const lower = query.toLowerCase();
    const results: IndexedSymbol[] = [];
    for (const sym of this.byFqn.values()) {
      if (fuzzyMatch(sym.name.toLowerCase(), lower)) {
        results.push(sym);
        if (results.length >= limit) break;
      }
    }
    return results;
  }

  get size(): number {
    return this.byFqn.size;
  }
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

function classLikeToSymbol(cls: PhpClassLike, uri: string): IndexedSymbol {
  const kindMap: Record<PhpClassLike['kind'], SymbolKind> = {
    class: 'class',
    interface: 'interface',
    trait: 'trait',
    enum: 'enum',
  };
  return {
    kind: kindMap[cls.kind],
    name: cls.name,
    fqn: cls.fqn,
    uri,
    range: lspRange(cls.range),
    doc: cls.doc?.summary,
  };
}

function lspRange(r: { start: { line: number; column: number }; end: { line: number; column: number } }) {
  return {
    start: { line: r.start.line, character: r.start.column },
    end: { line: r.end.line, character: r.end.column },
  };
}

/** Very simple fuzzy match: all characters of `needle` appear in `haystack` in order. */
function fuzzyMatch(haystack: string, needle: string): boolean {
  let hi = 0;
  for (let ni = 0; ni < needle.length; ni++) {
    const ch = needle[ni];
    const idx = haystack.indexOf(ch, hi);
    if (idx === -1) return false;
    hi = idx + 1;
  }
  return true;
}
