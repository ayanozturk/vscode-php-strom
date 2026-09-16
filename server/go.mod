module github.com/ayanozturk/vscode-php-strom

go 1.23

// Pin tracks go-php-parser main after binder dynamic braced-member walk
// (v0.0.0-20260916205532-05b1bea5770d). Sibling override: make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260916205532-05b1bea5770d
