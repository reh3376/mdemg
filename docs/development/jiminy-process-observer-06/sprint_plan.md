# JIMINY-PROCESS-OBSERVER-06 — Sprint Plan

**Task**: #165 (sixth and FINAL observer in the JIMINY-PROCESS-OBSERVER-{01..06} arc)
**Predecessor**: JIMINY-PROCESS-OBSERVER-05 (#164 / merged pending)
**Wall-clock estimate**: ~2h
**Type**: shipping — final observer + arc closeout

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint | JIMINY-PROCESS-OBSERVER-06 |
| Task # | #165 |
| Master arc | #95 JIMINY-CEILING-BREAK-2 |
| Branch | `reh3376_dev01` |
| Substrate touch? | 0 |
| Design source | `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` §Phase E row 6 (lowest priority) |
| Grades | `never-haiku-for-planning` rule |

## 2. Problem Statement

`never-haiku-for-planning` rule (user preference — "When planning complex features or fixes, ALWAYS use the most advanced coding model (Opus). Haiku only for simple mechanical tasks"). The Claude Code process exposes `ANTHROPIC_MODEL` in the environment; the hook can capture it on `sprint_plan.md` writes. Matcher grades: if model contains "haiku" (case-insensitive) → violation; else → followed.

## 3. Scope & Constraints

**In scope:**
- Hook extension `.claude/hooks/post-tool-observe.py` + template mirror:
  - **New emit branch** `model_call` (event_type already allowlisted): on **Write** tool (not Edit — Edit is iteration; Write is the planning moment) of a file whose name matches `sprint_plan.md` OR `plan.md`, emit `model_call` event with `event_subtype=planning`, `metadata.model=os.environ.get("ANTHROPIC_MODEL","")`, `metadata.file_path=<path>`.
- New matcher `internal/grader/process/matchers/never_haiku_planning.go`:
  - Terminal event: `model_call`
  - Fetch `metadata->>'model'` and `metadata->>'event_subtype'` (or use `event_subtype` field directly)
  - Only grade if subtype = "planning" (skip other model_call events reserved for future observers)
  - Empty model → skip (env not set on the emitter side)
  - Model contains "haiku" (case-insensitive) → `process_incomplete` (reason names model)
  - Else → `process_followed` (reason names model)
- New flag `PROCESS_MATCHER_NEVER_HAIKU_ENABLED` default false
- Wire in server.go

**Out of scope:**
- Detecting model choice on arbitrary tool calls (would over-fire); only Write of a planning-shaped file
- Session-wide model observation (model is set at Claude Code start; per-file check is sufficient — same model persists across a session)
- Cross-model comparison across sessions

**Constraints honored:** all prior + `never-haiku-for-planning` (this sprint grades it — dogfooded; my current session runs `claude-opus-4-7` which will grade `process_followed`).

## 4. Dependencies

- OBSERVER-01..-05 shipped ✅
- `ANTHROPIC_MODEL` env available to the hook process ✅ (verified live)

## 5. Implementation Plan (sequential epics)

- **E1** — Hook: new emit branch for Write of `sprint_plan.md` / `plan.md` → `model_call` with model+file_path metadata. Mirror template.
- **E2** — Config knob.
- **E3** — Matcher: 6 pin tests (opus→followed, haiku→incomplete, sonnet→followed, non-planning subtype→skip, empty model→skip, metadata pins).
- **E4** — Wire in server.go.
- **E5** — Live Tier-3 smoke: 2 synthetic sessions (opus model → followed; haiku model → incomplete). Since ANTHROPIC_MODEL isn't easily changeable mid-session, synthesize via POST directly.
- **E6** — Docs: feature doc, CLAUDE.md pin, CHANGELOG, sprint post + **arc closeout summary** (this is the last observer; sprint_post includes arc-level totals).

## 6. Testing Plan (3 tiers)

- Tier 1: 6 pin tests
- Tier 2: grader loop with capturingPool
- Tier 3: live e2e (mandatory)

## 7. Commit Strategy

Two commits:
1. `feat(process): never-haiku-for-planning matcher + hook model_call event (JIMINY-PROCESS-OBSERVER-06 E1-E4)`
2. `docs(process-observation): never-haiku observer + arc closeout (JIMINY-PROCESS-OBSERVER-06 E5-E6)`

## 8. Verification Checklist

- [ ] build + tests + lint clean
- [ ] All 5 drift checks clean
- [ ] Live smoke: opus→followed, haiku→incomplete
- [ ] Post-smoke cleanup
- [ ] Arc closeout summary in sprint post

## 9. Documentation Update

- Feature doc extended
- CLAUDE.md pin
- CHANGELOG entry
- Sprint post + arc-close summary

## 10. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Sprint plan writes fire high volume for the operator | Only Write (not Edit) qualifies; sprint plans are typically written once then edited — narrow surface. |
| ANTHROPIC_MODEL env not always set | Empty → skip; matcher fail-open. |
| Rule fires on incidental `plan.md` file writes | `sprint_plan.md` OR `plan.md` only; documented scope. If FP class emerges, tighten to `sprint_plan.md` only. |

## 11. Rollback

Flag flip.

## 12. Documents Accessed

- `docs/development/jiminy-metric-denominator-design-001/verdict_and_specs.md` (§B + §E row 6)
- `docs/development/jiminy-process-observer-{01..05}/{sprint_plan,sprint_post}.md`
- `docs/features/process-observation.md`
- `.claude/hooks/post-tool-observe.py` (extension target)
- Live env check (`ANTHROPIC_MODEL=claude-opus-4-7` verified)
- CLAUDE.md: `never-haiku-for-planning` rule, JIMINY-PROCESS-OBSERVER-{01..05}
