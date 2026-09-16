module github.com/ayanozturk/vscode-php-strom

go 1.23

// Pin tracks go-php-parser main with widened CST→AST lower (PHPDoc/enum/trait/hooks)
// (v0.0.0-20260916160752-cd31d45675a8). Sibling override: make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260916160752-cd31d45675a8

require gopkg.in/yaml.v2 v2.4.0 // indirect
