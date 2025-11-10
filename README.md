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
`)

result, _ := merger.Merge(baseConfig, prodOverlay)
```

The `api` service is matched by `name` (marked with `km:"primary"`) and deep-merged. The `worker` service is preserved. Database fields are merged, with the overlay overriding `host` and `pool_size` while preserving `port`.

## Features

- **Primary key matching:** Use `km:"primary"` tags to match and deep-merge list items by key fields (supports composite keys)
- **Struct tag control:** Fine-grained merge behavior per field with `km` tags (`mode=`, `dupe=`, `field=`)
- **Format-agnostic:** Works with YAML, JSON, TOML, or any format via unmarshal/marshal functions
- **Deletion support:** Mark items for removal with configurable delete marker key
- **List merge modes:** Concat (default), deduplicate, or replace for scalar lists
- **Type-safe or dynamic:** Use `Merger[T]` with struct tags, or `UntypedMerger` for runtime configs
- **Zero dependencies:** Pure Go with >90% test coverage

See [docs/guide.md](docs/guide.md) for comprehensive examples and patterns.

## How It Works

- **Maps:** Deep-merged recursively (overlay overrides base)
- **Lists with keys:** Items matched by primary key are deep-merged; unmatched items appended
- **Lists without keys:** Concatenated (or deduplicated/replaced based on mode)
- **Scalars:** Overlay replaces base
- **Deletion:** Items marked with delete key are removed

## API

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
