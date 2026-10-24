# ONBOARDING-TUTORIAL-SERVE-001 — Sprint Plan

**Date:** 2026-10-24 | **Branch:** `reh3376_dev01`
**Parent trigger:** operator directive (2026-10-24) — convert the shipped
MDEMG-ONBOARDING-DECK-001 deliverable from an Obsidian-only artifact into
a built-in product feature reachable from CLI, Grafana, and `/ui/`.
**Sibling sprint (sequential):** `ONBOARDING-TUTORIAL-ENDUSER-001`
authors the end-user path content after this sprint ships infrastructure.

## 1. Header & Metadata

Sprint-plan v1.0 format. Dev branch commits only (`reh3376_dev01`);
auto-PR to main. Ships infrastructure + landing fork + the "Build MDEMG"
branch (reusing the shipped 107-slide deck). The "Use MDEMG" branch
ships as a single stub slide pointing at `docs/user/`; its real content
lands in the sibling sprint. Total scope ~1 day.

## 2. Problem Statement

The shipped onboarding deck requires:
- Obsidian installed + the correct Advanced Slides fork
- A free TCP port for the plugin's internal preview server (just hit a
  port-3000 collision with Grafana live this morning)
- The user to be the project maintainer OR have a copy of this repo

For an operator who just ran `brew install mdemg` or `docker compose up
-d` and wants to understand what they've installed, none of those
preconditions hold. The onboarding deck exists but is unreachable from
the operator's actual environment.

Operator directive: `mdemg tutorial` from the CLI, or a button on the
Grafana overview dashboard, should open a browser to the full
slideshow. Any machine with a browser. Zero Obsidian dependency.

Operator also wants a user-choice fork at the landing slide:
- **Build MDEMG** → the current 107-slide dev-contributor deck
- **Use MDEMG** → an end-user walkthrough (built in Sprint 2)

## 3. Scope & Constraints

**In scope (6 sequential epics):**

- E1 — Build-time HTML export pipeline: `scripts/export_tutorial.sh`
  invokes `reveal-md` headlessly against the deck markdown; output
  normalized + staged under `internal/cli/tutorial_templates/staged/`.
  `make sync-tutorial-embed` wraps the invocation; `make
  verify-tutorial-embed` is the CI drift gate. Mirrors GRAFANA-SHIP
  pattern (RELEASE-HYGIENE-001).
- E2 — Server handler: `internal/api/handlers_tutorial.go` serves the
  embedded tree from `/ui/tutorial/`. Materializes to a temp dir on
  first request (reveal.js assets want real file paths). New config
  knob `TUTORIAL_ENABLED` (default true). Boot log reports status.
- E3 — Landing fork slide: edit
  `docs/onboarding/mdemg-onboarding-deck.md` to prepend a landing slide
  with two anchor-linked buttons (`[Build MDEMG]` → current slide 1;
  `[Use MDEMG]` → stub slide at the end). Light intro reframe only
  where needed; existing content stays otherwise unchanged.
- E4 — End-user stub slide: single reveal.js slide at the end of the
  deck reading "Coming soon — see `docs/user/` for now, or wait for
  the end-user tutorial sprint." Replaced by Sprint 2 content.
- E5 — Entry points:
  - CLI: `mdemg tutorial` (new leaf cmd in `internal/cli/`) opens
    default browser to `http://localhost:<MDEMG_PORT>/ui/tutorial/`
    via `exec.Command("open" | "xdg-open" | "cmd")`. Falls back to
    printing the URL if browser-open fails (headless servers).
  - Grafana: new text panel at top of `deploy/docker/grafana/dashboards/
    mdemg-overview.json` with a Markdown link to `/ui/tutorial/`. UID
    matched to sibling panels per ARC-TRAJECTORY-PANEL-001 arch rule.
    Staged mirror synced via `make sync-grafana-embed`.
- E6 — Docs + sprint post:
  - `docs/features/onboarding-tutorial.md` (why / choices / how it
    works / how to use)
  - CLAUDE.md note
  - `docs/development/onboarding-tutorial-serve-001/sprint_post.md`

**Out of scope:**

- End-user path content (sibling sprint
  `ONBOARDING-TUTORIAL-ENDUSER-001`)
- PDF export (deck stays in-browser; PDF export can land in the
  LinkedIn-share sprint after both tutorial sprints ship)
