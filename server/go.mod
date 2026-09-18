module github.com/ayanozturk/vscode-php-strom

go 1.23

// Pin tracks go-php-parser main, including the Phase 4 CST-direct
// analysis path (AnalysisContext.Content), the shared-parse fix that
// collapsed each file's CST-direct pass from ~10 syntax.Parse calls to 1,
// and the per-class lowering memoization fix in the return-type/missing-
// types/phpdoc rules (was O(N^2) per class, ~2.7x diagnostics speedup
// measured on the symfony corpus).
// Sibling override: make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260918095519-d106816453d9
