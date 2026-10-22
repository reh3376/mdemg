# Sprint MDEMG-ONBOARDING-DECK-001 — Welcome to the MDEMG Dev Team

## 1 Header & Metadata

- **Sprint code**: MDEMG-ONBOARDING-DECK-001
- **Author**: reh3376
- **Opened**: 2026-10-22
- **Target close**: multi-session; estimate ~5-7 working sessions across 2-3 weeks
- **Status**: Phase 1 (planning) — this doc + presentation spec + global roadmap
- **Primary deliverable**: Obsidian Advanced Slides deck + speaker notes + in-repo canonical home
- **Non-deliverable (explicitly)**: not a code change; not a product change; a teaching artifact for new team member onboarding
- **Sprint plan v1.0 compliance**: yes — follows mandatory 12-section format per `must-follow-12-section-format`

## 2 Problem Statement

MDEMG is a large, dense codebase that touches 20+ interlocking subsystems: Neo4j CMS, TimescaleDB telemetry, Jiminy enforcement, J17 AI-to-AI protocol, LoRA on Qwen, UxTS framework family, HITL review, RSIC self-improvement, recursive retraining loop, Grafana dashboards, alert evaluator, model distribution via Ollama Library + homebrew tap, multi-instance Docker compose, hook channels, MCP memory tools, security hardening, dormant-census registries, and so on. CLAUDE.md alone is >15k words; the `/docs/features/` directory holds 40+ feature docs. A new senior engineer or technical project manager cannot absorb this in a day — but today has no structured onboarding path, so they either read random docs in random order and build an incoherent mental model, or they bother the operator with 1:1 walkthroughs that don't scale.

**The problem**: make the first 90-120 minutes of a new team member's exposure to MDEMG produce an accurate, hierarchical mental model of what MDEMG is, why each subsystem exists, how they compose, and where to go next for depth.

## 3 Scope & Constraints

### In scope

- One **Obsidian Advanced Slides** deck (reveal.js-backed; markdown with `---` section separators), authored at `docs/onboarding/mdemg-onboarding-deck.md`
- Target walkthrough duration: **90-120 minutes** including Q&A pauses
- Target slide count: **80-120 slides** (roughly 1 slide per minute spoken, with fast context frames)
- **Audience**: senior engineers (10+ years, polyglot) OR technical project managers (ex-engineer, now coordination). NOT: non-technical PMs, interns, product managers without eng background. Content density calibrated to this audience.
- Companion artifact: **speaker notes** at `docs/onboarding/mdemg-onboarding-notes.md`
- Each major-component section follows a **5-beat template**: (1) what it is, (2) why we use it, (3) how it works, (4) why it's important, (5) how it will improve
- Content provenance tracked: every claim traceable to a canonical source (CLAUDE.md pin / feature doc / sprint record). No fabrication.
- All diagrams in **Mermaid** per `markdown-mermaid-tables-and-charts` operator preference

### Out of scope

- Non-Obsidian slide formats (PowerPoint, Google Slides); if an export is wanted later, Advanced Slides emits HTML + PDF natively
- Marketing-oriented material (benefit copy, testimonials)
- Day-1 operator-ergonomics guide (setup, env wiring, first-run) — belongs in a separate "mdemg quickstart" doc
- Beta tester onboarding — `packaging/homebrew-mdemg/mdemg_beta_testing.md` serves that
- Video recording — not sprint scope

### Constraints

- **`never-haiku-for-planning`**: this planning phase uses Opus; deep-dive agents may use Sonnet; summarization + authoring stays Opus
- **Sprint plan format v1.0** — this doc complies
- **`live-testing-tier-required` applies loosely**: Tier 3 "e2e" for a presentation = full walkthrough rehearsal with stopwatch + 1 pass from a cold-reader
- **`project-planning-docs-in-repo-only`**: planning docs under `docs/development/mdemg-onboarding-deck-001/`; the deck itself under `docs/onboarding/`
- **`must-master-data-pipelines`**: factual claims about the data substrate ≠ cosmetic; re-verify against CLAUDE.md every refresh

## 4 Dependencies

- **Obsidian + Advanced Slides plugin** on the operator's reviewing machine (`https://github.com/MSzturc/obsidian-advanced-slides`)
- Access to canonical sources: CLAUDE.md, `docs/features/*.md`, `docs/development/*/sprint_plan.md` + `sprint_post.md`
- Multi-agent capability for parallel section deep-dives (fork pattern — up to 6-10 forks in parallel per section family)
- Nothing in CI will gate this sprint — doc-only; no code change

## 5 Implementation Plan (sequential epics)

