# phpls — PHP Strom: Feature Specification

> A fully open-source, MIT-licensed, high-performance PHP Strom for VS Code and any LSP-capable editor.  
> All features listed here are **free**. There is no premium tier.

---

## Table of Contents

- [phpls — PHP Strom: Feature Specification](#phpls--php-strom-feature-specification)
  - [Table of Contents](#table-of-contents)
  - [1. Architecture Overview](#1-architecture-overview)
  - [2. Free Features](#2-free-features)
    - [2.1 Code Completion (IntelliSense)](#21-code-completion-intellisense)
    - [2.2 Signature Help](#22-signature-help)
    - [2.3 Hover](#23-hover)
    - [2.4 Go to Definition](#24-go-to-definition)
    - [2.5 Find All References](#25-find-all-references)
    - [2.6 Document Highlight](#26-document-highlight)
    - [2.7 Document Symbols](#27-document-symbols)
    - [2.8 Workspace Symbol Search](#28-workspace-symbol-search)
    - [2.9 Diagnostics](#29-diagnostics)
      - [Syntax errors](#syntax-errors)
      - [Undefined symbols](#undefined-symbols)
      - [Undefined variables](#undefined-variables)
      - [Type errors (configurable strictness)](#type-errors-configurable-strictness)
      - [Other checks](#other-checks)
      - [Suppression](#suppression)
    - [2.10 Formatting](#210-formatting)
    - [2.11 Embedded Languages](#211-embedded-languages)
    - [2.12 Inline Values](#212-inline-values)
  - [3. Premium-Equivalent Features](#3-premium-equivalent-features)
    - [3.1 Rename Symbol](#31-rename-symbol)
    - [3.2 Code Folding](#32-code-folding)
    - [3.3 Find All Implementations](#33-find-all-implementations)
    - [3.4 Go to Type Definition](#34-go-to-type-definition)
    - [3.5 Go to Declaration](#35-go-to-declaration)
    - [3.6 Smart Select](#36-smart-select)
    - [3.7 Code Actions](#37-code-actions)
    - [3.8 Type Hierarchy](#38-type-hierarchy)
    - [3.9 Code Lens](#39-code-lens)
    - [3.10 Inlay Hints](#310-inlay-hints)
    - [3.11 Document Links](#311-document-links)
    - [3.12 @mixin Support](#312-mixin-support)
  - [4. Type System](#4-type-system)
    - [4.1 Supported Types](#41-supported-types)
      - [Scalar](#scalar)
      - [Unit](#unit)
      - [Top / Bottom](#top--bottom)
      - [Literals _(PHPDoc only)_](#literals-phpdoc-only)
      - [Object](#object)
      - [Array](#array)
      - [Callable](#callable)
      - [Alias](#alias)
      - [Compound](#compound)
      - [Generic _(PHPDoc only)_](#generic-phpdoc-only)
      - [Advanced _(PHPDoc only)_](#advanced-phpdoc-only)
    - [4.2 Type Narrowing](#42-type-narrowing)
    - [4.3 Type Evolving](#43-type-evolving)
    - [4.4 PHPDoc Annotation Support](#44-phpdoc-annotation-support)
  - [5. Beyond](#5-beyond)
    - [5.1 Tree-sitter Parser Backend](#51-tree-sitter-parser-backend)
    - [5.2 Framework \& Library Intelligence](#52-framework--library-intelligence)
    - [5.3 Improved Diagnostics](#53-improved-diagnostics)
    - [5.4 Auto-PHPDoc Generation](#54-auto-phpdoc-generation)
    - [5.5 Refactoring Actions](#55-refactoring-actions)
    - [5.6 AI-Assisted Completions (Optional)](#56-ai-assisted-completions-optional)
    - [5.7 Test Integration](#57-test-integration)
    - [5.8 Better Multi-root Workspace Support](#58-better-multi-root-workspace-support)
  - [6. Configuration Reference](#6-configuration-reference)
  - [7. Diagnostic Codes](#7-diagnostic-codes)
  - [8. Extension Points / API](#8-extension-points--api)

---

## 1. Architecture Overview

```
VS Code
  └── src/extension.ts          (LSP client — vscode-languageclient)
        │
        │ IPC / stdio / socket
        ▼
  src/server/server.ts          (LSP server — vscode-languageserver)
        ├── parser/
        │     └── phpParser.ts  (IPhpParser — tree-sitter-php backend)
        ├── indexer/
        │     ├── workspaceIndexer.ts  (file discovery, incremental re-index)
        │     └── symbolIndex.ts      (in-memory FQN → symbol store)
        ├── typeSystem/
        │     ├── typeInferrer.ts     (expression type resolution)
        │     └── phpDocParser.ts     (PHPDoc tag & type parser)
        └── providers/
              ├── completionProvider.ts
              ├── definitionProvider.ts
              ├── hoverProvider.ts
              ├── diagnosticsProvider.ts
              ├── referencesProvider.ts
              ├── symbolProvider.ts
              ├── signatureHelpProvider.ts
              ├── formattingProvider.ts
              ├── highlightProvider.ts
              ├── renameProvider.ts
              ├── foldingProvider.ts
              ├── implementationProvider.ts
              ├── typeDefinitionProvider.ts
              ├── declarationProvider.ts
              ├── selectionRangeProvider.ts
              ├── codeActionProvider.ts
              ├── codeLensProvider.ts
              ├── inlayHintsProvider.ts
              ├── documentLinksProvider.ts
              └── typeHierarchyProvider.ts
```

**Key technology choices:**

| Concern | Choice | Rationale |
|---|---|---|
| Language | TypeScript | Matches VS Code ecosystem; strong typing |
| Parser | [tree-sitter-php](https://github.com/tree-sitter/tree-sitter-php) | Fastest incremental parser, error-tolerant, WASM |
| Stubs | [phpstorm-stubs](https://github.com/JetBrains/phpstorm-stubs) | Most complete built-in PHP symbol definitions |
| HTML/CSS/JS formatting | [js-beautify](https://github.com/beautify-web/js-beautify) | Handles embedded languages in `.php` files |
| File glob | [fast-glob](https://github.com/mrmlnc/fast-glob) | Sub-millisecond workspace file discovery |
| Transport | IPC (dev), stdio (prod) | IPC avoids encoding overhead during development |

---

## 2. Free Features

### 2.1 Code Completion (IntelliSense)

**LSP method:** `textDocument/completion` + `completionItem/resolve`  
**Trigger characters:** `$  >  :  \  /  '  "  *  .  <`  
**Keybinding:** `Ctrl+Space`

Completion suggestions for:

- **Variables** — all variables in the current scope, typed with inferred types
- **Class members** — properties and methods on typed expressions after `->` and `::`
- **Type names** — classes, interfaces, traits, enums (camelCase and underscore-case fuzzy match)
- **Functions & constants** — global, namespaced, and built-in
- **PHP keywords** — context-aware (e.g. no `return` at class level)
- **PHPDoc tags** — inside `/** */` comment blocks
- **Namespace segments** — after `\` in use declarations and FQNs
- **Array keys** — for typed array shapes
- **Named arguments** — `paramName:` syntax for PHP 8+
- **Match arm values** — for enum-typed match subjects
- **Attribute classes** — after `#[`
- **Automatic `use` insertion** — adds the import when completing a non-imported type

Each completion item includes:

- Full signature / type detail
- PHPDoc documentation (resolved lazily via `completionItem/resolve`)
- Sort text adjusted by relevance (prefix match, camelCase score)

---

### 2.2 Signature Help

**LSP method:** `textDocument/signatureHelp`  
**Trigger characters:** `(  ,  :`  
**Keybinding:** `Ctrl+Shift+Space`

- Full parameter list with types and documentation for functions and methods
- Active parameter highlighted as the cursor moves through arguments
- Multiple overloads shown when applicable
- Supports constructor calls (`new ClassName(...)`)
- Named argument awareness (`param: value` skips to the correct parameter)

---

### 2.3 Hover

**LSP method:** `textDocument/hover`  
**Keybinding:** `Ctrl+K Ctrl+I` / mouse-over

Hover card content:

- Full signature with types for functions, methods, and constructors
- Inferred or declared type for variables, properties, constants
- PHPDoc summary and description (rendered as Markdown)
- Link to [php.net manual](https://www.php.net) for built-in symbols
- Deprecation notice when `@deprecated` is set

---

### 2.4 Go to Definition

**LSP method:** `textDocument/definition`  
**Keybinding:** `F12` / right-click

- Navigates to the declaration of any symbol (class, function, method, property, constant, variable)
- For `new ClassName(...)` — returns both the class declaration and the constructor
- Resolves `use` aliases transparently
- Supports multi-definition responses (peek window in VS Code)

---

### 2.5 Find All References

**LSP method:** `textDocument/references`  
**Keybinding:** `Shift+F12` / right-click

- Workspace-wide reference search
- Hierarchy-aware: references to a base method include overrides
- Can optionally include the declaration itself (`includeDeclaration`)
- References to trait members include all classes using the trait

---

### 2.6 Document Highlight

**LSP method:** `textDocument/documentHighlight`  
**Trigger:** automatic at cursor position

- Highlights all references to the symbol under the cursor within the current file
- Distinguishes **read** and **write** contexts with different highlight colours
- Works for variables, class members, function parameters, and type names

---

### 2.7 Document Symbols

**LSP method:** `textDocument/documentSymbol`  
**Keybinding:** `Ctrl+Shift+O`

Hierarchical symbol tree powering:

- Outline view
- Breadcrumb navigation
- `@` search in the Go to Symbol dropdown

Symbols reported:

- Namespaces → classes → methods / properties / constants
- Top-level functions and constants
- `use` declaration groups

---

### 2.8 Workspace Symbol Search

**LSP method:** `workspace/symbol`  
**Keybinding:** `Ctrl+T`

- Camel/underscore case fuzzy search across all indexed symbols
- FQSEN query syntax (e.g. `A\M\C::m(` finds `App\Models\Comment::method(`)
- Sub-millisecond results via the in-memory SymbolIndex

---

### 2.9 Diagnostics

**LSP method:** `textDocument/publishDiagnostics`  
**Run:** `onType` (default) or `onSave`

#### Syntax errors

- Error-tolerant parser continues past errors and reports all issues

#### Undefined symbols

- Undefined classes, interfaces, traits, enums
- Undefined functions
- Undefined constants

#### Undefined variables

- Variables used before assignment
- Variable scope boundary violations

#### Type errors (configurable strictness)

| Setting | Default | Effect |
|---|---|---|
| `diagnostics.typeErrors` | `true` | Enable type checking |
| `diagnostics.relaxedTypeCheck` | `true` | Allow super-types for sub-type constraints |
| `diagnostics.noMixedTypeCheck` | `true` | Suppress mixed→narrower errors |
| `diagnostics.strictTypes` | `false` | Treat all files as strict_types=1 |
| `diagnostics.typeCheckDocumentedTypes` | `false` | Include PHPDoc types in checks |

#### Other checks

- Deprecated symbol usage (based on `@deprecated` PHPDoc)
- Duplicate class / function declarations
- Missing return statement
- Unreachable code after `throw` / `return` / `exit`

#### Suppression

```php
/** @disregard P1010 */
$undefined->method(); // suppressed
```

---

### 2.10 Formatting

**LSP methods:** `textDocument/formatting` + `textDocument/rangeFormatting`  
**Keybinding:** `Ctrl+Shift+I` (document) / `Ctrl+K Ctrl+F` (selection)

- **PHP regions:** PSR-12 / PER Coding Style compliant
- **Mixed files:** also formats embedded HTML, CSS, and JavaScript
- Brace style configurable: `per` (default), `allman`, `k&r`
- Lossless: only edits what needs changing, preserving blank lines and comments

---

### 2.11 Embedded Languages

PHP files may contain HTML, CSS, and JavaScript.  phpls provides:

- Syntax highlighting delegation (via VS Code's built-in grammar contributions)
- Code completion for HTML attributes and tags within PHP template files
- CSS class and ID completions within `<style>` blocks
- Basic JavaScript completions within `<script>` blocks
- Formatting of all embedded regions

---

### 2.12 Inline Values

**LSP method:** `textDocument/inlineValues`  
**Trigger:** automatic during an active debug session

Provides variable ranges to the Xdebug extension (`xdebug.php-debug`) so that current variable values are displayed inline in the editor during debugging.

---

## 3. Premium-Equivalent Features

> Other extensions require a paid licence. **php-strom ships them all for free.**

---

### 3.1 Rename Symbol

**LSP methods:** `textDocument/prepareRename` + `textDocument/rename`  
**Keybinding:** `F2` / right-click

- Rename classes, interfaces, traits, enums, methods, properties, constants, variables
- **PSR-4 file rename:** renaming a class also renames the file and updates all `use` references
- **Namespace rename:** updates all FQN references and `use` declarations throughout the workspace
- Alias-only rename when the symbol is locally imported with `use … as Alias`
- Type-hierarchy-aware: renaming a method renames the whole hierarchy

---

### 3.2 Code Folding

**LSP method:** `textDocument/foldingRange`  
**Keybinding:** `Ctrl+Shift+[` / `Ctrl+Shift+]`

Syntax-tree driven (more reliable than indent-based):

- Function / method / class bodies
- Control structures (if, for, foreach, while, switch, match, try/catch)
- PHPDoc and block comments
- `use` declaration groups
- Heredoc / nowdoc
- Custom `#region` / `#endregion` markers
- Array literals and match expressions

---

### 3.3 Find All Implementations

**LSP method:** `textDocument/implementation`  
**Keybinding:** `Ctrl+F12` / right-click

- All classes that implement an interface
- All concrete sub-classes of an abstract class
- All methods that override an abstract or interface method
- Trait usages (all classes that `use` a trait)

---

### 3.4 Go to Type Definition

**LSP method:** `textDocument/typeDefinition`  
**Keybinding:** right-click

- Navigates to the class/interface declaration of a typed variable or parameter
- Handles union types — offers multiple navigation targets

---

### 3.5 Go to Declaration

**LSP method:** `textDocument/declaration`  
**Keybinding:** right-click

- Navigates to the _initial_ (root) declaration in a type hierarchy
- Invoking on an overriding method → jumps to the abstract/interface declaration

---

### 3.6 Smart Select

**LSP method:** `textDocument/selectionRange`  
**Keybinding:** `Shift+Alt+→` (expand) / `Shift+Alt+←` (shrink)

Syntax-tree driven expansion sequence:

```
word → identifier → expression → statement → block → method → class → file
```

More precise than indent or regex-based providers.

---

### 3.7 Code Actions

**LSP method:** `textDocument/codeAction` + `codeAction/resolve`  
**Keybinding:** `Ctrl+.` / lightbulb icon

| Kind | Action |
|---|---|
| `quickfix` | **Import symbol** — add `use` declaration for undefined class/function/const |
| `quickfix` | **Create missing method** — generate stub for called-but-undefined method |
| `refactor` | **Add PHPDoc** — generate doc block with inferred types and `@throws` |
| `refactor` | **Implement all abstract methods** — generate stubs for unimplemented methods |
| `refactor` | **Extract method** — extract selection into a new method |
| `refactor` | **Convert to named argument** — `fn($a, $b)` → `fn(first: $a, second: $b)` |
| `source.organizeImports` | **Organise use declarations** — sort, deduplicate, remove unused |

---

### 3.8 Type Hierarchy

**LSP methods:** `textDocument/typeHierarchyPrepare` + `typeHierarchy/supertypes` + `typeHierarchy/subtypes`  
**Keybinding:** right-click → Show Type Hierarchy

- Navigable super-type and sub-type trees for classes, interfaces, traits, and enums
- Lazy loading: each node is expanded on demand

---

### 3.9 Code Lens

**LSP method:** `textDocument/codeLens` + `codeLens/resolve`  
**Trigger:** inline above declarations (disabled by default, configure via `phpls.codeLens.*`)

| Lens | Description |
|---|---|
| **References** | Count of workspace references → click to view |
| **Implementations** | Count of implementations → click to view |
| **Overrides** | Count of overriding methods → click to view |
| **Parent** | "overrides ParentClass::method" → click to navigate |
| **Usages** | Count of trait usages → click to view |

---

### 3.10 Inlay Hints

**LSP method:** `textDocument/inlayHint`  
**Trigger:** inline (enabled by default, configure via `phpls.inlayHints.*`)

| Hint | Example |
|---|---|
| **Parameter name** | `createUser(/*name:*/ 'Alice', /*age:*/ 30)` |
| **Closure parameter type** | `array_map(fn(/*int*/ $n) => $n * 2, $ints)` |
| **Inferred return type** | `function greet() /*: string*/ { … }` |

---

### 3.11 Document Links

**LSP method:** `textDocument/documentLink`  
**Keybinding:** `Ctrl+Click` / mouse-over

Clickable links for:

- `require` / `require_once` / `include` / `include_once` paths
- `@see` PHPDoc annotations referencing local files
- Respects `phpls.environment.documentRoot` for document-root-relative paths

---

### 3.12 @mixin Support

Classes that delegate to another class via `__call`, `__callStatic`, `__get`, or `__set` can annotate the mixin with `@mixin ClassName`.  phpls merges the mixin's members into the containing class for completion, hover, and definition navigation.

---

## 4. Type System

### 4.1 Supported Types

#### Scalar

`int`  `float`  `bool`  `string`

#### Unit

`void`  `null`  `true`  `false`  `never`

#### Top / Bottom

`mixed`  `never`

#### Literals _(PHPDoc only)_

`'string'`  `42`  `true`  `false`

#### Object

`object`  `ClassName`  `self`  `static`  `$this`  
`object{name: string, optional?: string}` _(PHPDoc only)_

#### Array

`array`  `TValue[]`  `array<TKey, TValue>`  
`array{key: Type, optional?: Type, ...<int, string>}` _(PHPDoc only)_

#### Callable

`callable`  `callable(TParam): TReturn`  `Closure`  `Closure(TParam): TReturn` _(PHPDoc only)_

#### Alias

`iterable`  `?Type` (nullable shorthand)

#### Compound

`A|B|C` (union)  `A&B&C` (intersection)  `A|B|(C&D)` (DNF)

#### Generic _(PHPDoc only)_

`ClassName<TypeArg1, TypeArg2>`  
Full support for `@template` / `@extends` / `@implements` / `@use`  
All built-in generic types: `Generator`, `Iterator`, `ArrayAccess`, `SplStack`, etc.

#### Advanced _(PHPDoc only)_

`(TSubject is TCompare ? TTrue : TFalse)` — conditional return types  
`key-of<TArray>`  `value-of<TArray>`  `TArray[TKey]` — index access  
`class-string<T>`  `resource`

---

### 4.2 Type Narrowing

Control-flow analysis narrows union types on each branch:

```php
function example(string|array|Foo|null $input): void {
    if ($input instanceof Foo) {
        // narrowed to Foo
    } elseif (is_string($input)) {
        // narrowed to string
    } elseif ($input) {
        // narrowed to array (non-empty)
    } else {
        // narrowed to string|array|null (falsey paths)
    }
}
```

**Narrowing triggers:**

- `instanceof` checks
- `is_*` type assertions (`is_string`, `is_int`, `is_array`, `is_null`, …)
- `assert()` expressions
- Equality comparisons (`=== null`, `=== false`, `!==`)
- Custom `@assert`, `@assert-if-true`, `@assert-if-false` annotations

---

### 4.3 Type Evolving

Variables evolve their type after assignment:

```php
$a = 'hello';   // string
$a = 42;        // int (evolved)

$b = [];        // evolving array
$b[] = 'foo';   // string[]
$b[] = 1;       // (string|int)[]

$c = [1, 2];    // int[] (NOT evolving — non-empty initialiser)
$c[] = 'foo';   // still int[]
```

Properties are only evolved within the declared/annotated type bounds.

---

### 4.4 PHPDoc Annotation Support

All standard PHPDoc tags are supported. Non-standard extensions:

| Tag | Purpose |
|---|---|
| `@template T of Constraint = Default` | Declare generic type parameter |
| `@template-extends Parent<T>` | Specify type arguments for extended parent |
| `@template-implements Interface<T>` | Specify type arguments for implemented interface |
| `@template-use Trait<T>` | Specify type arguments for used trait |
| `@param-closure-this Type $param` | Declare `$this` type inside a closure parameter |
| `@param-out Type &$param` | Declare out-type of a by-reference parameter |
| `@assert Type $param` | Declare a type assertion function |
| `@assert-if-true Type $param` | Assert type on the `true` branch of a boolean return |
| `@assert-if-false Type $param` | Assert type on the `false` branch of a boolean return |
| `@mixin ClassName` | Merge another class's members via magic methods |
| `@disregard PXXXX` | Suppress a specific diagnostic code |
| `@type-alias Name = Type` | Declare a file-scoped type alias |
| `@import-type Name as Alias` | Import a type alias from another file |

Psalm-prefixed (`@psalm-*`) and PHPStan-prefixed (`@phpstan-*`) variants are recognised and can be preferred via `phpls.compatibility.preferPsalmPhpstanPrefixedAnnotations`.

---

## 5. Beyond

### 5.1 Tree-sitter Parser Backend

phpls uses [tree-sitter-php](https://github.com/tree-sitter/tree-sitter-php) as its parser backend:

- **Incremental parsing** — only re-parses changed ranges, not the entire file
- **Error recovery** — continues building a useful AST even with syntax errors
- **WASM support** — can run in browser-based editors without Node.js
- **PHP 8.4 support** — property hooks, asymmetric visibility, new HTML5 modes
- **Fully open source** — no proprietary server binary required

---

### 5.2 Framework & Library Intelligence

Built-in support for popular PHP frameworks (no IDE helper files needed):

| Framework | Features |
|---|---|
| **Laravel** | Facades, service container resolution, Eloquent model properties/relations, route model binding |
| **Symfony** | Service container, console command signatures, form types |
| **WordPress** | Filter/action hooks, global variables, template tags |
| **Doctrine** | ORM entity metadata, repository methods |

Implemented via:

- Static metadata files (`.phpls/stubs/`)
- Composer plugin detection (`composer.json` `require` key scanning)
- Dynamic stub generation (replaces the need for `laravel-ide-helper`)

---

### 5.3 Improved Diagnostics

Additional diagnostic checks:

| Code | Check |
|---|---|
| P2001 | Parameter count mismatch (too few / too many arguments) |
| P2002 | Calling non-callable value |
| P2003 | Accessing property on non-object |
| P2004 | Accessing index on non-array |
| P2005 | Null pointer dereference |
| P2006 | Incompatible return type in overriding method |
| P2007 | Property type widening violation |
| P2008 | Using `$this` outside class context |
| P2009 | Calling abstract method directly |
| P2010 | Array key does not exist in typed array shape |

---

### 5.4 Auto-PHPDoc Generation

The "Add PHPDoc" code action generates contextually rich doc blocks:

```php
// Before
public function process(array $items, int $limit = 10): Generator
{
```

```php
// After
/**
 * @param array<int, Item> $items
 * @param int $limit
 * @return Generator<int, Item>
 * @throws RuntimeException
 */
public function process(array $items, int $limit = 10): Generator
{
```

- Infers generic array element types from usage
- Detects `throw` statements and lists `@throws` tags
- Respects `phpls.phpdoc.*` configuration
- Uses snippet placeholders for missing descriptions

---

### 5.5 Refactoring Actions

| Refactoring | Description |
|---|---|
| Extract Method | Move selected code into a new method with inferred parameters and return type |
| Introduce Variable | Assign a repeated expression to a local variable |
| Convert to Named Argument | Convert positional to named function arguments |
| Make Constructor Promotion | Convert `$this->x = $x` patterns to constructor property promotion |
| Add `readonly` modifier | Detect properties only ever written in constructor |
| Organise Imports | Sort alphabetically, group by namespace depth, remove unused |

---

### 5.6 AI-Assisted Completions (Optional)

When `phpls.ai.enable` is `true` and a compatible AI provider is configured:

- Context-aware multi-line completions (GitHub Copilot / Ollama / any OpenAI-compatible endpoint)
- AI-powered PHPDoc generation that includes business logic descriptions
- Natural language to PHP code conversion in comment-first workflow

AI completions are opt-in, privacy-preserving (local models supported), and never replace static analysis — they augment it.

---

### 5.7 Test Integration

- PHPUnit test discovery via code lens ("Run test" / "Debug test" above each `@test` method)
- Pest test support
- Test result diagnostics — failed assertions appear as diagnostics in the editor
- Code coverage display via Xdebug integration

---

### 5.8 Better Multi-root Workspace Support

- Automatic cross-project dependency detection via `composer.json` `path` repositories
- Symbols from symlinked vendor packages are navigable
- Per-folder configuration via `.phpls/config.json` (overrides workspace settings)

---

## 6. Configuration Reference

| Key | Type | Default | Description |
|---|---|---|---|
| `phpls.enable` | boolean | `true` | Enable/disable the server |
| `phpls.environment.phpVersion` | string | `"8.3"` | PHP version for analysis |
| `phpls.environment.includePaths` | string[] | `[]` | Extra paths to index |
| `phpls.environment.documentRoot` | string | `""` | Document root for include/require links |
| `phpls.files.associations` | string[] | `["**/*.php",…]` | Glob patterns for PHP files |
| `phpls.files.exclude` | string[] | `["**/.git/**",…]` | Glob patterns to exclude |
| `phpls.files.maxSize` | number | `1000000` | Max file size to index (bytes) |
| `phpls.stubs` | string[] | _(all core)_ | Built-in PHP stubs to include |
| `phpls.completion.insertUseDeclaration` | boolean | `true` | Auto-insert `use` on completion |
| `phpls.completion.fullyQualifyGlobalSymbols` | boolean | `false` | Use FQN instead of `use` for globals |
| `phpls.completion.maxItems` | number | `100` | Max completion items per request |
| `phpls.diagnostics.enable` | boolean | `true` | Enable diagnostics |
| `phpls.diagnostics.run` | enum | `"onType"` | `"onType"` or `"onSave"` |
| `phpls.diagnostics.undefinedSymbols` | boolean | `true` | Report undefined symbols |
| `phpls.diagnostics.undefinedVariables` | boolean | `true` | Report undefined variables |
| `phpls.diagnostics.typeErrors` | boolean | `true` | Report type mismatches |
| `phpls.diagnostics.strictTypes` | boolean | `false` | Global `strict_types=1` equivalent |
| `phpls.diagnostics.relaxedTypeCheck` | boolean | `true` | Allow super-types for sub-type constraints |
| `phpls.diagnostics.noMixedTypeCheck` | boolean | `true` | Suppress mixed→narrower errors |
| `phpls.diagnostics.exclude` | object | `{}` | Glob → code[] suppression map |
| `phpls.format.braceStyle` | enum | `"per"` | `"per"`, `"allman"`, `"k&r"` |
| `phpls.phpdoc.returnVoid` | boolean | `true` | Add `@return void` in generated docs |
| `phpls.codeLens.references.enable` | boolean | `false` | Show reference count lens |
| `phpls.codeLens.implementations.enable` | boolean | `false` | Show implementation count lens |
| `phpls.inlayHints.parameterNames.enable` | boolean | `true` | Show parameter name hints |
| `phpls.inlayHints.returnTypes.enable` | boolean | `true` | Show inferred return type hints |
| `phpls.compatibility.preferPsalmPhpstanPrefixedAnnotations` | boolean | `false` | Prefer `@psalm-*` / `@phpstan-*` tags |
| `phpls.trace.server` | enum | `"off"` | LSP trace level for debugging |

---

## 7. Diagnostic Codes

| Code | Severity | Message |
|---|---|---|
| P0001 | Error | Syntax error |
| P1001 | Error | Undefined class `{name}` |
| P1002 | Error | Undefined function `{name}` |
| P1003 | Error | Undefined constant `{name}` |
| P1004 | Error | Undefined method `{name}` |
| P1005 | Error | Undefined property `{name}` |
| P1006 | Warning | Undefined variable `${name}` |
| P1007 | Warning | Deprecated: `{name}` |
| P1008 | Error | Duplicate declaration of `{name}` |
| P1009 | Error | Missing return statement |
| P1010 | Error | Undefined type `{name}` |
| P1011 | Error | Cannot instantiate abstract class `{name}` |
| P1012 | Error | Cannot instantiate interface `{name}` |
| P2001 | Error | Expected `{n}` arguments, got `{m}` |
| P2002 | Error | Value of type `{type}` is not callable |
| P2003 | Error | Cannot access property `{name}` on `{type}` |
| P2004 | Error | Cannot access index on `{type}` |
| P2005 | Warning | Possible null pointer dereference |
| P2006 | Error | Return type `{actual}` is incompatible with declared `{expected}` |

---

## 8. Extension Points / API

phpls exposes a VS Code extension API for other extensions to consume:

```typescript
// Get the phpls extension API
const phpls = vscode.extensions.getExtension('phpls.phpls')?.exports as PhplsApi;

interface PhplsApi {
  // Wait until workspace indexing is complete
  onIndexingComplete(callback: () => void): vscode.Disposable;

  // Query the symbol index programmatically
  getSymbol(fqn: string): IndexedSymbol | undefined;
  searchSymbols(query: string): IndexedSymbol[];

  // Register custom stub files to inject into the index
  registerStubs(extensionId: string, stubGlobs: string[]): vscode.Disposable;

  // Register a custom diagnostic provider
  registerDiagnosticProvider(
    extensionId: string,
    provider: PhpDiagnosticProvider,
  ): vscode.Disposable;
}
```

This API allows framework-specific extensions to contribute additional stubs, diagnostics, and completions without forking phpls.

---

_phpls is fully open source under the MIT licence._  
_Contributions are welcome — see `CONTRIBUTING.md` for guidelines._
