module github.com/ayanozturk/vscode-php-strom

go 1.23

// Pin is pre-syntax (Sep 2025). Do not invent a fake commit — bump only after
// go-php-parser with syntax/ is pushed. Until then use make test-server-dev.
// Checklist: make pin-parser-checklist
require github.com/ayanozturk/go-php-parser v0.0.0-20260909074053-60cb8d72d6b7
