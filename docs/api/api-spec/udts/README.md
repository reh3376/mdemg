# UDTS — Universal DevSpace Test Specification

**Version:** 1.0.0  
**Purpose:** Contract and integration tests for gRPC services (Space Transfer, future DevSpace hub).

UDTS is to gRPC what UATS is to HTTP: a language-agnostic, spec-driven test format with optional hash verification for proto/contract stability.

---

## Overview

- **Spec format:** JSON (`.udts.json`) describing service, method, request, and expected response or stream behavior.
- **Runner:** Go test or CLI that dials a gRPC server (e.g. `space-transfer serve`), runs the RPC, and asserts per spec.
- **Hash verification:** Optional `proto_sha256` in spec or manifest to ensure proto changes are intentional.

---

## Quick Start

1. Start the gRPC server (e.g. Space Transfer with Neo4j):

   ```bash
   export NEO4J_URI=bolt://localhost:7687 NEO4J_USER=neo4j NEO4J_PASS=testpassword
   go run ./cmd/space-transfer/ serve -port 50051
   ```

2. Run UDTS contract tests:

   ```bash
   export UDTS_TARGET=localhost:50051
   go test ./tests/udts/... -v -count=1
   ```

   Or run the runner against a spec:

   ```bash
   go run ./tests/udts/runner/ -spec docs/api/api-spec/udts/specs/space_transfer_list_spaces.udts.json -target localhost:50051
   ```

   (If no runner CLI exists yet, tests in `tests/udts` can load specs and run gRPC calls.)

---

## Spec Structure

| Field | Description |
|-------|-------------|
| `udts_version` | Schema version (semver) |
| `service` | Fully qualified service name (e.g. `mdemg.transfer.v1.SpaceTransfer`) |
| `method` | RPC method name (e.g. `ListSpaces`) |
| `request` | Request message (JSON object or empty `{}` for no-arg) |
| `expected` | Assertions: `response_has_field`, `stream_chunk_count_min`, `status_code` (gRPC code name) |
| `config` | Optional `timeout_ms`, `proto_sha256` for contract hash |

---

## Files

- `schema/udts.schema.json` — JSON schema for `.udts.json` specs
- `specs/*.udts.json` — Canonical unary/stream RPC contract specs consumed by `tests/udts`
- `drafts/*.udts.json` — Draft scenario-style specs (not consumed by `tests/udts` yet)
- Runner: `tests/udts/` (Go tests that load specs and run gRPC calls)

---

## Hash Verification

To lock contract stability, compute SHA256 of the proto source and store in spec or a manifest:

```bash
shasum -a 256 api/proto/space-transfer.proto
```

Set `config.proto_sha256` in the spec; the runner can verify before running (optional).
