# Test APE Plugin

An APE (Active Participant Engine) module for MDEMG that performs background maintenance tasks.

## Overview

- **Module ID**: `test-ape-plugin`
- **Type**: APE
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
./test-ape-plugin --socket /tmp/test-ape-plugin.sock

# In another terminal, test with grpcurl
grpcurl -plaintext -unix /tmp/test-ape-plugin.sock \
    mdemg.module.v1.ModuleLifecycle/HealthCheck
```

## Deployment

Place the built binary and `manifest.json` in the MDEMG plugins directory:

```
plugins/
  test-ape-plugin/
    test-ape-plugin    # binary
    manifest.json
```

MDEMG will auto-discover the plugin on startup.

## APE Module

This module implements:
- **GetSchedule**: Define when this module should run
- **Execute**: Perform background maintenance tasks

### Customization

Edit `GetSchedule` to define your cron schedule and event triggers.
Edit the handler functions (`handleSessionEnd`, etc.) to implement your logic.


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
