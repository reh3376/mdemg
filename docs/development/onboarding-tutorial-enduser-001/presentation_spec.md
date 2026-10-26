# Presentation Spec — End-User Path

**Sprint**: ONBOARDING-TUTORIAL-ENDUSER-001
**Governance pattern**: adapted from [`../mdemg-onboarding-deck-001/presentation_spec.md`](../mdemg-onboarding-deck-001/presentation_spec.md)
**Status**: DRAFT — awaiting operator sign-off (E1 gate)
**Last updated**: 2026-10-26

> This document is the **content contract** for the end-user walkthrough content shipped into `docs/onboarding/mdemg-onboarding-deck.md` behind the `#end-user-path` fork. The dev-path 107-slide deck is governed by its sibling spec in `mdemg-onboarding-deck-001/`; this spec is what the END-USER PATH content is tested against.

---

## 1. Audience

**Target audience**: Operators of a running MDEMG instance.
- Someone who ran `brew install reh3376/mdemg/mdemg` or `mdemg init --defaults && docker compose up -d`
- Someone who wants to USE the system — not build it, not extend it, not understand its architecture
- Technical enough to use a terminal and read SQL / JSON output

**Explicit NON-audience**:
- Software developers extending MDEMG → they go through the dev-path 107-slide deck (shipped by MDEMG-ONBOARDING-DECK-001)
- Non-technical users / exec summaries → not an MDEMG use case today; different artifact if ever needed
- Beta testers specifically → they have `packaging/homebrew-mdemg/mdemg_beta_testing.md`; this walkthrough SUPPLEMENTS that doc but doesn't replace it

**Posture**:
- **Operator-first**: show what to click, type, watch — not why it works that way
- **Terse**: operators arrive with time pressure; dense slides lose them
- **CLI-example heavy**: show commands + expected output verbatim
- **Honest trajectory language**: inherited from dev-path governance — no "by design" framing on sub-target metrics

---

## 2. Slide-Count Envelope

**Target**: 28-40 slides total across U1-U7.
**Hard upper bound**: 45 slides (if the roadmap exceeds this, cut scope or split U-sections).
**Hard lower bound**: 20 slides (below this, the walkthrough is probably skipping something load-bearing).

**Rationale**: dev deck is 107 slides at ~115 min for senior engineers; end-user path targets operators who should get through the walkthrough in **~20-30 min**. Slide-density stays similar; count shrinks because scope shrinks.

**Per-section targets** (soft):

| § | Target slides | Rationale |
|---|---:|---|
| U1 Welcome + What You Have | 3-4 | Shortest; sets context, lists containers, names the 4-command happy path |
| U2 The /ui/ Dashboard | 5-7 | 4 tabs × ~1-2 slides each; the central operator surface |
| U3 Grafana — What To Actually Watch | 4-6 | Opinionated: don't walk 9 dashboards, name 3-5 panels |
| U4 CLI — The ~15 Commands You'll Use | 6-8 | 4 command groups × ~2 slides each; one slide per group's rationale, one per group's command list |
| U5 Memory — Inspect + Correct | 4-6 | observe / recall / correct are the operator-facing memory surface |
| U6 Alerts + Enforcement Escape Hatches | 4-6 | Reading the terminal banner + jiminy mode + override |
| U7 Where To Go Next | 2-3 | Pointers to docs/FAQ/issues; honest about which sprints are open |
| **Total** | **28-40** | Fits envelope |

---

## 3. Per-Slide Template

Dev deck uses a **5-beat template** (`what / why / how / honest / connect`) because every F-section explains an architectural COMPONENT. The end-user path explains OPERATIONS, which need a different shape.

**End-user slide template** (operator-task slides — ~70% of the deck):

```
TITLE: short imperative or question (e.g. "What's Running?", "Opening /ui/", "Reading an Alert")

1 sentence of context (what you're about to do / why this matters)

CLI example OR screenshot OR 3-bullet UI tour:
  $ mdemg healthz
  200 OK ...
  OR
  [ screenshot: /ui/ Memory tab, labeled callouts ]
  OR
  - Click X → see Y
  - If Z is red, run A
  - To toggle: B

note:
- Operator cue: when to say the PM-fallback phrasing
- Factual provenance: CLAUDE.md:line / feature doc / shipped code (stripped from final slide, kept in note block)
```