- Audience reframing of the existing 107-slide deck beyond the landing
  slide (deck targets senior engineers; the user-choice fork handles
  framing by letting the end-user path peel off before the dense
  content starts)
- Any Advanced Slides-specific features that don't translate to raw
  reveal.js (e.g., Obsidian-only comments). Deck audit in E1 covers
  this.
- Compiling node/reveal-md into the mdemg runtime (dev-tool dep only;
  only needed to regenerate the embed, not at operator runtime)

## 4. Dependencies

- Shipped `docs/onboarding/mdemg-onboarding-deck.md` (107 slides, 15
  Mermaid diagrams; MDEMG-ONBOARDING-DECK-001)
- Shipped `/ui/` handler + embed pattern (`internal/api/ui/` is the
  template)
- Shipped GRAFANA-SHIP embed + sync + verify pattern
  (RELEASE-HYGIENE-001 — same shape, directly reusable)
- Shipped CLI leaf-command pattern (`internal/cli/tutorial.go` modeled
  on `internal/cli/model.go` or similar operator-facing noun+verb)
- Node ≥ 18 + `reveal-md` ≥ 6 as a build-time dev dep (documented in
  the development setup instructions, not installed at runtime)

## 5. Implementation Plan (6 sequential epics + gates)

**E1 — Build pipeline**

- Audit deck markdown for Advanced-Slides-specific syntax that
  reveal-md doesn't understand (e.g., `<!-- .slide: ... -->` HTML
  comments are reveal.js-standard and work; Advanced Slides' custom
  frontmatter beyond basic reveal.js keys may need stripping). Produce
  a markdown normalizer if needed (`scripts/normalize_deck_for_revealmd.py`
  — pure stdlib Python, read in, transform, write out, CI-safe).
- Add `scripts/export_tutorial.sh`:
  1. Normalize `docs/onboarding/mdemg-onboarding-deck.md`
  2. Run `reveal-md --static <output-dir> --css docs/onboarding/slides-overrides.css`
  3. Copy output to `internal/cli/tutorial_templates/staged/`
- Add `internal/cli/tutorial_templates/embed.go` with `//go:embed
  all:staged/*` plus a `Materialize(destDir)` helper matching
  `grafana_templates`'s shape (atomic write, sha256-compare, operator
  edits preserved).
- Add `make sync-tutorial-embed` + `make verify-tutorial-embed` targets.
- Add `scripts/verify_tutorial_embed.py` (checks source SHA → staged
  SHA pair; CI fails merge if the source changed without the sync
  step).
- **Gate**: `make verify-tutorial-embed` passes; staged dir contains
  `index.html` + reveal.js assets; `index.html` references the embedded
  CSS; opening `staged/index.html` in a browser renders the full deck.

**E2 — Server handler**

- Add `internal/api/handlers_tutorial.go`:
  - Lazy materialize-on-first-request to a temp dir (bounded; cleaned
    on server shutdown)
  - `GET /ui/tutorial/` → serves `index.html`
  - `GET /ui/tutorial/*` → serves asset by path (strip-prefix +
    `http.ServeFile`)
  - 404 for paths escaping the materialized tree (path-safety via
    `pathsafe.SafeJoinUnderDir` per SEC-TRANCHE-2)
  - 503 when `TUTORIAL_ENABLED=false`
- Wire into `internal/api/server.go` route table.
- Add `TUTORIAL_ENABLED` to `internal/config/config.go` (default true).
- Boot log: `slog.Info("tutorial handler", "enabled=...", "url=/ui/tutorial/")`.
- **Gate**: `curl -I http://localhost:9999/ui/tutorial/` returns 200;
  `curl` of any asset path returns the embedded bytes; a path traversal
  attempt returns 404.

**E3 — Landing fork slide**

- Edit `docs/onboarding/mdemg-onboarding-deck.md`:
  - Prepend new slide (before current slide 1) with the title "MDEMG
    Tutorial" and two large HTML-styled anchor buttons:
    - `[→ Build MDEMG]` with `href="#/1"` (first slide of the dev deck)
    - `[→ Use MDEMG]` with `href="#/<N>"` (the stub slide at the end,
      N computed at export time; or named-anchor via reveal.js slide
      ID)
  - Prefer reveal.js slide IDs (`<!-- .slide: id="build-start" -->`
    and `<!-- .slide: id="use-start" -->`) over numeric anchors —
    renumber-safe
