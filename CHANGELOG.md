# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

### Changed
- Optimize path tracking in list merging: replaced `fmt.Sprintf` with `strconv.Itoa` for ~43% speedup and ~84% reduction in allocations

### Fixed
- Remove usage of YAML library in fuzz tests.

## [0.2.0] - 2025-11-08

### Added
- Sentinel errors for each custom error type (DuplicatePrimaryKeyError, NonComparablePrimaryKeyError)
- Fuzz tests for improved robustness testing

### Changed
- Validate options and return error when invalid
- Use reflection to determine comparability for more accurate type checking
- Changed list deduplication algorithm from O(n²) to O(n) for better performance
- Make error messages consistent across the codebase
- Convert tests to table-driven format
- Pre-allocate map capacity for better performance

### Fixed
- Invalid unicode characters in benchmarks
- Typo in test
- Fuzz tests now run correctly

## [0.1.1] - 2025-11-07

### Changed
- Minor optimization: pre-allocate base index map for better memory efficiency

## [0.1.0] - 2025-11-06

### Added
- Initial release of keymerge
- Format-agnostic configuration merging (YAML, JSON, TOML, etc.)
- Key-based list merging with configurable primary keys
- Deep merging support for nested maps and lists
- Deletion support via configurable delete markers
- Three scalar list merge modes:
  - `ScalarListConcat`: Concatenate base and overlay lists (default)
  - `ScalarListDedup`: Concatenate and deduplicate
  - `ScalarListReplace`: Replace base list with overlay
- `ObjectListMode` for handling duplicate primary keys:
  - `ObjectListUnique`: Return error on duplicates (default)
  - `ObjectListConsolidate`: Merge items with duplicate keys
- Detection of non-comparable primary key values with proper error reporting
- Apache 2.0 license
- GitHub Actions workflows for PR verification and releases
- Comprehensive test suite with >95% coverage
- Benchmark suite for performance testing
- golangci-lint configuration
- Issue templates and PR template
- Dependabot configuration
- Complete documentation in README

[Unreleased]: https://github.com/sam-fredrickson/keymerge/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/sam-fredrickson/keymerge/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/sam-fredrickson/keymerge/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/sam-fredrickson/keymerge/releases/tag/v0.1.0