Epics MUST execute sequentially per `sequential-epics` rule. Within each epic, parallel work IS allowed (e.g. parallel deep-dive forks within Epic 3).

### Epic 1 — Foundations (this turn)

- 1a: Sprint plan (this doc)
- 1b: Presentation specification — `presentation_spec.md` (content contract, template, Advanced Slides conventions, 5-beat beat sheet)
- 1c: Global roadmap — `global_roadmap.md` (ordered sections, estimated slide counts, dependency chain, section→owner→source mapping)

**Gate 1**: Operator reviews the three artifacts + green-lights the section inventory + slide-count envelope before Epic 2.

### Epic 2 — Section Roadmaps

- 2a: Per-section roadmap docs under `sections/<section-slug>-roadmap.md` — one per section identified in global roadmap. Each contains: 5-beat outline, canonical-source list, slide-by-slide skeleton (titles + 1-sentence summary), diagram inventory
- 2b: Identify diagrams needing Mermaid composition + diagrams needing screenshots (Grafana panels, UI dashboards)

**Gate 2**: Operator approves section-level outlines before content work starts.

### Epic 3 — Multi-Agent Deep Dives

- 3a: Spawn parallel fork agents (one per section-family) with explicit scoped prompts naming canonical sources + 5-beat template + slide skeleton from Epic 2. Agents produce `sections/<slug>-content.md` with filled 5-beat content + verified citations.
- 3b: Operator spot-checks each content doc for factual accuracy. Flagged corrections re-run through owning agent OR hand-corrected.

**Gate 3**: All section content docs factually clean per operator spot-check.

### Epic 4 — Slide Authoring

- 4a: Author slide markdown at `docs/onboarding/mdemg-onboarding-deck.md`, pulling from content docs into Advanced Slides format with `---` separators, speaker-note blocks, Mermaid diagram embeds
- 4b: Author speaker-notes companion at `docs/onboarding/mdemg-onboarding-notes.md`

### Epic 5 — Rendering + Visual Review

- 5a: Load deck in Obsidian + Advanced Slides; validate every slide renders; validate every Mermaid diagram renders; validate speaker-note blocks appear in presenter view
- 5b: Fix any rendering issues

### Epic 6 — Rehearsal + Timing (Tier-3 live validation)

- 6a: Operator walks the full deck with a stopwatch, confirming total duration lands in 90-120 min
- 6b: Cold-reader pass — someone unfamiliar with MDEMG consumes deck WITHOUT a presenter and reports where their mental model broke
- 6c: Refine content based on cold-reader feedback; re-run rehearsal if timing drifted >15 min

### Epic 7 — Documentation Update + Feature Doc

- 7a: `docs/features/mdemg-onboarding-deck.md` — permanent feature doc describing deck's purpose, location, update cadence, how to run a walkthrough. Mirrors other `docs/features/` entries.
- 7b: README.md cross-reference
- 7c: CHANGELOG.md entry under Unreleased/Added

### Epic 8 — Sprint Post

- 8a: `sprint_post.md` capturing what was built, what was skipped, what proved harder than expected, follow-ups disclosed
- 8b: Operator sign-off

## 6 Testing Plan (3 tiers)

Per `unit-integration-e2e-docs` rule, three tiers plus docs:

### Tier 1 — Unit

- Deck markdown passes lint (structure valid; `---` separators correctly delimit slides; no empty slides; every slide has `#` or `##` heading)
- Every Mermaid block parses without error
- Front-matter YAML (if any) validates
- `scripts/verify_doc_env_vars.py --strict` passes for the new doc paths

### Tier 2 — Integration

- Deck renders in Obsidian + Advanced Slides without visual errors across all slides
- Internal cross-references (`[[section-link]]` or `#section-anchor`) resolve
- Mermaid diagrams render in Obsidian preview
- Speaker-note view works

### Tier 3 — e2e (LIVE walkthrough; operator-run per `live-testing-tier-required`)

- Full timed walkthrough: operator presents deck end-to-end, stopwatch confirms 90-120 min
- Cold-reader validation: human unfamiliar with MDEMG reads deck + speaker notes without presenter, reports mental-model gaps
- Factual spot-check: 10 random claims from deck traced to canonical sources. Zero fabrications tolerated.

### Docs

- Feature doc at `docs/features/mdemg-onboarding-deck.md`
- Sprint post at `docs/development/mdemg-onboarding-deck-001/sprint_post.md`
- README cross-reference
- CHANGELOG entry

## 7 Commit Strategy

