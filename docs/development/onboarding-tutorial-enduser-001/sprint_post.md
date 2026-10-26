# ONBOARDING-TUTORIAL-ENDUSER-001 · Sprint Post

**Shipped**: 2026-10-26
**Branch**: `reh3376_dev01` → `main` via PR #704
**Format**: content extension to `docs/onboarding/mdemg-onboarding-deck.md` (reveal.js scroll view)
**Load-bearing differentiator**: operator-driven live-review hard gate (E5) distinguished this sprint from every sibling onboarding sprint, the agent isn't allowed to self-certify operator-facing content; the operator walks every slide against a running MDEMG instance before merge.

---

## 1 What Shipped

### Epic 1 · Content contract + global roadmap (shipped 2026-10-26)

- `sprint_plan.md`, 6-epic plan (E1 contract → E2 per-section roadmaps → E3 draft slides → E4 stitch into main deck → E5 operator-driven live-page-by-page review HARD GATE → E6 docs closeout)
- `presentation_spec.md`, operator-dense content contract (audience: operators of running MDEMG instances, NOT developers; posture: `how do I` not `why does`; depth: NO caps on slides/figures/tables/diagrams; opening framing: 4 strategic questions + integrator-view diagram at U1.6)
- `global_roadmap.md`, U1-U17 breakdown totaling 122 slides + 8 Mermaid diagrams

**Operator-reframing captured through 4 scope iterations** (v1 → v4):
- v1 → 7 U-sections / 34 slides / ~27 min
- v2 → topic-per-section U1-U17 / ~58 slides / added U15 APIs+gRPC
- v3 → "extensive and highly informative" / ~102 slides / ~85 min
- v4 → NO CAPS on slides/figures/tables/diagrams
- Final: 122 slides / 8 Mermaid diagrams / ~85 min
- End-user path is COMPLETE for operators by design, does NOT send to the dev path for any topic.

### Epic 2 · Per-section roadmaps (shipped 2026-10-26)

Authored one roadmap file per U-section at `roadmaps/U<N>-<title>.md`, 17 files total.

### Epic 3 · Draft slides (shipped 2026-10-26)

- `draft_slides.md`, 122 slides × ~20 lines each = 2431 lines; slide-by-slide draft content for every U-section before splicing into the main deck.

### Epic 4 · Stitch into main deck + resync embed (shipped 2026-10-26)

- Python helper spliced `draft_slides.md` content into `docs/onboarding/mdemg-onboarding-deck.md` replacing the `#end-user-path` stub shipped by the SERVE sprint
- Added "End User Path" TOC row after F1-F10 row
- `make sync-tutorial-embed` + `make verify-tutorial-embed` both clean
- Combined deck went from ~109 slides to **231 slides** total
- Staged embed resync + deck source edit landed in the same commit (per CLAUDE.md deck-source-edits-and-staged-embed-resync-land-in-the-same-commit rule)

### Epic 5 · Operator live review (THE HARD GATE)

