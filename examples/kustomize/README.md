# Kustomize Integration Example

This example demonstrates using `cfgmerge-krm` as a Kustomize KRM function to merge ConfigMaps.

## Quick Start

```bash
# Install cfgmerge-krm
go install github.com/sam-fredrickson/keymerge/cmd/cfgmerge-krm@latest

# Build the dev environment
cd examples/kustomize/envs/dev
kustomize build --enable-alpha-plugins --enable-exec .
```

## Directory Structure

```
examples/kustomize/
├── base/
│   ├── kustomization.yaml
│   ├── deployment.yaml       # Base Kubernetes Deployment
│   └── config.yaml           # Base ConfigMap (order=0)
├── features/
│   └── tracing/
│       ├── kustomization.yaml    # Component definition
│       ├── config-snippet.yaml   # Tracing config (order=10)
│       └── sidecar-patch.yaml    # Adds Jaeger sidecar
└── envs/
    └── dev/
        ├── kustomization.yaml      # Dev environment
        ├── config-env.yaml         # Dev-specific config (order=100)
        └── transformer-config.yaml # cfgmerge-krm transformer
```

## How It Works

ConfigMaps are merged by `config.keymerge.io/id` annotation:

1. **Base ConfigMap** (`order=0`): Defines baseline application config
2. **Feature Components** (`order=10-99`): Add optional features (like tracing)
3. **Environment Overlays** (`order=100+`): Apply environment-specific values

All ConfigMaps with matching `id` are merged into a single ConfigMap named by `final-name`.

### Example Flow

**Base** (`order=0`):
```yaml
server:
  port: 8080
  timeout: 30s
logging:
  level: info
```

**Tracing Feature** (`order=10`):
```yaml
features:
  enabled: [tracing]
tracing:
  endpoint: http://jaeger:14268/api/traces
```

**Dev Environment** (`order=100`):
```yaml
server:
  port: 3000  # Override
logging:
  level: debug  # Override
environment: development  # New field
```

**Result**:
```yaml
server:
  port: 3000        # From dev (highest order wins)
  timeout: 30s      # From base
logging:
  level: debug      # From dev
features:
  enabled: [tracing]  # From feature
tracing:            # From feature
  endpoint: http://jaeger:14268/api/traces
environment: development  # From dev
```

## Annotations Reference

All annotations use the `config.keymerge.io/` prefix:

### Required Annotations

- **`id`**: Groups ConfigMaps for merging
  - Example: `config.keymerge.io/id: "app-config"`
- **`order`**: Merge order (numeric, lower merges first)
  - Example: `config.keymerge.io/order: "0"`
- **`final-name`**: Name of merged ConfigMap (required on base/order=0 only)
  - Example: `config.keymerge.io/final-name: "app-config"`

### Optional Annotations

- **`keys`**: Override primary key names for list merging
  - Default: `"name,id"`
  - Example: `config.keymerge.io/keys: "id,uuid"`
- **`scalar-mode`**: How to merge scalar lists
  - Options: `concat`, `dedup`, `replace`
  - Default: `concat`
  - Example: `config.keymerge.io/scalar-mode: "dedup"`
- **`dupe-mode`**: How to handle duplicate object list keys
  - Options: `unique`, `consolidate`
  - Default: `unique`
  - Example: `config.keymerge.io/dupe-mode: "consolidate"`
- **`delete-marker`**: Deletion marker key
  - Default: `"_delete"`
  - Example: `config.keymerge.io/delete-marker: "__delete__"`

## Transformer Configuration

`transformer-config.yaml`:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cfgmerge-transformer
  annotations:
    config.kubernetes.io/function: |
      exec:
        path: cfgmerge-krm
```

Reference in `kustomization.yaml`:
```yaml
transformers:
  - transformer-config.yaml
```

**Important**: You must use both flags when building:
```bash
kustomize build --enable-alpha-plugins --enable-exec .
```

## Testing

Run the integration test to verify the complete workflow:

```bash
cd examples/kustomize
./test.sh
```

The test builds the dev environment with Kustomize and verifies the output matches the expected merged configuration.
