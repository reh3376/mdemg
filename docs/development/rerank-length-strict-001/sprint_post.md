# RERANK-LENGTH-STRICT-001 — Sprint Post

**Date:** 2026-08-13
**Branch:** `reh3376_dev01`
**Trigger:** Fable HITL bulk-grade session #110 (pass 2) found `rerank_cross` was the lowest-rated dataset (avg 2.60) with two distinct quality signals: (a) score arrays whose length ≠ candidate count in ~14/40 rows; (b) nondeterministic scoring — identical query+candidates gave different score arrays across rows. Investigation phase (task-list step 4) identified 4 root causes; this sprint ships all fixes as one commit per operator greenlight.

## Root causes

1. **No Temperature=0** on the reranker LLM call (`rerank.go` — `Complete(ctx, msgs, CompleteOpts{})`). Reranking is a relevance-scoring task; correct temperature is 0 (deterministic). Server-side default was 0.8 → Fable's observed nondeterminism.
2. **Prompt didn't state the required count** — `buildRerankPrompt` gave numbered candidates but never named N. LLM occasionally returned wrong-length arrays (15 cands → 16-17 scores).
3. **Go parser accepts wrong-length silently** — `Rerank()` at `rerank.go:196-200` had `if i < len(scores)` guard defaulting missing scores to 0.5, but no LOG or METRIC. The comment `"// Default if scores array is incomplete"` showed the author knew about the class but only defensively defaulted. The 14/40 finding sat undetected until Fable graded them.
4. **No corrective retry** on length mismatch — single bad call became 1 permanent bad audit row + 1 permanent poisoned RRF fusion score.

## Shipped

**`internal/retrieval/rerank.go`**:
- New package-level `var rerankTemperature = 0.0` — the determinism contract for rerank calls. Both provider paths (`doRerankWithOpenAI` + `rerankWithOllama`) now thread `Temperature: &rerankTemperature` through `CompleteOpts`. Applies to `openai` / `ollama` / default paths; the `neural` + `jina` paths return their own length-bounded outputs by construction.
- `buildRerankPrompt` now appends `"Return exactly N scores in a JSON array, one per candidate, in the same order as the candidates above."` at prompt tail. Names N in the immediate context of the candidates list — shifts LLM attention to the count constraint at generation time.
- New `buildRerankRetryPrompt(originalPrompt, expected, got)` constructs a corrective user prompt naming both the previous incorrect count AND the expected count verbatim, then re-appends the original candidates + count contract.
- New length-mismatch flow in `Rerank()`: if the initial call returned a valid parse but `len(scores) != topN` on an LLM provider path, log a WARN with `provider, expected, got, reason, space_id`, increment the `RetrievalRerankLengthMismatch(reason)` counter (`reason ∈ {short, long}`), and retry ONCE with the corrective prompt. If the retry recovers a matching length, swap in the corrected scores + increment `RetrievalRerankLengthMismatch("retry_recovered")`. If retry still mismatches, keep original scores + let the downstream 0.5-default guard handle it. Bounded to single-attempt to cap worst-case latency.

**`internal/metrics/collectors.go`**:
- New field `RetrievalRerankLengthMismatch func(reason string) *Counter` on `StandardMetrics`.
- Registration in `NewStandardMetrics`: emits `mdemg_retrieval_rerank_length_mismatch_total{reason}` with `reason ∈ {short, long, retry_recovered}`.

**Pin tests** (`internal/retrieval/rerank_length_strict_test.go` — new):
- `TestBuildRerankPrompt_NamesExpectedScoreCount` — asserts prompt tail contains `"Return exactly N scores"` for both `compress=false` and `compress=true` paths, with different Ns.
- `TestBuildRerankRetryPrompt_NamesBothCounts` — asserts retry prompt names both counts verbatim (`"5 scores but there are 2 candidates"`, `"Return exactly 2 scores"`) AND includes the original prompt verbatim.
- `TestRerankTemperature_IsZero` — package-level determinism contract pin + source-string pin asserting `Temperature: &rerankTemperature` threaded through ≥2 provider paths (openai + ollama).

## Live Tier-3 (mdemg-dev, 2026-08-13)

