---
created: 2026-04-02
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "97"
---

# Process Lifecycle

## Summary

**Feature**: Process Lifecycle Management
**Summary**: Daemon-mode process management (`mdemg start/stop/restart/status`) so you don't need a dedicated terminal for the MDEMG server. Includes Docker Compose as the primary deployment method.

## Vision & Goals

Developers need the server running in the background during their work sessions. Process lifecycle management provides start/stop/restart/status commands that work like any Unix daemon — PID file, log rotation, health checks. For production, Docker Compose with `restart: unless-stopped` replaces OS-level supervision.

## Current State

### Architecture

**Native Mode** — Server runs as a detached background process:
- Logs to `.mdemg/logs/mdemg.log` (truncated on each start)
- PID stored in `.mdemg/mdemg.pid`
- Port written to `.mdemg.port` for client discovery
- Config loading: `.env` (godotenv) -> `.mdemg/config.yaml` -> env vars -> CLI flags
- Auto-starts Neo4j container if Docker available and container exists but stopped

**Docker Mode** (Primary) — All 5 services via Docker Compose:
- `restart: unless-stopped` for automatic recovery
- `mdemg init --quick` initializes and starts the full stack

### Workflow

**Start/Stop/Restart:**

```bash
mdemg start                         # Background daemon
mdemg start --port 9999 --auto-migrate --mcp  # With options
mdemg stop                          # SIGTERM + 30s grace + SIGKILL
mdemg restart                       # Stop then start (inherits flags)
mdemg status                        # PID, port, uptime, health, Neo4j status
mdemg serve                         # Foreground (dev/debug)
```

**Stop** sends SIGTERM and waits up to 30 seconds. Stops MDEMG server only — Neo4j left running.

**Docker Compose:**

```bash
mdemg init --quick              # Initialize + start
docker compose ps               # Check status
docker compose logs -f mdemg    # Follow logs
docker compose restart           # Restart all
docker compose down              # Stop all
```

### Configuration

No special configuration — uses standard config priority chain.

## Notes

### Known Limitations

- `mdemg stop` only stops the MDEMG server, not Neo4j (by design — displays reminder)
- Log file truncated on each start (no rotation)

### Risks & Gaps

None identified.

### Future Improvements

- Log rotation with configurable retention
- `mdemg start --all` to also start Neo4j + TSDB containers

## API Endpoints

| Method | Endpoint | Description | UATS Spec |
|--------|----------|-------------|-----------|
| GET | `/healthz` | Basic health check (used by `mdemg status`) | `specs/health.uats.json` |

## CLI Commands

| Command | Description |
|---------|-------------|
| `mdemg start [flags]` | Start server in background (daemon mode) |
| `mdemg stop` | Stop background server (SIGTERM + SIGKILL fallback) |
| `mdemg restart [flags]` | Stop then start with new flags |
| `mdemg status` | Show PID, port, uptime, health, Neo4j status |
| `mdemg serve [flags]` | Start server in foreground (dev/debug) |

## Configuration Reference

None — uses standard config priority chain.

## Dependencies

| Feature | Relationship |
|---------|-------------|
| Docker | Optional — auto-starts Neo4j container if available |
| Config System | Requires — loads .env and config.yaml before forking |
| Service Resilience | Enhances — OS-level supervision for native deployments |

## Related Files

- `internal/cli/daemon.go` - Start/stop/restart/status implementation
- `internal/cli/serve.go` - Foreground server (`mdemg serve`)
- `.mdemg/mdemg.pid` - PID file
- `.mdemg/logs/mdemg.log` - Server log output
- `.mdemg.port` - Port discovery file
- `docs/features/service-resilience.md` - OS-level supervision details
