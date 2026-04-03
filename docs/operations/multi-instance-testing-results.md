# Multi-Instance Testing Results

**Date:** 2026-04-03
**Version:** v0.5.3 (dev build from commit 320e1fa)
**Machine:** macOS, Apple Silicon, 128 GB RAM, Docker Desktop
**Instances tested:** 4 simultaneous (1 dev + 3 test)

---

## Summary

All 6 test sessions passed. Multi-instance isolation works correctly through Docker Compose's COMPOSE_PROJECT_NAME scoping. Two gaps were confirmed: LaunchAgent label clash (known) and `mdemg teardown` not stopping Docker Compose services (new bug).

---

## Session 1: Init & Port Allocation — PASS

`FindFreePort()` correctly detected in-use ports and allocated the next available.

| Service | Dev | Instance A | Instance B | Instance C |
|---------|-----|------------|------------|------------|
| MDEMG server | 9999 | 10000 | 10001 | 10002 |
| Neo4j Bolt | 7687 | 7688 | 7689 | 7690 |
| Neo4j HTTP | 7474 | 7475 | 7476 | 7477 |
| TimescaleDB | 5433 | 5434 | 5435 | 5436 |
| Neural sidecar | 8100 | 8101 | 8102 | 8103 |
| Grafana | 3000 | 3001 | 3002 | 3003 |
| COMPOSE_PROJECT_NAME | mdemg | mdemg-instance-a | mdemg-instance-b | mdemg-instance-c |

All 20 containers started healthy. All healthz endpoints returned 200.

### Bug Found: Homebrew binary uses stale compose template

The Homebrew-installed binary (v0.5.1) embeds a compose template with `build: ./neural` for the neural-sidecar service. This fails for non-repo installs because the `./neural` directory doesn't exist. The fix (GHCR image reference) shipped in v0.5.2 but the Homebrew formula wasn't updated until v0.5.3. Users on v0.5.1 must use `mdemg upgrade` or `brew upgrade mdemg` to get the fix.

---

## Session 2: Data Isolation — PASS

- Created test nodes in instance-a's Neo4j (`space_id=test-a`) and instance-b's Neo4j (`space_id=test-b`)
- Cross-instance queries returned 0 results (zero leakage)
- Each instance has its own TimescaleDB with independent schema (9 tables each)
- Docker volumes fully scoped: `mdemg-instance-a_neo4j_data`, `mdemg-instance-b_neo4j_data`, etc.

---

## Session 3: Resource Measurement — PASS

| Metric | 1 instance (dev) | 2 instances | 3 instances | 4 instances | Under load (4) |
|--------|-----------------|-------------|-------------|-------------|----------------|
| Containers | 5 | 10 | 15 | 20 | 20 |
| Total memory | 5.7 GiB | 8.0 GiB | 10.4 GiB | 12.7 GiB | 12.7 GiB |
| CPU (idle) | ~1% | ~2% | ~3% | ~4% | 5.7% |
| Host free memory | - | - | - | 61.1 GB | - |

### Per-Instance Memory Breakdown

| Component | Dev (mature) | Fresh instance |
|-----------|-------------|----------------|
| Neo4j | 4.2 GiB | 1.5 GiB |
| Neural sidecar | 548 MiB | 555 MiB |
| TimescaleDB | 292 MiB | 215 MiB |
| Grafana | 149 MiB | 106 MiB |
| MDEMG server | 499 MiB | 9 MiB |
| **Total** | **5.7 GiB** | **2.3 GiB** |

**Key insight:** The dev instance uses 5.7 GiB because its Neo4j has 34,000+ nodes (4.2 GiB). Fresh instances use only 2.3 GiB. As graphs grow, expect Neo4j memory to scale proportionally.

### Hardware Recommendations

| Instance count | Min RAM | Recommended RAM | Notes |
|---------------|---------|-----------------|-------|
| 1 | 8 GB | 16 GB | Neo4j gets 512M page cache on 16 GB tier |
| 2 | 16 GB | 32 GB | ~4.6 GiB Docker + OS overhead |
| 3 | 32 GB | 64 GB | ~7 GiB Docker + headroom for graph growth |
| 4+ | 64 GB+ | 128 GB | Each additional instance adds ~2.3 GiB |

