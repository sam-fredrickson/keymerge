# keymerge

[![Go Reference](https://pkg.go.dev/badge/github.com/sam-fredrickson/keymerge.svg)](https://pkg.go.dev/github.com/sam-fredrickson/keymerge)
[![Go Report Card](https://goreportcard.com/badge/github.com/sam-fredrickson/keymerge)](https://goreportcard.com/report/github.com/sam-fredrickson/keymerge)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

A lightweight, format-agnostic Go library for merging configuration files with intelligent list handling.

## Why keymerge?

Configuration management often requires layering multiple config files (base + environment-specific overlays). Simple merging breaks when you have lists of objects that should be matched and merged intelligently rather than concatenated or replaced wholesale.

**keymerge** solves this by:
- Matching list items by primary keys (`id`, `name`, etc.) and deep-merging them
- Supporting deletion of specific items from base configs
- Working with any format (YAML, JSON, TOML) or pre-parsed data structures
- Providing zero-dependency, production-ready code with >90% test coverage

Perfect for: application configs, Kubernetes manifests, infrastructure-as-code, feature flags, and any scenario where you layer configs with complex nested structures.

## Installation

```bash
go get github.com/sam-fredrickson/keymerge
```

**Requirements:** Go 1.24 or later

## Quick Start

### Type-Safe Merging (Recommended)

Define your config structure with `km` tags for fine-grained control:

```go
import (
    "github.com/sam-fredrickson/keymerge"
    "github.com/goccy/go-yaml"
)

type Config struct {
    Database Database  `yaml:"database"`
    Services []Service `yaml:"services"`
}

type Database struct {
    Host     string `yaml:"host"`
    Port     int    `yaml:"port"`
    PoolSize int    `yaml:"pool_size"`
}

type Service struct {
    Name     string `yaml:"name" km:"primary"`
    Enabled  bool   `yaml:"enabled"`
    Replicas int    `yaml:"replicas"`
    Timeout  string `yaml:"timeout"`
}

// Create a typed merger with marshal functions
merger, _ := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)

baseConfig := []byte(`
database:
  host: localhost
  port: 5432
  pool_size: 10
services:
  - name: api
    enabled: true
    replicas: 2
  - name: worker
    enabled: true
    replicas: 1
`)

prodOverlay := []byte(`
database:
  host: prod.db.example.com
  pool_size: 50
services:
  - name: api
    replicas: 10
    timeout: 30s
`)

result, _ := merger.Merge(baseConfig, prodOverlay)

// Unmarshal into typed config
var config Config
yaml.Unmarshal(result, &config)
```

**Result:**
```yaml
database:
  host: prod.db.example.com   # overridden
  port: 5432                  # preserved
  pool_size: 50               # overridden
services:
  - name: api
    enabled: true             # preserved
    replicas: 10              # overridden
    timeout: 30s              # added
  - name: worker
    enabled: true             # preserved (no overlay)
    replicas: 1
```

The `api` service was matched by `name` (marked with `km:"primary"`) and deep-merged. The `worker` service was preserved as-is.

### Dynamic Merging

For truly dynamic configs where types aren't known ahead of time:

```go
opts := keymerge.Options{
    PrimaryKeyNames: []string{"name", "id"},
}

result, _ := keymerge.Merge(opts, yaml.Unmarshal, yaml.Marshal, baseConfig, prodOverlay)
```

## Features

### Struct Tag Directives

Control merge behavior per field with `km` tags:

```go
type Config struct {
    // Composite primary key (both region AND name must match)
    Endpoints []Endpoint `yaml:"endpoints"`
    
    // Deduplicate tags instead of concatenating
    Tags []string `yaml:"tags" km:"mode=dedup"`
    
    // Allow duplicate IDs and merge them together
    Metrics []Metric `yaml:"metrics" km:"dupe=consolidate"`
}

type Endpoint struct {
    Region string `yaml:"region" km:"primary"`
    Name   string `yaml:"name" km:"primary"`
    URL    string `yaml:"url"`
}
```

**Available directives:**
- `km:"primary"` - Mark field as (composite) primary key
- `km:"mode=concat|dedup|replace"` - Scalar list merge mode
- `km:"dupe=unique|consolidate"` - Object list duplicate handling
- `km:"field=name"` - Override field name detection

### Format-Agnostic

Works with any serialization format by accepting unmarshal/marshal functions:

```go
// YAML
keymerge.Merge(opts, yaml.Unmarshal, yaml.Marshal, docs...)

// JSON
keymerge.Merge(opts, json.Unmarshal, json.Marshal, docs...)

// TOML
keymerge.Merge(opts, toml.Unmarshal, toml.Marshal, docs...)

// Already unmarshaled data
keymerge.MergeUnstructured(opts, data1, data2, data3)
```

### Deletion Support

Remove items from base configs:

```go
type Config struct {
    Services []Service `yaml:"services"`
}

type Service struct {
    Name string `yaml:"name" km:"primary"`
}

merger, _ := keymerge.NewMerger[Config](keymerge.Options{
    DeleteMarkerKey: "_delete",
}, yaml.Unmarshal, yaml.Marshal)

overlay := []byte(`
services:
  - name: legacy-service
    _delete: true  # Removes this service from final result
`)
```

### Configurable List Modes

Control list behavior globally or per-field:

```go
type Config struct {
    // This field uses concat mode
    Features []string `yaml:"features" km:"mode=concat"`
    
    // This field uses dedup mode
    Tags []string `yaml:"tags" km:"mode=dedup"`
    
    // This field uses replace mode
    Envs []string `yaml:"envs" km:"mode=replace"`
}

// Or set globally via Options (applies to fields without km tags)
opts := keymerge.Options{
    ScalarListMode: keymerge.ScalarListDedup,
}
```

**Scalar list modes:**
- `concat` (default): Append all items
- `dedup`: Append and remove duplicates
- `replace`: Replace base with overlay

**Object list modes** (for handling duplicate primary keys):
```go
type Config struct {
    // Return error if duplicate IDs found (default)
    Users []User `yaml:"users" km:"dupe=unique"`
    
    // Merge items with duplicate IDs together
    Metrics []Metric `yaml:"metrics" km:"dupe=consolidate"`
}
```

- `unique` (default): Error on duplicates
- `consolidate`: Merge duplicates together

## How It Works

- **Maps:** Deep-merged recursively. Overlay values override base values.
- **Lists with primary keys:** Items matched by primary key are deep-merged. Unmatched items are appended.
- **Lists without primary keys:** Use `ScalarListMode` (concat, dedup, or replace).
- **Scalars:** Overlay replaces base.
- **Deletion:** Items with `DeleteMarkerKey` set to `true` are removed from the result.

Primary keys are checked in order from `PrimaryKeyNames`. The first matching field name is used. If no primary key is found, the list is treated as a scalar list.

## API Reference

### Typed Merging (Recommended)

**`NewMerger[T any](opts Options, unmarshal func([]byte, any) error, marshal func(any) ([]byte, error)) (*Merger[T], error)`**

Creates a type-safe merger that extracts merge directives from struct tags.

```go
merger, _ := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
result, _ := merger.Merge(docs...)
```

### Dynamic Merging

**`Merge(opts Options, unmarshal func([]byte, any) error, marshal func(any) ([]byte, error), docs ...[]byte) ([]byte, error)`**

Merges serialized byte documents using provided unmarshal/marshal functions.

**`MergeUnstructured(opts Options, docs ...any) (any, error)`**

Merges already-unmarshaled documents (maps, slices, primitives).

**`NewUntypedMerger(opts Options, unmarshal func([]byte, any) error, marshal func(any) ([]byte, error)) (*UntypedMerger, error)`**

Creates a reusable untyped merger for dynamic configs. Pass `nil, nil` for marshal functions if only using `MergeUnstructured()`.

### Options

```go
type Options struct {
    PrimaryKeyNames []string       // Field names to match list items (checked in order)
    DeleteMarkerKey string          // Field name marking items for deletion
    ScalarListMode  ScalarListMode  // How to merge lists without keys
    ObjectListMode  ObjectListMode  // How to handle duplicate primary keys
}
```

**ScalarListMode values:**
- `ScalarListConcat` (default) - Append all items
- `ScalarListDedup` - Append and deduplicate
- `ScalarListReplace` - Replace base with overlay

**ObjectListMode values:**
- `ObjectListUnique` (default) - Return error on duplicate keys
- `ObjectListConsolidate` - Merge items with duplicate keys

### Errors

- `DuplicatePrimaryKeyError` - Duplicate keys in `ObjectListUnique` mode
- `NonComparablePrimaryKeyError` - Primary key is a map/slice (not comparable)
- `MarshalError` - Unmarshal/marshal operation failed

Use `errors.Is()` and `errors.As()` for error checking.

## Documentation

- **[User Guide](docs/guide.md)** - Comprehensive examples and patterns
- **[API Docs](https://pkg.go.dev/github.com/sam-fredrickson/keymerge)** - Full API reference

## Performance

Optimized for config merging at application startup:

- Small configs (2-10 items): ~600ns
- Medium configs (100+ items, 5 overlays): ~38μs
- Large configs (100+ items, 20 overlays): ~156μs

Run benchmarks: `go test -bench=. ./bench/`

## License

Apache 2.0 - see [LICENSE](LICENSE)
