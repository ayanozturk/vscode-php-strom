module github.com/ayanozturk/vscode-php-strom

go 1.23

// Pin tracks go-php-parser main, including the Phase 4 CST-direct
// analysis path (AnalysisContext.Content). Sibling override: make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260917233316-5b36388d01ff
