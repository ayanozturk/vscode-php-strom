module github.com/ayanozturk/vscode-php-strom

go 1.23

// Pin tracks go-php-parser main after braced-string / dynamic-static / member-attr lowers
// (v0.0.0-20260916203959-0ab4a387b2d3). Sibling override: make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260916203959-0ab4a387b2d3
