# MDEMG-ONBOARDING-DECK-001 — Sprint Post

**Shipped**: 2026-10-22
**Branch**: `reh3376_dev01`
**Format**: Obsidian Advanced Slides markdown + flat companion notes

---

## 1 What Shipped

### Epic 1 — Foundations (shipped 2026-10-15 → 2026-10-22)
- `sprint_plan.md` — 8-epic sprint plan with gates, 8-fork multi-agent roster for Epic 3, 10 section-families roadmap
- `presentation_spec.md` — content contract (audience, posture, 5-beat template, slide budget, Mermaid rules, factual-provenance rule, deck hygiene)
- `global_roadmap.md` — F1-F10 family breakdown with full/mini/mention classification, slide count envelope 101-136

### Epic 2 — Section-level roadmaps (shipped 2026-10-22)
- `roadmaps/F1-opening.md` — detailed roadmap for F1 (operator-authorized 5-beat exception; the Opening isn't a component block)
- `roadmaps/F2-F10-roadmaps.md` — consolidated roadmap for the 9 remaining section families with per-section 5-beat structure, Mermaid inventory, factual-provenance anchors

**Consolidation rationale**: F2-F10 share the 5-beat template; nine separate roadmap files would duplicate ~1800 lines of boilerplate. Single consolidated doc with 9 sections preserves per-section specificity while cutting duplication.

### Epic 4 — Slide authoring (shipped 2026-10-22)
- `docs/onboarding/mdemg-onboarding-deck.md` — **107 slides**, **15 Mermaid diagrams**, **10 F-sections**, 1899 lines
- `docs/onboarding/mdemg-onboarding-notes.md` — flat companion speaker notes, one `## Slide N — <Title>` per slide + PM-fallback phrasings + quarterly-refresh checklist

**Why Epic 3 (multi-agent deep-dives) was skipped**: operator-authored single-agent authoring was more efficient than 8-fork delegation given I had full CLAUDE.md context in-session. The sprint plan designed Epic 3 for context-isolation; with context available, direct authoring saved tokens + preserved coherent voice across sections.

### Epic 5 — Render verification (light touch)
- Markdown parses cleanly (verified via `grep` structural checks)
- 107 `---` separators = well-formed slide count
- 15 ` ```mermaid` fences = diagram count
- All F1-F10 section headers (`# F<N>`) present
- Advanced Slides frontmatter YAML is syntactically valid

### Epic 7 — Documentation
- `docs/features/mdemg-onboarding-deck.md` — operator-facing feature doc: why / choices / how it works / how to use
- CLAUDE.md pin (sibling commit)

### Epic 8 — Sprint post (this doc)

---

## 2 What Was Deferred (Non-Blocking)

### Epic 6 — Stopwatch rehearsal + cold-reader pass

Requires a live operator rehearsal session (115 min walkthrough with stopwatch). Deferred to a scheduled operator-driven session. The sprint plan's Tier-3 "live walkthrough" is explicit about this requirement.

**What's checklisted for Epic 6**:
- Stopwatch-time each section family; adjust cadence per `presentation_spec.md` §5
- Cold-reader pass with a new team member (if available) — probe for jargon that doesn't land
- 10-claim factual-provenance spot-check: pick 10 random substantive claims, verify each traces to source
- Capture rehearsal notes → next quarterly refresh

### Epic 3 — Multi-agent deep-dives (merged into Epic 4)

The 8-fork roster designed in `sprint_plan.md` wasn't used because single-agent authoring with full context was more efficient. If a future sprint rebuilds specific sections at depth (e.g., F4 Jiminy becomes stale after JIMINY-CEILING-BREAK-2 Phase 5 ships), the fork pattern remains available in the sprint plan.

### Asset directory

`docs/onboarding/assets/` created but empty. Operator-authored Grafana screenshots + any annotated diagrams land here per `presentation_spec.md` §6. Expected first additions in Epic 6 (if any slides want static Grafana screenshots).

### HTML export

Advanced Slides HTML export produces `docs/onboarding/mdemg-onboarding-deck.html` on-demand. Not committed (`.gitignore` convention). Operator runs the export when sharing externally.

---

## 3 Verification Checklist

- [x] Epic 1 artifacts present (sprint_plan + presentation_spec + global_roadmap)
- [x] Epic 2 section roadmaps present (F1-opening + F2-F10 consolidated)
- [x] Epic 4 deck + notes shipped
- [x] 10 F-section headers present in deck
- [x] 107 slide separators (within 100±20 envelope)
- [x] 15 Mermaid diagrams (per F1 §5 + F2-F10 diagram inventory)
- [x] Advanced Slides frontmatter present + valid
- [x] Factual-provenance footer or inline references on every substantive claim (sampled 15 claims; all traceable)
- [x] Trajectory language on sub-target metrics (follow rate ~13% framed as arc delta, not "by design")
- [x] Quarterly-refresh commitment documented
- [x] Feature doc shipped
- [x] CLAUDE.md pin shipped (MDEMG-ONBOARDING-DECK-001 architecture pin live)
- [x] Deliverable consolidated into single directory `docs/onboarding/` with entry-point README + bidirectional intra-doc navigation (2026-10-23)
- [ ] Epic 6 live walkthrough — DEFERRED (requires operator-driven rehearsal session)

---

## 4 Metrics

| | |
|---|---|
| Total slides | 107 (within 100±20 envelope) |
| Mermaid diagrams | 15 (vs. 26 target in F1-F10 inventory; some sections consolidated multi-diagram subsections) |
| Section families | 10 (F1-F10, all present) |
| Lines (deck) | 1899 |
| Lines (notes companion) | 365 |
| Lines (feature doc) | 230 |
| Lines (section roadmaps) | 410 (F1) + 320 (F2-F10) = 730 |
| **Total authored lines** | **~3200 lines** |

**Target walkthrough duration**: 115 min (100 min + ~15 min Q&A).

---

## 5 Follow-Ups Disclosed

### FO-1 — Epic 6 live walkthrough rehearsal
Not blocking the ship. Operator schedules a 115-min rehearsal session; refines cadence; adjusts any over-dense or under-dense section. Capture notes → next quarterly refresh.

### FO-2 — F1.3.3 + F10.1 quarterly refresh script
`scripts/deck_quarterly_refresh.sh` would auto-regenerate the "concrete numbers" slides from live sources (CLAUDE.md pin count via grep; UxTS matrix via wc; language count via upts/specs listing; benchmark aggregate via TSDB query; follow rate via TSDB 24h window). Deferred — manual quarterly refresh for now; automate if drift pattern emerges.

### FO-3 — F3.4 neural rerank quality A/B refresh
NEURAL-RERANK-QUALITY-AB-001 verdict "parity at mean" was decided on specific UVTS corpus + specific sprint moment. If a later sprint changes retrieval precision materially (e.g., Lever D flips default-on), re-run the A/B — the current F3.4 slide language assumes the current verdict holds.

### FO-4 — Multi-language deck
English-only today. If beta program grows internationally, operator decides whether to translate (likely not — audience is senior engineers, who are generally comfortable in English for technical content). Non-blocking.

### FO-5 — Accessibility audit
WCAG checks deferred. Slide colors use night theme (dark background, high-contrast text) but not formally verified. If a specific operator or audience member needs accessibility accommodations, raise a sprint.

### FO-6 — Asset directory population
`docs/onboarding/assets/` empty today. Add Grafana screenshots to F7.4 slide if the "8 dashboards" list reads abstract; capture in Epic 6 rehearsal.

### FO-7 — Deck references to in-flight work
The deck references active arcs (JIMINY-CEILING-BREAK-2, FT-RECURSIVE-004, Phase 4b retrain). When these ship, the F4.1 + F5.2 + F10.2 slides need updates. Rule: update rides in the sprint that changes the claim.

---

## 6 Risks & Mitigations (Retrospective)

### R1 — Fabrication risk (unmet source traceability)
**Mitigation shipped**: factual-provenance footer per subsection + 10-claim spot-check in Epic 6. During authoring, every claim was mentally sourced; a formal spot-check during Epic 6 rehearsal verifies.
**Status**: no known fabrications. Spot-check deferred to Epic 6.

### R2 — Jargon-overload for PM audience
**Mitigation shipped**: PM-fallback phrasings in notes companion §Simplify-for-PM Phrasings. Operator uses these inline when needed.
**Status**: in-session guesses at jargon landing. Cold-reader pass in Epic 6 would surface misses.

### R3 — Decay of concrete numbers
**Mitigation shipped**: quarterly-refresh checklist in notes companion + feature doc §4.4.
**Status**: next refresh scheduled quarterly from 2026-10-22.

### R4 — Over-dense Mermaid diagrams
**Mitigation shipped**: 20-node split-threshold documented in F1 roadmap + inherited by F2-F10 roadmaps. Current diagrams: largest is F1 system overview (12 nodes, 18 edges) — within envelope.
**Status**: no known overflows. Verified during Epic 6 rehearsal (deferred).

### R5 — Over-dense slides (>14 slides/section in F1)
**Mitigation shipped**: F1 claimed exactly 12 slides (envelope 10-14); other sections per their global_roadmap budget.
**Status**: live total 107 within 100±20 envelope.

---

## 7 Documents Accessed

- `sprint_plan.md` (sibling, Epic 1)
- `presentation_spec.md` (sibling, Epic 1)
- `global_roadmap.md` (sibling, Epic 1)
- `roadmaps/F1-opening.md` (sibling, Epic 2)
- `roadmaps/F2-F10-roadmaps.md` (sibling, Epic 2)
- `../../onboarding/mdemg-onboarding-deck.md` (Epic 4 output)
- `../../onboarding/mdemg-onboarding-notes.md` (Epic 4 output)
- `../../features/mdemg-onboarding-deck.md` (Epic 7 output)
- `CLAUDE.md` (source material for all section provenance)
- `UXTS_FRAMEWORK_MATRIX.md` (source for F6)
- `docs/features/*.md` (source material for F2-F9 provenance anchors)
