# CONVERSATION-EMBEDDER-OPTIONAL-001 — Sprint Post

**Date:** 2026-08-06 | **Branch:** `reh3376_dev01`
**Trigger:** Beta-readiness Phase 2 walkthrough T2.2 (single observation POST) surfaced blocker B5 on the first ingest attempt after INIT-DEFAULTS-LOCAL-FIRST-001 shipped disabled-mode. The disabled-mode install started cleanly, dashboard rendered, /healthz was OK — but `POST /v1/conversation/observe` returned `{"error":"conversation service not available (embedder required)"}`. Effectively read-only for beta testers who didn't bring an embedder.

## Verdict

Shipped. `conversation.Service` is now constructed UNCONDITIONALLY in the API server — the pre-sprint `if emb != nil` gate around construction was overzealous. The service already handled nil embedder gracefully internally (`Observe()` skips embedding + surprise detection when embedder is nil; observations still land as raw content + metadata + graph nodes). The gate at construction was the only thing preventing observe from working in disabled mode.

## What was wrong

`internal/api/server.go:315-342` gated the ENTIRE conversation service construction on `emb != nil`:
```go
if emb != nil {
    convSvc = conversation.NewServiceWithConfig(driver, emb, ...)
    convSvc.SetLearningService(lea)
    // ... 20 lines of setup ...
} else {
    slog.Info("conversation service disabled (requires embedder)")  // ← silent lockout
}
```

Meanwhile `conversation.NewServiceWithConfig:69-75` and `Observe():272-361` ALREADY guarded on `embedder != nil`:
- `SurpriseDetector` — created only when embedder present (`service.go:69`)
- Observe's embedding call — guarded (`service.go:272`)
- Surprise detection — guarded (`service.go:361`)
- Constraint detection (regex-based, embedder-independent) — always runs

So the internal service was ready to handle nil-embedder; the server just refused to build it. Result: a fresh install in `EMBEDDING_PROVIDER=disabled` mode (the new default from INIT-DEFAULTS-LOCAL-FIRST-001 for testers without an OpenAI key) hit `503 Service Unavailable` on the very first observe attempt.

## What shipped

### `internal/api/server.go` — construct unconditionally
```go
// Was: if emb != nil { convSvc = ... } else { slog.Info("disabled") }
// Now:
convSvc := conversation.NewServiceWithConfig(driver, emb, cfg.VectorIndexName, cfg)
convSvc.SetLearningService(lea)
slog.Info("conversation service initialized",
    "vector_index", cfg.VectorIndexName,
    "constraint_detection", cfg.ConstraintDetectionEnabled,
    "embedder_available", emb != nil)   // ← honest visibility into the disabled path
```

Also:
- `ContextCatalog loader` still gated on `emb != nil` (fingerprints require an embedder — no point loading the catalog)
- `Context Cooler` created unconditionally (operates on graph-side stability scores; embedder-independent)

### `internal/conversation/service_nil_embedder_test.go` — 4 pin tests
- `TestNewServiceWithConfig_NilEmbedderConstructsCleanly` — regression pin for the fix
- `TestNewService_NilEmbedderConstructsCleanly` — the public convenience constructor path
- `TestNewServiceWithConfig_NilEmbedderNoConfigStillWorks` — the no-config overload
- `TestNewServiceWithConfig_SetLearningServiceSafeWithoutEmbedder` — `SetLearningService` called unconditionally in api/server.go; must not panic on nil-embedder service

All 4 pass. Constraint detection stays enabled (regex-based, embedder-independent) — verified by pin.

## Live Tier-3

**Blocker reproduction (pre-fix, from Phase 2 walkthrough):**
```
$ curl -X POST http://localhost:PORT/v1/conversation/observe -d '{"space_id":"...","content":"...","obs_type":"note"}'
{"error":"conversation service not available (embedder required)"}
```

**Post-fix verification approach**: unit-level (4 pin tests) + Docker-image validation deferred to next CI release build. The scratch install pulls `ghcr.io/reh3376/mdemg:latest` (the shipped image), NOT the local `./bin/mdemg` build — so live-verifying against the fresh Docker stack requires either (a) building a local image + overriding `MDEMG_VERSION`, or (b) waiting for the release-tag CI build.

