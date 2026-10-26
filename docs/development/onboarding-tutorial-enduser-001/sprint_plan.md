# Sprint Plan — ONBOARDING-TUTORIAL-ENDUSER-001

**Format**: Sprint Plan v1.0 (12-section)
**Status**: PLANNED
**Owner**: reh3376
**Opened**: 2026-10-26
**Supersedes**: `end-user-path` stub shipped by [ONBOARDING-TUTORIAL-SERVE-001](../onboarding-tutorial-serve-001/) Epic 4
**Sibling (shipped)**: [ONBOARDING-TUTORIAL-SERVE-001](../onboarding-tutorial-serve-001/)
**Parent arc**: [MDEMG-ONBOARDING-DECK-001](../mdemg-onboarding-deck-001/) (dev-path deck authored there; end-user path authored here)

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | ONBOARDING-TUTORIAL-ENDUSER-001 |
| Scope | Author the end-user walkthrough content behind the `#end-user-path` stub slide shipped by ONBOARDING-TUTORIAL-SERVE-001 E4. Target: ~30-40 slides, operator-focused, lighter than the 107-slide dev deck. |
| Audience (output) | **Operators using a running MDEMG instance** — not builders. Someone who installed via `brew install reh3376/mdemg/mdemg`, started the stack, and wants to know what to click/type/watch. |
| Not-audience | Builders (they go through the dev-path 107-slide deck); non-technical users (they get a different artifact, out of scope). |
| Deliverables | New content slides for U-sections (U1-U7) stitched into `docs/onboarding/mdemg-onboarding-deck.md`, replacing the single stub; feature-doc updated; CLAUDE.md pin; sprint post; README cross-ref. |
| Hard operator gate | **E5 live page-by-page review** by reh3376 is a BLOCKING Tier-3 gate between content ship (E4) and sprint closeout (E6). Review-driven revisions loop back through E3 until operator signs off. |
| Branch | `reh3376_dev01` (standard dev branch, per CLAUDE.md governance) |
| Target duration | 4-6 operator sessions (content authoring ~2-3 sessions, live review ~1 session, revision rounds ~1-2 sessions, closeout ~1 session) |

---

## 2. Problem Statement

### Context
ONBOARDING-TUTORIAL-SERVE-001 shipped the serving infrastructure for the onboarding deck at `/ui/tutorial/` + two entry points (`mdemg tutorial` CLI, Grafana overview link). The deck is forked at a landing slide into TWO paths:

- **Software Dev Onboarding** — the full 107-slide engineering walkthrough (shipped and operator-reviewed by MDEMG-ONBOARDING-DECK-001)
- **End User** — currently a **single stub slide** pointing at this sprint (`id=end-user-path`, appended after Thank You by E4 of the SERVE sprint)

### Problem
The end-user path is a one-slide stub. A beta tester who clicks "End User" at the fork lands on a message that says "coming soon, see sibling sprint." That's acceptable as a landing moment while infrastructure settles, but it is **not** an onboarding experience.

The gap: operators have no self-serve walkthrough of what MDEMG looks like from the operator chair — what to click, what to type, what to watch, what the alerts mean.

### What this sprint delivers
A ~30-40 slide end-user walkthrough that an operator can navigate (keyboard or click) to understand:
- What the running stack looks like (5 containers; `docker compose ps`)
- What to click in `/ui/` (Memory, Jiminy Rules, Review, Config tabs)
- What to watch in Grafana (the 3-5 panels that matter day-to-day; not all 9 dashboards)
- Which CLI commands they'll actually use (not all 90+ — the ~15 that matter)
- How to inspect / correct memory (`observe`, `recall`, `correct`)
- How to triage service alerts in their terminal
- How to toggle enforcement (`mdemg jiminy mode strict|suggest`)
- Where to find user docs and the FAQ

