# Test Ingestion Plugin

An INGESTION module for MDEMG that parses external sources into observations.

## Overview

- **Module ID**: `test-ingestion-plugin`
- **Type**: INGESTION
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
./test-ingestion-plugin --socket /tmp/test-ingestion-plugin.sock

# In another terminal, test with grpcurl
grpcurl -plaintext -unix /tmp/test-ingestion-plugin.sock \
    mdemg.module.v1.ModuleLifecycle/HealthCheck
```

## Deployment

Place the built binary and `manifest.json` in the MDEMG plugins directory:

```
plugins/
  test-ingestion-plugin/
    test-ingestion-plugin    # binary
    manifest.json
```

MDEMG will auto-discover the plugin on startup.

## Ingestion Module

This module implements:
- **Matches**: Determine if this module can handle a given source
- **Parse**: Convert raw content into MDEMG observations
- **Sync**: Incrementally sync with external sources

### Customization

Edit the `Matches` function to define which sources this module handles.
Edit the `Parse` function to implement your parsing logic.


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