The pin tests directly assert the invariants that were violated: `NewServiceWithConfig(nil, nil, ...)` returns a non-nil service; `SetLearningService(nil)` is safe. These are the exact code paths that were broken.

Beta-blocker B5 is CODE-VERIFIED FIXED; docker-image live verification will happen on `v0.11.0-beta.1` cut.

`go test ./internal/conversation/ ./internal/api/` clean; lint 0 issues.

## Rules pinned

⚠️ **Constructor-time gates on optional dependencies must match the gates inside the object being constructed** — if `Observe()` guards on `embedder != nil`, the SERVER must not refuse to construct the Service when embedder is nil. Every "graceful nil handling inside" that's contradicted by "hard refusal to construct outside" produces a lockout that the caller can't diagnose (the server just quietly doesn't wire the service; handlers all return "service unavailable"). Audit both sides of the boundary in the same commit.

⚠️ **`if X != nil { CONSTRUCT_ALL } else { LOG_DISABLED }` is a beta-blocker anti-pattern** — the log ONLY appears in server-side logs the beta tester won't grep. Users experience it as "the whole feature doesn't work" with no next-step. Prefer: construct always with the nullable dependency, let per-operation guards emit specific "this operation requires X" errors that name the exact operator action.

⚠️ **Docker-image validation of a fix requires either a local image build + `MDEMG_VERSION` override OR waiting for release-tag CI** — `docker compose up -d` in a fresh scratch dir pulls the shipped `ghcr.io/reh3376/mdemg:latest`, not the local build tree. Beta-readiness sprints that modify server construction MUST document this validation gap explicitly and either build-and-override or accept CI-time validation.

## Not shipped (intentional)

- **Rewrite of the 4 handler "embedder required" errors** at `handlers_conversation.go:33,164,222,330` — now dead code (`s.conversationSvc` is never nil post-fix) but harmless defensive checks. Removing them would widen the blast radius past the beta blocker; leave them as fail-safes.
- **`handlers_meta.go:26` "embedder required for meta-learning"** — meta-learning genuinely requires an embedder (embedding-cross-space similarity). Leave the error as-is; that endpoint should stay unavailable in disabled mode.
- **Retrieval graceful-degrade to BM25 in disabled mode** — separate concern (retrieval service, not conversation service). Verify in a follow-up sprint if `POST /v1/memory/retrieve` still works with no embedder — it should already, given `RETRIEVAL_COLUMN_EMBEDDING_ENABLED` can be false. Deferred to Phase 2 continuation.

## Follow-ups disclosed

- **B5 docker-image live verification** — cut `v0.11.0-beta.1-rc.1` OR run a local image build against a scratch stack; verify T2.2 succeeds. Do BEFORE the beta tag.
- **T1.9 walkthrough correction** — my Phase 2 walkthrough used the wrong URL (`/v1/embed/health` — 404 as expected). Real spec is `mdemg embeddings check` + `/v1/embedding/health` (singular). Both work in current build. No code change; update the walkthrough report only.
- **Retrieval-in-disabled-mode audit** — verify `POST /v1/memory/retrieve` returns BM25 results without embedder. Should already work per the shipped column-voting flags; needs live smoke.

## Rollback

Single-commit revert. Pre-sprint behavior restored: `conversationSvc` stays nil in disabled mode; every observe/correct/skill call returns 503 "embedder required."

## Documents Accessed

- `internal/api/server.go:315-342` (the gate; edited)
- `internal/conversation/service.go:47-99, 272, 361` (Service constructor + Observe path — the pre-existing nil-embedder guards this fix relies on)
- `internal/api/handlers_conversation.go:33,164,222,330` (the 4 handlers whose "embedder required" errors were firing pre-fix)
- INIT-DEFAULTS-LOCAL-FIRST-001 post (parent — introduced the `disabled` embedding provider that made B5 reachable)
- CONFIG-VALIDATE-TRANSIENT-DISTINGUISH-001 post (sibling — the beta-readiness Phase 1 arc this closes)
- `packaging/homebrew-mdemg/mdemg_beta_testing.md` T2.2 (the beta test that surfaced B5)
- Live `/tmp/mdemg-beta-b5` scratch reproduction (Phase 2 walkthrough evidence)
