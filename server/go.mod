module github.com/ayanozturk/vscode-php-strom

go 1.23

// Pin tracks go-php-parser main, including the Phase 4 CST-direct
// analysis path (AnalysisContext.Content), the shared-parse fix that
// collapsed each file's CST-direct pass from ~10 syntax.Parse calls to 1,
// the per-class lowering memoization fix in the return-type/missing-
// types/phpdoc rules (was O(N^2) per class, ~2.7x diagnostics speedup
// measured on the symfony corpus), and the positionAt O(N^2) fix for long
// single-line files (generated vendor files, e.g. AWS SDK API definitions,
// shaped as one huge array literal - 103s -> 45ms to lower a real 1.4MB
// example; indexing a real 10k-file workspace with such files went from
// 2m8s to 2.3s), a large ext/standard phpstubs coverage gap (326 missing
// functions) plus new Mbstring/PDO stubs, a $this-in-property-hooks binder
// fix, a reserved-word method/member name fix (e.g. `function declare()`),
// and an O(N^2) ProjectUsageGraph re-indexing fix (2m34s -> 2.7s to
// re-index a real 10k-file workspace, e.g. after a diagnostics-level
// config change).
// Sibling override: make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260918120802-e36ec3bf010a
