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

Install from the VS Code Marketplace, or build locally:

```sh
npm install
make install
```

## Configuration

Important settings include:

- `phpls.enable`
- `phpls.environment.phpVersion`
- `phpls.environment.includePaths`
- `phpls.environment.documentRoot`
- `phpls.files.exclude`
- `phpls.diagnostics.*`
- `phpls.format.*`
- `phpls.codeLens.*`
- `phpls.inlayHints.*`

See [FEATURES.md](./FEATURES.md) for the full configuration reference.

## Cross-platform Packaging

The extension bundles platform-specific language server binaries for:

- `darwin-arm64`
- `darwin-x64`
- `linux-arm64`
- `linux-x64`
- `win32-arm64`
- `win32-x64`

This allows a single published VSIX to work across supported macOS, Linux, and Windows environments.

## Build and Publish

```sh
make package
```

To publish:

```sh
export VSCE_PAT=your_marketplace_token
make publish
```

## Repository

- Source: https://github.com/ayanozturk/vscode-php-strom
- License: MIT
