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

## See It In Action

Here's a real-world example: merging a base config with production defaults, then applying customer-specific overrides. Shows deletion markers, tag deduplication, and format conversion.

**Watch for:** Services matched by `name` and deep-merged (api: 1→10→25 replicas), `debug-proxy` removed via `_delete` marker, tags deduplicated (us-east appears only once), and cross-format merging (YAML/JSON/TOML → JSON).

**base.yaml:**
```yaml
database:
  host: localhost
  pool_size: 10
  timeout: 30s

services:
  - name: api
    replicas: 1
    port: 8080
  - name: worker
    replicas: 1
    enabled: true
  - name: debug-proxy
    replicas: 1
    port: 9000

features:
  - rate-limiting
  - metrics

tags:
  - default
  - api
```

**prod.json** (different format - showing format-agnostic merging):
```json
{
  "database": {
    "host": "prod.db.example.com",
    "pool_size": 50,
    "timeout": "60s"
  },
  "services": [
    {
      "name": "api",
      "replicas": 10
    },
    {
      "name": "worker",
      "replicas": 5
    },
    {
      "name": "debug-proxy",
      "_delete": true
    }
  ],
  "features": [
    "tracing",
    "audit-logging"
  ],
  "tags": [
    "production",
    "us-east"
  ]
}
```

**customer1.toml:**
```toml
features = ["premium-support", "custom-reports"]

tags = ["customer1", "us-east", "premium"]

[database]
pool_size = 100
read_replicas = 3

[[services]]
name = "api"
replicas = 25
cache_enabled = true
```

**Merge them:**
```bash
cfgmerge -scalar dedup -format json -out config.json base.yaml prod.json customer1.toml
```

**Result (config.json):**
```json
{
  "database": {
    "host": "prod.db.example.com",
    "pool_size": 100,
    "timeout": "60s",
    "read_replicas": 3
  },
  "services": [
    {
      "name": "api",
      "replicas": 25,
      "port": 8080,
      "cache_enabled": true
    },
    {
      "name": "worker",
      "replicas": 5,
      "enabled": true
    }
  ],
  "features": [
    "rate-limiting",
    "metrics",
    "tracing",
    "audit-logging",
    "premium-support",
    "custom-reports"
  ],
  "tags": [
    "default",
    "api",
    "production",
    "us-east",
    "customer1",
    "premium"
  ]
}
```

**What happened:**
- **Services** matched by `name` and deep-merged (api: 1→10→25 replicas, worker: 1→5, debug-proxy removed)
- **debug-proxy** removed via `_delete: true` marker in prod layer
- **Database** settings progressively tuned (pool_size: 10→50→100, read_replicas added for customer)
- **Features** concatenated across all layers (base→prod→customer features combined)
- **Tags** deduplicated (via `-scalar dedup` flag - "us-east" appears only once)
- **Format conversion**: YAML + JSON + TOML → JSON output

## Installation

**CLI tools:**
```bash
# Config file merger
go install github.com/sam-fredrickson/keymerge/cmd/cfgmerge@latest

# Kustomize KRM function
go install github.com/sam-fredrickson/keymerge/cmd/cfgmerge-krm@latest

# Or download pre-built binaries from releases
# https://github.com/sam-fredrickson/keymerge/releases

# Or use Docker
docker pull samuelfredrickson/cfgmerge:latest
```

**Go library:**
```bash
go get github.com/sam-fredrickson/keymerge
```
Requires Go 1.24 or later.

## Quick Start: Kubernetes

Use `cfgmerge` in an `initContainer` to merge base and environment-specific configs at deployment time:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-configs
data:
  base.yaml: |
    database:
      host: localhost
      port: 5432
      pool_size: 10
      timeout: 30s
    features:
      - name: rate-limiting
        enabled: true
        limit: 1000
      - name: caching
        enabled: false
    log_level: info
  production.yaml: |
    database:
      host: prod-db.default.svc.cluster.local
      pool_size: 50
      timeout: 60s
    features:
      - name: rate-limiting
        limit: 10000
      - name: caching
        enabled: true
        ttl: 300
    log_level: warn
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  template:
    spec:
      initContainers:
      - name: merge-config
        image: samuelfredrickson/cfgmerge:latest
        args:
          - -out
          - /config/app-config.yaml
          - /configs/base.yaml
          - /configs/production.yaml
        volumeMounts:
          - name: config-sources
            mountPath: /configs
          - name: merged-config
            mountPath: /config

      containers:
      - name: app
        image: myapp:latest
        volumeMounts:
          - name: merged-config
            mountPath: /etc/myapp
        # App reads merged config from /etc/myapp/app-config.yaml

      volumes:
      - name: config-sources
        configMap:
          name: app-configs
      - name: merged-config
        emptyDir: {}
