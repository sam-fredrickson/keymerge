# keymerge

A lightweight Go library for merging configuration files with intelligent list handling.

## Features

- **Format-agnostic**: Works with YAML, JSON, TOML, or any serialization format
- **Key-based list merging**: Merges list items by matching on primary keys (like `id` or `name`)
- **Deep merging**: Recursively merges nested maps and lists
- **Deletion support**: Remove keys or list items via configurable delete markers
- **List merge modes**: Choose concat, deduplicate, or replace for scalar lists
- **Zero dependencies**: Core algorithm has no external dependencies
- **Simple API**: Just two functions to learn

## Installation

```bash
go get github.com/sam-fredrickson/keymerge
```

## Usage

### Basic Example (YAML)

```go
package main

import (
    "github.com/sam-fredrickson/keymerge"
    "github.com/goccy/go-yaml"
)

func main() {
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
}
```

### Format-Agnostic Usage

The library works with any format that can unmarshal to `map[string]any` or `[]any`:

```go
// With JSON
result, err := keymerge.MergeMarshal(opts, json.Unmarshal, json.Marshal, base, overlay)

// With already-unmarshaled data
data1 := map[string]any{"foo": 1}
data2 := map[string]any{"bar": 2}
merged, err := keymerge.Merge(opts, data1, data2)
```

### Deletion Example

```go
base := []byte(`
users:
  - name: alice
    role: admin
  - name: bob
    role: user
`)

overlay := []byte(`
users:
  - name: bob
    _delete: true  # Remove bob from the list
`)

opts := keymerge.Options{
    PrimaryKeyNames: []string{"name"},
    DeleteMarkerKey: "_delete",
}

result, err := keymerge.MergeMarshal(opts, yaml.Unmarshal, yaml.Marshal, base, overlay)
// Result: only alice remains
```

### List Mode Example

```go
base := []byte(`features: [auth, api, logging]`)
overlay := []byte(`features: [api, metrics]`)

// Deduplicate mode
opts := keymerge.Options{
    ScalarListMode: keymerge.ScalarListDedup,
}
result, _ := keymerge.MergeMarshal(opts, yaml.Unmarshal, yaml.Marshal, base, overlay)
// Result: [auth, api, logging, metrics]

// Replace mode
opts.ScalarListMode = keymerge.ScalarListReplace
result, _ = keymerge.MergeMarshal(opts, yaml.Unmarshal, yaml.Marshal, base, overlay)
// Result: [api, metrics]
```

### Duplicate Primary Key Handling

```go
base := []byte(`
users:
  - id: alice
    role: user
  - id: alice
    role: admin
`)

// Unique mode (default): returns error
opts := keymerge.Options{
    PrimaryKeyNames: []string{"id"},
    ObjectListMode:  keymerge.ObjectListUnique,
}
_, err := keymerge.MergeMarshal(opts, yaml.Unmarshal, yaml.Marshal, base)
// err: DuplicatePrimaryKeyError{Key: "alice", Positions: [0, 1]}

// Consolidate mode: merges duplicate items
opts.ObjectListMode = keymerge.ObjectListConsolidate
result, _ := keymerge.MergeMarshal(opts, yaml.Unmarshal, yaml.Marshal, base)
// Result: Single alice with merged fields (role: admin)
```

## How It Works

### Maps
Maps are deep-merged recursively. Later values override earlier ones.

### Lists
Lists are merged based on primary keys:
- If list items are maps with a primary key field, items are matched by key and deep-merged
- If no primary key exists, lists are merged according to `ScalarListMode`:
  - `ScalarListConcat` (default): Append overlay items to base items
  - `ScalarListDedup`: Concatenate and remove duplicates
  - `ScalarListReplace`: Replace base list entirely with overlay list
- New items are appended, existing items are updated
- Duplicate primary keys are handled according to `ObjectListMode`:
  - `ObjectListUnique` (default): Returns an error if duplicates are found
  - `ObjectListConsolidate`: Merges items with duplicate keys together

### Scalars
Later values replace earlier ones.

### Deletion
When `DeleteMarkerKey` is set, items can be explicitly removed:
- For map keys: `{key: {_delete: true}}` removes that key
- For list items: `{primaryKey: "foo", _delete: true}` removes the item with that primary key

## API

### `Merge(opts Options, docs ...any) (any, error)`

Merges already-unmarshaled documents. Format-agnostic and dependency-free. Returns an error if duplicate primary keys are found and `ObjectListMode` is set to `ObjectListUnique`.

### `MergeMarshal(opts Options, unmarshal, marshal, docs ...[]byte) ([]byte, error)`

Merges byte documents using provided unmarshal/marshal functions. Works with any serialization format.

### `Options`

```go
type Options struct {
    // List of field names to try as primary keys for list merging
    // Checked in order; first match is used
    PrimaryKeyNames []string

    // Field name that marks items for deletion (e.g., "_delete")
    // When set, maps with this field set to true are removed
    DeleteMarkerKey string

    // How to merge lists without primary keys
    // Default: ScalarListConcat
    ScalarListMode ScalarListMode

    // How to handle items with duplicate primary keys in object lists
    // Default: ObjectListUnique
    ObjectListMode ObjectListMode
}
```

### `ScalarListMode`

```go
const (
    ScalarListConcat  // Append overlay to base (default)
    ScalarListDedup   // Concatenate and remove duplicates
    ScalarListReplace // Replace base with overlay
)
```

### `ObjectListMode`

```go
const (
    ObjectListUnique       // Error if duplicate primary keys found (default)
    ObjectListConsolidate  // Merge items with duplicate primary keys
)
```

### `DuplicatePrimaryKeyError`

Returned when duplicate primary keys are found and `ObjectListMode` is `ObjectListUnique`:

```go
type DuplicatePrimaryKeyError struct {
    Key       any   // The duplicate primary key value
    Positions []int // Indices where the duplicate was found
}
```

### `NonComparablePrimaryKeyError`

Returned when a primary key value is not comparable (e.g., a map or slice). Primary key values must be comparable types that can be used as map keys:

```go
type NonComparablePrimaryKeyError struct {
    Key      any // The non-comparable primary key value
    Position int // Index where the non-comparable key was found
}
```

Valid primary key types: strings, numbers, bools, pointers, channels, interfaces.
Invalid primary key types: maps, slices.

## Performance

The library is aimed towards config merging at application startup. Typical performance:
- Small configs (2-10 items): ~500ns
- Medium configs (100+ items, 5 overlays): ~40μs
- Large configs (100+ items, 20 overlays): ~170μs

Run benchmarks: `go test -bench=. ./bench/`
