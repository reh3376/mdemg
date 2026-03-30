# Process Lifecycle

Phase 97 adds daemon-mode process management so you don't need a dedicated terminal for the MDEMG server.

## Commands

### Start (Background)

```bash
mdemg start                         # Start with defaults
mdemg start --port 9999             # Custom port
mdemg start --auto-migrate          # Apply migrations on startup
mdemg start --mcp                   # Start MCP server alongside
mdemg start --no-db                 # Don't auto-start Neo4j
```

The server runs as a detached background process. Logs go to `.mdemg/logs/mdemg.log` and the PID is stored in `.mdemg/mdemg.pid`.

Before forking the daemon child, `mdemg start` loads configuration into the parent process environment so the child inherits the correct values. The loading order matches the documented config priority: `.env` is loaded first (via `godotenv`), then `.mdemg/config.yaml` (which skips any env var already set). This ensures `.env` values take precedence over YAML defaults. The child process inherits the fully resolved environment via `os.Environ()`.

If Docker is available and a Neo4j container exists but is stopped, `mdemg start` will start it automatically. Use `--no-db` to skip this behavior.

### Stop

```bash
mdemg stop
```

Sends SIGTERM and waits up to 30 seconds for graceful shutdown. If the process doesn't exit, SIGKILL is sent.

This stops the MDEMG server only — Neo4j is left running. The output includes a reminder:

```
MDEMG server stopped
Note: Neo4j container may still be running (stop with: mdemg db stop)
```

### Restart

```bash
mdemg restart                       # Stop then start
mdemg restart --port 9999           # Stop then start with new port
```

Supports all flags from `mdemg start`.

### Status

```bash
mdemg status
```

Output:

```
MDEMG Status
============
  Server:    running (pid=12345)
  Port:      9999
  Uptime:    2h 15m
  Log:       .mdemg/logs/mdemg.log
  Health:    ok
  Neo4j:     running (mdemg-neo4j-dev)
```

When the server is running, `mdemg status` also hits `/healthz` to verify the HTTP server is responsive.

### Foreground (Existing)

```bash
mdemg serve                         # Foreground mode (unchanged)
```

`mdemg serve` continues to work as before for development and debugging.

## Files

| Path | Purpose |
|------|---------|
| `.mdemg/mdemg.pid` | PID of the running server process |
| `.mdemg/logs/mdemg.log` | Server log output (truncated on each start) |
| `.mdemg.port` | Port file written by the server for client discovery |

## Typical Workflow

```bash
# One-time setup
mdemg db start                      # Start Neo4j
mdemg db migrate                    # Apply schema

# Daily development
mdemg start --auto-migrate          # Start server in background
# ... work on code ...
mdemg status                        # Check health
mdemg restart                       # After config changes
mdemg stop                          # End of day
mdemg db stop                       # Optional: stop Neo4j too
```

## Docker Deployment (Primary)

Docker Compose is the primary deployment method. All 5 services (mdemg, neo4j, timescaledb, neural-sidecar, grafana) run as containers with `restart: unless-stopped` for automatic recovery.

```bash
mdemg init --quick              # Initialize + start Docker stack
docker compose ps               # Check service status
docker compose logs -f mdemg    # Follow server logs
docker compose restart          # Restart all services
docker compose down             # Stop all services
```

Docker's `restart: unless-stopped` replaces LaunchAgent/systemd for server process supervision. See `docs/user/quickstart-docker.md` for the full Docker deployment guide.

## Process Supervision (Native, Dev-Only)

For native development builds (not Docker), persistent process supervision uses OS-level mechanisms. See `docs/features/service-resilience.md` and `mdemg service install`.
