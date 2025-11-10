# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Type-safe `Merger[T]`** - New primary API using generics and struct tags for fine-grained merge control
  - `km:"primary"` tag for marking primary key fields (supports composite keys)
  - `km:"mode=concat|dedup|replace"` for per-field scalar list merge modes
  - `km:"dupe=unique|consolidate"` for per-field object list duplicate handling
  - `km:"field=name"` for overriding field name detection
  - Automatic field name detection from `yaml`, `json`, and `toml` struct tags
  - Metadata-driven merging with path-aware context lookup
- Composite primary key support - multiple fields marked `km:"primary"` create AND-based matching
- `docs/guide.md` comprehensive user guide with typed and dynamic merging examples
- New struct tag reference section in guide covering all directives

### Changed
- **BREAKING**: Renamed `Merger` → `UntypedMerger` (for dynamic configs without known types)
- **BREAKING**: Renamed `NewMerger()` → `NewUntypedMerger()`
- **BREAKING**: New generic `Merger[T]` & `NewMerger[T]()` (new primary/recommended API)
- **BREAKING**: Renamed `MergeMarshal()` → `Merge()` (for both package-level function and methods)
- **BREAKING**: Renamed `Merge()` → `MergeUnstructured()` (for already-unmarshaled data)
- **BREAKING**: `NewMerger[T]()` and `NewUntypedMerger()` now require `unmarshal` and `marshal` functions
- **BREAKING**: `Merger[T].Merge()` and `UntypedMerger.Merge()` now take `docs ...[]byte` (no unmarshal/marshal params)
- **BREAKING**: Renamed `ScalarListMode` → `ScalarMode` (enum type and `Options.ScalarListMode` field)
- **BREAKING**: Renamed scalar list mode constants: `ScalarListConcat` → `ScalarConcat`, `ScalarListDedup` → `ScalarDedup`, `ScalarListReplace` → `ScalarReplace`
- **BREAKING**: Renamed `ObjectListMode` → `DupeMode` (enum type and `Options.ObjectListMode` field)
- **BREAKING**: Renamed object list mode constants: `ObjectListUnique` → `DupeUnique`, `ObjectListConsolidate` → `DupeConsolidate`
- Refactored typed merger code into separate `typed.go` file
- Updated all documentation to lead with type-safe `Merger[T]` API
- Optimize path tracking in list merging: replaced `fmt.Sprintf` with `strconv.Itoa` for ~43% speedup and ~84% reduction in allocations
- Internal: Refactored primary key handling to use `compositeKey` type for unified single/composite key support

### Fixed
- Remove usage of YAML library in fuzz tests

### Migration Guide

**For new projects**: Use `Merger[T]` with struct tags:
```go
// Old approach (v0.2.0)
opts := keymerge.Options{PrimaryKeyNames: []string{"name"}}
result, _ := keymerge.MergeMarshal(opts, yaml.Unmarshal, yaml.Marshal, docs...)

// New approach (v0.3.0)
type Config struct {
    Services []Service `yaml:"services"`
}
type Service struct {
    Name string `yaml:"name" km:"primary"`
}
merger, _ := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
result, _ := merger.Merge(docs...)
```

**For existing projects with dynamic configs**:
```go
// Before (v0.2.0)
m, _ := keymerge.NewMerger(opts)
result, _ := m.MergeMarshal(yaml.Unmarshal, yaml.Marshal, docs...)

// After (v0.3.0)
m, _ := keymerge.NewUntypedMerger(opts, yaml.Unmarshal, yaml.Marshal)
result, _ := m.Merge(docs...)
```

**For package-level functions**:
```go
// Before (v0.2.0)
result, _ := keymerge.MergeMarshal(opts, yaml.Unmarshal, yaml.Marshal, docs...)
mergedAny, _ := keymerge.Merge(opts, mapDocs...)

// After (v0.3.0)
result, _ := keymerge.Merge(opts, yaml.Unmarshal, yaml.Marshal, docs...)
mergedAny, _ := keymerge.MergeUnstructured(opts, mapDocs...)
```

**For Options struct fields**:
```go
// Before (v0.2.0)
opts := keymerge.Options{
    PrimaryKeyNames: []string{"id"},
    ScalarListMode:  keymerge.ScalarListDedup,
    ObjectListMode:  keymerge.ObjectListConsolidate,
}

// After (v0.3.0)
opts := keymerge.Options{
    PrimaryKeyNames: []string{"id"},
    ScalarMode:      keymerge.ScalarDedup,
    DupeMode:        keymerge.DupeConsolidate,
}
```

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