**Section-landing slide template** (one per U-section — ~15% of the deck):

```
TITLE: section number + name (e.g. "U4 · CLI — The ~15 Commands You'll Use")

1-sentence intent for the section ("You'll use these daily; everything else is optional.")

- Bulleted preview of the 3-4 subsection themes
- Target time to walk this section
- Natural Q&A anchor marker

note:
- Operator pacing cue
```

**Pure-info slide template** (TOC, Thank You, pointer slides — ~15% of the deck):

```
TITLE + 1-sentence body + link table or bulleted next-steps.
```

**Never allowed**:
- Walls of prose (>3 lines of flat text without a list / code block / diagram break)
- Slides whose body is purely explanatory ("MDEMG is a cognitive substrate..." — operator doesn't care here; that's dev-path territory)
- Slides without a concrete operator action, UI reference, or next-step pointer

---

## 4. Content Shape Rules

### 4.1 CLI-example heaviness

- Every slide that references a CLI command SHOULD show the command verbatim in a `bash` block, and SHOULD show truncated expected output.
- CLI example block height: **≤ 8 lines** of actual output; longer outputs get `...` truncation + a note like `(14 more lines)`.
- Commands MUST be copy-paste-ready: no `<placeholders>` without a defined-value example above.
- Operator sign-off at E5 includes paste-and-run against a live instance.

### 4.2 Mermaid policy

- **Default: NO Mermaid.** End-user content explains OPERATIONS, not architecture. If a slide feels like it needs Mermaid, ask: "would the operator be able to do their task without this diagram?" If yes → cut it.
- **Soft cap**: **0-2 Mermaid diagrams across the entire end-user path.** Dev deck has 15 across 107 slides (~14%). End-user is a different animal.
- Permitted Mermaid use cases (operator pre-approval required):
  - U1 might benefit from a tiny `graph LR: Docker → MDEMG server → You` sketch (judgment call)
  - U6 might benefit from a tiny alert-flow sketch (operator → terminal banner → `mdemg jiminy override`)

### 4.3 Screenshot policy

- **Default: NO screenshots.** `docs/onboarding/assets/` is empty by design (dev-deck convention); we honor it.
- **Operator decides at E5 review**: if a /ui/ tab description feels unclear without a visual, operator captures the screenshot THEN, adds it to `assets/`, and E4 re-stitches.
- Screenshot captures MUST be scrubbed of operator-personal info (no email, no identifiable paths — same hygiene rules as beta-share bundles).

### 4.4 Section ordering

**Fixed** (operator sign-off at E1 can revise):

1. U1 Welcome — frame the "running stack" mental model first; everything else assumes you can see containers
2. U2 /ui/ Dashboard — operators arrive at :9999/ui/ via the Grafana link; it's the first thing they click
3. U3 Grafana — complementary surface to /ui/; the one most operators underuse
4. U4 CLI — the thing operators type when the UI doesn't cover it
5. U5 Memory — the thing operators tell OTHER operators about ("you can inspect what MDEMG remembers")
6. U6 Alerts + Enforcement — the thing operators HANDLE when something's off
7. U7 Where To Go — honest exit

**Not negotiable**: U6 (Alerts + Enforcement) stays late because operators need the first 5 sections to understand WHAT they're triaging / toggling.

---

## 5. Invariants Inherited from Dev-Deck Governance

Per [`../mdemg-onboarding-deck-001/presentation_spec.md`](../mdemg-onboarding-deck-001/presentation_spec.md) + CLAUDE.md pins:

### 5.1 Factual provenance (HARD)
Every substantive claim traces to:
- (a) CLAUDE.md pin + date, OR
- (b) a feature doc path, OR
- (c) a sprint record path, OR
- (d) shipped code at file+line.

Fabrication = sprint failure. E5 operator review spot-checks ≥10 claims at random.

