# Changelog

All notable changes to PHP Strom are documented in this file.

## 0.1.38 - 2026-09-08

### Fixed

- Reduced false-positive argument type diagnostics for list-shaped values.
- Preserved iterable PHPDoc on enum methods and promoted parameters.

### Changed

- Made `onSave` the default diagnostics mode so PHP files are not fully re-analysed while typing.

## 0.1.37 - 2026-09-08

### Fixed

- Correctly treated code after a return followed by a trailing comment as unreachable.

## 0.1.36 - 2026-09-08

### Added

- Added memory limit handling and prioritised class members in completion results.

### Changed

- Removed the unused TypeScript language server and stopped advertising unsupported LSP capabilities.
- Built the extension client into `dist/` consistently for Makefile builds, tests, and extension-host debugging.
- Simplified the diagnostics view and updated PHP 8.5 stubs.

### Fixed

- Applied ternary and property-guard type narrowing to editor diagnostics.
- Honoured the configured analysis level and excluded vendored paths from reporting.
- Indexed enum `$name` and `$value` members and removed incorrect Doctrine value hovers.

## 0.1.35 - 2026-09-04

### Added

- Added Quick Config for choosing the analysis level.

### Fixed

- Improved PHPDoc type, bound, callable, argument, inheritance, and declaration diagnostics.
- Aligned built-in `DateTime` return types and expanded expression analysis.
- Mapped diagnostic spans to UTF-16 editor ranges.
- Improved callable, array-shape, union, nullable, and expression receiver analysis.
- Reduced false positives for built-in and trait methods.

### Changed

- Added per-family analysis toggles and removed PHPStan branding from analysis levels.
- Reused semantic snapshots across analysis, incrementally refreshed workspace semantics, and scoped cache invalidation by dependency.
- Reduced semantic fact and flow storage overhead.

## 0.1.29 - 2026-08-30

### Added

- Added workspace diagnostics progress tracking.
- Enabled the workspace diagnostics scan on startup by default.
- Added an editor latency regression gate.

### Changed

- Improved workspace indexing and cached definition and declaration lookups.

## 0.1.28 - 2026-08-27

Maintenance release.

## 0.1.27 - 2026-08-25

### Changed

- Integrated parser compatibility improvements.

## 0.1.26 - 2026-08-23

### Fixed

- Corrected generic PHPDoc method argument counts.

## 0.1.25 - 2026-08-22

### Changed

- Hardened parser fuzz integration and workspace file discovery.
