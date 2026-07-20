# Sprint NEURAL-RERANK-PRECHECK-001 — provider-aware rerank budget pre-check

## 1. Header & Metadata
- **Sprint ID:** NEURAL-RERANK-PRECHECK-001
- **Sprint line:** `docs/development/neural-rerank-precheck-001/`
- **Date opened:** 2026-07-20
- **Target version:** v0.11.6 (patch)
- **Estimated effort:** ~0.4 dev-day, 5 sequential epics
- **OpenAI spend:** $0
- **Risk level:** Low — single-knob split into two-knobs, additive default; existing LLM-path behavior unchanged

## 2. Problem Statement
LLM-HEALTH-INVESTIGATION-001 E2 added a caller-budget pre-check to `Rerank()` with a single knob `RerankMinBudgetMs` (default 12000ms = LLM path p99 + margin). The pre-check runs BEFORE the provider switch, so **every provider inherits the LLM-calibrated threshold** — including the neural sidecar path whose typical timeout is 1000ms.

For provider=neural: a caller with 5s remaining and neural provider would be skipped by the 12000ms threshold even though the neural sidecar would comfortably complete in ~1s. The pre-check is correctly guarding the LLM path from cancellation but is **over-skipping** the neural path.

Not a live-observable regression yet (mdemg-dev currently uses `RERANK_PROVIDER=openai`), but a lurking bug for any operator who switches to `RERANK_PROVIDER=neural` — they'd see silent rerank-skip rates rise.

## 3. Scope & Constraints

### In scope
- New config `NeuralRerankMinBudgetMs int` (env `NEURAL_RERANK_MIN_BUDGET_MS`, default 1500 = neural default timeout 1000ms + 500ms margin; floor 500; 0 = disabled per-provider).
- Rerank pre-check dispatches on `s.cfg.RerankProvider`:
  - `neural` → use `NeuralRerankMinBudgetMs`.
  - `openai` / `ollama` / `jina` / default → use existing `RerankMinBudgetMs` (unchanged behavior).
- Log the effective budget in the WARN message so an operator can grep for pre-check-skip patterns per provider.
- Tier-1 tests: provider-aware truth table.
- Live Tier-3 within reachable scope (default provider still exhibits E2 behavior; neural verified if sidecar is up).
- Canonical docs.

### Out of scope
- Retuning the LLM knob (E2's 12000ms stays; it's calibrated to observed p99).
- Adding budgets for other providers (`ollama` / `jina`) — they share `RerankMinBudgetMs` because they're the same LLM shape and no operator has a hand-tuned budget for them yet.
- Making the pre-check derive budget from the provider's own timeout (was considered; `RerankTimeoutMs` is worst-case not p95, so derivation is unreliable). Two-knob wins.
- Any change to the neural sidecar itself.

