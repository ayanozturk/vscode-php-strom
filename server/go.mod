module github.com/ayanozturk/vscode-php-strom

go 1.23

// Pin tracks go-php-parser main with syntax/ binder + ReferencesAt
// (v0.0.0-20260916105428-3900e855400c). Sibling override: make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260916105428-3900e855400c
