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
// `OtherEnum::CASE->value`); and a LineTable rebuild-per-diagnostic fix
// (diagnostics scan on a real 10k-file workspace: 31s -> 5s).
// Sibling override: make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260918133318-24add2acf3bb
