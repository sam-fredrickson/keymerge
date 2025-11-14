# keymerge User Guide

This guide provides comprehensive examples and patterns for using keymerge effectively.

## Table of Contents

- [Getting Started](#getting-started)
  - [Type-Safe Merging](#type-safe-merging)
  - [Dynamic Merging](#dynamic-merging)
  - [CLI Usage](#cli-usage)
  - [Choosing Your API](#choosing-your-api)
- [Basic Concepts](#basic-concepts)
- [Struct Tag Reference](#struct-tag-reference)
- [Working with Different Formats](#working-with-different-formats)
- [Core Features](#core-features)
  - [Primary Key Matching](#primary-key-matching)
  - [Composite Keys](#composite-keys)
  - [Deletion Semantics](#deletion-semantics)
  - [List Merging Modes](#list-merging-modes)
- [Error Handling](#error-handling)
- [Advanced Patterns](#advanced-patterns)
- [Performance Considerations](#performance-considerations)
- [Common Pitfalls](#common-pitfalls)

## Getting Started

### Type-Safe Merging

For most use cases, start with `Merger[T]` which uses struct tags to control merge behavior:

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/goccy/go-yaml"
    "github.com/sam-fredrickson/keymerge"
)

// Define your config structure with km tags
type Config struct {
    App      AppConfig  `yaml:"app"`
    Database Database   `yaml:"database"`
    Services []Service  `yaml:"services"`
}

type AppConfig struct {
    Name    string `yaml:"name"`
    Version string `yaml:"version"`
}

type Database struct {
    Host string `yaml:"host"`
    Port int    `yaml:"port"`
}

type Service struct {
    Name     string `yaml:"name" km:"primary"`
    Enabled  bool   `yaml:"enabled"`
    Replicas int    `yaml:"replicas"`
}

func main() {
    // Create typed merger
    merger, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
    if err != nil {
        log.Fatal(err)
    }
    
    base := []byte(`
app:
  name: myapp
  version: 1.0.0
database:
  host: localhost
  port: 5432
services:
  - name: api
    enabled: true
    replicas: 2
  - name: worker
    enabled: true
    replicas: 1
`)
    
    overlay := []byte(`
app:
  version: 1.1.0
database:
  host: prod.db.example.com
services:
  - name: api
    replicas: 10
`)
    
    result, err := merger.Merge(base, overlay)
    if err != nil {
        log.Fatal(err)
    }
    
    // Unmarshal into typed config
    var config Config
    if err := yaml.Unmarshal(result, &config); err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("App: %s v%s\n", config.App.Name, config.App.Version)
    fmt.Printf("Database: %s:%d\n", config.Database.Host, config.Database.Port)
    for _, svc := range config.Services {
        fmt.Printf("Service %s: replicas=%d\n", svc.Name, svc.Replicas)
    }
}
```

**Benefits:**
- Compile-time type safety
- Self-documenting merge behavior (tags show intent)
- Fine-grained control per field
- No need to remember global primary key names

### Dynamic Merging

For truly dynamic configs where types aren't known ahead of time:

```go
import "github.com/sam-fredrickson/keymerge"

opts := keymerge.Options{
    PrimaryKeyNames: []string{"name", "id"},
}

result, err := keymerge.Merge(opts, yaml.Unmarshal, yaml.Marshal, baseData, overlayData)
```

**Use cases:**
- Plugin systems with unknown config schemas
- Generic config processing tools
- Working with arbitrary JSON/YAML

### CLI Usage

For one-off config merges without writing code, use the `cfgmerge` command-line tool:

**Installation:**

```bash
go install github.com/sam-fredrickson/keymerge/cmd/cfgmerge@latest
```

**Basic example:**

```bash
# Merge production overlay into base config
cfgmerge -out config.yaml base.yaml production.yaml
```

```yaml
# base.yaml
database:
  host: localhost
  port: 5432
  pool_size: 5
services:
  - name: api
    enabled: true
    replicas: 2
  - name: worker
    enabled: true
    replicas: 1
```

```yaml
# production.yaml
database:
  host: prod.db.example.com
  pool_size: 20
services:
  - name: api
    replicas: 10
```

```yaml
# Result (config.yaml)
database:
  host: prod.db.example.com
  port: 5432
  pool_size: 20
services:
  - name: api
    enabled: true
    replicas: 10
  - name: worker
    enabled: true
    replicas: 1
```

The `api` service was matched by name and deep-merged. The `worker` service from base was preserved.

**Command-line flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `-keys` | `name,id` | Comma-separated list of primary key field names |
| `-scalar` | `concat` | Scalar list mode: `concat`, `dedup`, or `replace` |
| `-dupe` | `unique` | Duplicate key mode: `unique` or `consolidate` |
| `-delete-marker` | `_delete` | Key name for deletion markers |
| `-out` | stdout | Output file path (use `-` for stdout) |
| `-format` | auto | Output format: `json`, `yaml`, or `toml` (auto-detects from first file) |
| `-version` | | Show version and exit |

**Advanced examples:**

```bash
# Format conversion (YAML to JSON)
cfgmerge -format json -out config.json base.yaml overlay.yaml

# Multiple overlays (applied left-to-right)
cfgmerge -out final.yaml base.yaml prod.yaml us-east.yaml

# Deduplicate scalar lists
cfgmerge -scalar dedup -out merged.yaml configs/*.yaml

# Allow duplicate primary keys (consolidate them)
cfgmerge -dupe consolidate -out config.yaml base.yaml overlay.yaml

# Custom primary keys
cfgmerge -keys id,uuid,identifier -out merged.json *.json
```

**When to use:**

- **CLI (`cfgmerge`)**: One-off merges, shell scripts, CI/CD pipelines, quick config generation
- **Library (type-safe)**: Application runtime with known config schema, compile-time safety needed
- **Library (dynamic)**: Plugin systems, generic config tools, runtime type flexibility needed

### Choosing Your API

keymerge is optimized for config merging at application startup (microsecond range), not high-frequency runtime merging.

| Use Case | Recommended API | Why |
|----------|----------------|-----|
| Known config struct at compile time | `Merger[T]` (type-safe) | Compile-time safety, self-documenting tags, fine-grained control |
| Unknown schema (plugin configs, generic tools) | `UntypedMerger` or `MergeUnstructured()` | Runtime flexibility, works with any structure |
| One-off merges, shell scripts, CI/CD | `cfgmerge` CLI | No code needed, format conversion, quick iteration |
| Application startup (any API) | All APIs work | Performance is excellent for startup (<200μs for large configs) |
| Hot path (thousands/sec) | None - pre-merge at startup | Not designed for high-frequency runtime use |

For merge behavior details (primary keys, deletion, list modes), see the sections below.

## Basic Concepts

### The Merge Model

keymerge operates on three types of values:

1. **Maps** (`map[string]any`) - Deep-merged recursively
2. **Lists** (`[]any`) - Merged based on primary keys or scalar mode
3. **Scalars** (strings, numbers, bools, nil) - Replaced by overlay

When merging multiple documents, they are processed left-to-right. Each document overlays the accumulated result.

### Simple Map Merging

```go
import "github.com/sam-fredrickson/keymerge"

base := map[string]any{
    "timeout": 30,
    "retries": 3,
    "endpoint": map[string]any{
        "host": "localhost",
        "port": 8080,
    },
}

overlay := map[string]any{
    "retries": 5,
    "endpoint": map[string]any{
        "port": 9000,
        "tls":  true,
    },
}

opts := keymerge.Options{}
result, err := keymerge.MergeUnstructured(opts, base, overlay)
if err != nil {
    panic(err)
}

// result:
// {
//   "timeout": 30,
//   "retries": 5,
//   "endpoint": {
//     "host": "localhost",
//     "port": 9000,
//     "tls": true
//   }
// }
```

Notice how nested maps are also deep-merged. The `endpoint.host` from base is preserved, while `endpoint.port` is overridden and `endpoint.tls` is added.

## Struct Tag Reference

Quick reference for `km:` struct tags. See [Core Features](#core-features) for detailed examples and behavior.

### Available Tags

| Tag | Values | Description | Example |
|-----|--------|-------------|---------|
| `km:"primary"` | N/A | Mark field as (part of) primary key | `ID string \`km:"primary"\`` |
| `km:"mode=..."` | `concat`, `dedup`, `replace` | Scalar list merge mode for this field | `Tags []string \`km:"mode=dedup"\`` |
| `km:"dupe=..."` | `unique`, `consolidate` | Duplicate key handling for this field | `Items []Item \`km:"dupe=consolidate"\`` |
| `km:"field=..."` | Any string | Override field name detection | `Data []string \`custom:"x" km:"field=x"\`` |

### Multiple Tags

Combine tags with commas:

```go
type Config struct {
    Tags []string `yaml:"tags" km:"field=tags,mode=dedup"`

    Services []Service `yaml:"services" km:"dupe=consolidate"`
}

type Service struct {
    Region string `yaml:"region" km:"primary"`  // Composite key (part 1)
    Name   string `yaml:"name" km:"primary"`    // Composite key (part 2)
    URL    string `yaml:"url"`
}
```

### Field Name Detection

Priority order (highest to lowest):
1. `km:"field=..."` override
2. `yaml:"..."`
3. `json:"..."`
4. `toml:"..."`
5. Struct field name

### Detailed Documentation

- **Primary Keys & Composite Keys**: See [Primary Key Matching](#primary-key-matching) and [Composite Keys](#composite-keys)
- **List Modes**: See [List Merging Modes](#list-merging-modes)
- **Deletion**: See [Deletion Semantics](#deletion-semantics)

## Working with Different Formats

### YAML

```go
import (
    "github.com/sam-fredrickson/keymerge"
    "github.com/goccy/go-yaml"
)

base := []byte(`
app:
  name: myapp
  version: 1.0.0
`)

overlay := []byte(`
app:
  version: 1.1.0
  features:
    - feature-x
`)

opts := keymerge.Options{}
result, err := keymerge.Merge(opts, yaml.Unmarshal, yaml.Marshal, base, overlay)
if err != nil {
    panic(err)
}

// result is []byte containing merged YAML
```

### JSON

```go
import (
    "encoding/json"
    "github.com/sam-fredrickson/keymerge"
)

base := []byte(`{"timeout": 30, "retries": 3}`)
overlay := []byte(`{"retries": 5, "pool_size": 10}`)

opts := keymerge.Options{}
result, err := keymerge.Merge(opts, json.Unmarshal, json.Marshal, base, overlay)
if err != nil {
    panic(err)
}

// result: {"pool_size":10,"retries":5,"timeout":30}
```

### TOML

```go
import (
    "github.com/BurntSushi/toml"
    "github.com/sam-fredrickson/keymerge"
)

base := []byte(`
[database]
host = "localhost"
port = 5432
`)

overlay := []byte(`
[database]
host = "prod.example.com"
`)

opts := keymerge.Options{}
result, err := keymerge.Merge(opts, toml.Unmarshal, toml.Marshal, base, overlay)
if err != nil {
    panic(err)
}
```

### Pre-parsed Data

If you've already unmarshaled your data, use `MergeUnstructured()` directly:

```go
var base, overlay map[string]any

// ... unmarshal into base and overlay ...

opts := keymerge.Options{}
result, err := keymerge.MergeUnstructured(opts, base, overlay)
if err != nil {
    panic(err)
}

// result is map[string]any
merged := result.(map[string]any)
```

## Core Features

This section covers keymerge's key features with both type-safe (struct tag) and dynamic (untyped) examples.

### Primary Key Matching

Primary keys allow list items to be matched and deep-merged intelligently.

**Type-safe approach** (using struct tags):

```go
type User struct {
    ID   string `yaml:"id" km:"primary"`
    Name string `yaml:"name"`
    Role string `yaml:"role"`
}

type Config struct {
    Users []User `yaml:"users"`
}
```

When merging lists of `User`, items with matching `id` values are deep-merged:

```yaml
# base.yaml
users:
  - id: alice
    name: Alice
    role: user

# overlay.yaml
users:
  - id: alice
    role: admin  # Updates Alice's role
  - id: bob
    name: Bob
    role: user   # New user added

# result
users:
  - id: alice
    name: Alice
    role: admin  # Merged!
  - id: bob
    name: Bob
    role: user
```

**Dynamic approach** (using Options):

```go
base := map[string]any{
    "users": []any{
        map[string]any{"name": "alice", "role": "user"},
        map[string]any{"name": "bob", "role": "user"},
    },
}

overlay := map[string]any{
    "users": []any{
        map[string]any{"name": "alice", "role": "admin"},
        map[string]any{"name": "charlie", "role": "user"},
    },
}

opts := keymerge.Options{
    PrimaryKeyNames: []string{"name"},
}

result, err := keymerge.MergeUnstructured(opts, base, overlay)

// result.users:
// - {name: alice, role: admin}    (merged)
// - {name: bob, role: user}       (from base)
// - {name: charlie, role: user}   (from overlay)
```

### Multiple Primary Key Candidates

Use multiple key names when different lists use different keys:

```go
base := map[string]any{
    "users": []any{
        map[string]any{"name": "alice", "role": "user"},
    },
    "services": []any{
        map[string]any{"id": "svc-1", "port": 8080},
    },
}

overlay := map[string]any{
    "users": []any{
        map[string]any{"name": "alice", "role": "admin"},
    },
    "services": []any{
        map[string]any{"id": "svc-1", "port": 9000},
    },
}

opts := keymerge.Options{
    PrimaryKeyNames: []string{"name", "id"},
}

result, err := keymerge.MergeUnstructured(opts, base, overlay)

// Both lists are matched correctly:
// - users list uses "name" as primary key
// - services list uses "id" as primary key
```

The first matching field name from `PrimaryKeyNames` is used for each list item.

### Deep Merging Matched Items

When list items are matched by primary key, they are deep-merged recursively:

```go
base := map[string]any{
    "services": []any{
        map[string]any{
            "name": "api",
            "config": map[string]any{
                "timeout": 30,
                "retries": 3,
            },
        },
    },
}

overlay := map[string]any{
    "services": []any{
        map[string]any{
            "name": "api",
            "config": map[string]any{
                "retries": 5,
                "pool_size": 10,
            },
        },
    },
}

opts := keymerge.Options{
    PrimaryKeyNames: []string{"name"},
}

result, err := keymerge.MergeUnstructured(opts, base, overlay)

// result.services[0].config:
// {timeout: 30, retries: 5, pool_size: 10}
```

### Items Without Primary Keys

If a list item doesn't have any of the primary key fields, it's appended to the result:

```go
base := map[string]any{
    "items": []any{
        map[string]any{"id": 1, "value": "a"},
        map[string]any{"value": "b"}, // No "id" field
    },
}

overlay := map[string]any{
    "items": []any{
        map[string]any{"id": 1, "value": "updated"},
        map[string]any{"value": "c"}, // No "id" field
    },
}

opts := keymerge.Options{
    PrimaryKeyNames: []string{"id"},
}

result, err := keymerge.MergeUnstructured(opts, base, overlay)

// result.items:
// - {id: 1, value: "updated"}   (merged by id)
// - {value: "b"}                (from base, no id)
// - {value: "c"}                (from overlay, no id)
```

### Composite Keys

Multiple `km:"primary"` tags create a composite key where ALL fields must match for items to be merged.

**Type-safe approach:**

```go
type Endpoint struct {
    Region string `yaml:"region" km:"primary"`
    Name   string `yaml:"name" km:"primary"`
    URL    string `yaml:"url"`
}

type Config struct {
    Endpoints []Endpoint `yaml:"endpoints"`
}
```

Items match only when BOTH region AND name are equal:

```yaml
# base.yaml
endpoints:
  - region: us-east
    name: api
    url: v1-east.example.com
  - region: us-west
    name: api
    url: v1-west.example.com

# overlay.yaml
endpoints:
  - region: us-east
    name: api
    url: v2-east.example.com  # Only updates us-east/api

# result
endpoints:
  - region: us-east
    name: api
    url: v2-east.example.com  # Updated
  - region: us-west
    name: api
    url: v1-west.example.com  # Unchanged (different region)
```

**Perfect for:**
- Multi-region configs
- Namespaced resources (namespace + name)
- Versioned settings (version + environment)

**Note:** For the untyped API, there's no direct composite key support. Use a single field that combines the values (e.g., `key: "us-east/api"`).

### Deletion Semantics

Set `DeleteMarkerKey` to enable deletion of specific items:

```go
opts := keymerge.Options{
    PrimaryKeyNames: []string{"name"},
    DeleteMarkerKey: "_delete",
}

base := map[string]any{
    "users": []any{
        map[string]any{"name": "alice", "role": "admin"},
        map[string]any{"name": "bob", "role": "user"},
    },
}

overlay := map[string]any{
    "users": []any{
        map[string]any{"name": "bob", "_delete": true},
    },
}

result, err := keymerge.MergeUnstructured(opts, base, overlay)

// result.users:
// - {name: "alice", role: "admin"}
// (bob was deleted)
```

**Deleting Map Keys:**

You can also delete entire map keys:

```go
opts := keymerge.Options{
    DeleteMarkerKey: "_delete",
}

base := map[string]any{
    "feature_a": map[string]any{"enabled": true},
    "feature_b": map[string]any{"enabled": true},
    "timeout":   30,
}

overlay := map[string]any{
    "feature_b": map[string]any{"_delete": true},
    "timeout":   map[string]any{"_delete": true},
}

result, err := keymerge.MergeUnstructured(opts, base, overlay)

// result:
// {feature_a: {enabled: true}}
// (feature_b and timeout were deleted)
```

**Delete Markers are Stripped:**

The delete marker itself is removed from the final result:

```go
opts := keymerge.Options{
    PrimaryKeyNames: []string{"id"},
    DeleteMarkerKey: "_delete",
}

base := map[string]any{
    "items": []any{
        map[string]any{"id": 1, "value": "keep"},
        map[string]any{"id": 2, "value": "remove"},
    },
}

overlay := map[string]any{
    "items": []any{
        map[string]any{"id": 2, "_delete": true},
    },
}

result, err := keymerge.MergeUnstructured(opts, base, overlay)

// result.items:
// - {id: 1, value: "keep"}
// (id: 2 was removed, and "_delete" field is not present in result)
```

### List Merging Modes

For type-safe merging, these modes can be controlled via struct tags (see [Struct Tag Reference](#struct-tag-reference)).

**Scalar Lists:**

When a list contains only scalars (no maps with primary keys), use `ScalarMode`:

**Concat (default):**

Appends all items from all documents:

```go
opts := keymerge.Options{
    ScalarMode: keymerge.ScalarConcat, // or omit (default)
}

base := map[string]any{"tags": []any{"api", "service"}}
overlay := map[string]any{"tags": []any{"production", "api"}}

result, err := keymerge.MergeUnstructured(opts, base, overlay)

// result.tags: ["api", "service", "production", "api"]
```

**Dedup:**

Appends items and removes duplicates (order preserved, later occurrences removed):

```go
opts := keymerge.Options{
    ScalarMode: keymerge.ScalarDedup,
}

base := map[string]any{"tags": []any{"api", "service"}}
overlay := map[string]any{"tags": []any{"production", "api"}}

result, err := keymerge.MergeUnstructured(opts, base, overlay)

// result.tags: ["api", "service", "production"]
// (second "api" was deduplicated)
```

**Replace:**

Overlay list completely replaces base list:

```go
opts := keymerge.Options{
    ScalarMode: keymerge.ScalarReplace,
}

base := map[string]any{"tags": []any{"api", "service"}}
overlay := map[string]any{"tags": []any{"production"}}

result, err := keymerge.MergeUnstructured(opts, base, overlay)

// result.tags: ["production"]
```

**Object Lists with Duplicate Keys:**

`DupeMode` controls how to handle duplicate primary keys **within a single document**. Duplicate keys **across documents** are always merged together (that's the whole point of keymerge!).

**Unique (default):**

Returns an error if duplicate primary keys are found within one document:

```go
opts := keymerge.Options{
    PrimaryKeyNames: []string{"id"},
    DupeMode:  keymerge.DupeUnique, // or omit (default)
}

base := map[string]any{
    "items": []any{map[string]any{"id": 1, "a": 1}},
}

// This document has INTERNAL duplicates (two items with id: 2)
overlay := map[string]any{
    "items": []any{
        map[string]any{"id": 2, "b": 2},
        map[string]any{"id": 2, "c": 3}, // Duplicate!
    },
}

result, err := keymerge.MergeUnstructured(opts, base, overlay)
// err is DuplicatePrimaryKeyError
// (id: 2 appears twice in the overlay document)
```

Note: Items with the same key across different documents are merged, not treated as duplicates:

```go
// This is VALID - same key across documents is expected
doc1 := map[string]any{
    "items": []any{map[string]any{"id": 1, "a": 1}},
}
doc2 := map[string]any{
    "items": []any{map[string]any{"id": 1, "b": 2}},
}
result, _ := keymerge.MergeUnstructured(opts, doc1, doc2)
// result.items: [{id: 1, a: 1, b: 2}] - merged together
```

**Consolidate:**

Merges items with duplicate keys together within a single document:

```go
opts := keymerge.Options{
    PrimaryKeyNames: []string{"id"},
    DupeMode:  keymerge.DupeConsolidate,
}

base := map[string]any{
    "items": []any{map[string]any{"id": 1, "a": 1}},
}

// This document has internal duplicates - consolidate mode merges them
overlay := map[string]any{
    "items": []any{
        map[string]any{"id": 2, "b": 2},
        map[string]any{"id": 2, "c": 3}, // Duplicate within overlay
    },
}

result, err := keymerge.MergeUnstructured(opts, base, overlay)
// No error!
// result.items:
// - {id: 1, a: 1}
// - {id: 2, b: 2, c: 3}  (duplicates consolidated)
```

## Error Handling

### Error Types

keymerge defines three error types with detailed context:

#### DuplicatePrimaryKeyError

Returned when duplicate primary keys are found in `DupeUnique` mode:

```go
import (
    "errors"
    "fmt"
    "github.com/sam-fredrickson/keymerge"
)

opts := keymerge.Options{
    PrimaryKeyNames: []string{"id"},
}

base := map[string]any{
    "users": []any{map[string]any{"id": 1, "name": "alice"}},
}

// Overlay document has INTERNAL duplicate
overlay := map[string]any{
    "users": []any{
        map[string]any{"id": 2, "name": "bob"},
        map[string]any{"id": 2, "name": "charlie"}, // Duplicate!
    },
}

result, err := keymerge.MergeUnstructured(opts, base, overlay)
if err != nil {
    // Check error type
    if errors.Is(err, keymerge.ErrDuplicatePrimaryKey) {
        fmt.Println("Duplicate key found!")
    }

    // Extract details
    var dupErr *keymerge.DuplicatePrimaryKeyError
    if errors.As(err, &dupErr) {
        fmt.Printf("Key: %v\n", dupErr.Key)
        fmt.Printf("Path: %v\n", dupErr.Path)
        fmt.Printf("Positions: %v\n", dupErr.Positions)
        fmt.Printf("Document index: %d\n", dupErr.DocIndex)
    }
}
```

#### NonComparablePrimaryKeyError

Returned when a primary key value is a map or slice (not comparable in Go):

```go
opts := keymerge.Options{
    PrimaryKeyNames: []string{"config"},
}

base := map[string]any{
    "items": []any{
        map[string]any{
            "config": map[string]any{"key": "value"}, // Maps are not comparable!
            "data":   "something",
        },
    },
}

result, err := keymerge.MergeUnstructured(opts, base)
if err != nil {
    if errors.Is(err, keymerge.ErrNonComparablePrimaryKey) {
        var ncErr *keymerge.NonComparablePrimaryKeyError
        if errors.As(err, &ncErr) {
            fmt.Printf("Key: %v (type %T)\n", ncErr.Key, ncErr.Key)
            fmt.Printf("Position: %d\n", ncErr.Position)
            fmt.Printf("Path: %v\n", ncErr.Path)
            fmt.Printf("Document index: %d\n", ncErr.DocIndex)
        }
    }
}
```

To fix this, use a comparable field (string, number, bool) as the primary key.

#### MarshalError

Returned when unmarshal or marshal operations fail:

```go
opts := keymerge.Options{}

invalidYAML := []byte(`this is not: [valid: yaml`)

result, err := keymerge.Merge(opts, yaml.Unmarshal, yaml.Marshal, invalidYAML)
if err != nil {
    if errors.Is(err, keymerge.ErrMarshal) {
        var marshalErr *keymerge.MarshalError
        if errors.As(err, &marshalErr) {
            fmt.Printf("Operation: %s\n", marshalErr.Operation)
            fmt.Printf("Document index: %d\n", marshalErr.DocIndex)
            fmt.Printf("Cause: %v\n", marshalErr.Err)
        }
    }
}
```

### Best Practices

1. **Always check errors** - Don't ignore the error return value
2. **Use `errors.Is()` for sentinel errors** - Check `ErrDuplicatePrimaryKey`, etc.
3. **Use `errors.As()` for details** - Extract typed error for full context
4. **Validate options early** - Call `NewMerger()` to validate options once

```go
// Validate options once, reuse merger
merger, err := keymerge.NewUntypedMerger(opts, yaml.Unmarshal, yaml.Marshal)
if err != nil {
    return fmt.Errorf("invalid merge options: %w", err)
}

result1, err := merger.Merge(docs1...)
if err != nil {
    return fmt.Errorf("merge failed: %w", err)
}

result2, err := merger.Merge(docs2...)
if err != nil {
    return fmt.Errorf("merge failed: %w", err)
}
```

## Advanced Patterns

### Reusable Merger

Create a `Merger` instance to validate options once and reuse it:

```go
opts := keymerge.Options{
    PrimaryKeyNames: []string{"name", "id"},
    DeleteMarkerKey: "_delete",
    ScalarMode:  keymerge.ScalarDedup,
}

merger, err := keymerge.NewUntypedMerger(opts, yaml.Unmarshal, yaml.Marshal)
if err != nil {
    panic(err) // Invalid options
}

// Reuse merger for multiple merge operations
result1, err := merger.Merge(firstDocs...)
result2, err := merger.Merge(secondDocs...)
result3, err := merger.Merge(thirdDocs...)
```

**Note:** `Merger` is not thread-safe. Create separate instances for concurrent use.

### Layered Configuration

Merge base config + environment-specific overlays:

```go
baseConfig := loadYAML("config/base.yaml")
envConfig := loadYAML("config/" + env + ".yaml")
userConfig := loadYAML(homeDir + "/.myapp.yaml")

opts := keymerge.Options{
    PrimaryKeyNames: []string{"name", "id"},
    DeleteMarkerKey: "_delete",
}

merger, err := keymerge.NewUntypedMerger(opts, yaml.Unmarshal, yaml.Marshal)
if err != nil {
    panic(err)
}

final, err := merger.Merge(baseConfig, envConfig, userConfig)
```

## Performance Considerations

### Design for Startup, Not Runtime

keymerge is optimized for config merging at application startup, not high-frequency runtime merging. Typical performance:

- Small configs (2-10 items): ~600ns
- Medium configs (100+ items, 5 overlays): ~38μs
- Large configs (100+ items, 20 overlays): ~156μs

This is fast enough for startup config loading but may not be suitable for hot paths processing thousands of merges per second.

### Reuse Merger Instances

Creating a `Merger` validates options once. Reuse it for multiple merges:

```go
// Good - validate once
merger, _ := keymerge.NewUntypedMerger(opts, yaml.Unmarshal, yaml.Marshal)
for _, configSet := range manySets {
    result, _ := merger.Merge(configSet...)
}

// Less efficient - validates every time
for _, configSet := range manySets {
    result, _ := keymerge.Merge(opts, yaml.Unmarshal, yaml.Marshal, configSet...)
}
```

### Pre-parse When Possible

If you're merging the same documents multiple times with different overlays, pre-parse them:

```go
// Good - unmarshal once
var baseData, overlay1Data, overlay2Data map[string]any
yaml.Unmarshal(baseYAML, &baseData)
yaml.Unmarshal(overlay1YAML, &overlay1Data)
yaml.Unmarshal(overlay2YAML, &overlay2Data)

opts := keymerge.Options{}
result1, _ := keymerge.MergeUnstructured(opts, baseData, overlay1Data)
result2, _ := keymerge.MergeUnstructured(opts, baseData, overlay2Data)

// Less efficient - unmarshals base twice
result1, _ := keymerge.Merge(opts, yaml.Unmarshal, yaml.Marshal, baseYAML, overlay1YAML)
result2, _ := keymerge.Merge(opts, yaml.Unmarshal, yaml.Marshal, baseYAML, overlay2YAML)
```

### Scalar List Dedup Performance

`ScalarDedup` uses a map for O(n) deduplication. This is fast but requires values to be comparable:

```go
// Fast - strings are comparable
opts := keymerge.Options{ScalarMode: keymerge.ScalarDedup}
base := map[string]any{"tags": []any{"a", "b", "c", "a"}}
result, _ := keymerge.MergeUnstructured(opts, base)

// Won't work - maps are not comparable
base := map[string]any{"items": []any{
    map[string]any{"x": 1},
    map[string]any{"x": 1}, // Can't deduplicate
}}
```

For non-comparable values, use `ScalarConcat` instead.

### Memory Allocation

keymerge pre-allocates result maps and slices when the size is known, minimizing allocations. For very large configs, you may see:

- ~15 allocs for small configs
- ~50 allocs for medium configs
- ~200 allocs for large configs

This is generally negligible for startup config loading.

## Common Pitfalls

### 1. Forgetting Primary Key Names

Without `PrimaryKeyNames`, object lists are treated as scalar lists and concatenated:

```go
// Wrong - lists are concatenated
opts := keymerge.Options{}
base := map[string]any{
    "users": []any{
        map[string]any{"name": "alice", "role": "user"},
    },
}
overlay := map[string]any{
    "users": []any{
        map[string]any{"name": "alice", "role": "admin"},
    },
}
result, _ := keymerge.MergeUnstructured(opts, base, overlay)
// result.users has 2 items: [{name: alice, role: user}, {name: alice, role: admin}]

// Correct - lists are matched by name
opts := keymerge.Options{PrimaryKeyNames: []string{"name"}}
result, _ := keymerge.MergeUnstructured(opts, base, overlay)
// result.users has 1 item: [{name: alice, role: admin}]
```

### 2. Using Non-Comparable Primary Keys

Maps and slices are not comparable in Go and cannot be used as primary keys:

```go
// Wrong - "config" is a map (not comparable)
opts := keymerge.Options{PrimaryKeyNames: []string{"config"}}
base := map[string]any{
    "items": []any{
        map[string]any{
            "config": map[string]any{"key": "value"},
            "data":   "something",
        },
    },
}
result, err := keymerge.MergeUnstructured(opts, base)
// err is NonComparablePrimaryKeyError

// Correct - use a string/number/bool field
base := map[string]any{
    "items": []any{
        map[string]any{
            "id":     "item-1",
            "config": map[string]any{"key": "value"},
        },
    },
}
opts := keymerge.Options{PrimaryKeyNames: []string{"id"}}
```

### 3. Delete Markers Don't Work Without DeleteMarkerKey

```go
// Wrong - delete marker ignored
opts := keymerge.Options{PrimaryKeyNames: []string{"name"}}
overlay := map[string]any{
    "users": []any{
        map[string]any{"name": "bob", "_delete": true},
    },
}
// "_delete" field is treated as regular data

// Correct - set DeleteMarkerKey
opts := keymerge.Options{
    PrimaryKeyNames: []string{"name"},
    DeleteMarkerKey: "_delete",
}
```

### 4. Nil vs Empty vs Missing

keymerge treats `nil`, empty values, and missing keys differently:

```go
base := map[string]any{
    "timeout": 30,
    "retries": 3,
}

overlay1 := map[string]any{
    "timeout": nil, // Keeps base value (30)
}

overlay2 := map[string]any{
    "timeout": 0, // Replaces with 0
}

overlay3 := map[string]any{
    // "timeout" missing - keeps base value (30)
}
```

To delete a key, use delete markers:

```go
opts := keymerge.Options{DeleteMarkerKey: "_delete"}
overlay := map[string]any{
    "timeout": map[string]any{"_delete": true},
}
```

### 5. Merger is Not Thread-Safe

```go
merger, _ := keymerge.NewUntypedMerger(opts, yaml.Unmarshal, yaml.Marshal)

// Wrong - race condition
go merger.Merge(docs1...)
go merger.Merge(docs2...)

// Correct - separate instances or synchronization
merger1, _ := keymerge.NewUntypedMerger(opts, yaml.Unmarshal, yaml.Marshal)
merger2, _ := keymerge.NewUntypedMerger(opts, yaml.Unmarshal, yaml.Marshal)
go merger1.Merge(docs1...)
go merger2.Merge(docs2...)
```

### 6. Order Matters

Documents are merged left-to-right. Later documents override earlier ones:

```go
doc1 := map[string]any{"value": 1}
doc2 := map[string]any{"value": 2}
doc3 := map[string]any{"value": 3}

result, _ := keymerge.MergeUnstructured(opts, doc1, doc2, doc3)
// result.value = 3

result, _ := keymerge.MergeUnstructured(opts, doc3, doc2, doc1)
// result.value = 1
```

### 7. Misunderstanding DupeMode

`DupeMode` controls duplicates **within a single document**, not across documents:

```go
opts := keymerge.Options{
    PrimaryKeyNames: []string{"id"},
    DupeMode:  keymerge.DupeUnique, // default
}

// Same ID across documents is EXPECTED and always merged
doc1 := map[string]any{"items": []any{map[string]any{"id": 1, "a": 1}}}
doc2 := map[string]any{"items": []any{map[string]any{"id": 1, "b": 2}}}

result, _ := keymerge.MergeUnstructured(opts, doc1, doc2)
// result.items: [{id: 1, a: 1, b: 2}] - No error, items merged

// But duplicates WITHIN a document are errors in Unique mode
base := map[string]any{"items": []any{}}
overlay := map[string]any{
    "items": []any{
        map[string]any{"id": 1, "a": 1},
        map[string]any{"id": 1, "b": 2}, // Duplicate within overlay!
    },
}
result, err := keymerge.MergeUnstructured(opts, base, overlay)
// err is DuplicatePrimaryKeyError

// Use DupeConsolidate to allow internal duplicates
opts.DupeMode = keymerge.DupeConsolidate
result, _ = keymerge.MergeUnstructured(opts, base, overlay)
// result.items: [{id: 1, a: 1, b: 2}] - Internal duplicates consolidated
```

---

For more examples, see the [test suite](../merge_test.go) and [benchmarks](../bench/merge_bench_test.go).
