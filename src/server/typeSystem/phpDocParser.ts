/**
 * PHPDoc Parser
 *
 * Parses PHPDoc comment blocks into structured PHPDoc objects.
 *
 * Supported standard tags:
 *   @param, @return, @var, @throws, @deprecated, @see, @link, @author,
 *   @property, @property-read, @property-write, @method
 *
 * Supported non-standard / static-analysis tags:
 *   @template, @template-extends, @template-implements, @template-use
 *   @param-closure-this, @param-out
 *   @assert, @assert-if-true, @assert-if-false
 *   @mixin
 *   @disregard
 *   @type-alias, @import-type
 *   @phpstan-type, @phpstan-import-type (aliased to above)
 *   @psalm-type, @psalm-import-type (aliased to above)
 *
 * When preferPsalmPhpstanPrefixedAnnotations is true, prefixed variants
 * take precedence over un-prefixed ones.
 */

export interface ParsedPhpDoc {
  summary: string;
  description: string;
  params: PhpDocParam[];
  returnTag?: PhpDocReturn;
  varTags: PhpDocVar[];
  templateTags: PhpDocTemplate[];
  throwsTags: PhpDocThrows[];
  deprecatedTag?: PhpDocDeprecated;
  mixinTags: PhpDocMixin[];
  disregardTags: PhpDocDisregard[];
  typeAliasTags: PhpDocTypeAlias[];
  importTypeTags: PhpDocImportType[];
  propertyTags: PhpDocProperty[];
  methodTags: PhpDocMethod[];
  assertTags: PhpDocAssert[];
}

export interface PhpDocParam {
  type?: string;
  name: string;
  description?: string;
  out?: boolean;        // @param-out
  closureThis?: string; // @param-closure-this
}

export interface PhpDocReturn {
  type: string;
  description?: string;
}

export interface PhpDocVar {
  type: string;
  name?: string;
  description?: string;
}

export interface PhpDocTemplate {
  name: string;
  constraint?: string;
  defaultType?: string;
  covariant?: boolean;
  contravariant?: boolean;
}

export interface PhpDocThrows {
  type: string;
  description?: string;
}

export interface PhpDocDeprecated {
  description?: string;
}

export interface PhpDocMixin {
  type: string;
}

export interface PhpDocDisregard {
  code: string;
}

export interface PhpDocTypeAlias {
  name: string;
  type: string;
}

export interface PhpDocImportType {
  name: string;
  alias?: string;
  from?: string;
}

export interface PhpDocProperty {
  type: string;
  name: string;
  description?: string;
  readOnly: boolean;
  writeOnly: boolean;
}

export interface PhpDocMethod {
  name: string;
  returnType?: string;
  params: PhpDocParam[];
  static: boolean;
  description?: string;
}

export interface PhpDocAssert {
  type: string;
  param: string;
  conditional?: 'if-true' | 'if-false';
}

// ─── Parser ──────────────────────────────────────────────────────────────────

const EMPTY_DOC: ParsedPhpDoc = {
  summary: '',
  description: '',
  params: [],
  varTags: [],
  templateTags: [],
  throwsTags: [],
  mixinTags: [],
  disregardTags: [],
  typeAliasTags: [],
  importTypeTags: [],
  propertyTags: [],
  methodTags: [],
  assertTags: [],
};

export function parsePhpDoc(
  comment: string,
  preferPrefixed = false,
): ParsedPhpDoc {
  // TODO: Implement a proper PHPDoc lexer/parser.
  //
  // Approach:
  //   1. Strip the /** ... */ delimiters and leading * per line.
  //   2. Split into a summary line, optional description block, and tag lines.
  //   3. For each tag line, parse the tag name, optional type expression,
  //      optional parameter name, and free-form description.
  //   4. When preferPrefixed is true, collect both prefixed and un-prefixed
  //      versions and prefer the prefixed one.
  //   5. Parse complex type expressions using a recursive descent sub-parser
  //      that handles union, intersection, generic, array shape, callable,
  //      conditional, key-of, value-of, and index access types.

  void comment;
  void preferPrefixed;
  return { ...EMPTY_DOC };
}
