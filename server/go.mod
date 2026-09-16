module github.com/ayanozturk/vscode-php-strom

go 1.23

// Pin tracks go-php-parser main with declaration-tier CST→AST lower
// (v0.0.0-20260916155546-274b6ce5fbbb). Sibling override: make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260916155546-274b6ce5fbbb

require gopkg.in/yaml.v2 v2.4.0 // indirect
