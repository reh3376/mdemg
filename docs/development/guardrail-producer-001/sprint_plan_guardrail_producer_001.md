# Sprint GUARDRAIL-PRODUCER-001 — a live producer for guardrail.evaluate

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | GUARDRAIL-PRODUCER-001 |
| Owner | Roger Henley |
| Branch | `reh3376_dev01` |
| Format | Sprint plan v1.0 (12-section) |
| Effort | ~1 dev-day |
| Parent | CLAUDE.md open FT work: "guardrail.evaluate has only ~3 production rows (no live producer; the MCP `validate_changes` tool is the sole producer and no workflow invokes it), so it cannot be retrained until a producer exists" — prerequisite for the recursive-retraining loop covering all 16 call sites |

## 2. Problem Statement

`guardrail.evaluate` is one of the 16 production LLM call sites, with a full
stack shipped and enabled (`GUARDRAIL_ENABLED=true`, service wired, HTTP
endpoint `/v1/memory/guardrail/validate`, MCP `validate_changes`, ULTS spec,
UBENCH rows) — but nothing invokes it in normal operation, so
`llm_interactions` holds ~3 rows and the task can never accumulate
production training data. Meanwhile every Claude Write/Edit already flows
through `post-tool-observe.py` (PostToolUse), which builds action summaries
and fires jiminy feedback — the natural place to also produce real guardrail
evaluations of real code changes against the real constraint corpus.
(Historical intent exists: `.env` carries orphaned `GUARDRAIL_HOOK_TIMEOUT_MS`
from a hook that was documented but never built — DOC-CURRENCY-002 deleted
the fabricated doc rows; this sprint builds the real thing under new,
code-read names.)

## 3. Scope & Constraints

**In scope:**
- **Server — async producer path**: `async` field on `GuardrailValidateRequest`.
  When true: gate on `GUARDRAIL_PRODUCER_ENABLED` (default **false**; flipped
  in `.env` only after live smoke, per the JIMINY-CONTRADICTED-BRIDGE-001
  contract), detach evaluation from the request context
  (`context.WithTimeout(context.WithoutCancel(r.Context()), GUARDRAIL_TIMEOUT_MS)`
  — the handlers.go:634 jiminy-feedback idiom; a hook's short curl must never
  caller-cancel the evaluation mid-LLM), return **202 {status:"queued"}**
  immediately. Disabled → 200 {status:"disabled"} (hook discards output).
- **Saturation guard**: at most `GUARDRAIL_PRODUCER_MAX_CONCURRENT` (default 1)
  detached evaluations at once — busy → 200 {status:"dropped"} + counter.
  (Tonight's fleet-saturation window is the cautionary tale; the
  constraint-match short-circuit in `Validate` already means zero LLM cost
  when no constraints match, so the semaphore bounds only the matching tail.)
- **Hook producer** (`post-tool-observe.py` — template AND live copy in the
  same commit, HOOKSYNC-001): on successful Write/Edit, synthesize a minimal
  diff (Write → `+` lines of content; Edit → `-` old_string / `+` new_string),
  cap at `MDEMG_GUARDRAIL_DIFF_MAX_BYTES` (default 8000), POST with
  `async:true, agent_trust_level:"standard"` via the existing detached
  `subprocess.Popen(curl)` idiom. Fail-open, never blocks the hook.
- **Observability**: counters `mdemg_guardrail_producer_total{status}`
  (queued/dropped/disabled) via the server metrics registry; detached
  completion logged with status + violation counts.
- **Contracts**: UATS variants on `guardrail_validate.uats.json` for the
  async path; unit tests (gate off, queue, drop-when-busy, detach-survives-
  client-cancel); Tier-3 live smoke.
