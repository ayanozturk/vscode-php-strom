module github.com/ayanozturk/vscode-php-strom

go 1.23

// Pin tracks go-php-parser main after Interface NS-segment + bare count() fixes
// (v0.0.0-20260916223444-68b315370f75). Sibling override: make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260916223444-68b315370f75