---

## Session 4: Observability — PASS (with confirmed known gap)

- Browser UI: each instance's `/ui/` showed only its own data
- Grafana: each instance connected to its own TSDB datasource
- Docker container logs: fully isolated (`docker logs mdemg-instance-a-mdemg-1` vs `mdemg-instance-b-mdemg-1`)

### LaunchAgent Clash — CONFIRMED

- `mdemg service install` from instance-a wrote `com.mdemg.server.plist` with `WorkingDirectory=/tmp/mdemg-multi/instance-a`
- `mdemg service install` from instance-b **overwrote** the plist with `WorkingDirectory=/tmp/mdemg-multi/instance-b`
- Labels are hardcoded in `internal/cli/service_darwin.go:16-24`
- **Recommendation:** Only use `mdemg service install` for your primary instance. Other instances should rely on Docker Compose's `restart: unless-stopped` policy.

### Shared Log Paths — CONFIRMED (low impact)

- LaunchAgent log paths (stdout/stderr) point to `~/.mdemg/logs/` for all instances
- Docker container logs are per-container and fully isolated
- Impact is low because Docker Compose is the recommended deployment, not LaunchAgents

---

## Session 5: Edge Cases — PASS (with new bug found)

- Shutting down one instance: others completely unaffected
- Restarting a stopped instance: data persisted through Docker volume lifecycle
- Init third instance while two running: ports auto-allocated correctly (20 containers)

### Bug Found: `mdemg teardown` doesn't stop Docker Compose services

`mdemg teardown --yes` in a Docker Compose project directory:
1. Looked for container `mdemg-neo4j-instance-c` (legacy naming) — not found
2. Looked for volume `mdemg-neo4j-data-instance-c` (legacy naming) — not found
3. Did NOT run `docker compose down` to stop the 5 running services
4. All 5 containers remained running after teardown completed

**Root cause:** `teardown` was written for the legacy `mdemg db start` single-container model. It doesn't know about Docker Compose deployments.

**Fix needed:** When `docker-compose.yml` exists in the project directory, `teardown` should run `docker compose down -v` before cleaning up config files.

---

## Session 6: Cleanup — PASS

All test artifacts removed:
- 15 test containers stopped and removed
- 12 test volumes deleted
- `/tmp/mdemg-multi/` removed
- LaunchAgents cleaned
- Dev instance verified healthy (all 5 containers, healthz OK)

---

## Bugs Found

| # | Bug | Severity | File | Status |
|---|-----|----------|------|--------|
| 1 | `mdemg teardown` doesn't stop Docker Compose services | MEDIUM | `internal/cli/teardown.go` | NEW — needs fix |
| 2 | Homebrew v0.5.1 binary embeds stale compose template (`build: ./neural`) | LOW | Fixed in v0.5.2 | RESOLVED — `mdemg upgrade` |

---

## Known Limitations (Document in User Guide)

1. **LaunchAgent labels are not instance-scoped** — `mdemg service install` from a second instance overwrites the first. Use Docker Compose `restart: unless-stopped` for multi-instance.
2. **~/.mdemg/logs/ shared** — LaunchAgent-managed services write to the same log directory. Docker container logs are isolated.
3. **Port range:** Each instance scans +100 from preferred port. With default preferred ports, this allows ~16 instances before range exhaustion.
4. **`mdemg teardown` doesn't stop Docker Compose services** — must run `docker compose down -v` manually before or after teardown.

---

## Documents Accessed

- `internal/cli/init.go` — port allocation, COMPOSE_PROJECT_NAME derivation
- `internal/cli/docker.go` — FindFreePort implementation
- `internal/cli/service_darwin.go` — LaunchAgent label definitions
- `internal/cli/teardown.go` — teardown command (Docker Compose gap)
- `packaging/launchd/*.plist` — LaunchAgent templates
- `docker-compose.yml` — compose service definitions