### What this sprint does NOT deliver
- Builder/architecture content (that's the dev path — already shipped)
- Non-operator audiences (non-technical users, exec summaries — out of scope)
- Mermaid-diagram-heavy explanations (end-user path should be screenshot/CLI-example heavy, Mermaid-light)
- New screenshots that require operator setup beyond what's already shipped (defer any multi-container capture to operator at review time)

---

## 3. Scope & Constraints

### In scope
1. New U-section family (U1-U7) authored directly into `docs/onboarding/mdemg-onboarding-deck.md` BETWEEN the existing `#end-user-path` stub slide and the end-of-file.
2. Rework the existing `#end-user-path` stub into a U-section landing slide (preserve the anchor ID — the fork links to it).
3. **Preserve existing deck structure** — F1-F10 dev-path sections untouched; TOC updated to add a U-section pointer.
4. Section-level roadmaps (U1-U7) authored under `docs/development/onboarding-tutorial-enduser-001/roadmaps/`.
5. Feature doc updates: `docs/features/tutorial-serve.md` section covering the end-user path; `docs/onboarding/README.md` section/table pointing at U-sections.
6. Live page-by-page operator review as the Tier-3 gate.

### Out of scope (deferred to follow-up sprints)
- Dedicated end-user feature doc split from `tutorial-serve.md` (one doc covers both until it gets unwieldy)
- Playwright e2e against the rendered deck (overkill for static content; dev-path never got one)
- Translation / localization
- Standalone end-user HTML export separate from `/ui/tutorial/` serving
- Operator-captured screenshots embedded in the deck (operator decides at review time if any are worth adding; assets dir empty-by-design today)

### Hard constraints (invariants that MUST hold)
1. **Factual provenance** — every substantive end-user claim (dashboard panel names, CLI flags, env vars, alert names) traces to shipped code, a feature doc, or CLAUDE.md. Fabrication = sprint failure.
2. **Trajectory language** — same as MDEMG-ONBOARDING-DECK-001; no sub-target metric framed as "by design". Example: cache hit ratio ~75% is `(current ~75%, honest baseline per DH-004)`, NEVER `(low by design)`.
3. **Deck-rides-the-claim** — any shipped sprint that changes an end-user claim (new UI tab, renamed env var, new CLI command) must update the U-section slide in the same commit. Enforced by `trust-signal-must-be-persisted-never-ignore-honest`.
4. **Preserve `#end-user-path` anchor ID** — the landing fork links to it; renaming breaks the fork silently.
5. **Deck-source edits + staged embed resync in the same commit** — enforced by `make verify-tutorial-embed` CI drift gate (CI-DEDUP-001 preserves this gate).
6. **Operator live review is a HARD gate** — not advisory. E6 (closeout) cannot ship without operator sign-off recorded in the sprint post.

### Deliberate posture
- **Shorter beats longer**. 30-40 slides is a soft upper bound; if 25 cover it cleanly, ship 25.
- **CLI-example heavy, prose-light**. End-users want to see what to type and what response to expect.
- **No Mermaid unless it earns its place**. The dev deck has 15 Mermaid diagrams because architecture needs them; end-user content rarely does.
- **PM-fallback phrasings welcome** (mirrors `mdemg-onboarding-notes.md` convention from MDEMG-ONBOARDING-DECK-001).

---

## 4. Dependencies

### Shipped (prerequisite — all green)
- **ONBOARDING-TUTORIAL-SERVE-001** (merged in PR 699 + PR 701) — serves the deck at `/ui/tutorial/`, provides `mdemg tutorial` CLI + Grafana link, mounts the stub.
- **MDEMG-ONBOARDING-DECK-001** (shipped 2026-10-22) — dev-path 107-slide deck + presentation_spec + governance (factual-provenance rule + trajectory-language rule inherited here).
- **CI-DEDUP-001** (shipped 2026-10-26) — halves CI runtime; the deck-source edit → embed-resync → push → PR cycle is now faster per iteration.

### Required operator availability
- Operator-authored live page-by-page review session (E5 Tier-3) — ~60-90 min single sitting OR split across sessions as needed. BLOCKING for E6.

### No downstream dependencies
- No code changes
- No schema changes
- No new env vars
- No new metrics / alerts / dashboards (the deck DESCRIBES existing ones; does not add any)
- No external services

---

## 5. Implementation Plan (Sequential Epics + Gates)

**Rule**: epics run sequentially per CLAUDE.md `sequential-epics` directive. Each epic completes fully before the next begins. The operator-review gate between E4 and E6 can loop back to E3 as needed.

### Epic 1 — Content contract + section roadmap
**Deliverables**:
- `docs/development/onboarding-tutorial-enduser-001/presentation_spec.md` — audience / posture / 5-beat template adapted for end-user content / slide-count envelope (30-40) / anchor-preservation rule / Mermaid-sparing rule
- `docs/development/onboarding-tutorial-enduser-001/global_roadmap.md` — U1-U7 slide-count breakdown with gates and natural stopping points
- Decision record: Mermaid-light vs Mermaid-forbidden; CLI-block height limit; screenshot inclusion policy

**U-section proposed structure** (operator may revise):

| § | Section | Target slides | What it covers |
|---|---|---:|---|
| U1 | **Welcome + What You Have** | 3-4 | 5 containers, 1 server, "it's running — now what?" + the 4-command shortest path (`init / up -d / healthz / open ui`) |
| U2 | **The /ui/ Dashboard** | 5-7 | Memory / Jiminy Rules / Review / Config tabs; what each shows, what you can do |
| U3 | **Grafana — What To Actually Watch** | 4-6 | The 3-5 panels that matter day-to-day across the 9 dashboards; red/yellow/green thresholds |
| U4 | **CLI — The ~15 Commands You'll Use** | 6-8 | Lifecycle (start/stop/restart/status), data (observe/recall/correct), ops (backup/upgrade/data export), introspection (healthz/synergy/watchdog) |
| U5 | **Memory — Inspect + Correct** | 4-6 | How MDEMG remembers; `mdemg data recall`, `mdemg data correct`, when to use each |
| U6 | **Alerts + Enforcement Escape Hatches** | 4-6 | Reading the terminal alert banner; `mdemg jiminy mode`; override escape hatch (`mdemg jiminy override`) |
| U7 | **Where To Go Next** | 2-3 | Documentation surfaces, FAQ, how to file issues, how to contribute observations |

Target total: ~28-40 slides (meets envelope).

**Gate before E2**: operator signs off on content contract + U1-U7 scope + slide-count envelope. If operator wants different U-sections or different ordering, E1 iterates until green-lit.

### Epic 2 — Per-section roadmaps (U1-U7)
**Deliverables**:
- `docs/development/onboarding-tutorial-enduser-001/roadmaps/U1-welcome.md` through `U7-next.md` — one per U-section
- Each roadmap: slide-by-slide outline (title, 1-sentence intent, draft content pointers to code/features/docs for provenance), target slide count, natural Q&A anchors

**Gate before E3**: all 7 roadmaps written; operator spot-checks 2-3 roadmaps against expected content shape.

### Epic 3 — Content authoring
**Deliverables**:
- Draft U1-U7 slides authored into a NEW staging file `docs/development/onboarding-tutorial-enduser-001/draft_slides.md`
- Each slide: title + body + `note:` block (operator cues + PM-fallback phrasings per MDEMG-ONBOARDING-DECK-001 convention)
- Factual-provenance comments inline (`<!-- source: CLAUDE.md:... -->` style breadcrumbs) — stripped at E4 stitch time but kept during authoring for review
- No stitching into `mdemg-onboarding-deck.md` yet — all content lives in the draft file so operator can review before deck surgery

**Gate before E4**: operator reads the draft file end-to-end (first-pass review, pre-deck-stitch). Any structural concerns surface here before invasive deck edits.

### Epic 4 — Stitch into deck + resync embed
**Deliverables**:
- `docs/onboarding/mdemg-onboarding-deck.md` updated:
  - `#end-user-path` stub replaced by U1 landing slide (preserves anchor ID — fork link stays live)
  - U2-U7 slides appended as new sections
  - TOC updated to add End-User section pointer (`#u1--welcome` or similar; chosen at authoring time)
  - Provenance comments stripped
- `make sync-tutorial-embed` run; `make verify-tutorial-embed` clean
- `docs/development/onboarding-tutorial-enduser-001/draft_slides.md` left intact as the audit trail (operator may delete after E6 or leave as the authoring record)

**Gate before E5**: `make verify-tutorial-embed` passes; local Go test suite for tutorial package + handler + grafanapin all green; `/ui/tutorial/` live-rendered locally and the end-user fork link resolves to U1.

### Epic 5 — Operator live page-by-page review (HARD GATE)
**Deliverables**:
- Operator opens `http://localhost:9999/ui/tutorial/` in a browser against a locally-running `mdemg`
- Operator advances through the deck slide-by-slide including the end-user path
- Review notes captured in `docs/development/onboarding-tutorial-enduser-001/review_notes.md` — per-slide pass/fix verdicts
- Each `fix` verdict blocks sprint closeout; loop back to E3 (fix content) → E4 (restitch + resync) → E5 (re-review the fixed slide only) until operator signs off on every slide

**Explicit scope of review**:
- Factual accuracy of every claim (operator spot-checks against CLAUDE.md / shipped code)
- Trajectory-language audit (no "by design" on sub-target metrics)
- Readability + flow (fresh-reader pass: does the next slide follow?)
- CLI examples work verbatim against the operator's running instance
- Keyboard nav works (←/→/ESC/F) across the new section
- Fork link still resolves to U1 landing

**No code / no deck / no doc changes during E5** — review session is pure capture. Fixes go through E3/E4 round-trip.

**Gate before E6**: operator signs off in `review_notes.md` with a final `all slides reviewed, sign-off: <date>` line.

### Epic 6 — Documentation closeout
**Deliverables**:
- `docs/features/tutorial-serve.md` — add End-User Path section describing U1-U7 structure + reach via `#end-user-path` fork
- `docs/onboarding/README.md` — update Section Index to include U-sections; update What's Open to remove the "sibling sprint pending" note
- CLAUDE.md — new Architecture Notes pin at TOP of Architecture Notes list; summarize epics, operator-review gate, arch rules pinned
- `docs/development/onboarding-tutorial-enduser-001/sprint_post.md` — ship record per standard template; call out live-review gate explicitly as the sprint's differentiator vs sibling sprints

**Gate before merge**: pre-commit inventory gate clean, route inventory unchanged (no new routes), CI green, PR body includes operator sign-off reference.

---

## 6. Testing Plan (3 tiers)

### Tier 1 — Unit tests
**Scope**: no new Go code; no unit tests needed for content.
**Exception**: if any U-section roadmap introduces a new `trust-signal` rule or similar invariant worth code-enforcing (unlikely for pure content), add targeted unit test then.

### Tier 2 — Integration tests
**Scope**:
- `make verify-tutorial-embed` passes after every E4 stitch (ensures source + embed in sync; CI gate enforces at merge)
- Local Go test suite still passes against the resynced embed (handler + embed-package tests already shipped by SERVE-001 cover the serving surface; they SHOULD continue to pass since we only edit content, not structure)
- `python3 scripts/verify_route_consumers.py` — no new routes expected; sanity-check it stays at 195/199
- `make verify-grafana-embed` — unchanged; sanity check

### Tier 3 — Live end-to-end (THE OPERATOR-GATE)
**Scope**:
- `mdemg start` → running server at :9999
- Browser → `http://localhost:9999/ui/tutorial/` → page loads with new content
- Operator navigates landing fork → clicks End User → lands on U1 (anchor preserved)
- Operator advances slide-by-slide through U1-U7 using keyboard (←/→/ESC)
- Each U-section's embedded CLI examples copied/pasted into a terminal → produce the described output (CLI-example provenance verified LIVE, not just read)
- Operator captures per-slide verdicts in `review_notes.md`

**This Tier 3 is the sprint's load-bearing checkpoint** — distinct from SERVE-001's live smoke (which proved the handler worked). THIS sprint's Tier 3 proves the CONTENT works.

---

## 7. Commit Strategy

Per CLAUDE.md `sequential-epics` + the segmentation preference established during the SERVE sprint:

- **E1** → single commit, planning artifacts only, docs path
- **E2** → single commit per section's roadmap OR one commit all-7 roadmaps (operator preference at the time)
- **E3** → single commit for draft slides (no deck mutation yet)
- **E4** → single commit for deck mutation + staged embed resync (both land together per the deck-source-edit-ride-the-commit rule)
- **E5** → operator-driven commits: per-fix round-trip through E3 → E4 → E5 produces a `fix(enduser): <slide-id> ...` commit at each loop iteration
- **E6** → single commit for feature doc + CLAUDE.md pin + sprint post + README cross-ref

**Push cadence**: push after each epic (not each commit); after CI-DEDUP-001 this is a single CI run per push, so segmentation stays fast.

**Attribution**: every commit ends `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>` per CLAUDE.md governance.

**PR scope**: one PR for the whole sprint OR per-epic PRs if operator prefers (operator preference recorded at E1 contract).

---

## 8. Verification Checklist

- [ ] E1 presentation_spec + global_roadmap authored; operator signs off on U1-U7 scope
- [ ] E2 all 7 roadmaps written; operator spot-checks ≥2
- [ ] E3 draft_slides.md authored end-to-end; operator first-pass reads it
- [ ] E4 deck stitched; `#end-user-path` anchor preserved; TOC updated; staged embed resynced; `make verify-tutorial-embed` clean
- [ ] E4 local Go test suite: tutorial handler + embed-package + grafanapin tests PASS
- [ ] E4 `python3 scripts/verify_route_consumers.py` — 195/199 OK (unchanged)
- [ ] **E5 Tier-3 live operator review**: `mdemg tutorial` opens the deck; operator advances slide-by-slide through U1-U7; per-slide verdict captured in `review_notes.md`; **operator sign-off line recorded**
- [ ] E5 every embedded CLI example copied/pasted into a terminal against a live local MDEMG; output matches what the slide describes
- [ ] E5 landing fork link `#end-user-path` resolves to U1 (not the old stub)
- [ ] E5 trajectory-language audit: no sub-target metric framed as "by design"
- [ ] E5 factual-provenance spot-check: operator picks 10 claims at random; each traces to CLAUDE.md / feature doc / shipped code
- [ ] E6 feature doc + README cross-ref + CLAUDE.md pin + sprint post landed in one commit
- [ ] E6 PR body carries operator sign-off reference line
- [ ] CI green on the final PR (post CI-DEDUP-001: single `pull_request` run)
- [ ] No regressions on `make verify-tutorial-embed`, `make verify-grafana-embed`, pre-commit inventory gate, route consumer inventory

---

## 9. Documentation Update (final epic)

E6 produces the four durable artifacts:

1. **`docs/features/tutorial-serve.md`** — new End-User Path section describing U1-U7 + reach via `#end-user-path` fork + content provenance
2. **`docs/onboarding/README.md`** — Section Index gains U-section rows; What's Open updated
3. **`CLAUDE.md`** — Architecture Notes pin at TOP summarizing epics, operator-review gate as the sprint's load-bearing differentiator, arch rules pinned
4. **`docs/development/onboarding-tutorial-enduser-001/sprint_post.md`** — ship record (standard template) explicitly naming the live-review gate

**Rule enforcement**:
- Deck-rides-the-claim: future sprints that touch a U-section claim (new UI tab, renamed CLI flag) MUST update the U-slide in the same commit (same rule as MDEMG-ONBOARDING-DECK-001)
- Any new U-section proposed by a future sprint reuses this sprint's shape (roadmap → draft → stitch → operator review)

---

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| **Operator-review session finds systemic issues (U-section structure wrong)** | Medium | High (loops back to E1) | Front-load E1 content contract review; operator spot-checks 2-3 U-section roadmaps at E2 before draft authoring; cheap-to-fix at roadmap stage, expensive-to-fix after slides written |
| **CLI examples drift between authoring and operator-review** | Low-Medium | Medium (each fix is 1 slide + resync) | CLI commands + flags captured with explicit `--version` evidence at authoring time (E3); live-run during E5 is the authoritative check |
| **Dev-path slide numbering shifts if we rearrange TOC** | Low | Low (anchors use heading slugs, not numbers) | TOC uses heading-slug anchors (shipped pattern); no numeric slide IDs |
| **Content drifts after sprint ships due to a feature change elsewhere** | Medium | Low (that's the deck-rides-the-claim rule's job) | Pin the rule in CLAUDE.md; operator catches at next quarterly refresh if a sprint omits the slide update |
| **Operator availability for E5 is bottleneck** | Medium | Low-Medium (sprint stays open, no code sits broken) | Live-review doesn't block ANY other work; the deck is served by SERVE-001 and the stub slide is honest about its stub-ness; sprint can idle at E5 without consequence |
| **Mermaid-light posture tempts us into text-wall slides** | Medium | Medium (readability tanks) | Enforce CLI-example-heavy via content contract; roadmap review (E2) spot-checks for text-wall slides early |
| **Scope creep into end-user feature additions ("while we're here...")** | Medium | Medium | E1 content contract names the "not in scope" boundary explicitly; follow-up sprints capture new features; this sprint ships end-user CONTENT about what's already shipped |

---

## 11. Documents Accessed

- [`docs/development/mdemg-onboarding-deck-001/sprint_plan.md`](../mdemg-onboarding-deck-001/sprint_plan.md) — the dev-path sprint plan, source of the 12-section v1.0 format + 5-beat template + factual-provenance / trajectory-language rules this sprint inherits.
- [`docs/development/mdemg-onboarding-deck-001/presentation_spec.md`](../mdemg-onboarding-deck-001/presentation_spec.md) — content contract shape being adapted here.
- [`docs/development/mdemg-onboarding-deck-001/global_roadmap.md`](../mdemg-onboarding-deck-001/global_roadmap.md) — reference for the F1-F10 → U1-U7 structural pattern.
- [`docs/development/onboarding-tutorial-serve-001/sprint_plan.md`](../onboarding-tutorial-serve-001/sprint_plan.md) + [`sprint_post.md`](../onboarding-tutorial-serve-001/sprint_post.md) — sibling sprint that shipped the infrastructure this sprint fills.
- [`docs/features/tutorial-serve.md`](../../features/tutorial-serve.md) — the feature doc this sprint will extend with an End-User Path section.
- [`docs/features/mdemg-onboarding-deck.md`](../../features/mdemg-onboarding-deck.md) — deck governance (factual-provenance invariants).
- [`docs/onboarding/README.md`](../../onboarding/README.md) — the entry-point doc this sprint will update.
- [`docs/onboarding/mdemg-onboarding-deck.md`](../../onboarding/mdemg-onboarding-deck.md) — the deck itself; `#end-user-path` stub slide lives here, this sprint grows U1-U7 content around it.
- [`CLAUDE.md`](../../../CLAUDE.md) — governance pins consulted: `sequential-epics`, `mandatory-feature-docs`, `must-follow-12-section-format`, `trust-signal-must-be-persisted-never-ignore-honest`, `live-testing-tier-required`.

---

## 12. Rollback Procedures

Pure content sprint — rollback is simple:

1. **Pre-E4 (content lives in draft file only)**: delete `docs/development/onboarding-tutorial-enduser-001/draft_slides.md`; no mutation to `mdemg-onboarding-deck.md` yet.
2. **Post-E4 but pre-ship**: `git revert` the stitch commit; `make sync-tutorial-embed`; `make verify-tutorial-embed` → clean. Stub slide restored.
3. **Post-E6 if operator wants to pull it back later** (unlikely): same `git revert` path on the stitch commit + the closeout commit; feature doc + CLAUDE.md pin reverted; sprint post marked rolled-back with reason.
4. **Zero substrate mutation anywhere in the sprint** — no DB schema, no new routes, no new metrics, no config field adds, no env vars. The ONLY production surface this sprint touches is a markdown file and its //go:embed snapshot.

---

**End of plan.** Awaiting operator sign-off on content contract (E1 gate) before authoring begins.