- Rebuilt binary + `launchctl kickstart -k gui/501/com.mdemg.server` → server up at 12:29 EDT, `/healthz` all subsystems ok
- **Determinism live-verified**: 2 identical retrieves (`POST /v1/memory/retrieve {space_id:"mdemg-dev", query_text:"constraint promotion gate", top_k:5}`) produced byte-identical top-5 ordering + identical scores (0.482, 0.48, 0.406, 0.406, 0.405). Pre-fix, server-side temp=0.8 would have produced score drift.
- **Metric registered**: `/v1/metrics/snapshot` confirms the counter exists; no fires on happy-path retrieves (expected — increments only on actual length mismatch).
- Full test suite green: `go test ./internal/retrieval/ ./internal/metrics/` PASS
- Lint clean: `golangci-lint run ./internal/retrieval/ ./internal/metrics/` — 0 issues

## Two arch rules pinned (CLAUDE.md)

1. **Reranking LLM calls MUST set `Temperature: &rerankTemperature` (=0.0).** Reranking is a relevance-scoring task, not a generation task; nondeterminism produces score arrays that differ across identical (query, candidates) pairs, which makes the audit corpus useless for pattern analysis (was Fable's finding — "same query gives different score arrays"). The `rerankTemperature` const is the single source of truth; when adding a new LLM-provider rerank path, thread the same pointer through `CompleteOpts`. Applies only to LLM providers (openai/ollama/default); neural + jina return length-bounded outputs by construction.

2. **Score-array length-mismatch classes MUST be observable + retried, NEVER silently defaulted.** The pre-fix pattern (`if i < len(scores) { rerankScore = scores[i] } else { rerankScore = 0.5 }`) defensively defaulted but emitted no log / metric — 14/40 rows landed with silently-poisoned scores. Correct shape: (a) log a WARN naming provider + expected + got + reason, (b) increment a labeled counter (`mdemg_retrieval_rerank_length_mismatch_total{reason}`), (c) retry ONCE with a corrective prompt naming both counts verbatim, (d) if retry still mismatches, fall through to the default guard. Applies to any future LLM call whose response is a fixed-length array indexed by candidate/position.

## Follow-ups disclosed

- **`retrieval_audit` writer meta capture** — the length-mismatch counter is real-time observability, but per-call context (which query + which candidate ordering) would help debug. Consider adding `length_mismatch bool` + `length_mismatch_reason string` to `retrieval_audit` row meta so post-hoc queries can find the specific traces. Deferred; counter is sufficient for aggregate trend.
- **Extend the same pattern to other position-indexed LLM outputs** — audit `internal/consulting`, `internal/jiminy`, `internal/hidden` for LLM calls that expect a fixed-length array. If any exist with the same defensive-default class, extend the observability shape.
- **Grafana panel for the counter** — add a "Rerank length-mismatch rate" panel to `mdemg-llm-routing.json` (short + long + retry_recovered as separate series). Deferred; if the raw counter shows sustained non-zero, ship the panel.
- **Neural rerank quality re-audit** — Fable's finding was on the LLM path; NEURAL-RERANK-QUALITY-AB-001 previously showed neural = LLM on 120q UVTS. Given LLM path is now more deterministic, re-run the A/B in a follow-up to see if the quality gap has shifted.

## Documents Accessed

- `internal/retrieval/rerank.go` — the fix site (Temperature, prompt, length-check, retry)
- `internal/retrieval/rerank_budget_test.go` — test infrastructure reference
- `internal/metrics/collectors.go` — `StandardMetrics` field + `NewStandardMetrics` registration site
- `internal/llmclient/client.go` — `CompleteOpts.Temperature *float64` shape (nil = omit)
- CLAUDE.md pins: LLM-HEALTH-INVESTIGATION-001 (caller_canceled tagging that clarifies pass 2's noise floor), NEURAL-RERANK-QUALITY-AB-001 (prior rerank A/B baseline)
- Task #110 (HITL-BULK-GRADE-SESSION-002) sprint post — the Fable run that surfaced the class
- Live: 2× identical `/v1/memory/retrieve` determinism verify, `/v1/metrics/snapshot` counter registration verify
