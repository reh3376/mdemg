# Echo Module

A working reference INGESTION module for MDEMG. Echoes any `echo://` URI or
`text/plain` content as observations. Use this module to test the plugin system
and as a starting point for your own modules.

## Overview

| Field | Value |
|-------|-------|
| Module ID | `echo-module` |
| Type | INGESTION |
| Version | 1.0.0 |
| Handles | `echo://` URIs, `text/plain` content |

## Building

```bash
# From repo root
go build -o plugins/echo-module/echo-module ./plugins/echo-module/

# Or using make (from this directory)
make build
```

## Testing Locally

Run the module standalone and test with grpcurl:

```bash
# Terminal 1: start the module
make run
# Or: ./echo-module --socket /tmp/echo-module.sock

# Terminal 2: test health check
grpcurl -plaintext -unix /tmp/echo-module.sock \
    mdemg.module.v1.ModuleLifecycle/HealthCheck
```

## Deployment

This module is pre-built and checked in. MDEMG auto-discovers it from the
`plugins/` directory on startup.

Verify it loaded:

```bash
curl -s http://localhost:9999/v1/modules | jq '.data.modules[] | select(.id=="echo-module")'
```

## How It Works

The echo module implements three INGESTION RPCs:

- **Matches** — Returns `true` for any `echo://` URI or `text/plain` content type.
- **Parse** — Wraps the raw content bytes as a single observation node tagged `["echo", "test"]`.
- **Sync** — Returns a single placeholder sync response tagged `["echo", "sync"]`.

All logic lives in a single `main.go` file. This is intentionally minimal — for
larger modules, split handler logic into a separate `handler.go` (the pattern
used by `mdemg plugin scaffold`).

## Reference

- Full tutorial: [`docs/user/module-developer-tutorial.md`](../../docs/user/module-developer-tutorial.md)
- SDK reference: [`docs/development/SDK_PLUGIN_GUIDE.md`](../../docs/development/SDK_PLUGIN_GUIDE.md)
- Module reference: [`docs/development/MODULE_DEVELOPMENT_GUIDE.md`](../../docs/development/MODULE_DEVELOPMENT_GUIDE.md)
