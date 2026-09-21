module github.com/ayanozturk/vscode-php-strom

go 1.23

// Pin tracks go-php-parser main. Recent notable fixes (see go-php-parser's
// own commit history for detail): Phase 4 CST-direct analysis path
// (AnalysisContext.Content); several O(N^2) performance fixes in the
// CST-direct diagnostics/lowering/indexing paths (shared-parse reuse,
// per-class lowering memoization, positionAt on long single-line files,
// ProjectUsageGraph re-indexing); a large ext/standard phpstubs coverage
// gap plus new Mbstring/PDO/ddtrace stubs; a $this-in-property-hooks
// binder fix; a reserved-word method/member name fix (e.g. `function
// declare()`); a false-positive fix for enum case values that are valid
// PHP 8.1+ constant expressions we can't statically evaluate (e.g.
// `OtherEnum::CASE->value`); a LineTable rebuild-per-diagnostic fix
// (diagnostics scan on a real 10k-file workspace: 31s -> 5s); and a
// LexAllContext token-slice pre-sizing fix (biggest single allocator on a
// real 16.6k-file workspace, -38% on its own footprint); and three parser
// correctness fixes (reserved-word class names in expression position,
// `self::$$dynamicProp`, and `yield from` nested in an expression - the
// last was silently building the wrong tree even in cases that didn't
// error, e.g. Symfony's own AmpResponseV4.php).
// Sibling override: make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260921111902-a8f401aa7d26
