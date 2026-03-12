# UxTS Module Testing Guide

## Test Tiers

| Tier | Tag | Requires | Run Command |
|------|-----|----------|-------------|
| Unit | (none) | Nothing | `go test ./plugins/uxts-module/...` |
| Integration | `integration` | Nothing (in-process gRPC) | `go test -tags=integration ./plugins/uxts-module/...` |
| E2E | `e2e` | Built binary, real spec files | `go test -tags=e2e ./plugins/uxts-module/...` |
| UDTS Contract | (env var) | Running plugin on socket | See below |

---

## 1. Unit Tests

**What they test:** Pure logic — drift hashing, spec index parsing, candidate annotation, score adjustment, HTTP client mocking, event handler polling.

```bash
go test -v ./plugins/uxts-module/...
```

**Expected:** All tests pass with 0 failures. Uses temp directories and mock HTTP servers — no external dependencies.

**Coverage areas:**
- `drift.go` — canonical JSON, SHA256 computation, drift detection, path traversal
- `spec_index.go` — loading, parsing, indexing, lookup maps, template stripping
- `reasoning.go` — Process RPC, annotation, score boost/penalty, top-k limiting
- `handler.go` — Handshake config parsing, HealthCheck metrics, Shutdown
- `results.go` — HTTP observe client, error handling, drift result reporting
- `event_handler.go` — polling, spec refresh, drift reporting, unreachable server

---

## 2. Integration Tests

**What they test:** Full gRPC lifecycle through Unix socket — handshake, health check, Process, shutdown. Also: multi-framework indexing, drift detection, concurrent access, score adjustment, and real spec file scanning.

```bash
go test -v -tags=integration ./plugins/uxts-module/...
```

**Key tests:**
- `TestIntegration_FullLifecycle` — complete happy path: start → handshake → health → process → shutdown
- `TestIntegration_MultiFrameworkIndex` — loads UATS + UDTS specs, verifies counts
- `TestIntegration_DriftDetection` — clean + drifted specs, verifies `uxts_drift` metadata
- `TestIntegration_RealSpecFiles` — scans actual `docs/` directory (if present)
- `TestIntegration_ConcurrentProcessing` — 10 concurrent Process calls, verifies thread safety
- `TestIntegration_ScoreAdjustment` — verifies boost and penalty application

---

## 3. E2E Tests

**What they test:** Manifest validation, binary compilation, real spec file counts against known minimums, drift detection consistency, lookup performance.

```bash
# Build binary first
go build -o plugins/uxts-module/uxts-module ./plugins/uxts-module/

# Run E2E tests
go test -v -tags=e2e ./plugins/uxts-module/...
```

**Key tests:**
- `TestE2E_ManifestValid` — validates all manifest.json fields
- `TestE2E_BinaryBuilds` — compiles binary, checks size
- `TestE2E_BinaryShowsHelp` — verifies socket flag requirement
- `TestE2E_RealSpecCount` — scans all frameworks, checks against known minimums
- `TestE2E_DriftDetectionMatchesPython` — validates Go drift detection against real specs
- `TestE2E_SpecIndexLookupPerformance` — 1000 path lookups + 100 text searches

---

## 4. UDTS Contract Tests

**What they test:** The plugin's own gRPC contract via 5 UDTS specs.

### Prerequisites

Start the uxts-module plugin on a Unix socket:

```bash
# Build the binary
go build -o plugins/uxts-module/uxts-module ./plugins/uxts-module/

# Start it on a socket
./plugins/uxts-module/uxts-module --socket /tmp/mdemg-plugins/mdemg-uxts-module.sock
```

### Run UDTS contract tests

```bash
UDTS_MODULE_SOCKET=/tmp/mdemg-plugins/mdemg-uxts-module.sock \
  go test -v ./tests/udts/... -run "TestUxTS"
```

### UDTS Spec files (in `docs/api/api-spec/udts/specs/`):

| Spec | Tests |
|------|-------|
| `uxts_module_handshake.udts.json` | Handshake returns REASONING type |
| `uxts_module_healthcheck.udts.json` | HealthCheck returns healthy with metrics |
| `uxts_module_shutdown.udts.json` | Shutdown returns success |
| `uxts_module_process_empty.udts.json` | Process with empty candidates |
| `uxts_module_process_with_candidates.udts.json` | Process annotates candidates |

---

## 5. Plugin Validator (via MDEMG API)

If MDEMG server is running with plugins enabled:

```bash
# List plugins
curl -s http://localhost:9999/v1/plugins | jq

# Validate uxts-module (4-level: manifest → proto → health → lifecycle)
curl -s -X POST http://localhost:9999/v1/plugins/uxts-module/validate | jq
```

---

## 6. Manual Verification Checklist

### Pre-flight

- [ ] `go build ./plugins/uxts-module/` — compiles cleanly
- [ ] `go test ./plugins/uxts-module/...` — all unit tests pass
- [ ] `golangci-lint run ./plugins/uxts-module/...` — 0 issues

### Spec Index

- [ ] `go test -v -tags=e2e -run TestE2E_RealSpecCount ./plugins/uxts-module/` — logs real counts
- [ ] Verify UATS count >= 100 (matrix says 124+)
- [ ] Verify UDTS count >= 10 (12 canonical: 7 original + 5 uxts-module)
- [ ] Verify multiple frameworks found (>= 5)

### Drift Detection

- [ ] `go test -v -tags=e2e -run TestE2E_DriftDetection ./plugins/uxts-module/` — runs drift scan
- [ ] Review drift count — should be minimal (only specs with outdated hashes)

### gRPC Lifecycle

- [ ] `go test -v -tags=integration -run TestIntegration_FullLifecycle ./plugins/uxts-module/`
- [ ] Handshake returns Ready=true with REASONING type
- [ ] HealthCheck reports total_specs > 0 after handshake
- [ ] Process annotates matching candidates with uxts_* metadata
- [ ] Process leaves non-matching candidates with coverage_count=0
- [ ] Shutdown returns success=true

### Score Adjustment

- [ ] `go test -v -tags=integration -run TestIntegration_ScoreAdjustment ./plugins/uxts-module/`
- [ ] Covered candidate: score > original (boost applied)
- [ ] Untested candidate: score unchanged

### Concurrency

- [ ] `go test -v -tags=integration -run TestIntegration_Concurrent ./plugins/uxts-module/`
- [ ] 10 concurrent Process calls complete without errors
- [ ] Request counter = 10 after concurrent calls

### UDTS Contract

- [ ] Start plugin: `./plugins/uxts-module/uxts-module --socket /tmp/uxts-test.sock`
- [ ] In another terminal: `UDTS_MODULE_SOCKET=/tmp/uxts-test.sock go test -v ./tests/udts/... -run TestUxTS`
- [ ] All 5 UDTS tests pass

---

## Running All Tiers

```bash
# Full suite (unit + integration + e2e)
go test -v -tags="integration,e2e" ./plugins/uxts-module/...

# Quick smoke (unit only, fastest)
go test ./plugins/uxts-module/...

# CI-safe (unit + integration, no external deps)
go test -tags=integration ./plugins/uxts-module/...
```
