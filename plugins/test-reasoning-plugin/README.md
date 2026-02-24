# Test Reasoning Plugin

A REASONING module for MDEMG that re-ranks and filters retrieval results.

## Overview

- **Module ID**: `test-reasoning-plugin`
- **Type**: REASONING
- **Version**: 1.0.0

## Building

```bash
make build
```

## Development

1. Edit `handler.go` to implement your custom logic
2. Update `manifest.json` with your capabilities
3. Build with `make build`
4. Test locally: `make run`

## Testing Locally

```bash
# Start the module
./test-reasoning-plugin --socket /tmp/test-reasoning-plugin.sock

# In another terminal, test with grpcurl
grpcurl -plaintext -unix /tmp/test-reasoning-plugin.sock \
    mdemg.module.v1.ModuleLifecycle/HealthCheck
```

## Deployment

Place the built binary and `manifest.json` in the MDEMG plugins directory:

```
plugins/
  test-reasoning-plugin/
    test-reasoning-plugin    # binary
    manifest.json
```

MDEMG will auto-discover the plugin on startup.

## Reasoning Module

This module implements:
- **Process**: Re-rank/filter retrieval candidates

### Customization

Edit the `Process` function to implement your ranking algorithm.
The default implementation uses keyword-based boosting.


## Configuration

Configuration is passed via `manifest.json`:

```json
{
  "config": {
    "key": "value"
  }
}
```

Access configuration in your handler via `req.Config` in the Handshake RPC.
