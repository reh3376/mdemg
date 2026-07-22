# GUARDRAIL-PRODUCER-001 — Sprint Post

**Date:** 2026-07-22 | **Branch:** `reh3376_dev01`
**Parent:** CLAUDE.md open FT work — the guardrail.evaluate producer gap
blocking its retraining line.

## What shipped

- **E1 config**: `GUARDRAIL_PRODUCER_ENABLED` (default false),
  `GUARDRAIL_PRODUCER_MAX_CONCURRENT` (default 1, floor 1) — defaults +
  floor pin-tested.
- **E2 server**: `async:true` on `GuardrailValidateRequest` → gate →
  semaphore try-acquire (busy = drop, not queue) → evaluation detached from
  the request context (fresh Background + `GUARDRAIL_TIMEOUT_MS`, timeout
  created inside the goroutine so gosec sees the cancel) → 202
  `{status:"queued"}`. Counters
  `mdemg_guardrail_producer_total{queued|dropped|disabled}`. Pin tests:
  disabled gate; detached-eval-survives-already-canceled-client;
  drop-when-busy + slot-refree.
- **E3 hook**: `send_guardrail_producer` in `post-tool-observe.py`
  (template + live, parity-verified modulo SPACE_ID): Write/Edit → minimal
  synthesized diff capped at `MDEMG_GUARDRAIL_DIFF_MAX_BYTES` (8000) →
  detached `Popen(curl)`, `agent_trust_level:"standard"`. Fail-open.
- **E4 UATS**: env-robust `async_producer_accepted` variant (status
  `[200,202]`, body status `one_of [queued,disabled,dropped]`, `$.data`
  `not_exists`); spec hash re-pinned. 5/5 live.

## The surprise defect (own fix-commit)

First smoke: instant Pass, zero constraints matched **at any floor** (0.3
and a forced 0.05). Root cause — `semanticSearch` ran
`db.index.vector.queryNodes` **global top-200, then** filtered
`role_type='constraint'`: the JIMINY-ACTIONABILITY-001 Lever-C class.
Constraints are 63 of ~85k+ live mdemg-dev nodes (~0.07%) and never appear
in a global top-200 for arbitrary text; tiny UATS spaces fit entirely inside
200, so contracts stayed green for the defect's whole life. **Guardrail
semantic retrieval had been structurally blind on production-scale spaces
since Phase 104.** Fix: the shipped Lever-C pattern — role-filtered
`vector.similarity.cosine` over only the constraint partition (O(63));
`VectorIndexName` no longer used by this query (config field retained).

## Live Tier-3 (mdemg-dev)

1. Disabled path on the new binary: `{"status":"disabled"}`, validator
   untouched.
2. Flag flipped in `.env` (operator-authorized enable-after-smoke), rebuild,
   kickstart.
3. Real hook script + PostToolUse-shaped stdin (a Write committing directly
   to main with hardcoded keys) → 202 in **0ms** → detached eval **1.7s** →
   **the first production `guardrail.evaluate` row ever**
   (`tokens_in=1489` — constraints in the prompt; prior newest row:
   2026-04-22, the 3 hand-made test rows).
4. Cost reality: at floor 0.3 even a cooking-recipe edit matched ≥1
   constraint → ~every sampled edit runs the LLM (~1.5–2.7s). The
   effective cost control is the concurrency bound + drop policy, not the
   floor — and broad matching is fine for training data (Pass rows are the
   negatives the model must learn).
5. Drop path: two concurrent calls → second `{"status":"dropped"}`;
   counters `queued=3, dropped=1`.
6. State: forced `GUARDRAIL_CONSTRAINT_SIM_FLOOR` env override removed
   (default restored); scratch fixtures in /tmp only.

## Verification checklist

- [x] Unit + package + config tests green; build + lint (incl. gosec) clean
- [x] UATS guardrail_validate 5/5 live incl. new variant; hashes verified
- [x] Live: hook → 202 → detached LLM → TSDB row with hook space_id
- [x] Live: drop path + counters
- [x] Hook template ↔ live parity (modulo SPACE_ID)
- [x] Docs: feature doc, CHANGELOG, CLAUDE.md FT-note amendment
- [x] Env-var drift checker clean after doc edits

## Follow-ups (disclosed)

- Corrections (`role_type='correction'`, 33 nodes) in guardrail retrieval.
- Producer-specific sim floor — only if HITL grades show noise outweighing
  negative-example value (data-decide).
- Sampling knobs if drop-based bounding proves too coarse.

## Documents Accessed

`internal/guardrail/{guardrail,constraint_retrieval,diff_parser}.go`;
`internal/api/{handlers_guardrail.go,server.go}`; `internal/models/models.go`;
`internal/cli/hook_templates/post-tool-observe.py` (+ live);
`internal/metrics/collectors.go`;
`docs/api/api-spec/uats/specs/guardrail_validate.uats.json` + runner;
CLAUDE.md (Lever-C precedent, HOOKWIRE/HOOKSYNC, detach idiom); live
`llm_interactions` / Neo4j partition queries / server.log.
