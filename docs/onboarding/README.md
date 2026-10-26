# MDEMG Onboarding

> Self-contained walkthrough deck for new engineers, technical PMs, and operators of running MDEMG instances.
> Two paths from a landing fork: **Dev** (F1-F10, ~115 min, engineering-dense) and **End User** (U1-U17, ~85 min, operator-dense).

**Last verified**: 2026-10-26 · **Owner**: reh3376 · **Quarterly refresh cadence**: see [§Governance](#governance) below.

---

## Start Here

| If you want to… | Open this |
|---|---|
| **View it in a browser** (running mdemg instance, zero setup) | `mdemg tutorial` — opens the embedded deck at `/ui/tutorial/`. See [`../features/tutorial-serve.md`](../features/tutorial-serve.md). |
| **Present it live** (operator-driven, 115 min) | [`mdemg-onboarding-deck.md`](./mdemg-onboarding-deck.md) in Obsidian with Advanced Slides plugin → "Start Slideshow" |
| **Read it async** (self-serve, ~45-60 min skim) | [`mdemg-onboarding-deck.md`](./mdemg-onboarding-deck.md) on GitHub — Mermaid diagrams render inline |
| **Pre-brief yourself** before a live session | [`mdemg-onboarding-notes.md`](./mdemg-onboarding-notes.md) — flat speaker notes without Obsidian |
| **Jump to a specific section** | Use the [Section Index](#section-index) below |
| **Understand how this deck is governed** | [`../features/mdemg-onboarding-deck.md`](../features/mdemg-onboarding-deck.md) + [§Governance](#governance) |

---

## Section Index

The combined deck is **231 slides** total: Dev path (F1-F10, 107 slides, 15 Mermaid) + End-User path (U1-U17, 122 slides, 8 Mermaid). Pick your path from the landing fork on slide 2 (or jump to its anchor).

### Dev Path (F1-F10) — for engineers / technical PMs

| § | Section | Slides | Deck | Notes |
|---|---|---|---|---|
| F1 | **Opening** — mission, analogy, history, 10k-ft system view | 12 | [deck ▸ F1](./mdemg-onboarding-deck.md#f1--opening) | [notes ▸ F1](./mdemg-onboarding-notes.md#f1--opening) |
| F2 | **Substrate Core** — Neo4j CMS, Hebbian, consolidation, TSDB | 12-16 | [deck ▸ F2](./mdemg-onboarding-deck.md#f2--substrate-core) | [notes ▸ F2](./mdemg-onboarding-notes.md#f2--substrate-core) |
| F3 | **Retrieval Pipeline** — RRF 4-column, fingerprinting, sparse gate, rerank | 10-14 | [deck ▸ F3](./mdemg-onboarding-deck.md#f3--retrieval-pipeline) | [notes ▸ F3](./mdemg-onboarding-notes.md#f3--retrieval-pipeline) |
| F4 | **Jiminy + Enforcement + J17** (headline) — guidance, strict mode, deny taxonomy | 20-26 | [deck ▸ F4](./mdemg-onboarding-deck.md#f4--jiminy--enforcement--j17) | [notes ▸ F4](./mdemg-onboarding-notes.md#f4--jiminy--enforcement--j17) |
| F5 | **LoRA + Qwen + Model Distribution** — Phase 5 SFT, GGUF pipeline, homebrew tap | 12-16 | [deck ▸ F5](./mdemg-onboarding-deck.md#f5--lora--qwen--model-distribution) | [notes ▸ F5](./mdemg-onboarding-notes.md#f5--lora--qwen--model-distribution) |
| F6 | **UxTS Framework Family** — the forcing-function matrix | 7-10 | [deck ▸ F6](./mdemg-onboarding-deck.md#f6--uxts-framework-family) | [notes ▸ F6](./mdemg-onboarding-notes.md#f6--uxts-framework-family) |
| F7 | **Ops** — RSIC, HITL, backup/restore, Grafana | 14-18 | [deck ▸ F7](./mdemg-onboarding-deck.md#f7--ops) | [notes ▸ F7](./mdemg-onboarding-notes.md#f7--ops) |
| F8 | **CLI, Hooks, MCP, Multi-Instance** | 6-8 | [deck ▸ F8](./mdemg-onboarding-deck.md#f8--cli-hooks-mcp-multi-instance) | [notes ▸ F8](./mdemg-onboarding-notes.md#f8--cli-hooks-mcp-multi-instance) |
| F9 | **Beta + Security + Dormant Census** | 4-6 | [deck ▸ F9](./mdemg-onboarding-deck.md#f9--beta-program--security--dormant-census) | [notes ▸ F9](./mdemg-onboarding-notes.md#f9--beta-program--security--dormant-census) |
| F10 | **Conclusion** — where we are, where we're going, Q&A | 6-8 | [deck ▸ F10](./mdemg-onboarding-deck.md#f10--conclusion) | [notes ▸ F10](./mdemg-onboarding-notes.md#f10--conclusion) |

**Natural Q&A anchors (Dev)**: pause after F1.5 recap · F1.3.2 pivots · F4.1 Jiminy why · F10.6 final.

### End-User Path (U1-U17) — for operators of running MDEMG instances

Reach via the landing fork (click **End User**) or anchor `#end-user-path`. Each U-section opens the deck at that section header (reveal.js scroll view).

| § | Section | Slides | Deck |
|---|---|---|---|
| U1 | **Opening** — 4 strategic questions + integrator-view diagram | 6 | [deck ▸ U1](./mdemg-onboarding-deck.md#end-user-path) |
| U2 | **Install + Start the Stack** — docker compose, healthz, first login | 8 | [deck ▸ U2](./mdemg-onboarding-deck.md#u2--install--start-the-stack) |
| U3 | **Core Data Flows** — observe, recall, correct | 9 | [deck ▸ U3](./mdemg-onboarding-deck.md#u3--core-data-flows) |
| U4 | **Jiminy** — guidance surfaces, strict mode, override escape hatch | 10 | [deck ▸ U4](./mdemg-onboarding-deck.md#u4--jiminy) |
| U5 | **RSIC Self-Improvement** — the loop operators see | 7 | [deck ▸ U5](./mdemg-onboarding-deck.md#u5--rsic-self-improvement) |
| U6 | **HITL Review Surfaces** — grading operator-review datasets | 8 | [deck ▸ U6](./mdemg-onboarding-deck.md#u6--hitl-review-surfaces) |
| U7 | **Autograde Cadence** — review autograde loop + reinforcement invariant | 7 | [deck ▸ U7](./mdemg-onboarding-deck.md#u7--autograde-cadence) |
| U8 | **Benchmarks** — model freshness + the benchmark-staleness rule | 7 | [deck ▸ U8](./mdemg-onboarding-deck.md#u8--benchmarks) |
| U9 | **Backup + Restore** — the 3-layer gate + the drill | 7 | [deck ▸ U9](./mdemg-onboarding-deck.md#u9--backup--restore) |
| U10 | **Grafana** — the five dashboards operators actually open | 8 | [deck ▸ U10](./mdemg-onboarding-deck.md#u10--grafana) |
| U11 | **Alert Triage** — reading the alert bar + closing the loop | 7 | [deck ▸ U11](./mdemg-onboarding-deck.md#u11--alert-triage) |
| U12 | **CLI Reference by Group** — the operator-facing command catalog | 8 | [deck ▸ U12](./mdemg-onboarding-deck.md#u12--cli-reference-by-group) |
| U13 | **Hook Channel Health** — observing + healing the Claude Code hook pipe | 6 | [deck ▸ U13](./mdemg-onboarding-deck.md#u13--hook-channel-health) |
| U14 | **Multi-Instance** — isolation, teardown, when to split instances | 7 | [deck ▸ U14](./mdemg-onboarding-deck.md#u14--multi-instance) |
| U15 | **APIs + gRPC** — direct consumers, integrating MDEMG into other tools | 8 | [deck ▸ U15](./mdemg-onboarding-deck.md#u15--apis--grpc) |
| U16 | **The Override Escape Hatch** — when to override Jiminy, how, audit trail | 5 | [deck ▸ U16](./mdemg-onboarding-deck.md#u16--the-override-escape-hatch) |
| U17 | **Where to Go Next** — operator playbook + closing + return-to-fork | 4 | [deck ▸ U17](./mdemg-onboarding-deck.md#u17--where-to-go-next) |

**Natural stopping points (End-User)**: after U3, U5, U8, U11, U14, U17 (per `global_roadmap.md` §4).

The end-user path is COMPLETE for operators by design — it does NOT send operators to the dev path for any topic. Dev-only concerns (training pipeline, model distribution internals, UxTS framework authoring) are intentionally out-of-scope.

---

## How to Navigate the Deck

### Obsidian Advanced Slides (recommended for live walkthrough)

1. Open [`mdemg-onboarding-deck.md`](./mdemg-onboarding-deck.md) in Obsidian with the **Advanced Slides** plugin enabled
2. Click **"Start Slideshow"** (or `Ctrl/Cmd+P` → "Start presentation")
3. Keyboard:
   - **←** / **→** — previous / next slide
   - **ESC** — slide overview grid (jump anywhere)
   - **F** — fullscreen
   - **S** — speaker-notes view (shows the `note:` block + next slide)
   - **?** — show all keyboard shortcuts
4. The deck's internal Table of Contents is the slide right after the title. Use **ESC overview** to jump to any F-section.

### GitHub / IDE flat-markdown (async read)

- Mermaid diagrams render natively on GitHub
- Every F-section heading has a GitHub-slug anchor (links in the [Section Index](#section-index) above use these)
- Use `Cmd+F` / in-page search to jump by keyword
- Pair with [`mdemg-onboarding-notes.md`](./mdemg-onboarding-notes.md) for the operator notes you'd otherwise see in Obsidian's speaker view

### Export to HTML (shareable, offline)

In Obsidian: `Ctrl/Cmd+Shift+P` → **"Export reveal.js presentation to HTML"** → saves next to the deck.
The HTML export is **gitignored** — do not commit. Share via `file://` or a private host; the canonical copy lives here in-repo.

---

## Audience + Posture

Per [`presentation_spec.md`](../development/mdemg-onboarding-deck-001/presentation_spec.md) §1:

- **Target**: senior engineers (10+ years, polyglot) OR technical PMs who were previously senior engineers
- **Not target**: non-technical PMs, beginner engineers, beta testers (they have their own doc: `packaging/homebrew-mdemg/mdemg_beta_testing.md`)
- **Posture**: engineering-dense · no fluff · honest about failures and compromises · trajectory-framed (never "sub-target by design")

[`mdemg-onboarding-notes.md`](./mdemg-onboarding-notes.md) carries **PM-fallback phrasings** per piece of engineering jargon — use them inline when needed.

---

## Governance

**Full governance contract**: [`../features/mdemg-onboarding-deck.md`](../features/mdemg-onboarding-deck.md) §5.

Load-bearing invariants:

1. **Factual provenance** — every substantive claim traces to CLAUDE.md pin + date, feature doc, sprint record, or shipped code at file+line. Fabrication = sprint failure. 10-claim spot-check in Epic 6 rehearsal enforces this.
2. **Trajectory language** — any slide reading a sub-target metric MUST frame it as trajectory, NOT "by design." Example: `Follow rate 13% (target ≥80% via JIMINY-CEILING-BREAK-2)` — NOT `Follow rate 13% by design.` Per the 2026-08-01 `trust-signal-must-be-persisted-never-ignore-honest` architectural directive.
3. **Sprint-rides-the-claim** — when a shipped sprint changes a claim the deck makes (follow rate, benchmark aggregate, framework count), that sprint's commit MUST include the slide update. Deferred fixes rot the deck.
4. **Change control** — any change to deck structure (slide-count envelope, section families, 5-beat template) MUST update [`../development/mdemg-onboarding-deck-001/presentation_spec.md`](../development/mdemg-onboarding-deck-001/presentation_spec.md) in the same PR.
5. **Quarterly refresh** — operator re-walks CLAUDE.md pins + feature docs + checks the `Last verified` date on every section-header slide. Checklist in [`mdemg-onboarding-notes.md`](./mdemg-onboarding-notes.md#quarterly-refresh-checklist).

---

## Directory Contents

```
docs/onboarding/
├── README.md                       ← you are here (entry point + index)
├── mdemg-onboarding-deck.md        ← 231-slide combined deck (Dev F1-F10 + End-User U1-U17)
├── mdemg-onboarding-notes.md       ← flat speaker-notes companion (Dev path)
├── slides-overrides.css            ← theme overrides for reveal.js serving
└── assets/                         ← operator-authored screenshots (empty by design today)
```

**Related artifacts** (durable, outside this directory):

- Feature / governance doc (dev-path source + deck invariants): [`../features/mdemg-onboarding-deck.md`](../features/mdemg-onboarding-deck.md)
- Serving doc (how `/ui/tutorial/` works): [`../features/tutorial-serve.md`](../features/tutorial-serve.md)
- **Dev-path sprint**: [`../development/mdemg-onboarding-deck-001/`](../development/mdemg-onboarding-deck-001/)
  - [`sprint_plan.md`](../development/mdemg-onboarding-deck-001/sprint_plan.md) · 8-epic plan with gates
  - [`presentation_spec.md`](../development/mdemg-onboarding-deck-001/presentation_spec.md) · content contract (audience / posture / templates)
  - [`global_roadmap.md`](../development/mdemg-onboarding-deck-001/global_roadmap.md) · F1-F10 slide-count breakdown
  - [`roadmaps/F1-opening.md`](../development/mdemg-onboarding-deck-001/roadmaps/F1-opening.md) · detailed F1 roadmap
  - [`roadmaps/F2-F10-roadmaps.md`](../development/mdemg-onboarding-deck-001/roadmaps/F2-F10-roadmaps.md) · consolidated F2-F10 roadmap
  - [`sprint_post.md`](../development/mdemg-onboarding-deck-001/sprint_post.md) · ship summary + deferred items
- **End-User-path sprint** (SHIPPED 2026-10-26): [`../development/onboarding-tutorial-enduser-001/`](../development/onboarding-tutorial-enduser-001/)
  - [`sprint_plan.md`](../development/onboarding-tutorial-enduser-001/sprint_plan.md) · 6-epic plan with the live-review gate
  - [`presentation_spec.md`](../development/onboarding-tutorial-enduser-001/presentation_spec.md) · operator-dense content contract
  - [`global_roadmap.md`](../development/onboarding-tutorial-enduser-001/global_roadmap.md) · U1-U17 breakdown
  - [`roadmaps/`](../development/onboarding-tutorial-enduser-001/roadmaps/) · one roadmap per U-section
  - [`sprint_post.md`](../development/onboarding-tutorial-enduser-001/sprint_post.md) · ship summary + deferred items

---

## What's Open

The combined deck is **shipped and operator-reachable** via `mdemg tutorial` at `/ui/tutorial/`. Both paths are complete end-to-end.

Non-blocking follow-ups (opt-in, not sprint-gating):

- **Dev-path F1.4 + F3.1 Mermaid diagrams** render too wide in scroll-view (text crushed). Fix is `graph LR → graph TD` + shortened labels; belongs to a dev-path follow-up sprint (same shape as the U1.6/U4.2/U16.5 fixes applied to the end-user path).
- **Dev-path F1 alignment with 4-question opening framing** (apply the end-user path's U1 pattern to F1 so both paths open the same way) — deferred to `MDEMG-ONBOARDING-DECK-002`.
- **Dev-path topology section** detailing the 5-container stack with cross-refs to F2/F3/F7 — same follow-up sprint.
- **Server HTTP gzip middleware** compresses 404 bodies without setting `Content-Encoding: gzip`, so a bad relative link renders garbled bytes in the browser. Reproducer: `curl -H 'Accept-Encoding: gzip' http://localhost:9999/ui/tutorial/foo.md`. Belongs to its own sprint; the deck sidesteps it by using absolute GitHub URLs for cross-repo references.
- **Epic 6 live stopwatch rehearsal** for the dev path (115-min walkthrough with cadence verification + 10-claim spot-check). Schedule when calendar permits.
- **Quarterly-refresh script automation**, WCAG accessibility audit, HTML export sharing policy, deck-rides-sprint rule enforcement in future sprint commits. See [`../development/mdemg-onboarding-deck-001/sprint_post.md`](../development/mdemg-onboarding-deck-001/sprint_post.md) §5.
- **LinkedIn share** of the combined deck — deferred until operator decides on surface + audience framing.
