# keymerge

A lightweight Go library for merging configuration files with intelligent list handling.

## Features

- Format-agnostic: works with YAML, JSON, TOML, or any serialization format
- Key-based list merging: matches list items by primary keys (`id`, `name`, etc.)
- Deep merging with deletion support and configurable list modes
- Zero dependencies, simple API

## Installation

```bash
go get github.com/sam-fredrickson/keymerge
```

## Usage

### Basic Example

```go
import (
    "github.com/sam-fredrickson/keymerge"
    "github.com/goccy/go-yaml"
)

base := []byte(`
users:
  - name: alice
    role: user
  - name: bob
    role: user
`)

overlay := []byte(`
users:
  - name: alice
    role: admin
  - name: charlie
    role: user
`)

opts := keymerge.Options{
    PrimaryKeyNames: []string{"name", "id"},
}

result, err := keymerge.MergeMarshal(opts, yaml.Unmarshal, yaml.Marshal, base, overlay)
// Result: alice becomes admin, bob unchanged, charlie added
```

Works with any format (JSON, TOML, etc.) or already-unmarshaled data:

```go
result, err := keymerge.MergeMarshal(opts, json.Unmarshal, json.Marshal, base, overlay)
merged, err := keymerge.Merge(opts, data1, data2) // Already unmarshaled
```

### Deletion

```go
opts := keymerge.Options{
    PrimaryKeyNames: []string{"name"},
    DeleteMarkerKey: "_delete",
}

overlay := []byte(`
users:
  - name: bob
    _delete: true  # Removes bob from merged result
`)
```

### Scalar List Modes

```go
base := []byte(`features: [auth, api, logging]`)
overlay := []byte(`features: [api, metrics]`)

// ScalarListConcat (default): [auth, api, logging, api, metrics]
// ScalarListDedup: [auth, api, logging, metrics]
// ScalarListReplace: [api, metrics]

opts := keymerge.Options{ScalarListMode: keymerge.ScalarListDedup}
```

### Duplicate Primary Keys

```go
// ObjectListUnique (default): returns DuplicatePrimaryKeyError
// ObjectListConsolidate: merges items with duplicate keys together

opts := keymerge.Options{
    PrimaryKeyNames: []string{"id"},
    ObjectListMode:  keymerge.ObjectListConsolidate,
}
```

## How It Works

**Maps:** Deep-merged recursively. Later values override earlier ones.

**Lists:** Items with matching primary keys are deep-merged. Items without keys are appended. Scalar lists (no keys) use `ScalarListMode` (concat/dedup/replace). Duplicate keys use `ObjectListMode` (error/consolidate).

**Scalars:** Later values replace earlier ones.

**Deletion:** When `DeleteMarkerKey` is set, items with that field set to `true` are removed from the final result.

## API

### Functions

**`Merge(opts Options, docs ...any) (any, error)`**
Merges already-unmarshaled documents. Returns error if duplicate keys found in `ObjectListUnique` mode or if primary key values are non-comparable (maps/slices).

**`MergeMarshal(opts Options, unmarshal, marshal, docs ...[]byte) ([]byte, error)`**
Merges byte documents using provided unmarshal/marshal functions.

### Options

```go
type Options struct {
    PrimaryKeyNames []string      // Field names to match list items (checked in order)
    DeleteMarkerKey string         // Field marking items for deletion (e.g., "_delete")
    ScalarListMode  ScalarListMode // How to merge lists without keys (default: Concat)
    ObjectListMode  ObjectListMode // How to handle duplicate keys (default: Unique)
}
```

**ScalarListMode:** `ScalarListConcat` (append), `ScalarListDedup` (append + dedup), `ScalarListReplace` (replace)

**ObjectListMode:** `ObjectListUnique` (error on duplicates), `ObjectListConsolidate` (merge duplicates)

### Errors

**`DuplicatePrimaryKeyError`** - Duplicate primary keys found in `ObjectListUnique` mode
**`NonComparablePrimaryKeyError`** - Primary key value is a map or slice (not comparable)

## Performance

The library is aimed towards config merging at application startup. Typical performance:
- Small configs (2-10 items): ~600ns
- Medium configs (100+ items, 5 overlays): ~38μs
- Large configs (100+ items, 20 overlays): ~156μs

Run benchmarks: `go test -bench=. ./bench/`
