# Presentation Specification — MDEMG Onboarding Deck

**Status**: Phase 1 planning artifact (Epic 1b of MDEMG-ONBOARDING-DECK-001)
**Owner**: reh3376
**Governed by**: `sprint_plan.md` in this directory

This spec is the **content contract**. Section-level roadmaps and content docs in later epics MUST conform. If this spec changes, every downstream artifact must be re-verified against the new contract.

---

## 1 Audience + Posture

### Target reader (declared)

- **Senior software engineers** (10+ years, polyglot, comfortable with distributed systems + ML + databases simultaneously), OR
- **Technical project managers** who were previously senior engineers (now coordinate, still read code when needed)

### Explicitly NOT the audience

- Non-technical PMs, product owners, business stakeholders — a separate "exec summary" artifact belongs with marketing, not here
- Beginner engineers / interns — too much implicit context required; blocks the ability to be dense
- Beta testers — `packaging/homebrew-mdemg/mdemg_beta_testing.md` covers that path

### Posture

- **Engineering-dense, no fluff.** No "unlock your potential"-shaped copy. No "revolutionary paradigm" framing. Technical, specific, cite-everything.
- **Honest about failures and compromises.** The 2026-04-22 MoE→dense pivot, the 2026-05-03 mlx_lm.server → llama.cpp cutover, the current actionable follow-rate at ~13% targeting ≥80% — these are PART of the story. Hiding them produces a brittle mental model.
- **Trajectory-framed, not "by design"-framed** per `trust-signal-must-be-persisted-never-ignore-honest`. Where current state is below target, say so + name the arc that owns the delta.

## 2 Format

### Platform

- **Obsidian + Advanced Slides plugin** (reveal.js-based)
- Single markdown file: `docs/onboarding/mdemg-onboarding-deck.md`
- Section separators: `---` (horizontal) for new slide; `--` (vertical) for sub-slide stacks
- Speaker notes: `note:` block inline per slide, mirrored in companion `docs/onboarding/mdemg-onboarding-notes.md`

### Advanced Slides front-matter template

```yaml
---
theme: night
transition: slide
width: 1280
height: 720
---
```

### Slide header shape

Every slide starts with `# <Title>` or `## <Sub-title>`. No bare-prose slides. The title bar carries the hierarchy for the audience.

### Visual conventions

- **Mermaid for all diagrams** per `markdown-mermaid-tables-and-charts`
- Code blocks fenced with language hint (go, bash, python, cypher, sql, json)
- Tables for comparisons, enumerated specs, and config matrices
- Blockquotes `>` for operator quotes + architectural directives
- No images from the web (brittle, licensing issues). Screenshots of Grafana + UI tabs are allowed, stored co-located in `docs/onboarding/assets/`.

## 3 Content Contract — The 5-Beat Template

Every **major-component section** follows the same five beats. Deviation requires an operator-authorized exception documented in the section roadmap.

| Beat | Question | Target slide count | Typical content |
|---|---|---|---|
| 1 | **What it is** | 1-2 slides | One-sentence definition; where it lives in the code (`internal/<pkg>`); its one-line role in the system |
| 2 | **Why we use it** | 1-2 slides | The problem it solves; the alternative we rejected and why; 1-2 forcing-function moments (incident / defect / insight that birthed it) |
| 3 | **How it works** | 3-6 slides | The mechanism; data flow diagram (Mermaid); key API / schema / interface; the ONE invariant that must survive future refactors |
| 4 | **Why it's important** | 1-2 slides | What breaks if it's removed; which other subsystems depend on it; current honest health signal (metric, dashboard, failure mode) |
| 5 | **How it will improve** | 1-2 slides | Named follow-up sprints; architectural directions; open questions; the arc that owns the next delta |

**5-beat total: 7-14 slides per major component.** 20+ components identified in the global roadmap × ~10 slides each = 200+ slides is too much. Hence the global roadmap compresses: some components get a FULL 5-beat treatment (the headline subsystems: Jiminy, J17, LoRA/Qwen, UxTS, HITL/RSIC, Grafana, Model Dist); some get a **MINI treatment** (3-4 slides: name + what + why + where to go for depth); some get a **MENTION** (one slide listing them with doc pointers).

The decision of full / mini / mention is made in the global roadmap.

## 4 Slide-Count Budget

