# PHP Strom

PHP Strom is a high-performance, fully open-source PHP language intelligence extension for VS Code.

It provides a complete PHP language experience with code completion, go to definition, hover, references, diagnostics, formatting, code actions, inlay hints, document links, type hierarchy, and more.

## Why PHP Strom

- Fully open source under MIT
- Native bundled language server for macOS, Linux, and Windows in one VSIX
- Fast workspace indexing and incremental updates
- Rich PHP language features without a premium tier
- PHPDoc-aware analysis and configurable diagnostics

## Features

PHP Strom includes support for:

- IntelliSense and symbol-aware completion
- Signature help and hover information
- Go to definition, declaration, type definition, and implementations
- Find references and document highlights
- Workspace symbols and document symbols
- Diagnostics for undefined symbols, undefined variables, and type issues
- Formatting with configurable brace style and indentation
- Code actions, code lens, inlay hints, and folding
- Document links and type hierarchy

A full feature specification is available in [FEATURES.md](./FEATURES.md).

## Installation

Install from the VS Code Marketplace.

## Development Build

`make build-server`, `make build-server-local`, and `make test-server` no longer depend on a sibling checkout of the parser repo.

Those targets prefer a sibling `../go-php-parser` checkout when it exists. Otherwise they fetch `go-php-parser` into `.cache/go-php-parser` on first use and generate a temporary Go workspace file so the server can build without any manual local setup.

Override the source if needed:

```sh
make build-server-local PHP_PARSER_REPO=https://github.com/ayanozturk/go-php-parser.git PHP_PARSER_REF=main
```

## Configuration

Important settings include:

- `phpstrom.enable`
- `phpstrom.environment.phpVersion`
- `phpstrom.environment.includePaths`
- `phpstrom.environment.documentRoot`
- `phpstrom.files.exclude`
- `phpstrom.diagnostics.*`
- `phpstrom.diagnostics.overrides`
- `phpstrom.format.*`
- `phpstrom.codeLens.*`
- `phpstrom.inlayHints.*`

See [FEATURES.md](./FEATURES.md) for the full configuration reference.

`phpstrom.diagnostics.overrides` accepts rule-specific matchers. For example:

```json
{
	"phpstrom.diagnostics.overrides": {
		"PSR1.Classes.ClassDeclaration.PascalCase": {
			"classes": ["/^Legacy_/", "SpecialClass"]
		}
	}
}
```

Matching classes are ignored by the scanner for that diagnostic code.

## Cross-platform Packaging

The extension bundles platform-specific language server binaries for:

- `darwin-arm64`
- `darwin-x64`
- `linux-arm64`
- `linux-x64`
- `win32-arm64`
- `win32-x64`

This allows a single VSIX to work across supported macOS, Linux, and Windows environments.

## Repository

- Source: https://github.com/ayanozturk/vscode-php-strom
- License: MIT
