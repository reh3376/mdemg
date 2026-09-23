# HITL-ERROR-VISIBLE-001 — Sprint Plan

## 1. Header & Metadata

- **Sprint ID**: HITL-ERROR-VISIBLE-001
- **Roadmap slot**: Q5 §5 self-disclosed follow-up (from PR 671 sprint post)
- **Parent**: JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001 (Q5 §3 #2)
- **Date**: 2026-09-20
- **Effort**: ~30 min
- **Impact class**: operational (operator-facing error UX)

## 2. Problem Statement

JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001 live smoke revealed a UX gap: when the `human_class_queue` sink refuses an `auto:*` grader with a NAMED error (`errAutograderRejected`), the HITL handler wraps it via `sanitizeError` as the opaque `"internal error during review reinforcement apply"`. The specific taxonomy-citing message the sink authored is lost.

`sanitizeError` deliberately hides the underlying error text (security contract — don't leak internals to clients), so a blanket "expose all sink errors" fix would regress that. What's needed: a targeted opt-in for errors the SINK explicitly marks as safe-to-surface.

## 3. Scope & Constraints

**In-scope**:
- Small opt-in interface `ClientVisibleError` in `internal/api` (or shared package)
- Extend `sanitizeError` to check the interface via `errors.As` BEFORE falling back to the generic message
- Convert the shipped `errAutograderRejected` in `internal/api/human_class_queue_dataset.go` to implement the interface
- Pin tests: (a) generic errors still sanitize, (b) `ClientVisibleError` surfaces its message, (c) `errAutograderRejected` yields its taxonomy-citing text through the handler
- Live smoke: send `auto:*` grade to human_class_queue → verify response error body contains the sink's message

**Out-of-scope**:
- Cross-handler audit for other sinks that might want to opt in (this sprint plumbs the mechanism; future sprints can adopt)
- Refactoring `sanitizeError`'s log-level or observability
- Changes to `writeInternalError`'s HTTP status logic (caller-cancel → 499 preserved)

**Constraints**:
- `must-follow-12-section-format` — this doc ✓
- `never-hardcode-config` — no new knobs (interface is a code contract, not a runtime toggle)
- `unit-integration-e2e-docs` — 3 tier plan below
- `live-testing-tier-required` — real curl smoke against real binary
- `end-with-docs-accessed` — §12 populated

## 4. Dependencies

- **Upstream (live)**: JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001 (shipped errAutograderRejected + sink)
- **Downstream (unblocked)**: any future sink that wants to expose a specific-error message to clients

## 5. Implementation Plan

Single epic; small scope.

### Epic 1 — Interface + sanitizer extension + sink update

- **E1.1**: Add `ClientVisibleError` interface in `internal/api/server.go` (co-located with `sanitizeError`):
  ```go
  // ClientVisibleError is an opt-in for errors that carry a safe-to-surface
  // message. sanitizeError uses it via errors.As before falling back to the
  // generic "internal error during X". Sinks/handlers implement this only for
  // errors that MUST NOT leak internal detail; every other error stays
  // sanitized (preserves the security contract).
  type ClientVisibleError interface {
      error
      ClientVisible() string
  }
  ```
- **E1.2**: Extend `sanitizeError` to check `errors.As(err, &cv)` BEFORE the generic fallback. Log at INFO with the sink's message; return it verbatim to the client.
- **E1.3**: Convert `errAutograderRejected` in `internal/api/human_class_queue_dataset.go` from `fmt.Errorf` to a `clientVisibleError` wrapper implementing `ClientVisibleError`. Preserve the existing `errors.Is` semantics via a sentinel (`errors.Is` unchanged for sink-internal checks).
- **Gate**: `go build ./...` clean; existing pin tests for the sink (`TestHumanClassSink_AutograderRejected` uses `errors.Is`) still green.

## 6. Testing Plan

**Tier 1 — Unit**:
- `TestSanitizeError_ClientVisibleErrorSurfacesMessage` (NEW, `internal/api/server_test.go` or a new file) — construct a `clientVisibleError`, verify `sanitizeError(err, "op")` returns the client-visible message verbatim.
- `TestSanitizeError_GenericErrorStaysGeneric` (NEW) — plain `errors.New("boom")` → returns `"internal error during op"` (regression pin).
- `TestSanitizeError_CallerCancelPreserved` (NEW) — caller-cancelled error still routes to "request cancelled during op" (contract preserved).
- Existing `TestHumanClassSink_AutograderRejected` — verifies `errors.Is(err, errAutograderRejected)` still fires after the wrapping change.

**Tier 2 — Integration**:
- N/A — no cross-package integration (the interface is intra-package)

**Tier 3 — Live e2e**:
- Curl `POST /v1/review/grade` on `human_class_queue` with `grader_id=auto:test@sha, force=true` → verify response JSON's `error` field contains "JIMINY-METRIC-DENOMINATOR-DESIGN-001" (the taxonomy citation the sink authored).

## 7. Commit Strategy

Single commit:
1. `feat(api): ClientVisibleError interface + sanitizer opt-in + human_class_queue sink adopts (HITL-ERROR-VISIBLE-001)`

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `go test ./internal/api/... -count=1` green (3 new pins + existing sink pin)
- [ ] `golangci-lint run ./internal/api/...` 0 issues
- [ ] All 5 drift checks clean (docs / metrics / config / tsdb / routes)
- [ ] Live Tier-3 curl smoke returns error body containing "JIMINY-METRIC-DENOMINATOR-DESIGN-001"

## 9. Documentation Update

- Sprint post `docs/development/hitl-error-visible-001/sprint_post.md`
- CHANGELOG entry (small — infrastructure)
- CLAUDE.md pin ONLY if the interface class is worth pinning (candidate — client-visible error opt-in is a shape reusable across handlers)

## 10. Risks & Mitigations

| Risk | Class | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| Sink error leaks internal detail | security | LOW | MED | Interface is OPT-IN — errors must explicitly implement it. Every other error path unchanged. |
| `errors.Is` semantics break at the sentinel check | correctness | LOW | LOW | Preserve `errAutograderRejected` as a package-level sentinel; wrap Error() around it; keep the sink's `if errors.Is(err, errAutograderRejected)` intact. |
| Handler skips the wrap by using `writeJSON` directly | drift | LOW | LOW | Handler already uses `writeInternalError` uniformly (verified pre-sprint). No other paths need touching. |

## 11. Rollback Procedures

`git revert <commit>`. Pure additive code + one wrapper struct in the sink. Zero substrate mutation.

## 12. Documents Accessed

- `docs/development/jiminy-hitl-human-class-integration-001/sprint_post.md` — the parent + disclosed follow-up
- `internal/api/server.go::sanitizeError` (line 3344) + `writeInternalError` (line 3357) — extension points
- `internal/api/human_class_queue_dataset.go::errAutograderRejected` — the error to convert
- `internal/api/human_class_queue_dataset_test.go::TestHumanClassSink_AutograderRejected` — sink pin to preserve
- `internal/api/handlers_review.go` (line 343) — the sanitizer call site
