# HITL-ERROR-VISIBLE-001 — Sprint Post

**Ship date**: 2026-09-20
**Wall-clock**: ~25 min
**Status**: SHIPPED (single-commit; opt-in interface)
**Parent**: JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001 disclosed follow-up

## Goal delivered

Closes the JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001 sprint post's self-disclosed follow-up: the `human_class_queue` sink's named `errAutograderRejected` was wrapped as `"internal error during review reinforcement apply"` before reaching the client, hiding the taxonomy-citing rejection reason.

Live smoke result:

```
POST /v1/review/grade  (grader_id=auto:*  human_class_queue  force=true)

pre-fix:
  {"error": "internal error during review reinforcement apply"}

post-fix:
  {"error": "human_class_queue: auto-grader rejected — human-verifiability
    class is operator-only by construction (LLM cannot verify these rules;
    see JIMINY-METRIC-DENOMINATOR-DESIGN-001 taxonomy)"}
```

## What shipped

| File | Change |
|---|---|
| `internal/api/server.go` | New `ClientVisibleError` interface + `sanitizeError` opt-in via `errors.As` |
| `internal/api/human_class_queue_dataset.go` | `errAutograderRejected` re-typed as `*clientVisibleError`; sentinel semantics + `errors.Is` preserved |
| `internal/api/caller_cancel_test.go` | 4 new pin tests: generic-stays-generic, caller-cancel-preserved, client-visible-surfaces, errors.As-unwrap |
| `docs/development/hitl-error-visible-001/sprint_plan.md` | 12-section plan |
| `docs/development/hitl-error-visible-001/sprint_post.md` | This file |

## Design

Opt-in interface:

```go
type ClientVisibleError interface {
    error
    ClientVisible() string
}
```

`sanitizeError` checks via `errors.As` BEFORE the generic fallback. This:
- Preserves the security contract by default (every plain error still sanitized)
- Requires explicit opt-in per error type
- Works through `fmt.Errorf("...%w", err)` wrapping via `errors.As`

Sinks author errors that MUST surface (policy violations citing a taxonomy, operator-actionable validation) as `*clientVisibleError`; every other error stays opaque.

## Verification (all green)

- ✅ `go build ./...` clean
- ✅ `go test ./internal/api/... -run "TestSanitizeError|TestWriteInternalError|TestHumanClassSink"` — 4 new + 4 existing pins pass
- ✅ Live Tier-3 smoke: `auto:*` grade → sink's full text surfaced (see TL;DR above)
- ✅ `errors.Is(err, errAutograderRejected)` still matches — sink-internal semantics preserved
- ✅ Cleanup: smoke rows deleted post-verification

## Arch rule proposal (not unilaterally pinned)

**Client-visible error opt-in via `ClientVisibleError` interface**: when a handler needs to surface a specific error message to the client while preserving the "don't leak internal detail" default, the error type MUST implement `ClientVisible() string`. `sanitizeError` uses `errors.As` for the opt-in check. Applies to any HTTP-facing handler under `internal/api/`.

Left as proposal — the pattern is documented in code comments; formalization to CLAUDE.md is operator's call.

## Rollback

`git revert` on the single commit undoes:
- Interface definition
- Sanitizer extension
- Sink error re-typing
- Pin tests

Zero substrate mutation.

## Documents Accessed

- `docs/development/jiminy-hitl-human-class-integration-001/sprint_post.md` — parent + disclosed follow-up
- `internal/api/server.go::sanitizeError` (line 3344) — extension point
- `internal/api/human_class_queue_dataset.go::errAutograderRejected` — the error re-typed
- `internal/api/caller_cancel_test.go` — pattern for pin tests
- `internal/api/handlers_review.go` (line 343) — call site (unchanged; auto-uses the extension)
