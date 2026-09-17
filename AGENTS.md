# Codex Instructions

This repo is the PHP Strom VS Code extension and Go language server. Parser work lives in sibling `go-php-parser`; production pin is `server/go.mod` (validate with `GOWORK=off`).

## Testing coverage (lossless parser cutover)

Do **not** chase a single line-% on the whole repo. Use layered targets.

### Must-hold (correctness gates)

- **100% lossless invariant** on fixtures + corpus: `Print(Parse*(src)) == src` for every file that lexes (parse-error cases round-trip via missing/error tokens).
- **Gold trees** for lossy hotspots: attributes, DNF types, names (qualified/relative/FQ), heredoc/nowdoc + interpolation, mixed-case keywords, trivia attachment on decls.
- **Token parity** fixtures vs `token_get_all` (kind + text) for Zend edge cases — small set, high value.

### Package-level (unit)

- `syntax` + new binder paths: aim very high (≈90–100% of *new* code).
- Lexer state machines (attribute/`#[`, encapsed, heredoc): branch-complete on new states, not “lines in lexer.go”.
- Legacy `ast` / old string parsers: coverage may fall as code is deleted — don’t maintain 100% on code being removed.

### Analyse / Strom

- Prefer **behavioral coverage**: binding, rename/refs, format identity, type-from-syntax — not line % on giant rule files.
- Keep differential/level fixtures green; add cases when string→syntax edges move.
- go-php-parser's Phase 4 added an optional `AnalysisContext.Content []byte`
  field: when set, `analyse.RunAnalysisRulesWithContext` uses a CST-direct
  fused analysis path instead of the `[]ast.Node` walk (additive; left nil,
  behavior is unchanged). `server/providers/diagnostics.go`'s
  `runAnalysisRulesForSource` does **not** set it yet — doing so today would
  break the pinned `GOWORK=off` build, since `Content` doesn't exist on the
  go-php-parser version currently pinned in `server/go.mod` (confirmed by
  trying it and reverting). Validated only via `make test-server-dev`
  against a local sibling checkout with the field. Wiring `ctx.Content =
  source` into `diagnostics.go` is a one-line follow-up, gated on bumping
  `server/go.mod`'s pin past the go-php-parser commit that adds the field
  (which itself requires pushing go-php-parser to GitHub first — needs
  explicit user confirmation, not yet requested).

### Perf (not coverage %)

- Bench gates: allocs/op, tokens/KB on Symfony/WordPress-sized inputs — regressions fail CI even if coverage is high.

### Practical CI target

- New `syntax` / trivia / binder packages: **≥95%** (or 100% if critical-unit-testing bar applies to those packages only).
- Whole-module line coverage: **non-goal** during cutover; round-trip + gold + analyse suite are the merge bar.

**Summary:** 100% on the identity/gold contract, very high on the new kernel, corpus/analyse green for integration — not “100% of go-php-parser.”