- **Epic 1 commit**: this sprint plan + presentation spec + global roadmap (one commit; small docs that logically ship together)
- **Epic 2 commit**: all section roadmaps (one commit; same reason)
- **Epic 3 commits**: one commit per section family (if 6 families, 6 commits — each is a sub-agent's returned content pass)
- **Epic 4 commit**: initial deck markdown + speaker notes (one commit)
- **Epic 5 commits**: rendering fixes as encountered (small per-fix commits)
- **Epic 6 commits**: refinement from rehearsal (one commit per refinement pass)
- **Epic 7 commit**: feature doc + README cross-link + CHANGELOG (one commit)
- **Epic 8 commit**: sprint post (one commit)

Per `must-comment-sprint-summary-on-pr` rule: every PR comment ends with a sprint summary. The sprint spans multiple PRs — each PR summary names its epic.

## 8 Verification Checklist

Per the live-testing-tier-required rule, at least one checklist item MUST be a Tier-3 live-system item.

- [ ] 12-section sprint plan committed
- [ ] Presentation specification approved by operator
- [ ] Global roadmap approved by operator (section count + slide-count envelope agreed)
- [ ] Per-section roadmaps complete; each lists canonical sources
- [ ] Multi-agent deep-dive content docs returned for every section
- [ ] Operator spot-check pass on each content doc; corrections applied
- [ ] Deck markdown authored; renders in Obsidian + Advanced Slides without errors
- [ ] All Mermaid diagrams render
- [ ] Speaker notes companion authored
- [ ] **Live Tier-3**: full timed walkthrough by operator confirms 90-120 min duration — stopwatch recorded in sprint post
- [ ] **Live Tier-3**: cold-reader pass completed; mental-model gaps identified + closed
- [ ] **Live Tier-3**: 10-claim factual spot-check against canonical sources returns 0 fabrications
- [ ] Feature doc `docs/features/mdemg-onboarding-deck.md` committed
- [ ] README.md cross-reference committed
- [ ] CHANGELOG.md Unreleased/Added entry committed
- [ ] Sprint post committed
- [ ] All drift verifiers green

## 9 Documentation Update

Final epic — never cut per `must-follow-12-section-format`:

- `docs/features/mdemg-onboarding-deck.md` — purpose, location, how to update, update cadence (quarterly re-verification against CLAUDE.md), how to run a walkthrough, prerequisites
- `README.md` — one-line pointer under a new "New team members" section
- `CHANGELOG.md` — Unreleased/Added entry mirroring existing cadence
- `docs/development/mdemg-onboarding-deck-001/sprint_post.md` — honest sprint summary

## 10 Risks & Mitigations

- **Scope creep → 300-slide monster**: enforce 80-120 slide envelope in Epic 1; cut aggressively in Epic 5. Operator holds the knife.
- **Stale content**: pin quarterly re-verification cadence in feature doc. Deck WILL drift as subsystems evolve; visible "last verified: YYYY-MM-DD" footer per section forces re-visitation.
- **Deep-dive agent fabricates a feature detail**: Epic 3 spot-check explicit about this; every claim MUST cite its canonical source. `must-validate-all-claims-before-commit` applies.
- **Walkthrough drifts long**: cold-reader pass in Epic 6 catches content that reads well silently but takes 3× as long to speak.
- **Audience miscalibration**: declared audience = senior engineers + ex-eng PMs; speaker notes carry "simplify-for-PM" fallback phrasings; deck itself stays technical-engineer level.
- **Component inventory misses something major**: Epic 1c global roadmap is the forcing function; peer review (operator) catches gap before Epic 2 starts.
- **Advanced Slides plugin missing on operator machine**: declared dependency; verified pre-Epic 5. Fallback: deck renders as plain markdown slideshow via any reveal.js tool.

## 11 Documents Accessed (initial — will grow per epic)

- `CLAUDE.md` — architectural notes, 100+ sprint pins, operator preferences
- `docs/features/jiminy-lora-qwen.md` — shipped 2026-10-14
- `docs/features/uxts-frameworks.md` — shipped 2026-10-15
- `docs/features/j17-agent-briefing-packet.md` + `-v1.0-superseded.md`
- `docs/development/UXTS_FRAMEWORK_MATRIX.md` — canonical UxTS inventory
- `docs/development/session-notes/2026-10-15-session-status.md`
- Prior sprint-post docs referenced in CLAUDE.md pins
- `docs/features/` directory listing
- `packaging/homebrew-mdemg/README.md`

## 12 Rollback Procedures

Not applicable in the destructive-ops sense — this sprint ships documentation only, no code change, no schema change, no substrate mutation. Rollback = `git revert <commit>` on any epic's commit if the content needs withdrawal.

If a published slide contains an outdated or wrong claim after merge: open a tiny fix-commit (same shape as DOC-DRIFT-LANG-COUNT-001); do NOT revert the deck wholesale.
