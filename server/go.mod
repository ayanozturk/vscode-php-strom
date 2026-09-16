module github.com/ayanozturk/vscode-php-strom

go 1.23

// Pin tracks go-php-parser main with body lower + ParseAST analyse cutover
// (v0.0.0-20260916175407-095c05fdb4d3). Sibling override: make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260916175407-095c05fdb4d3