**Out of scope:** Bash-tool evaluation (no reliable diff synthesis);
blocking enforcement (that's /strict + pre-write-check territory);
retraining itself (FT-RECURSIVE line); MCP tool changes.
**Constraints:** no hardcoded values (all knobs env-driven with defaults);
producer must be invisible to hook latency (fire-and-forget end to end);
`space_id` propagation via `llmclient.WithSpaceID` (existing pattern) so
rows attribute to the hook's space.

## 4. Dependencies

✅ Guardrail service enabled + fail-open with constraint-match short-circuit
(`guardrail.go:118`); ✅ endpoint + models + UATS spec exist; ✅ hook
already parses tool_input for Write/Edit and has the Popen-curl idiom;
✅ detach idiom at handlers.go:634; ✅ live constraint corpus (61 constraint
+ 33 correction nodes post-JIMINY-CORPUS-001) for smoke matching.

## 5. Implementation Plan (sequential)

- **E0** this plan.
- **E1** config: `GUARDRAIL_PRODUCER_ENABLED` (false),
  `GUARDRAIL_PRODUCER_MAX_CONCURRENT` (1, floor 1). Struct + FromEnv +
  defaults tests.
- **E2** server: `Async bool` on the request model; handler async branch
  (gate → semaphore try-acquire → detached goroutine with WithoutCancel +
  GUARDRAIL_TIMEOUT_MS bound → 202) + producer counters + completion log.
  Unit tests incl. a canceled-client-context test proving the detached
  evaluation completes.
- **E3** hook: `send_guardrail_producer(tool_name, tool_input)` in
  `post-tool-observe.py` (template + live, byte-identical modulo SPACE_ID);
  diff synthesis for Write/Edit; cap; Popen curl `-m 5`. Python syntax check.
- **E4** UATS: async variants (producer disabled → 200 disabled; enabled
  path is env-dependent so tag appropriately per the UATS-GAP-001 rules).
- **E5** live Tier-3: flip `GUARDRAIL_PRODUCER_ENABLED=true` in `.env`,
  rebuild + kickstart; simulate the hook (real stdin fixture through
  `post-tool-observe.py`) with an edit that semantically matches a real
  constraint (e.g. text about committing directly to main) → verify 202,
  detached completion log, **new `guardrail.evaluate` row in
  `llm_interactions`** with the hook's space_id; then a no-match edit →
  short-circuit row-free Pass (no LLM cost); drop-when-busy path forced.
- **E6** docs: feature doc `docs/features/guardrail-producer.md`; CHANGELOG;
  CLAUDE.md open-FT-work amendment (GUARDRAIL-PRODUCER-001 → SHIPPED);
  post.md. Disclose the stale `.env` `GUARDRAIL_HOOK_TIMEOUT_MS` line.

## 6. Testing Plan

Tier 1: config/handler/semaphore unit tests. Tier 2: `go test ./...` +
UATS suite locally. Tier 3: E5 live end-to-end (hook → 202 → detached LLM →
TSDB row), plus negative (no-match short-circuit) and drop paths.

## 7. Commit Strategy

`docs(E0)` → `feat(E1+E2)` → `feat(E3+E4)` → `docs(E5 evidence + E6)`.
Surprise defects during live smoke get their own fix-commits.

## 8. Verification Checklist

unit green · build+lint green · UATS guardrail specs green · live smoke:
hook-shaped stdin → 202 → `guardrail.evaluate` row visible in TSDB with
correct space_id · no-match edit produces no LLM row · busy path drops with
counter · hook latency unaffected (Popen) · docs · pushed.

## 9. Documentation Update

New `docs/features/guardrail-producer.md`; CHANGELOG Added; CLAUDE.md FT
open-work note updated; sprint post with live evidence.

## 10. Risks & Mitigations

| Risk | Sev | Mitigation |
|---|---|---|
| llama-server load during agent fleets | Med | Constraint-match short-circuit (zero LLM when no match) + MAX_CONCURRENT=1 semaphore + default-off flag |
| Detached goroutine leaks | Low | Hard timeout bound (GUARDRAIL_TIMEOUT_MS); one-shot (not a loop — SUPERVISOR-002 targets loops); semaphore releases in defer |
| Diff synthesis quality (training-data garbage-in) | Med | Only Write/Edit (structured inputs); capped; the ULTS prompt already handles arbitrary diffs; rows are curatable downstream (HITL gold-only sink exists) |
| Hook regression (the channel that must never break) | Med | HOOKSYNC-001 parity; fail-open everywhere; syntax-checked; live smoke through the real hook script |

## 11. Rollback

Set `GUARDRAIL_PRODUCER_ENABLED=false` (kills the server path); revert the
hook commit to stop client attempts. No data mutation to roll back —
produced rows are ordinary llm_interactions.

## 12. Documents Accessed

`internal/guardrail/{guardrail.go,llm_evaluator.go}`;
`internal/api/handlers_guardrail.go` + `server.go` (wiring, :2668);
`internal/models/models.go` (GuardrailValidateRequest);
`internal/cli/hook_templates/post-tool-observe.py` (+ live copy);
`internal/api/handlers.go:634` (detach idiom);
`docs/api/api-spec/uats/specs/guardrail_validate.uats.json`;
CLAUDE.md (open FT work, HOOKSYNC-001, NOSILENT/SUPERVISOR contracts);
`.env` (GUARDRAIL_* live values).
