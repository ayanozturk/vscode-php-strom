module github.com/ayanozturk/vscode-php-strom

go 1.23

// Pin tracks go-php-parser main after property-hook Expr/Body + defaults lower
// (v0.0.0-20260916202054-d383dbefa434). Sibling override: make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260916202054-d383dbefa434
