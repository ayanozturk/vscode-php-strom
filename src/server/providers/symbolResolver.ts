import { Position } from 'vscode-languageserver/node';
import { TextDocument } from 'vscode-languageserver-textdocument';

import { IndexedSymbol, SymbolIndex } from '../indexer/symbolIndex.js';

interface UseImport {
  alias: string;
  fqn: string;
  useType: 'class' | 'function' | 'const';
}

export function resolveSymbolsAtPosition(
  index: SymbolIndex,
  doc: TextDocument,
  position: Position,
): IndexedSymbol[] {
  const token = symbolAt(doc, position);
  if (!token) return [];

  const candidates = candidateFqns(doc.getText(), doc.offsetAt(position), token);
  const exactMatches = candidates
    .map((fqn) => index.getByFqn(fqn))
    .filter((symbol): symbol is IndexedSymbol => symbol !== undefined);

  if (exactMatches.length > 0) {
    return exactMatches;
  }

  const symbols = index.getByName(lastSegment(token));
  if (symbols.length <= 1) {
    return symbols;
  }

  const namespace = currentNamespace(doc.getText(), doc.offsetAt(position));
  return rankSymbols(symbols, namespace);
}

function candidateFqns(text: string, offset: number, token: string): string[] {
  const imports = collectUseImports(text, offset);
  const namespace = currentNamespace(text, offset);
  const normalized = normalizeToken(token);
  const candidates: string[] = [];

  if (token.startsWith('\\')) {
    candidates.push(token);
    return unique(candidates);
  }

  const parts = normalized.split('\\').filter(Boolean);
  if (parts.length === 0) return [];

  if (parts.length > 1) {
    const aliasTarget = imports.find((entry) => entry.alias.toLowerCase() === parts[0].toLowerCase());
    if (aliasTarget) {
      candidates.push(joinFqn(aliasTarget.fqn, parts.slice(1)));
    }
    candidates.push(`\\${normalized}`);
    if (namespace) {
      candidates.push(`\\${namespace}\\${normalized}`);
    }
    return unique(candidates);
  }

  const directImport = imports.find((entry) => entry.alias.toLowerCase() === normalized.toLowerCase());
  if (directImport) {
    candidates.push(directImport.fqn);
  }
  if (namespace) {
    candidates.push(`\\${namespace}\\${normalized}`);
  }
  candidates.push(`\\${normalized}`);

  return unique(candidates);
}

function collectUseImports(text: string, offset: number): UseImport[] {
  const imports: UseImport[] = [];
  const source = text.slice(0, offset);
  const usePattern = /^\s*use\s+(?:(function|const)\s+)?([^;]+);/gm;

  for (const match of source.matchAll(usePattern)) {
    const useType = (match[1] as UseImport['useType'] | undefined) ?? 'class';
    const clause = match[2]?.trim();
    if (!clause) continue;

    for (const item of expandUseClause(clause)) {
      const parsed = parseUseItem(item, useType);
      if (parsed) {
        imports.push(parsed);
      }
    }
  }

  return imports;
}

function expandUseClause(clause: string): string[] {
  const trimmed = clause.trim();
  const groupStart = trimmed.indexOf('{');
  const groupEnd = trimmed.lastIndexOf('}');
  if (groupStart === -1 || groupEnd === -1 || groupEnd < groupStart) {
    return splitTopLevel(trimmed);
  }

  const prefix = trimmed.slice(0, groupStart).trim().replace(/\\$/, '');
  const groupBody = trimmed.slice(groupStart + 1, groupEnd);
  return splitTopLevel(groupBody).map((segment) => `${prefix}\\${segment.trim()}`);
}

function splitTopLevel(input: string): string[] {
  const parts: string[] = [];
  let current = '';
  let depth = 0;

  for (const ch of input) {
    if (ch === '{') depth++;
    if (ch === '}') depth--;
    if (ch === ',' && depth === 0) {
      if (current.trim().length > 0) parts.push(current.trim());
      current = '';
      continue;
    }
    current += ch;
  }

  if (current.trim().length > 0) {
    parts.push(current.trim());
  }
  return parts;
}

function parseUseItem(item: string, defaultUseType: UseImport['useType']): UseImport | undefined {
  const match = item.match(/^(?:(function|const)\s+)?(.+?)(?:\s+as\s+([A-Za-z_][A-Za-z0-9_]*))?$/i);
  if (!match) return undefined;

  const useType = (match[1]?.toLowerCase() as UseImport['useType'] | undefined) ?? defaultUseType;
  const rawName = normalizeToken(match[2] ?? '');
  if (!rawName) return undefined;

  const alias = match[3] ?? lastSegment(rawName);
  return {
    alias,
    fqn: `\\${rawName}`,
    useType,
  };
}

function currentNamespace(text: string, offset: number): string | undefined {
  const source = text.slice(0, offset);
  const namespacePattern = /^\s*namespace\s+([^;{\s]+(?:\\[^;{\s]+)*)\s*[;{]/gm;
  let namespace: string | undefined;

  for (const match of source.matchAll(namespacePattern)) {
    namespace = match[1]?.trim();
  }

  return namespace;
}

function rankSymbols(symbols: IndexedSymbol[], namespace?: string): IndexedSymbol[] {
  return [...symbols].sort((left, right) => scoreSymbol(right, namespace) - scoreSymbol(left, namespace));
}

function scoreSymbol(symbol: IndexedSymbol, namespace?: string): number {
  let score = 0;
  if (namespace && symbol.fqn.startsWith(`\\${namespace}\\`)) {
    score += 10;
  }
  if (symbol.kind === 'class' || symbol.kind === 'interface' || symbol.kind === 'trait' || symbol.kind === 'enum') {
    score += 2;
  }
  if (!symbol.fqn.includes('::')) {
    score += 1;
  }
  return score;
}

function symbolAt(doc: TextDocument, pos: Position): string {
  const text = doc.getText();
  const offset = doc.offsetAt(pos);
  let start = offset;
  let end = offset;

  while (start > 0 && /[A-Za-z0-9_\\]/.test(text[start - 1])) start--;
  while (end < text.length && /[A-Za-z0-9_\\]/.test(text[end])) end++;

  return text.slice(start, end);
}

function normalizeToken(token: string): string {
  return token.trim().replace(/^\\+/, '').replace(/\\+/g, '\\');
}

function lastSegment(name: string): string {
  const normalized = normalizeToken(name);
  const parts = normalized.split('\\').filter(Boolean);
  return parts[parts.length - 1] ?? normalized;
}

function joinFqn(prefix: string, suffix: string[]): string {
  const base = normalizeToken(prefix);
  return `\\${[base, ...suffix].join('\\')}`;
}

function unique(values: string[]): string[] {
  return [...new Set(values.filter((value) => value.length > 0))];
}