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

```go
import (
    "github.com/sam-fredrickson/keymerge"
    "github.com/goccy/go-yaml"
)

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

opts := keymerge.Options{
    PrimaryKeyNames: []string{"name", "id"},
}

result, _ := keymerge.MergeMarshal(opts, yaml.Unmarshal, yaml.Marshal, baseConfig, prodOverlay)
```

**Result:**
```yaml
database:
  host: prod.db.example.com  # overridden
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

The `api` service was matched by `name` and deep-merged. The `worker` service was preserved as-is. Database settings were deep-merged at the map level.

## Features

### Format-Agnostic

Works with any serialization format by accepting unmarshal/marshal functions:

```go
// YAML
keymerge.MergeMarshal(opts, yaml.Unmarshal, yaml.Marshal, docs...)

// JSON
keymerge.MergeMarshal(opts, json.Unmarshal, json.Marshal, docs...)

// TOML
keymerge.MergeMarshal(opts, toml.Unmarshal, toml.Marshal, docs...)

// Already unmarshaled data
keymerge.Merge(opts, data1, data2, data3)
```

### Deletion Support

Remove items from base configs:

```go
opts := keymerge.Options{
    PrimaryKeyNames: []string{"name"},
    DeleteMarkerKey: "_delete",
}

overlay := []byte(`
services:
  - name: legacy-service
    _delete: true  # Removes this service from final result
`)
```

### Configurable List Modes

**Scalar lists** (primitives without primary keys):

```go
base := []byte(`features: [auth, api, logging]`)
overlay := []byte(`features: [api, metrics]`)

// ScalarListConcat (default): [auth, api, logging, api, metrics]
// ScalarListDedup: [auth, api, logging, metrics]
// ScalarListReplace: [api, metrics]

opts := keymerge.Options{
    ScalarListMode: keymerge.ScalarListDedup,
}
```

**Object lists** with duplicate primary keys:

```go
// ObjectListUnique (default): returns error if duplicates found
// ObjectListConsolidate: merges duplicates together

opts := keymerge.Options{
    PrimaryKeyNames: []string{"id"},
    ObjectListMode:  keymerge.ObjectListConsolidate,
}
```

## How It Works

- **Maps:** Deep-merged recursively. Overlay values override base values.
- **Lists with primary keys:** Items matched by primary key are deep-merged. Unmatched items are appended.
- **Lists without primary keys:** Use `ScalarListMode` (concat, dedup, or replace).
- **Scalars:** Overlay replaces base.
- **Deletion:** Items with `DeleteMarkerKey` set to `true` are removed from the result.

Primary keys are checked in order from `PrimaryKeyNames`. The first matching field name is used. If no primary key is found, the list is treated as a scalar list.

## API Reference

### Core Functions

**`Merge(opts Options, docs ...any) (any, error)`**

Merges already-unmarshaled documents (maps, slices, primitives).

**`MergeMarshal(opts Options, unmarshal UnmarshalFunc, marshal MarshalFunc, docs ...[]byte) ([]byte, error)`**

Merges serialized byte documents using provided unmarshal/marshal functions.

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
