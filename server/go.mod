module github.com/ayanozturk/vscode-php-strom

go 1.23

// Pin tracks go-php-parser main with MethodCamelCase name spans
// (v0.0.0-20260916121358-63977fac30a6). Sibling override: make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260916121358-63977fac30a6

require gopkg.in/yaml.v2 v2.4.0 // indirect
