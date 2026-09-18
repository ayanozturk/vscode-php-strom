module github.com/ayanozturk/vscode-php-strom

go 1.23

// Pin tracks go-php-parser main, including the Phase 4 CST-direct
// analysis path (AnalysisContext.Content) and the shared-parse fix that
// collapsed each file's CST-direct pass from ~10 syntax.Parse calls to 1.
// Sibling override: make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260918092014-606a066e02e6
