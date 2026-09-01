# PHP Strom

Open-source PHP language support for VS Code. A TypeScript client talks to a bundled Go language server that runs [go-php-parser](https://github.com/ayanozturk/go-php-parser) for parsing, indexing, and diagnostics.

Not a complete Intelephense/PhpStorm stand-in. Several LSP methods are advertised but still return empty results (formatting, rename, folding, code actions, code lens, inlay hints, document links, type hierarchy, implementations, highlights).

## What it does now

- Workspace indexing of PHP files, with bundled extension stubs and Composer PHP-version detection (`auto` → `composer.json`, else 8.3)
- Diagnostics from the parser analyser (syntax, undefined symbols/variables, class model, invalid calls, language, types, visibility, throws, deprecated, unreachable, empty statements, assignment-in-condition; optional side-effects and style). Toggle families under `phpstrom.diagnostics.analysis.*`
- Hover (inferred type + PHPDoc when indexed)
- Go to definition and declaration, including typed `->` / `::` members
- Go to type definition
- Signature help for calls and `new`
- Completion from the symbol index plus PHP keywords
- Document outline and workspace symbol search
- Problems view (list/tree, path/regex/error filters) and status for indexing/analysis
- Commands: restart server, clear cache, re-index, refresh problems scan

## Install

Marketplace publisher: `AOSSoftware`. Or build a VSIX locally:

```sh
make install    # Go server + extension, package, install into VS Code
# or
make package    # VSIX only
```

Needs Go 1.23+, Node.js, and the `code` CLI on `PATH` for `make install`.

## Develop

Release and local builds use the parser revision pinned in `server/go.mod`. They do not require a sibling checkout.

```sh
make build-server-local   # host language-server binary
make build-ext
make test-server
make test-ext
make test-editor-latency  # synthetic editor-path regression gate
```

To build against a local parser checkout:

```sh
make build-server-dev LOCAL_PHP_PARSER_DIR=/path/to/go-php-parser
make test-server-dev
```

## Configuration

Settings live under `phpstrom.*` in VS Code. The ones that affect behaviour today:

- `phpstrom.enable`
- `phpstrom.environment.phpVersion` / `phpVersionOverride` / `includePaths`
- `phpstrom.files.associations` / `files.exclude` / `files.maxSize`
- `phpstrom.stubs`
- `phpstrom.diagnostics.enable` / `run` / `workspaceScanOnStart` / `exclude` / `overrides`
- `phpstrom.diagnostics.analysis.*` (style and side-effects default off)

`phpstrom.diagnostics.overrides` example:

```json
{
  "phpstrom.diagnostics.overrides": {
    "PSR1.Classes.ClassDeclaration.PascalCase": {
      "classes": ["/^Legacy_/", "SpecialClass"]
    }
  }
}
```

Format, completion `use`-insert, PHPDoc generation, code lens, and inlay-hint settings exist in the schema but have no working Go providers yet.

## Packaging

One VSIX bundles language-server binaries for `darwin-arm64`, `darwin-x64`, `linux-arm64`, `linux-x64`, `win32-arm64`, and `win32-x64`.

## License

MIT. Source: https://github.com/ayanozturk/vscode-php-strom
