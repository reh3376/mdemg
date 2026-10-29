# Presentation Spec · End-User Path

**Sprint**: ONBOARDING-TUTORIAL-ENDUSER-001
**Governance pattern**: adapted from [`../mdemg-onboarding-deck-001/presentation_spec.md`](../mdemg-onboarding-deck-001/presentation_spec.md)
**Status**: DRAFT, awaiting operator sign-off (E1 gate)
**Last updated**: 2026-10-26

> This document is the **content contract** for the end-user walkthrough content shipped into `docs/onboarding/mdemg-onboarding-deck.md` behind the `#end-user-path` fork. The dev-path 107-slide deck is governed by its sibling spec in `mdemg-onboarding-deck-001/`; this spec is what the END-USER PATH content is tested against.

---

## 1a. Opening Framing (operator directive 2026-10-26)

The slideshow (both paths, end-user AND dev) opens by answering 4 strategic questions + an integrating slide, and each subsequent section is an in-depth answer to a piece of those questions:

1. **What is MDEMG?**
2. **What problem(s) does it attempt to solve?**
3. **How does it solve those problems?**
4. **How does it continuously get better at solving those problems?**
5. **How does it all work together?** (the integrator, one Mermaid showing the whole loop)

**Where answered**:
- At the top of each path: U1 (end-user) / F1 (dev), one slide per question + landing
- Then each U-section (resp. F-section) is one or more in-depth answers mapped back to those questions

**End-user U-section → question mapping**:

| Question | Where deep |
|---|---|
| Q1 What is MDEMG? | U1 (overview); recurring throughout as the system's identity surfaces |
| Q2 What problems does it solve? | U3 Memory (context loss) · U4 Jiminy (behavioral drift) · U16 Alerts (silent failures) |
| Q3 How does it solve them? | U2 Stack · U3-U15 the surfaces that implement the solution |
| Q4 How does it continuously improve? | U5 RSIC · U6 UxTS · U7 HITL · U8 Learning/Training · the trajectory language across the whole deck |

Dev-path F1 is already shipped and partially aligned (opens with "What MDEMG Is Trying To Do" mission quote). **Deferred follow-up**: audit dev-path F1 against this 4-question structure in a future sprint; may require minor F1 slide additions.

---

## 1. Audience

**Target audience**: Operators of a running MDEMG instance.
- Someone who ran `brew install reh3376/mdemg/mdemg` or `mdemg init --defaults && docker compose up -d`
- Someone who wants to USE the system, the operator's view

The slideshow forks at the landing slide into **2 paths**: end-user (this one) and software dev / contributor (the shipped F1-F10 107-slide dev deck). Each path is comprehensive for the audience of that path (end-user path for operators, dev path for developers/contributors); viewers pick one path and stay in it.

**Explicit NON-audience**:
- Software developers extending MDEMG → they take the dev-path at the fork
- Non-technical users / exec summaries → not an MDEMG use case today
- Pure architecture / "how is this built" questions → that's the dev-path's job

**Posture** (operator-directed 2026-10-26: "extensive and highly informative onboarding mechanism"):
- **Comprehensive-first, terse-second**: cover every major topic in genuine depth; don't trim for brevity
- **Operator-task shaped**: show what to click / type / watch, plus the mental model, current state, and gotchas for each topic so the operator truly OWNS the surface
- **CLI-example heavy**: commands + output verbatim remain the backbone
- **Honest trajectory language**: inherited from dev-path governance, no "by design" framing on sub-target metrics

---

## 2. Slide-Count + Visual-Element Policy

**No caps** (operator directive 2026-10-26): "do not set unnecessary constraints on figures, tables, diagrams, or slides. This will take as much as necessary to provide the data required."

- No upper bound on total slide count.
- No upper bound on Mermaid diagrams, tables, images, code blocks, or any other visual element per slide or per section.
- No lower bound either, a topic that covers cleanly in 2 slides doesn't need padding to 5.

**Rule**: each slide / figure / table / diagram EARNS its place by conveying information the viewer needs AND cannot easily get from the preceding slide. If removing an element would leave a gap the viewer can't fill from context, keep it. If it's redundant with the slide before, drop it.

**Depth template** (guide, not straitjacket):

Each major topic (U-section) typically covers:
1. **Section landing**: what this covers, why it matters
2. **Mental model**: the mental model the operator should carry away (Mermaid if it genuinely helps)
3. **Where to see it**: UI tab(s), Grafana panel(s), log patterns
4. **How to touch it**: CLI commands, API endpoints
5. **Current state (honest)**: today's state + arc moving the needle (trajectory language)
6. **Gotchas**: common footguns, misconceptions
7. **Where to go deeper**: feature doc, CLAUDE.md pin, sprint record

**Topics may expand elements as needed**:
- U3 Memory may need 2-3 Mermaid diagrams (hierarchy + Hebbian flow + consolidation lifecycle) if that's what the operator needs to carry the mental model.
- U6 UxTS may need a full table of the 15 shipped frameworks + their forcing-function pattern per family.
- U15 APIs may need the full route-table reference as a scroll-through slide.

The current `global_roadmap.md` section breakdown is a **working estimate**, not a budget. Author authors to the information requirement; the table updates to match. Operator E5 live-review is the final arbiter of "enough / too much / missing."

**Walk-time**: unmeasured. Operator can pause anytime (keyboard ESC → overview grid → resume anywhere).

---

## 3. Per-Slide Template

Dev deck uses a **5-beat template** (`what / why / how / honest / connect`) because every F-section explains an architectural COMPONENT. The end-user path explains OPERATIONS, which need a different shape.

**End-user slide template** (operator-task slides, ~70% of the deck):

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

**Section-landing slide template** (one per U-section, ~15% of the deck):

