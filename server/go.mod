module github.com/ayanozturk/vscode-php-strom

go 1.23

// Pin tracks go-php-parser main after classic production parse cutover
// (v0.0.0-20260916180706-f663d2698c8a). Sibling override: make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260916185757-a454cee8fb43