| Section family | Full / Mini / Mention mix | Est. slide count |
|---|---|---|
| F1 — Opening (Welcome, Overall Goal, History, How-it-works at 10k ft) | 4 full-ish | 10-14 |
| F2 — Substrate Core (Neo4j CMS, 5-layer, Hebbian, TSDB) | 1 full + 1 mini | 12-16 |
| F3 — Retrieval Pipeline (RRF, sparse gate, context fingerprint, rerank) | 1 full + 2 mini | 10-14 |
| F4 — Jiminy + Enforcement + J17 | 2 full + 1 mini | 20-26 |
| F5 — LoRA + Qwen + Model Distribution | 1 full + 1 mini + homebrew-mdemg mini | 12-16 |
| F6 — UxTS Framework Family | 1 full | 7-10 |
| F7 — Ops (Grafana/alerts, Backup, HITL, RSIC, Scheduled jobs, Supervisor) | 2 full + 2 mini | 14-18 |
| F8 — CLI, Hooks, MCP, Multi-instance | 2 mini + mentions | 6-8 |
| F9 — Beta Program + Security Hardening + Dormant Census | 2 mini + mentions | 4-6 |
| F10 — Conclusion (what's left, end goals, continuous improvement loop) | 1 full | 6-8 |
| **TOTAL** | — | **~101-136** |

Envelope: aim for 100 ± 20. Over 120 → cut in Epic 5. Under 80 → add depth to F10 (the "where we're going" story carries weight).

## 5 Cadence + Timing

- **1 min/slide average** → 100 slides = 100 min + ~15 min Q&A = **115 min total walkthrough**
- Fast context frames (section-title slides, table-only slides): **~30 sec**
- Diagram-heavy mechanism slides: **~90 sec**
- Component "why it's important" slides often provoke Q&A → budget 2 min on those
- Opening (first 10 slides): aim for 10 min (fast, set the frame)
- Conclusion (last 8 slides): aim for 10 min (slow, invite questions, land the long-term vision)

Timing is validated via **Tier-3 live walkthrough with stopwatch** in Epic 6. If total > 120 min, cut; if < 90 min, slow down or add content to F10.

## 6 Diagram Standard (Mermaid)

### Required diagrams (operator-authored, Epic 2 identifies the list)

- MDEMG system overview (10k ft) — all major subsystems + their edges
- 5-layer memory hierarchy (L0 obs → L1 constraint/correction → L2+ emergent concept)
- Retrieval pipeline (RRF column voting, rerank, sparse gate)
- Jiminy enforcement flow (hook → classify → warn → block)
- J17 protocol message shape (CODES / SEV / ESC / FMT / TICKET / SEQ)
- LoRA on Qwen pipeline (MLX safetensors → fuse → f16 GGUF → Q5_K_M → llama-server)
- Recursive retraining loop (drift → export → curate → train → convert → gate → promote)
- HITL review loop (candidate queue → operator grade → sink → substrate mutation OR L0 correction)
- UxTS forcing-function shape (schema → spec → runner → CI → matrix adjudication)
- Grafana + alert evaluator flow (metric → TSDB → evaluator → alert → dispatcher)
- Model distribution (Ollama Library + homebrew tap + brew-install)
- Beta corpus loop (tester → export bundle → maintainer import → janitor retention)

### Diagram rules

- Light-theme-neutral (dark + light both render legibly in Obsidian)
- Node labels concise; longer prose in speaker notes
- Edge labels name the ACTION (e.g. `promotes`, `rerank-feeds`, `scope-gates`) not the DATA
- If a diagram needs > ~20 nodes, split into layered sub-diagrams on sequential slides

## 7 Speaker Notes Companion

### Why a separate file

- Advanced Slides supports `note:` blocks inline, but the operator may want to brief new team members who don't have Obsidian installed
- The companion `docs/onboarding/mdemg-onboarding-notes.md` is a flat markdown doc, one `## Slide N — <Title>` section per slide, matching the deck's slide-count

### Note content

- 2-4 sentences per slide: what the presenter says
- Explicit "pause-for-questions" marks on specific slides
- Specific examples: substitute concrete numbers / names (`"actionable follow rate 13.2% as of 2026-10-15"`) that may rot — the quarterly refresh updates these
- Fallback "simplify-for-PM" phrasing when a slide uses specific engineering jargon (e.g. "RRF" → "the thing that fuses 4 different ranking signals")

## 8 Factual Provenance Rule

Every substantive claim in a slide (every number, every architectural decision, every "why we did X" statement) MUST be traceable to:

1. A **CLAUDE.md pin** (named sprint + date), OR
2. A **feature doc** at `docs/features/*.md`, OR
3. A **sprint record** at `docs/development/<sprint-slug>/sprint_plan.md` or `sprint_post.md`, OR
4. **Shipped code** at a specific file + line (for interface / API / schema claims)

Each content doc produced in Epic 3 MUST include a provenance footer listing its sources. The 10-claim spot-check in Epic 6 Tier-3 is explicit about this.

**Fabrication = sprint failure.** Peer-Opus-5.5 review of the J17 briefing packet surfaced this exact class — numbers in docs that don't match the code erode trust. The deck is operator-facing; a fabrication risks a new team member calling it out in their first week.

## 9 Deck Hygiene

- "Last verified: YYYY-MM-DD" footer per section header slide (the F# sections above). Forces visible staleness.
- Quarterly re-verification cadence per `docs/features/mdemg-onboarding-deck.md` (Epic 7). Operator or delegate re-walks CLAUDE.md + feature docs + checks each verified-date footer.
- When a shipped sprint changes a claim the deck makes (e.g. the Jiminy follow-rate arc hits ≥80%), the deck update rides in the sprint that changes the claim, NOT as a separate deferred fix.

## 10 Non-Goals (reaffirmed)

- No slide decks for individual sprints (would be ~100 decks and all would rot)
- No per-feature-doc deck equivalents (feature docs are reference; this deck is onboarding — different shape)
- No video or audio content in-line (reveal.js supports it; we don't need it; it increases maintenance surface)
- No external hosting of the deck beyond the repo (operator may export HTML for sharing but canonical is in-repo)

## 11 Documents Accessed

- `docs/development/mdemg-onboarding-deck-001/sprint_plan.md` (sibling, this sprint)
- `docs/features/jiminy-lora-qwen.md` (template for 5-beat engineer-dense content)
- `docs/features/uxts-frameworks.md` (template for 5-beat framework explainer)
- `docs/features/j17-agent-briefing-packet.md` v1.1 (reference for compressed-contract content)
- CLAUDE.md § Architecture Notes (source of ~100 pins, source for all history + decisions)
- `docs/development/UXTS_FRAMEWORK_MATRIX.md` (ground-truth for UxTS claims)
- Obsidian Advanced Slides plugin README (`https://github.com/MSzturc/obsidian-advanced-slides`)
