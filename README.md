# keymerge

[![Go Reference](https://pkg.go.dev/badge/github.com/sam-fredrickson/keymerge.svg)](https://pkg.go.dev/github.com/sam-fredrickson/keymerge)
[![Go Report Card](https://goreportcard.com/badge/github.com/sam-fredrickson/keymerge)](https://goreportcard.com/report/github.com/sam-fredrickson/keymerge)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

A lightweight Go library and CLI tool for merging configuration files with intelligent list handling.

## Why keymerge?

Configuration management often requires layering multiple config files (base + environment-specific overlays). Simple merging breaks when you have lists of objects that should be matched and merged intelligently rather than concatenated or replaced wholesale.

**keymerge** solves this by:
- Matching list items by primary keys (`id`, `name`, etc.) and deep-merging them
- Supporting deletion of specific items from base configs
- Working with any format (YAML, JSON, TOML) or pre-parsed data structures
- Providing zero-dependency, production-ready code with >90% test coverage

Perfect for: application configs, Kubernetes manifests, infrastructure-as-code, feature flags, and any scenario where you layer configs with complex nested structures.

## Installation

**CLI tool:**
```bash
go install github.com/sam-fredrickson/keymerge/cmd/cfgmerge@latest
```
Or download pre-built binaries from [releases](https://github.com/sam-fredrickson/keymerge/releases).

**Go library:**
```bash
go get github.com/sam-fredrickson/keymerge
```
Requires Go 1.24 or later.

## Quick Start

### CLI

Merge environment-specific overlays into a base config:

```bash
cfgmerge base.yaml production.yaml > config.yaml
```

With `base.yaml`:
```yaml
database:
  host: localhost
  pool_size: 10
services:
  - name: api
    replicas: 2
  - name: worker
    replicas: 1
```

And `production.yaml`:
```yaml
database:
  host: prod.db.example.com
  pool_size: 50
services:
  - name: api
    replicas: 10
```

The result merges the `api` service (matched by `name`), preserves `worker`, and updates database settings.

Run `cfgmerge -h` for options including custom primary keys, list modes, and format conversion.

### Library

```go
import (
    _ "embed"
    "github.com/sam-fredrickson/keymerge"
    "github.com/goccy/go-yaml"
)

// Using the same files as in the CLI example.
// In real life you'd read them from disk.

//go:embed base.yaml
var baseConfig []byte
//go:embed production.yaml
var prodOverlay []byte

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
}

merger, _ := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
result, _ := merger.Merge(baseConfig, prodOverlay)
```

Just like in the CLI example, the `api` service is matched by `name` and deep-merged. Unlike that example, though,
rather than assuming that `name` is a primary key, the `Name` field is explicitly marked via `km:"primary"`.

## Features

- **Primary key matching:** Match and deep-merge list items by key fields like `name` or `id` (supports composite keys)
- **Format-agnostic:** YAML, JSON, TOML, or any format via unmarshal/marshal functions
- **Deletion support:** Mark items for removal with configurable delete marker key
- **List merge modes:** Concat (default), deduplicate, or replace for scalar lists
- **CLI tool:** Standalone binary for merging config files without writing code
- **Library modes:** Type-safe `Merger[T]` with struct tags, or `UntypedMerger` for dynamic configs
- **Zero dependencies:** Pure Go with >90% test coverage

See [docs/guide.md](docs/guide.md) for comprehensive examples and patterns.

## How It Works

- **Maps:** Deep-merged recursively (overlay overrides base)
- **Lists with keys:** Items matched by primary key are deep-merged; unmatched items appended
- **Lists without keys:** Concatenated (or deduplicated/replaced based on mode)
- **Scalars:** Overlay replaces base
- **Deletion:** Items marked with delete key are removed

## Library API

The library provides two APIs:

- **`NewMerger[T](opts, unmarshal, marshal)`** - Type-safe merger using struct tags (recommended)
- **`NewUntypedMerger(opts, unmarshal, marshal)`** - Dynamic merger for runtime configs

Both support reusable instances and `MergeUnstructured()` for pre-parsed data.

See [API documentation](https://pkg.go.dev/github.com/sam-fredrickson/keymerge) for full reference.

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