```
TITLE: section number + name (e.g. "U4 · CLI, The ~15 Commands You'll Use")

1-sentence intent for the section ("You'll use these daily; everything else is optional.")

- Bulleted preview of the 3-4 subsection themes
- Target time to walk this section
- Natural Q&A anchor marker

note:
- Operator pacing cue
```

**Pure-info slide template** (TOC, Thank You, pointer slides, ~15% of the deck):

```
TITLE + 1-sentence body + link table or bulleted next-steps.
```

**Never allowed**:
- Walls of prose (>3 lines of flat text without a list / code block / diagram break)
- Slides whose body is purely explanatory ("MDEMG is a cognitive substrate...", operator doesn't care here; that's dev-path territory)
- Slides without a concrete operator action, UI reference, or next-step pointer

---

## 4. Content Shape Rules

### 4.1 CLI-example heaviness

- Every slide that references a CLI command SHOULD show the command verbatim in a `bash` block, and SHOULD show truncated expected output.
- CLI example block height: **≤ 8 lines** of actual output; longer outputs get `...` truncation + a note like `(14 more lines)`.
- Commands MUST be copy-paste-ready: no `<placeholders>` without a defined-value example above.
- Operator sign-off at E5 includes paste-and-run against a live instance.

### 4.2 Mermaid policy

- **No cap** (operator directive 2026-10-26). Diagrams earn their place by conveying mental models that prose can't easily carry.
- **Minimum expected diagrams** (these are the ones the roadmap LOCKS as required; sections may add more as authoring demands):
  - **U1.6**: the integrated system flow (memory + retrieval + Jiminy + RSIC + UxTS + HITL + retrain fitting together)
  - **U2**: the 5-container + mdemg server stack sketch
  - **U3 Memory**: L0→L1→L2+ hierarchy sketch; may need additional diagrams for Hebbian co-activation and consolidation lifecycle
  - **U4 Jiminy**: guidance → outcome → trust-signal loop
  - **U5 RSIC**: admission → cycle → outcome loop
  - **U6 UxTS**: the forcing-function diamond (spec → fixture → runner → CI gate); likely needs a family-map table as well
  - **U15 APIs + gRPC**: surface map (operators via CLI → REST; plugins/transfer via gRPC); may need per-service protocol diagrams
  - **U16 Alerts**: override-flow sketch (block → operator → override → audit trail)
- Sections that need MORE diagrams for completeness take them. Operator E5 review decides "enough / too much."

### 4.3 Screenshot policy

- **Default: NO screenshots.** `docs/onboarding/assets/` is empty by design (dev-deck convention); we honor it.
- **Operator decides at E5 review**: if a /ui/ tab description feels unclear without a visual, operator captures the screenshot THEN, adds it to `assets/`, and E4 re-stitches.
- Screenshot captures MUST be scrubbed of operator-personal info (no email, no identifiable paths, same hygiene rules as beta-share bundles).

### 4.4 Section ordering

**Fixed** (operator sign-off at E1 can revise):

1. U1 Welcome, frame the "running stack" mental model first; everything else assumes you can see containers
2. U2 /ui/ Dashboard, operators arrive at :9999/ui/ via the Grafana link; it's the first thing they click
3. U3 Grafana, complementary surface to /ui/; the one most operators underuse
4. U4 CLI, the thing operators type when the UI doesn't cover it
5. U5 Memory, the thing operators tell OTHER operators about ("you can inspect what MDEMG remembers")
6. U6 Alerts + Enforcement, the thing operators HANDLE when something's off
7. U7 Where To Go, honest exit

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

**End-user-specific application**: if a slide shows `Cache Hit Ratio: LOW alert`, name it as "current ~75% vs DH-004 honest baseline", NOT "LOW by design".

### 5.3 Deck-rides-the-claim (HARD, forward-only)
Any shipped sprint that CHANGES an end-user claim (new UI tab, renamed CLI flag, new env var, retired dashboard panel) MUST update the affected U-slide in the same commit.

Enforcement: operator catches at next quarterly refresh if a sprint omits the slide update; filer-sweep follow-up if drift becomes systematic.

### 5.4 Anchor preservation (sprint-specific, HARD)
The `#end-user-path` anchor on the U1 landing slide MUST survive this sprint and all future revisions.

**Why**: `ONBOARDING-TUTORIAL-SERVE-001` shipped a landing fork slide that links to `#end-user-path`. Renaming the anchor silently breaks the fork; the operator who clicks "End User" lands at the Thank You slide (default fall-through) with no indication of what went wrong.

**Enforcement**: E4 regression check greps `mdemg-onboarding-deck.md` for both `id="end-user-path"` AND `href="#end-user-path"` (or `#end-user-path` in bare markdown link form), both must be present and well-formed in the resynced embed.

### 5.5 Quarterly refresh (soft)
End-user content refresh cadence follows dev-deck's quarterly schedule. Owner: reh3376. Refresh checklist to live in `mdemg-onboarding-notes.md` section (shared with dev deck).

---

## 6. Change Control

**Any change to this spec** (slide-count envelope, section ordering, per-slide template, Mermaid/screenshot policy) must:
1. Open a comment or discussion in the sprint tracking surface
2. Operator sign-off before any U-section roadmap or draft slide changes
3. Spec version-bumped at TOP of file (currently v1.0, this document)
4. CLAUDE.md pin re-pointed if invariant changed

**Any change to a U-section's scope** (slide count, section ordering, intent) within the shipped structure:
1. Must land in `global_roadmap.md` AND the relevant `roadmaps/U*.md` in the same commit
2. Operator spot-check at E2 covers this during initial authoring; later revisions follow the deck-rides-the-claim rule
3. If a change WIDENS scope beyond the 45-slide hard upper bound, it's a scope creep, defer to a follow-up sprint

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