### Constraints
- Sequential epics.
- Live Tier-3 required (with the accommodation that neural sidecar may be unavailable — E3 documents this and validates what's reachable).
- No hardcoded literals beyond the ontology (`"neural"` string comparison — matches existing switch statement).
- Additive-only config; backward compat: an operator who never sets `NEURAL_RERANK_MIN_BUDGET_MS` gets the new default; existing `RerankMinBudgetMs` behavior unchanged.
- Fail-open preserved.

## 4. Dependencies
- **LLM-HEALTH-INVESTIGATION-001** (merged as PR #510) — introduced the single-knob E2 pre-check this sprint refines.
- No new env vars beyond `NEURAL_RERANK_MIN_BUDGET_MS`.

## 5. Implementation Plan

### Epic 0 — Sprint plan committed
This document.

### Epic 1 — Provider-aware pre-check
- `internal/config/config.go` — add `NeuralRerankMinBudgetMs int` struct field, `FromEnv` reader with floor, struct literal entry.
- `internal/retrieval/rerank.go::Rerank` — refactor pre-check to compute `minBudgetMs` from provider:
  ```go
  minBudgetMs := s.cfg.RerankMinBudgetMs
  if s.cfg.RerankProvider == "neural" {
      minBudgetMs = s.cfg.NeuralRerankMinBudgetMs
  }
  if minBudgetMs > 0 { ... existing check ... }
  ```
- WARN message includes `provider` field for grepability.

### Epic 2 — Tier-1 tests
Extend `rerank_budget_test.go` (from E4 of LLM-HEALTH-INVESTIGATION-001) with provider-aware cases:
- neural provider + 2s remaining under 12000ms LLM knob but above 1500ms neural knob → **allow** (proves the split works).
- openai provider + same 2s remaining → **skip** (LLM knob still enforced).
- neural provider + 500ms remaining (below neural knob) → **skip**.
- neural provider + no deadline → **bypass**.

### Epic 3 — Live Tier-3
1. Verify the existing E2 test still holds: default `RERANK_PROVIDER=openai`, forced `RerankMinBudgetMs=60000` still skips rerank on retrieve.
2. Neural provider verification: if the neural sidecar is up (`curl :8100/health`), temporarily set `RERANK_PROVIDER=neural`, trigger a retrieve, verify rerank runs (no skip WARN) even under a moderate budget.
3. Revert overrides; capture in `live_verification.md`.

### Epic 4 — Docs
- CLAUDE.md architecture note on the two-knob pattern (with the "when a global pre-check gates a switchable dispatch path, tune per-branch" pin).
- CHANGELOG `[Unreleased] > Fixed` entry.
- `post.md` sprint close.

## 6. Testing (3 tiers)
- **Tier 1**: 4 pins per §Epic 2.
- **Tier 2**: no new integration test — the tsdb/dataset_builder path is unaffected.
- **Tier 3**: live sequence in §Epic 3.

## 7. Commit Strategy
One commit per epic on `reh3376_dev01`:
1. `docs(neural-rerank-precheck-001): E0 — sprint plan`
2. `feat(neural-rerank-precheck-001): E1 — provider-aware rerank pre-check budget`
3. `test(neural-rerank-precheck-001): E2 — provider-aware pre-check tests`
4. `docs(neural-rerank-precheck-001): E3 — live Tier-3 verification`
5. `docs(neural-rerank-precheck-001): E4 — CLAUDE.md/CHANGELOG/post`

Auto-PR fires. Sprint summary comment after E4.

## 8. Verification Checklist
- [ ] E0 committed
- [ ] `NeuralRerankMinBudgetMs` struct field + FromEnv + literal
- [ ] Pre-check dispatches on `RerankProvider`
- [ ] WARN log includes `provider` field
- [ ] Existing E2 behavior unchanged for LLM providers
- [ ] `go build ./...` clean
- [ ] `go test ./...` full-suite green
- [ ] `golangci-lint run` clean
- [ ] Live: default openai retrieve → E2 skip works as before
- [ ] Live: neural sidecar up → neural retrieve completes on tight budget (or documented as not-verified if sidecar down)
- [ ] CLAUDE.md note
- [ ] CHANGELOG entry
- [ ] `post.md`

## 9. Documentation Update — Epic 4.

## 10. Risks & Mitigations
| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| NeuralRerankMinBudgetMs default too high → over-skips even for neural | Low | Low | Default 1500 = default timeout 1000ms + 500ms; matches the E2 pattern (timeout + safety margin). Operator can lower. |
| NeuralRerankMinBudgetMs default too low → cancellations still slip through | Low | Low | The pre-check is a SAFETY net; the neural sidecar's own timeout is the primary bound. If 1500ms proves inadequate, operator raises via env. |
| The `RerankProvider` string comparison drifts from the switch's cases | Very Low | Low | Comparison is against the existing switch's exact strings; a pin test checks the alignment. |
| Neural sidecar unavailable during E3 → partial Tier-3 coverage | Medium | Low | Document as "verified default-provider; neural path verified via Tier-1 truth table + code path inspection; full live-verify deferred until operator sets provider=neural in production" |

## 11. Documents Accessed
- `internal/retrieval/rerank.go::Rerank` (E2 pre-check location + provider switch)
- `internal/retrieval/rerank_neural.go` (default timeout 1000ms; circuit breaker; fail-open path)
- `internal/config/config.go::RerankMinBudgetMs` + `NeuralRerankTimeoutMs` (existing knobs)

## 12. Rollback Procedures
- Additive config knob; a rolled-back binary ignores it and uses the single-knob path.
- Revert commit to restore single-knob behavior.

## Acceptance Criteria
1. `NeuralRerankMinBudgetMs` config surfaces with a sensible default (1500ms).
2. Pre-check uses `NeuralRerankMinBudgetMs` when `RerankProvider=neural`.
3. Pre-check uses `RerankMinBudgetMs` for openai/ollama/jina/default (existing behavior preserved).
4. WARN log includes provider field.
5. Full test suite green; lint clean.
6. Canonical docs updated.
