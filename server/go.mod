module github.com/ayanozturk/vscode-php-strom

go 1.23

// Pin tracks go-php-parser main with ElseIfDeclaration style spans
// (v0.0.0-20260916120113-175f041f7902). Sibling override: make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260916120113-175f041f7902