**End-user-specific application**: if a slide says `mdemg healthz returns status=degraded when a subsystem is down`, that claim's provenance is `internal/api/handlers_health.go` (or the healthz feature doc). The slide text doesn't carry the citation; the slide's `note:` block captures it for E5 review.

### 5.2 Trajectory language (HARD)
Any slide reading a sub-target metric MUST frame it as trajectory, NOT "by design":
- WRONG: `Follow rate 13% by design (classifier is honest about its ceiling)`
- RIGHT: `Follow rate ~13% (current baseline; target ≥80% via JIMINY-CEILING-BREAK-2)`

Per the 2026-08-01 `trust-signal-must-be-persisted-never-ignore-honest` directive.

**End-user-specific application**: if a slide shows `Cache Hit Ratio: LOW alert`, name it as "current ~75% vs DH-004 honest baseline" — NOT "LOW by design".

### 5.3 Deck-rides-the-claim (HARD, forward-only)
Any shipped sprint that CHANGES an end-user claim (new UI tab, renamed CLI flag, new env var, retired dashboard panel) MUST update the affected U-slide in the same commit.

Enforcement: operator catches at next quarterly refresh if a sprint omits the slide update; filer-sweep follow-up if drift becomes systematic.

### 5.4 Anchor preservation (sprint-specific, HARD)
The `#end-user-path` anchor on the U1 landing slide MUST survive this sprint and all future revisions.

**Why**: `ONBOARDING-TUTORIAL-SERVE-001` shipped a landing fork slide that links to `#end-user-path`. Renaming the anchor silently breaks the fork; the operator who clicks "End User" lands at the Thank You slide (default fall-through) with no indication of what went wrong.

**Enforcement**: E4 regression check greps `mdemg-onboarding-deck.md` for both `id="end-user-path"` AND `href="#end-user-path"` (or `#end-user-path` in bare markdown link form) — both must be present and well-formed in the resynced embed.

### 5.5 Quarterly refresh (soft)
End-user content refresh cadence follows dev-deck's quarterly schedule. Owner: reh3376. Refresh checklist to live in `mdemg-onboarding-notes.md` section (shared with dev deck).

---

## 6. Change Control

**Any change to this spec** (slide-count envelope, section ordering, per-slide template, Mermaid/screenshot policy) must:
1. Open a comment or discussion in the sprint tracking surface
2. Operator sign-off before any U-section roadmap or draft slide changes
3. Spec version-bumped at TOP of file (currently v1.0 — this document)
4. CLAUDE.md pin re-pointed if invariant changed

**Any change to a U-section's scope** (slide count, section ordering, intent) within the shipped structure:
1. Must land in `global_roadmap.md` AND the relevant `roadmaps/U*.md` in the same commit
2. Operator spot-check at E2 covers this during initial authoring; later revisions follow the deck-rides-the-claim rule
3. If a change WIDENS scope beyond the 45-slide hard upper bound, it's a scope creep — defer to a follow-up sprint

---

## 7. What This Spec Does NOT Define

- The actual slide titles (U-section roadmaps define those at E2)
- The actual slide bodies (E3 draft file carries those)
- The specific CLI examples (operator may revise at E5 review)
- Visual theming (reveal.js theme stays `night`; CSS override from `slides-overrides.css` stays untouched this sprint)
- Keyboard navigation (reveal.js defaults; unchanged by this sprint)
- The HTML export pipeline (shipped by SERVE-001; this sprint is a content layer on top)

---

**Operator sign-off (E1 gate)**:

- [ ] Audience + non-audience match operator's mental model of who will use this
- [ ] 28-40 slide envelope is right (not too dense, not too thin)
- [ ] U-section ordering is right (U1→U7)
- [ ] Per-slide template fits (operator-task + section-landing + pure-info)
- [ ] CLI-example heaviness + Mermaid-light + screenshot-opt-in policies feel right
- [ ] All 5 inherited invariants hold
- [ ] No scope creep lurking

If any checkbox fails, iterate here before E2 kicks off.