- Verify Advanced Slides preview still renders the new landing slide
  cleanly in Obsidian (does not break the maintainer's authoring flow).
- **Gate**: landing slide renders in both Obsidian preview AND the
  exported HTML (opened in a browser); both buttons navigate to the
  correct anchors.

**E4 — End-user stub slide**

- Append single reveal.js slide to the deck with slide-id `use-start`:
  - Heading: "End-User Tutorial — Coming Soon"
  - Body: short note explaining this is a stub; link to
    `docs/user/` and mention Sprint 2's name
  - Footer: `[← Back to Landing]` button linking to `#/0`
- **Gate**: stub slide renders; `use-start` anchor resolves from the
  landing-slide button.

**E5 — Entry points (CLI + Grafana)**

- Add `internal/cli/tutorial.go`:
  - Noun+verb pattern: `mdemg tutorial [--port N] [--no-open]`
  - Resolves URL: `http://127.0.0.1:<MDEMG_PORT>/ui/tutorial/`
    (MDEMG_PORT env var → default 9999)
  - Pre-flight: `GET /healthz`; if unreachable, print "MDEMG server not
    running — start it first with `mdemg start` or `docker compose up
    -d`" and exit 1
  - If reachable: platform-dispatched browser open (`darwin: open`,
    `linux: xdg-open`, `windows: cmd /c start`), falling back to
    printing the URL on any error OR when `--no-open` is passed
- Wire via `internal/cli/root.go` leaf registration.
- Add Grafana text panel at top of
  `deploy/docker/grafana/dashboards/mdemg-overview.json`:
  - Panel type `text`, mode `markdown`
  - Content: `## 📖 [Open MDEMG Tutorial](http://localhost:9999/ui/tutorial/)`
    + 1-line descriptor
  - Panel UID matches the sibling-panel pattern (literal, not env-var
    reference)
- Run `make sync-grafana-embed`.
- **Gate**: `mdemg tutorial` opens the browser (or prints URL in
  `--no-open` mode); Grafana overview dashboard shows the panel; the
  link resolves to the tutorial.

**E6 — Docs**

- `docs/features/onboarding-tutorial.md` — why / choices / how it
  works / how to use. Document the two entry points. Document the
  Build-pipeline + sync + verify contract. Document CSS override
  travel. Mention the sibling sprint.
- CLAUDE.md: add an Architecture Note entry naming the sprint, the
  two entry points, the config knob, the build-pipeline arch rule,
  and the sibling sprint pointer.
- `sprint_post.md`: metrics (slide count, binary size delta, latency
  of first `/ui/tutorial/` request), what shipped, what's deferred.

## 6. Testing Plan (3 tiers)

**Tier 1 — Unit**
- `tutorial_templates/embed_test.go` — asserts `index.html` present
  in embed, `Materialize` writes files matching their SHA
- `handlers_tutorial_test.go` — asserts 200 on `/ui/tutorial/`, 404 on
  path traversal, 503 when `TUTORIAL_ENABLED=false`
- `tutorial_test.go` (CLI) — asserts URL build from `MDEMG_PORT` env
  var, exits 1 on unreachable server, `--no-open` prints URL
- Pin tests: `TestExportTutorial_StagedDirShape` (staged dir has
  `index.html` + at least one asset), `TestMaterialize_IdempotentOnEditPreserved`
  (operator-edited file not clobbered)

**Tier 2 — Integration**
- `make sync-tutorial-embed` → `make verify-tutorial-embed` round-trip
  on a clean checkout
- Start mdemg (`./bin/mdemg start`), hit `/ui/tutorial/` via Go test
  HTTP client, confirm first few bytes are reveal.js index HTML
- Grafana provisioning reload picks up the new panel

**Tier 3 — Live e2e** (required per CLAUDE.md's live-testing rule)
- Build the binary (`go build -o bin/mdemg ./cmd/mdemg`)
- Start it against the live docker stack
- Browser: hit `http://localhost:9999/ui/tutorial/` manually, confirm
  landing slide renders, both fork buttons work, dev path walks all
  107 slides, stub slide renders, back-to-landing button works
- CLI: `mdemg tutorial` opens the browser to the right URL
- Grafana: overview dashboard shows the new button panel; clicking the
  link opens the tutorial in a new tab
- Verify the previously-fixed "What MDEMG Is Trying To Do" slide
  renders without bottom clipping (slides-overrides.css travels
  through the export)

## 7. Commit Strategy

Six commits mirroring the six epics (one per gate pass), ending in
PR to main via auto-PR. Each commit message ends with the attribution
line from the session reminder.

Commit cadence:
- `feat(tutorial): build-time export pipeline + go:embed scaffold (E1)`
- `feat(tutorial): /ui/tutorial/ server handler (E2)`
- `feat(tutorial): landing fork slide for build vs use paths (E3)`
- `feat(tutorial): end-user path stub slide (E4)`
- `feat(tutorial): mdemg tutorial CLI + Grafana entry buttons (E5)`
- `docs(tutorial): feature doc + sprint post + CLAUDE.md pin (E6)`

## 8. Verification Checklist

- [ ] `make sync-tutorial-embed` + `make verify-tutorial-embed` round-trip clean
- [ ] All unit tests pass (`go test ./internal/cli/tutorial_templates/... ./internal/api/handlers_tutorial... ./internal/cli/tutorial_test.go`)
- [ ] `golangci-lint run ./...` clean
- [ ] Live Tier-3 drill: landing slide renders, both forks navigate correctly, dev deck walks all 107 slides without clipping, CLI opens browser, Grafana button appears
- [ ] `docs/features/onboarding-tutorial.md` shipped
- [ ] CLAUDE.md pin shipped
- [ ] Sprint post shipped
- [ ] Dependabot alerts not increased

## 9. Documentation Update (Epic 6)

- `docs/features/onboarding-tutorial.md` — NEW
- CLAUDE.md — Architecture Note entry
- `docs/onboarding/README.md` — add 1-line "Also available as `mdemg
  tutorial` or via the Grafana overview dashboard button" cross-ref
- `sprint_post.md` — NEW
- Reference the sibling sprint `ONBOARDING-TUTORIAL-ENDUSER-001` in
  all three

## 10. Risks & Mitigations

**R1 — reveal-md doesn't render Advanced-Slides-specific syntax
cleanly.** Mitigation: E1 audits the deck first; produces a normalizer
if needed. Normalizer is deterministic pure-stdlib Python and CI-safe.
Fallback: hand-edit the problematic Advanced-Slides syntax out of the
deck and keep both renderers compatible going forward.

**R2 — Build-time dep on node + reveal-md adds friction.** Mitigation:
dev-tool only (not operator runtime); documented in dev setup;
`make sync-tutorial-embed` works on any machine with node ≥ 18. CI
job can install reveal-md on-the-fly if needed.

**R3 — Binary size grows ~2 MB.** Mitigation: accepted; current
binary ~85 MB; HTML + reveal.js runtime + Mermaid + CSS adds low-single-digit
MB. Documented in sprint post for transparency.

**R4 — Landing slide HTML buttons may not render identically in
Obsidian preview vs exported reveal.js.** Mitigation: E3 gate verifies
both renderers; if divergent, use plain reveal.js vertical-slide
navigation with text labels as a fallback.

**R5 — Grafana panel UID collision with sibling panels.** Mitigation:
E5 explicitly references the ARC-TRAJECTORY-PANEL-001 arch rule
(literal UID matching sibling panels); verified during E5 gate.

**R6 — Operator runs `mdemg tutorial` without server running.**
Mitigation: pre-flight `/healthz` check with clear next-step message
("start it first with `mdemg start`").

## 11. Rollback

- Infrastructure: `TUTORIAL_ENABLED=false` disables the handler (503
  on any `/ui/tutorial/*` request); `--no-open` disables the CLI's
  browser-launch. Both are config-flip rollbacks, no code revert.
- Full rollback: `git revert` the six commits; the embed strips from
  the binary on next build; Grafana sync removes the panel.
- Risk assessment: additive sprint — zero shipped functionality
  changes; the deck's canonical source stays at the same path.

## 12. Documents Accessed

- `docs/development/mdemg-onboarding-deck-001/sprint_plan.md`
- `docs/development/mdemg-onboarding-deck-001/sprint_post.md`
- `docs/development/release-hygiene-001/sprint_plan.md` (GRAFANA-SHIP
  reference pattern)
- `docs/onboarding/mdemg-onboarding-deck.md` (deck source)
- `docs/onboarding/slides-overrides.css` (CSS override traveling with
  the export)
- `docs/onboarding/README.md`
- `CLAUDE.md` (RELEASE-HYGIENE-001 entry, GRAFANA-SHIP pattern,
  ARC-TRAJECTORY-PANEL-001 arch rule, SEC-TRANCHE-2 pathsafe helper)
