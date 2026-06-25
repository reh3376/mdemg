# Jiminy Actionability — Surface Bias + Directive Synthesis (JIMINY-ACTIONABILITY-001)

## Why

Diagnostics (JIMINY-RELEVANCE-001) showed guidance is ignored not because it is *off-topic* but because it is *not actionable*: the abstraction class (`pattern`/`learning`/`concept`) is ~90% of surfaced/outcome guidance and is ignored 54–66%, while the actionable class (`constraint`/`correction`) is ~10% and ignored only 27–35%. This sprint is the near-term surfacing lever — bias what `Guide()` *surfaces*, and how it is *phrased*, toward the actionable.

## Choices

Two independent, default-off, config-driven levers:

- **Lever A — surface-composition reweighting.** A per-type sort weight (`JIMINY_SURFACE_ACTIONABLE_WEIGHT`), a min-actionable quota (`JIMINY_SURFACE_MIN_ACTIONABLE` / `_FRACTION`) reserving surfaced slots before the cut to `max_items`, and an abstraction cap (`JIMINY_SURFACE_MAX_ABSTRACTION_FRACTION`) dropping the abstraction tail first. All default to no-ops, so at defaults the surfaced set is byte-identical to the prior plain truncation; actionable items are never dropped to satisfy the cap.
- **Lever B — directive synthesis.** When `JIMINY_DIRECTIVE_SYNTHESIS_ENABLED=true`, the synthesis system prompt is augmented to render abstraction-type evidence as **imperative, task-specific directives** ("Do X", "Before Y, do Z") instead of advisory prose, bounded by `JIMINY_DIRECTIVE_SYNTHESIS_MAX_PROMPT_TOKENS`. Reuses the existing `jiminy.synthesize` call — no new LLM call on the hot path.

A surfaced-composition gauge (`mdemg_jiminy_surfaced_actionable_fraction` / `_abstraction_fraction`) measures the surfaced side directly.

## How it works

- `internal/jiminy/service.go`: `isActionableType` partitions types; `guidanceTypeWeight` applies Lever A's sort multiplier; `applyActionableComposition` enforces the quota + abstraction cap on the already-sorted list before truncation (default-preserving fast path when no quota + no cap). `Guide()` emits the surfaced-composition gauge from the final `filtered` set.
- `internal/jiminy/synthesizer.go` + `guidance_prompt.go`: in directive mode the system prompt gets `directiveSynthesisInstruction`, and `boundDirectivePrompt` keeps the user prompt within the token budget.

## How to use

| Env var | Default | Meaning |
|---|---|---|
| `JIMINY_SURFACE_ACTIONABLE_WEIGHT` | 1.0 (no-op) | Sort-key multiplier for actionable types |
| `JIMINY_SURFACE_MIN_ACTIONABLE` | 0 | Absolute min actionable items reserved |
| `JIMINY_SURFACE_MIN_ACTIONABLE_FRACTION` | 0.0 | Min actionable as a fraction of `max_items` |
| `JIMINY_SURFACE_MAX_ABSTRACTION_FRACTION` | 1.0 (no cap) | Max abstraction items as a fraction of `max_items` |
| `JIMINY_DIRECTIVE_SYNTHESIS_ENABLED` | false | Render abstractions as imperative directives |
| `JIMINY_DIRECTIVE_SYNTHESIS_MAX_PROMPT_TOKENS` | 3500 | Directive-mode prompt token bound |

## Live A/B result (Epic 4)

Levers off vs on, same 6 contexts against mdemg-dev (`docs/development/jiminy-actionability-001/epic4_live_ab.md`):

- **Lever B works** — directive synthesis produces imperative narratives ("it is imperative... You MUST clean up... You MUST NOT delete..."). This is the higher-impact lever: it makes guidance actionable in *phrasing* regardless of *type*.
- **Lever A is mechanically correct but modest** — surfaced actionable fraction 6.7% → 10.5%. It pulls actionables up *where they exist in the candidate pool* but cannot manufacture them; for most contexts retrieval surfaces no actionable candidates.
- **Finding:** the binding constraint is **upstream retrieval candidate composition**, not the surfacing cut. Biasing retrieval toward actionable nodes (role-scoped boost) is the higher-leverage follow-up — **jiminy-actionability-002**.

Ships **default-off**; operator recommendation is to enable Lever B. See `docs/development/jiminy-actionability-001/`.