```

This pattern keeps your base config in version control and environment-specific overrides in ConfigMaps, merging them at runtime.

**Want to customize?** Run `cfgmerge -h` to see all options: custom primary keys (`-keys`), list merge modes (`-scalar`, `-dupe`), deletion markers (`-delete-marker`), and more.

## Quick Start: Kustomize

Use `cfgmerge-krm` as a Kustomize transformer to merge ConfigMaps declaratively using annotations:

```yaml
# base/config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-base-config
  annotations:
    config.keymerge.io/id: "app-config"
    config.keymerge.io/order: "0"
    config.keymerge.io/final-name: "app-config"
data:
  app-config.yaml: |
    server:
      port: 8080
    logging:
      level: info
```

```yaml
# features/tracing/config-snippet.yaml (Kustomize Component)
apiVersion: v1
kind: ConfigMap
metadata:
  name: tracing-config
  annotations:
    config.keymerge.io/id: "app-config"
    config.keymerge.io/order: "10"
data:
  app-config.yaml: |
    tracing:
      enabled: true
      endpoint: http://jaeger:14268/api/traces
```

```yaml
# envs/dev/config-env.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: dev-overlay
  annotations:
    config.keymerge.io/id: "app-config"
    config.keymerge.io/order: "100"
data:
  app-config.yaml: |
    server:
      port: 3000
    logging:
      level: debug
```

Configure the transformer in your `kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../base
  - config-env.yaml
components:
  - ../../features/tracing
transformers:
  - transformer-config.yaml
```

```yaml
# transformer-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cfgmerge-transformer
  annotations:
    config.kubernetes.io/function: |
      exec:
        path: cfgmerge-krm
```

Build with:
```bash
kustomize build --enable-alpha-plugins --enable-exec envs/dev
```

Result: Single merged ConfigMap with base config, tracing feature, and dev overrides applied in order.

**See the full example:** `examples/kustomize/` includes a complete working setup with base, features, and environment overlays.

## Library Usage

For programmatic config merging in Go:

```go
import (
    "github.com/sam-fredrickson/keymerge"
    "github.com/goccy/go-yaml"
)

type Config struct {
    Database Database  `yaml:"database"`
    Services []Service `yaml:"services"`
}

type Service struct {
    Name     string `yaml:"name" km:"primary"`
    Replicas int    `yaml:"replicas"`
}

merger, _ := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
result, _ := merger.Merge(baseConfig, prodOverlay)
```

The `km:"primary"` struct tag marks `Name` as the primary key for matching service items during merge.

**Two API styles:**
- **`NewMerger[T]`** - Type-safe with struct tags (recommended)
- **`NewUntypedMerger`** - Dynamic for runtime configs

See the [User Guide](docs/guide.md) for comprehensive examples, patterns, and advanced features like composite keys, field-specific merge modes, and error handling.

## Documentation

- **[User Guide](docs/guide.md)** - Comprehensive examples, patterns, and best practices
- **[API Reference](https://pkg.go.dev/github.com/sam-fredrickson/keymerge)** - Complete API documentation

## License

Apache 2.0 - see [LICENSE](LICENSE)

## Alternatives

If keymerge doesn't fit your use case, consider:
- **[uber/config](https://github.com/uber-go/config)** - YAML-based config with advanced merging (requires all inputs to be YAML)
- **[kustomize](https://kustomize.io/)** - Kubernetes-native config management (K8s-specific, strategic merge patches)
- **[yq](https://github.com/mikefarah/yq)** - YAML/JSON/XML processing (manual scripting required for complex merges)

keymerge's niche is **format-agnostic merging with intelligent list matching** - if you need primary-key-based list merging across YAML/JSON/TOML, this is the tool.
