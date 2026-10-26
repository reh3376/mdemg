# ONBOARDING-TUTORIAL-SERVE-001 — Sprint Post

**Status**: SHIPPED
**Shipped**: 2026-10-26
**Branch**: `reh3376_dev01`
**Sprint plan**: [`sprint_plan.md`](./sprint_plan.md)
**Feature doc**: [`docs/features/tutorial-serve.md`](../../features/tutorial-serve.md)

---

## What Shipped

The onboarding deck shipped by MDEMG-ONBOARDING-DECK-001 (108 slides, Obsidian Advanced Slides) is now served by the mdemg API server as pre-rendered reveal.js HTML at `/ui/tutorial/`, with two operator entry points (CLI + Grafana) and a landing fork offering a dev vs end-user path. Zero external tooling required at operator time.

### Epic-by-epic

| Epic | Scope | Commit |
|---|---|---|
| **E1** | Build-time HTML export pipeline (`scripts/tutorial/export.mjs`, reveal-md@6.1.4 + mermaid@11.4.1 pinned) + `//go:embed all:staged` scaffold in `internal/cli/tutorial_templates/` + `make sync-tutorial-embed` / `verify-tutorial-embed` CI drift gates mirroring GRAFANA-SHIP shape | `6ffdc378` |
| **E2** | `/ui/tutorial/` HTTP handler (`internal/api/handlers_tutorial.go`) + `TUTORIAL_ENABLED` config knob (default true) + boot-log status line + 6 handler pin tests covering disabled/enabled/asset/path-traversal/404/containsDotDot | `8e57a47f` |
| **E3** | Landing fork slide (id=landing-fork) inserted between title and TOC; 2-column table forks the viewer dev-path vs end-user-path | `6f8d011a` (bundled with E4) |
| **E4** | End-user stub slide (id=end-user-path) appended after Thank You; explicitly stub for sibling sprint ONBOARDING-TUTORIAL-ENDUSER-001 | `6f8d011a` |
| **E5** | `mdemg tutorial` CLI leaf (`--url` / `--check` flags, OS-dispatched browser launch) + Grafana overview dashboard tutorial link (new tab, idempotent generator) | `7b4d09f6` |
| **E6** | Feature doc (`docs/features/tutorial-serve.md`) + CLAUDE.md Architecture Notes bullet + this sprint post + README cross-ref | (this commit) |

### CI fixes shipped alongside
- Trivy HIGH CVE-2026-112940 on filelock bumped 3.25.2 → 4.2.4 (`fd2e31ae`)
- Neural requires-python bumped 3.11 → 3.14 (`95062292`)
- Obsidian deck CSS path fix (vault-relative) (`cacffc57`)
- Tutorial embed `.gitignore` dist/* negate escape + Trivy skip for scripts/tutorial/ (`70cc2933`)
- Route consumer inventory adjudication for `/ui/tutorial/` (`cec41750`)

All piled onto PR 699 (operator segmentation preference emerged mid-sprint; E3-E5 shipped as separate commits/pushes going forward).

---

## Verification

- `make sync-tutorial-embed` succeeds; 71 files / 7.73 MB
- `make verify-tutorial-embed`: OK — no drift
- `internal/cli/tutorial_templates` + `internal/api` + CLI tutorial tests all PASS
- `golangci-lint` clean on all new files
- Grafana pin tests PASS; overview dashboard renders both links
- Direct URL verification deferred to operator (requires running server; E2 live smoke in PR 699 established baseline)

---

## Architecture Rules Pinned

1. **Build-time dev-tool deps belong under `scripts/<tool>/` and are excluded from runtime security scans** (`TRIVY_SKIP_DIRS`). The reveal-md + mermaid npm tree is a BUILD input, not shipped at runtime; scanning it blocks merges on vulnerabilities operators never touch.

2. **`//go:embed` + staged mirror pattern for canonical files that live outside the package**: when `//go:embed` can't reach the source (cross-package or `..`), create a `staged/` mirror under the package AND a `make sync-X-embed` target AND a `make verify-X-embed` CI drift gate. GRAFANA-SHIP (RELEASE-HYGIENE-001 E2) established; TUTORIAL-SERVE extends.

3. **Every new mux-registered route needs a `docs/api/route_consumer_inventory.json` entry in the same PR** (DORMANT-CENSUS-001 forcing function enforced by `scripts/verify_route_consumers.py`; pre-commit hook catches at commit time).

4. **Operator-facing entry points to a feature MUST exist in >1 surface** — the deck is reachable via CLI (`mdemg tutorial`), via Grafana (dashboard link), via direct URL. An operator who lives in one surface shouldn't need to context-switch to another to reach the feature.

5. **Deck-source edits + staged embed resync land in the same commit**. `//go:embed` is a build-time snapshot; a source-only commit without the resync means the running binary serves stale HTML. The CI drift gate (`make verify-tutorial-embed`) enforces this at merge time; the pattern generalizes to any `//go:embed all:X` with a canonical source outside the package.

---

## Open / Deferred

1. **ONBOARDING-TUTORIAL-ENDUSER-001** (sibling sprint, not yet started) — authors the real end-user path content behind the stub E4 slide. Scope: dashboards walkthrough, Grafana panels operators actually watch, ~30 CLI commands, alert triage, memory inspection, enforcement escape hatches. Target: ~30-40 slides, lighter than the dev deck. Trigger: operator schedules + the first beta tester gives feedback on what they'd want.

2. **Pre-existing CodeQL consolidated failure** on PR 699 — SEC-TRANCHE-3/4 backlog from 2026-08-12, not caused by this sprint. PR 699 was admin-merged after the operator's explicit decision; the CodeQL backlog remains open as a separate security tranche.

3. **Dependabot alert #19** (sentence-transformers CRITICAL) — DISMISSED stale 2026-10-26 (`dismissed_reason: no_bandwidth`). Lock has 6.1.0 > patched 5.6.0; `pyproject.toml` force-pins >=5.6.0. The alert was flagging the lock-reported version shape, not real exposure.

4. **LinkedIn share** — original user intent that spawned this sprint. Deferred until BOTH ONBOARDING-TUTORIAL-SERVE-001 AND ONBOARDING-TUTORIAL-ENDUSER-001 complete + live-tested. Current state: 50% (dev-path infrastructure shipped; end-user content still stub).

---

## Metrics / Observability

No new TSDB tables, no new metrics-registry gauges, no new alert rules. The handler emits a single-line slog at boot (file count + bytes); operators who want traffic observability add stdlib access logging in a follow-up (deliberately out of scope — the tutorial is a static embed, not a hot path).

---

## Rollback

**Fully reversible, no substrate mutation.** Three independent rollback levers:

1. `TUTORIAL_ENABLED=false` in `.env` + restart → handler returns 503 immediately.
2. `git revert` of E1 through E6 commits → embed drops out of the binary; handler returns 503 "embed not built".
3. Delete the Grafana overview link (Dashboard settings → Links → delete "MDEMG Onboarding Tutorial"); CLI command stays but opens an unreachable URL.