- Created `review_notes.md` harness with 5-line sign-off block
- Operator walked the deck in a browser at `http://localhost:9999/ui/tutorial/#end-user-path`
- Six iterative revision cycles against operator feedback + agent self-check via claude-in-chrome:
  - **Rev 1** (`c5761470`), CSS `overflow-y: auto` attempt (INEFFECTIVE, reveal.js `.reveal-viewport` forces `overflow: hidden`)
  - **Rev 2** (`769e582e`), Enabled reveal.js 5.x scroll-view via frontmatter (`view: scroll`, `scrollProgress: auto`). Entire deck became one scrollable document.
  - **Rev 3** (`929e521f`), Converted 18 relative `.md` links to absolute GitHub URLs (operator reported garbled text on relative links; root-caused to a server HTTP gzip middleware bug on 404s, filed as separate-sprint follow-up)
  - **Rev 4** (`a16692b8`), Added `<!-- .slide: id="..." -->` directives before all F1-F10 + U2-U17 section headings so TOC fragment links scroll correctly (reveal.js markdown plugin doesn't auto-generate heading anchors in scroll view)
  - **Rev 5** (`22c960fb`), Fixed duplicate `thank-you` ID (dev F10 + U17 both had auto-generated `thank-you`); renamed U17 heading to "Thank You, End User Path" + explicit `id="u17-thank-you"`
  - **Rev 6** (`438e069b`), Agent-run claude-in-chrome plumbing sweep caught: Mermaid diagrams rendering as RAW CODE instead of SVG (patched `scripts/tutorial/export.mjs` to post-process `index.html` replacing reveal-md's `.slides > .present:not(.stack)` selector with `.slides` for scroll-view compatibility, all 23 Mermaid diagrams now render as SVG); 3 unreadable Mermaid diagrams fixed via LR→TD + shortened labels (U1.6 integrator, U4.2 Jiminy loop, U16.5 Override flow); trajectory-language audit found 9 `by design` occurrences, all 9 legitimate.

### Epic 6 · Documentation closeout (this epic)

- `docs/features/tutorial-serve.md`, added End-User Path section + updated Landing fork + Related to reference this sprint as shipped
- `docs/onboarding/README.md`, restructured Section Index into Dev (F1-F10) + End-User (U1-U17) tables; updated front matter, directory listing, Related artifacts, What's Open
- CLAUDE.md, new Architecture Notes pin at top of the list
- `docs/development/onboarding-tutorial-enduser-001/sprint_post.md`, this doc

---

## 2 Architecture Rules Pinned

1. **Agent must verify UI PLUMBING before operator reviews content/UX**: load claude-in-chrome, click every link class, verify scroll/nav/rendering, confirm no 404s, screenshot waypoints. The operator's time is content + UX decisions only; plumbing failures (dead links, missing anchors, 404s, raw-code renders) are mechanically verifiable and the agent must have caught them first. Memory rule `feedback_agent_tests_ui_before_operator.md` + MEMORY.md index. (Operator correction captured mid-sprint 2026-10-26.)

2. **Reveal.js scroll view requires explicit slide IDs**: in reveal.js 5.x `view: scroll`, the markdown plugin does NOT auto-generate heading anchors. Any TOC that uses fragment links to section headers MUST declare `<!-- .slide: id="..." -->` before the heading. 28 unique IDs across the combined deck; a duplicate ID (U17 vs dev F10 "Thank You") causes the browser to always scroll to the first match. Rename headings when disambiguation is needed; always add an explicit ID directive at creation time, not retroactively.

3. **Reveal-md's default Mermaid init is slide-centric and breaks under scroll view**: reveal-md generates `mermaid.init(mermaidOptions, '.slides > .present:not(.stack) pre code.mermaid')`, which only renders diagrams on the currently-presented slide. Scroll view has no `.present` slide, so every Mermaid block renders as raw code. The fix is post-processing `index.html` in `scripts/tutorial/export.mjs` to replace the selector with `.slides pre code.mermaid`. Future scroll-view-targeted deck builds MUST apply this patch.

4. **Mermaid diagrams with wide natural viewBox (>2x rendered width) become unreadable**: a `graph LR` with long labels can produce a natural viewBox 2412-3828px wide; rendered at ~1708px with proportional scaling, text crushes below readable size. The fix is `graph LR → graph TD` + label shortening. Checked at author time via SVG viewBox vs rendered-container width.

5. **Operator-facing serving content MUST use absolute URLs for cross-repo references**: relative paths (`./README.md`, `../features/foo.md`) are fine in a markdown editor but break under reveal-md serving because (a) they resolve under `/ui/tutorial/_assets/...` not the markdown's authoring directory, and (b) a 404 on the serving stack hits the mdemg HTTP gzip middleware bug (compresses 404 bodies without setting `Content-Encoding: gzip`). The deck sidesteps both by using absolute GitHub URLs for every cross-repo link.

---

## 3 What Was Deferred (Non-Blocking)

- **Dev-path F1.4 "History, Build Trajectory" Mermaid**: too wide, text unreadable; needs LR→TD conversion. Belongs to `MDEMG-ONBOARDING-DECK-002`.
- **Dev-path F3.1 "RRF, How It Works" Mermaid**: too wide, text unreadable; needs LR→TD conversion. Same sibling sprint.
- **Dev-path F1 alignment with 4-question opening framing**: apply the end-user path's U1 pattern (4 strategic questions + integrator diagram) to F1 so both paths open the same way. Same sibling sprint.
- **Dev-path topology section** detailing the 5-container stack with cross-refs to F2/F3/F7. Same sibling sprint.
- **Server HTTP gzip middleware bug**: compresses 404 bodies without `Content-Encoding: gzip` header. Reproducer: `curl -H 'Accept-Encoding: gzip' http://localhost:9999/ui/tutorial/foo.md`. Browser renders raw gzip bytes. Its own sprint.
- **LinkedIn share of the combined deck**: operator deferred until surface + audience framing decided.
- **Epic 6 operator-driven 115-min stopwatch rehearsal from MDEMG-ONBOARDING-DECK-001**: still pending; this sprint didn't block on it.
- **Quarterly-refresh script automation**: opt-in follow-up, not sprint-gating.

---

## 4 Verification

- `make verify-tutorial-embed`, clean (no drift between deck source + staged embed)
- `make sync-grafana-embed` + `make verify-grafana-embed`, not touched this sprint (no Grafana changes)
- Route inventory unchanged (`scripts/verify_route_consumers.py`, no new routes)
- Pre-commit inventory gate, clean
- `golangci-lint run ./...`, not touched (no Go changes this sprint)
- `/ui/tutorial/#end-user-path`, browser-verified end-to-end via claude-in-chrome + operator review
- All 23 Mermaid diagrams render as SVG (not raw code)
- 28 section IDs resolve correctly from TOC
- 18 external GitHub links resolve to correct paths
- 9 `by design` occurrences audited, all legitimate (teaching the rule / stable features / quoting panel conventions)

---

## 5 Key File Paths

- Deck source: `docs/onboarding/mdemg-onboarding-deck.md` (231 slides combined)
- CSS override: `docs/onboarding/slides-overrides.css`
- Entry page: `docs/onboarding/README.md`
- Sprint directory: `docs/development/onboarding-tutorial-enduser-001/`
- Build tooling: `scripts/tutorial/{package.json,package-lock.json,export.mjs}`
- Embed target: `internal/cli/tutorial_templates/staged/`
- Server handler: `internal/api/handlers_tutorial.go`
- Feature doc: `docs/features/tutorial-serve.md`
- Memory rule: `/Users/reh3376/.claude/projects/-Users-reh3376-mdemg/memory/feedback_agent_tests_ui_before_operator.md`

---

## 6 PR Reference

[PR #704](https://github.com/reh3376/mdemg/pull/704), merged 2026-10-26 as commit `2ef05bb2`. Full E1-E5 package including the 6 live-review revision commits.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
