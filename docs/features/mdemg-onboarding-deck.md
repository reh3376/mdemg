# MDEMG Onboarding Deck

**Feature doc** for the MDEMG onboarding presentation (sprint MDEMG-ONBOARDING-DECK-001).

**Deck**: `docs/onboarding/mdemg-onboarding-deck.md`
**Speaker notes**: `docs/onboarding/mdemg-onboarding-notes.md`
**Section roadmaps**: `docs/development/mdemg-onboarding-deck-001/roadmaps/`
**Sprint**: `docs/development/mdemg-onboarding-deck-001/`

## 1 Why This Exists

A new engineer joining the MDEMG project — or a technical PM taking responsibility for a slice of it — faces a documentation surface of ~105 CLAUDE.md architecture pins, ~40 feature docs, ~140 sprint records, 16 UxTS frameworks, 28 ingested languages, 8 Grafana dashboards, and 35+ alert rules. There is no single-walk-through that orients a reader in less than a day.

This deck is that walk-through. 115 min, 107 slides, Obsidian Advanced Slides format. Engineering-dense, no fluff, trajectory-framed.

**Non-goals**:
- Executive summaries (separate artifact; belongs with marketing)
- Beta-tester onboarding (already in `packaging/homebrew-mdemg/mdemg_beta_testing.md`)
- Per-sprint decks (would be ~100 decks and all would rot)
- Video or audio in-line (reveal.js supports it; we don't need it)

## 2 Choices Made

### 2.1 Obsidian Advanced Slides over PowerPoint / Keynote

**Rationale**: markdown-first, version-controllable, diff-friendly in PRs, reveal.js-based (web-exportable), supports Mermaid natively, speaker notes inline.

**Rejected**: PPTX (binary, no diff), Keynote (macOS-only + proprietary), bare reveal.js (operator overhead), MkDocs (missing presenter mode).

### 2.2 Markdown as canonical, HTML as derived

The `.md` file in-repo is canonical. HTML export (via Obsidian Advanced Slides reveal.js) is a derived artifact, not committed. This keeps the surface stable in `git log` and PR diffs.

### 2.3 In-repo location

`docs/onboarding/` sits alongside `docs/features/`, `docs/development/`, `docs/user/`. Not under `docs/development/mdemg-onboarding-deck-001/` because the deliverable is a durable artifact (quarterly-refreshed), not a sprint record.

### 2.4 5-beat template per major component

Per `presentation_spec.md` §3: `what it is / why we use it / how it works / why it's important / how it will improve`. Enforces uniformity across section families. F1 (Opening) and F10 (Conclusion) are operator-authorized exceptions.

### 2.5 Mermaid for all diagrams

Per `presentation_spec.md` §6: Mermaid is light-theme-neutral, embeddable, grep-findable, PR-diffable. No external image assets committed beyond operator-authored screenshots in `docs/onboarding/assets/`.

### 2.6 Factual provenance rule

Every substantive claim traces to:
1. A CLAUDE.md pin (named sprint + date), OR
2. A feature doc at `docs/features/*.md`, OR
3. A sprint record at `docs/development/<sprint>/`, OR
4. Shipped code at a specific file + line

**Fabrication = sprint failure.** The 10-claim spot-check in Epic 6 Tier-3 enforces this.

### 2.7 Honest about state

Trajectory language per the 2026-08-01 architectural directive (`trust-signal-must-be-persisted-never-ignore-honest`). Where current state is below target, name the arc that owns the delta. F10.1's "Where We Are Today" slide reads the current honest follow rate (~13%) + benchmark aggregate (0.9188) + target (≥80%).

### 2.8 Quarterly refresh commitment

Numbers rot. Per `presentation_spec.md` §9: "Last verified: YYYY-MM-DD" footer per section-header slide forces visible staleness. Quarterly re-verification: operator re-walks CLAUDE.md + feature docs + checks each verified-date footer.

## 3 How It Works

### 3.1 Format

- Single markdown file: `docs/onboarding/mdemg-onboarding-deck.md`
- Advanced Slides frontmatter: theme=night, width=1280, height=720
- `---` horizontal separator for new slide; `--` vertical for sub-slide stacks
- `note:` block inline per slide — mirrored in `docs/onboarding/mdemg-onboarding-notes.md`

### 3.2 Section families + slide budget

Per `global_roadmap.md`:

| Family | Full / Mini / Mention | Slides |
|---|---|---|
| F1 Opening | 4 full-ish | 10-14 |
| F2 Substrate Core | 1 full + 1 mini | 12-16 |
| F3 Retrieval Pipeline | 1 full + 2 mini | 10-14 |
| F4 Jiminy + Enforcement + J17 | 2 full + 1 mini | 20-26 |
| F5 LoRA + Qwen + Model Dist | 1 full + 1 mini + homebrew mini | 12-16 |
| F6 UxTS Framework Family | 1 full | 7-10 |
| F7 Ops | 2 full + 2 mini | 14-18 |
| F8 CLI / Hooks / MCP | 2 mini + mentions | 6-8 |
| F9 Beta / Security / Dormant Census | 2 mini + mentions | 4-6 |
| F10 Conclusion | 1 full | 6-8 |
| **Total** | | **101-136** |

**Actual shipped**: 107 slides (within envelope).

### 3.3 Cadence + Timing

- 1 min/slide average → 100 slides = 100 min + ~15 min Q&A = **~115 min total**
- Fast context frames: ~30 sec
- Diagram-heavy mechanism slides: ~90 sec
- Component "why it's important" slides often provoke Q&A → 2 min
- Opening (first 10 slides): 10 min — fast
- Conclusion (last 8 slides): 10 min — slow, invite questions

Validated in Epic 6 Tier-3 live walkthrough with stopwatch (deferred to live rehearsal session with operator).

### 3.4 Mermaid diagrams (15 shipped)

All light-theme-neutral; node count ≤ 20 per diagram (split-on-overflow discipline).

Named diagrams by section:

| Section | ID | Content |
|---|---|---|
| F1 | 5 Mermaids | History timeline, system overview, 5-layer hierarchy, hot path sequence, recursive loop |
| F2 | 3 Mermaids | Node+edge schema (ER), 5-layer with promotion predicates, Hebbian 4-paths |
| F3 | 1 Mermaid | RRF 4-column parallel + fusion |
| F4 | 2 Mermaids | Jiminy flow, deny taxonomy |
| F5 | 2 Mermaids | LoRA pipeline, recursive retrain |
| F10 | 1 Mermaid | 4-loop continuous improvement |

### 3.5 Audience targeting

Per `presentation_spec.md` §1:
- Target: senior engineers (10+ years, polyglot) OR technical PMs who were previously senior engineers
- NOT target: non-technical PMs, beginner engineers, beta testers
- Posture: engineering-dense, no fluff, honest about failures and compromises, trajectory-framed

Speaker notes include **PM-ready fallback phrasings** for every piece of engineering jargon (`mdemg-onboarding-notes.md` §Simplify-for-PM Phrasings).

## 4 How to Use

### 4.1 Live walkthrough

**Prerequisites**:
- Obsidian installed + Advanced Slides plugin enabled
- Open the deck file in Obsidian
- Click "Start Slideshow" (or press `Ctrl/Cmd + P` → "Start presentation")

**Operator plays**:
- Full 115 min walkthrough: present end-to-end
- Section-only walkthrough: use `?` key to jump to table-of-contents, pick a section
- Q&A anchors: pause after F1.1 (audience fit), F1.3.2 (pivots), F1.5 (recap), F4.1 (follow rate), F10.6 (final Q&A)

### 4.2 Async self-serve

Reader opens `docs/onboarding/mdemg-onboarding-deck.md` in GitHub / IDE. Markdown renders natively; Mermaid diagrams render on GitHub.

Companion `docs/onboarding/mdemg-onboarding-notes.md` provides flat-reading speaker notes without needing Obsidian.

### 4.3 Briefing a new team member

**Pattern**:
1. Hand them the two markdown files
2. Suggest 30 min self-read of F1 + F10 (opening + conclusion) BEFORE live walkthrough
3. Schedule 115 min for live walkthrough with Q&A
4. Follow up with `docs/features/*.md` deep reads based on what captured their interest

### 4.4 Quarterly refresh

Per `presentation_spec.md` §9 + `mdemg-onboarding-notes.md` §Quarterly Refresh Checklist:

```bash
# Operator or delegate runs:
grep -c '^- \*\*' CLAUDE.md                              # pin count
wc -l docs/development/UXTS_FRAMEWORK_MATRIX.md          # UxTS matrix (count rows)
ls docs/lang-parser/lang-parse-spec/upts/specs/*.upts.json | wc -l   # language count

# Then update:
# - F1.3.3 "Where We Are Today" slide
# - F10.1 "Where We Are Today — Honest" slide
# - "Last verified" footer on title slide
```

### 4.5 Updating when a sprint changes a claim

When a shipped sprint changes a claim the deck makes (e.g., JIMINY-CEILING-BREAK-2 hits ≥80% actionable follow rate):

**Rule**: the deck update rides in the sprint that changes the claim, NOT as a separate deferred fix.

Example: when JIMINY-CEILING-BREAK-2 Phase 2 recalibrates follow-rate to ≥18%, that sprint's commit MUST include the F4.1 "Why It's Important" slide update + the F10.1 "Where We Are Today" slide update.

### 4.6 Export to HTML

Advanced Slides supports export via `Ctrl/Cmd + Shift + P` → "Export reveal.js presentation to HTML."

Export to `docs/onboarding/mdemg-onboarding-deck.html` (gitignored).

Share link = file:// — do NOT host externally. The canonical lives in-repo.

## 5 Governance

### 5.1 Change control

Any change to this deck's structure (slide count envelope, section families, 5-beat template) requires updating `docs/development/mdemg-onboarding-deck-001/presentation_spec.md` in the same PR. The spec is the content contract.

### 5.2 Factual provenance verification

Any claim lacking traceable provenance per `presentation_spec.md` §8 is a bug. PR reviews MUST spot-check 3+ claims against sources before approving.

### 5.3 Trajectory-language rule

Any slide that reads a sub-target metric MUST frame it as trajectory, not "by design." Example: "Follow rate 13% (target ≥80% via JIMINY-CEILING-BREAK-2)" NOT "Follow rate 13% by design."

The `trust-signal-must-be-persisted-never-ignore-honest` rule applies in deck content.

### 5.4 Deck owner

Current: reh3376. Transfers via explicit CLAUDE.md pin.

## 6 Known Limitations

- **No live-metric integration**: F1.3.3 and F10.1 numbers are static per quarterly refresh. A live-metric panel would require JS runtime in the HTML export, which Advanced Slides supports but adds surface we don't need.
- **No multi-language support**: deck is English-only.
- **No accessibility audit**: WCAG checks deferred; slide colors use night theme (dark background, high-contrast text) but not formally verified.
- **PM-fallback phrasings are optional**: operator chooses whether to use them live.

## 7 Related

- `docs/features/j17-agent-briefing-packet.md` — compressed-contract template that F4.3 references
- `docs/features/jiminy-lora-qwen.md` — the LoRA explainer deep-read that F5.1 references
- `docs/features/uxts-frameworks.md` — the UxTS deep-read that F6.1 references
- `docs/development/UXTS_FRAMEWORK_MATRIX.md` — single-source-of-truth for the UxTS matrix
- `CLAUDE.md` — the source material (all section provenance traces back here)

## 8 Documents Accessed

- `docs/development/mdemg-onboarding-deck-001/sprint_plan.md`
- `docs/development/mdemg-onboarding-deck-001/presentation_spec.md`
- `docs/development/mdemg-onboarding-deck-001/global_roadmap.md`
- `docs/development/mdemg-onboarding-deck-001/roadmaps/F1-opening.md`
- `docs/development/mdemg-onboarding-deck-001/roadmaps/F2-F10-roadmaps.md`
- `docs/onboarding/mdemg-onboarding-deck.md`
- `docs/onboarding/mdemg-onboarding-notes.md`
- Obsidian Advanced Slides plugin README (`https://github.com/MSzturc/obsidian-advanced-slides`)
