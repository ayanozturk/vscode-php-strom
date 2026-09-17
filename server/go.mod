module github.com/ayanozturk/vscode-php-strom

go 1.23

// Pin tracks go-php-parser main after syntax-parser cancellation and security gates.
// (v0.0.0-20260917072854-bad58b3279ff). Sibling override: make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260917074927-cb0663c413ed

require gopkg.in/yaml.v2 v2.4.0 // indirect
