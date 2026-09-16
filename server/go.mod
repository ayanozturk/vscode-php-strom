module github.com/ayanozturk/vscode-php-strom

go 1.23

// Pin tracks go-php-parser main after keyword-member / named-match / switch-case fixes
// (v0.0.0-20260916210721-137c105b043e). Sibling override: make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260916210721-137c105b043e
