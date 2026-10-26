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
- Someone who wants to USE the system — the operator's view

The slideshow forks at the landing slide into **2 paths**: end-user (this one) and software dev / contributor (the shipped F1-F10 107-slide dev deck). Each path is comprehensive for the audience of that path (end-user path for operators, dev path for developers/contributors); viewers pick one path and stay in it.

**Explicit NON-audience**:
- Software developers extending MDEMG → they take the dev-path at the fork
- Non-technical users / exec summaries → not an MDEMG use case today
- Pure architecture / "how is this built" questions → that's the dev-path's job

**Posture** (operator-directed 2026-10-26: "extensive and highly informative onboarding mechanism"):
- **Comprehensive-first, terse-second**: cover every major topic in genuine depth; don't trim for brevity
- **Operator-task shaped**: show what to click / type / watch — plus the mental model, current state, and gotchas for each topic so the operator truly OWNS the surface
- **CLI-example heavy**: commands + output verbatim remain the backbone
- **Honest trajectory language**: inherited from dev-path governance — no "by design" framing on sub-target metrics

---

## 2. Slide-Count Envelope

**Target**: **comprehensive — no upper cap** (operator-directed 2026-10-26). Expected range ~80-120 slides.
**Lower bound**: 60 slides (below this, major topics are getting undercovered).

**Rationale**: operator directed "extensive and highly informative onboarding mechanism for users and software devs / contributors" + "I am not overly concerned about the total number of slides or time required to review." Prior envelopes (28-40 → 45-60) were optimizing for brevity; this one optimizes for depth of each topic. Dev-path deck is 107 slides at ~115 min; end-user path can be similar scale when covering 17 topics at full depth.

**Per-topic depth guideline** (soft; `global_roadmap.md` carries live numbers):

Each major topic (U-section) covers:
1. **Section landing** — 1 slide — what this covers, why it matters
2. **Mental model** — 1-2 slides, optionally Mermaid — the thing operator should carry away
3. **Where to see it** — 1-2 slides — UI tab(s), Grafana panel(s), log patterns
4. **How to touch it** — 1-2 slides — CLI commands, API endpoints
5. **Current state (honest)** — 1 slide — "today it looks like this; here's why; here's the arc moving it"
6. **Gotchas** — 0-1 slides — common footguns, misconceptions
7. **Where to go deeper** — 1 slide — feature doc link, CLAUDE.md pin reference, sprint record

That's **5-9 slides per topic**. 17 topics × avg 6-7 = **~100-120 slides** total.

Per-section ballpark targets:

| § | Topic | Target slides |
|---|---|---:|
| U1 | Welcome / What's Running | 5 |
| U2 | Status + Health | 5 |
| U3 | Memory (CMS) | 8 |
| U4 | Jiminy | 9 |
| U5 | RSIC | 7 |
| U6 | UxTS Framework Family | 8 |
| U7 | Review / HITL | 6 |
| U8 | Learning / Training Data | 6 |
| U9 | Backup + Ops | 5 |
| U10 | Logs | 4 |
| U11 | Config + Features | 6 |
| U12 | Plugins | 4 |
| U13 | Grafana | 6 |
| U14 | CLI | 7 |
| U15 | APIs + gRPC | 7 |
| U16 | Alerts + Enforcement | 6 |
| U17 | Where To Go Next | 3 |
| **Total** | | **~102** |

Walk-time at ~45-60 sec/slide ≈ **~80-100 min**. Comparable to the dev-path deck's 115 min — appropriate for comprehensive onboarding of two audiences (operators + contributors).

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

- **Default: minimal Mermaid.** End-user content explains OPERATIONS, not architecture — but at full-depth topic coverage, a few mental-model diagrams earn their place.
- **Soft cap**: **4-6 Mermaid diagrams across the entire end-user path** (revised 2026-10-26 for full-depth coverage). Dev deck has 15 across 107 slides (~14%); end-user keeps similar density (~10%).
- Operator-approved Mermaid slots (locked at E1):
  - **U1.2** — the 5-container + mdemg server sketch (stack mental model)
  - **U3** Memory — L0→L1→L2+ hierarchy sketch (how Hebbian consolidation flows)
  - **U4** Jiminy — guidance → outcome → trust-signal loop
  - **U5** RSIC — admission → cycle → outcome loop
  - **U6** UxTS — the forcing-function diamond (spec → fixture → runner → CI gate)
  - **U15** APIs + gRPC — surface map (operators via CLI → REST; plugins/transfer via gRPC)
  - **U16** Alerts — override-flow sketch (block → operator → override → audit trail)
- Any Mermaid NOT in the above list requires operator E5-review approval before shipping.

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
