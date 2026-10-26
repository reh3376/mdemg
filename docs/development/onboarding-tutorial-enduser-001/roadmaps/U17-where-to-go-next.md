# U17 — Where To Go Next (4 slides)

**Intent**: Honest exit — documentation surfaces + current HIGH alerts (arcs-in-progress) + how to file issues + return-links.
**Target slides**: 4
**Mermaid**: 0

---

## U17.1 — Documentation Surfaces

**Title**: `U17 · Where To Go — Documentation`

**Body**:
- `docs/features/*.md` — **41+ feature docs** (one per major shipped feature; each covers Why / Choices / How-it-works / How-to-use)
- `docs/user/*` — user-facing operational docs (installation, multi-instance, upgrade paths)
- `docs/development/UXTS_FRAMEWORK_MATRIX.md` — the authoritative UxTS registry (U6)
- `docs/api/*.md` — endpoint reference + inventory
- `CLAUDE.md` — architecture pins + governance (104+ shipped sprint records)
- `README.md` — top-level entry + install + credits
- Beta-tester guide: `packaging/homebrew-mdemg/mdemg_beta_testing.md`

**Provenance**:
- Repo directory structure

**note**:
- Operator cue: "If you remember ONE pointer, remember `CLAUDE.md`. It's the master index into every sprint arc, every arch rule, every invariant."

---

## U17.2 — Honest Open Items

**Title**: `Honest — What's Open Right Now`

**Body**:
- Current HIGH alerts (trajectory language per `trust-signal-must-be-persisted-never-ignore-honest`):
  - **FT Benchmark Not Refreshed** — latest `benchmark_runs` row > 7d old; arc = schedule the next run or operator-triggered via `python -m neural.benchmarks.run_benchmark --apply-tsdb` (U8)
  - **Scheduled-Job Staleness (Neo4j backup)** — no successful Neo4j backup in window; arc = investigate the scheduler
  - **Scheduled-Job Staleness (TSDB backup)** — no successful TSDB backup in window; arc = investigate the scheduler
  - **Health probe failures** — neo4j/tsdb/neural_sidecar probes failing on current session (likely Docker Desktop in low-power/paused state)
- MEDIUM arc-in-progress (expected during JIMINY-CEILING-BREAK-2):
  - Jiminy Follow Rate Drop (classifier/process/hybrid) — current ~13%; target ≥80% via JIMINY-CEILING-BREAK-2 arc
- LOW arcs:
  - Cache Hit Ratio low — honest ~75% vs DH-004 baseline; expected pattern
- These are HONEST and TRACKED; they fire because the system is honest about its state, not because operation is broken

**Provenance**:
- Current session's `/v1/alerts/list` output
- CLAUDE.md "JIMINY-CEILING-BREAK-2 arc" + "FT-BENCH-REFRESH-001" + "NOSILENT-001"

**note**:
- Operator cue: "If you want to help: pick an open HIGH + check its arc + propose the next sprint. The system is honest about where it needs help."

---

## U17.3 — How To Contribute

**Title**: `Contributing Observations + Issues`

**Body**:
- Operator contributions LAND at TWO points:
  1. **Observations** via `mdemg data observe --obs-type correction --content "..."` — becomes an L0 obs, consolidates to L1 constraint/correction, feeds Jiminy (U3-U4)
  2. **Rules authored in /ui/ Rules** — operator-authored durable rules land directly as L1 constraint nodes (JIMINY-RULES-UI-001; `JIMINY_RULES_UI_WRITE_ENABLED=true` to flip)
- Issue filing:
  - GitHub Issues on the mdemg repo (operator-authored)
  - RSIC CapabilityGap filer (FT-RECURSIVE-003) files class-5 issues for the retrain pipeline automatically
  - MCP tool `linear_create_issue` from inside a Claude Code session (MCP-REVIVE-001)
- Beta corpus: beta testers run `mdemg data export` → maintainer runs `mdemg beta-import <bundle>` (BETA-IMPORT-001) → data lands in `space_id=beta-tester-<submission_id>` (never maintainer's own space)

**Provenance**:
- CLAUDE.md "JIMINY-RULES-UI-001" + "BETA-IMPORT-001" + "MCP-REVIVE-001"

**note**:
- Operator cue: "The system expects feedback. Observations are the lightest-weight contribution; rules in /ui/ are the highest-leverage."

---

## U17.4 — Thank You + Return Links

**Title**: `Thank You`

**Body**:
- **MDEMG** — Cognitive Substrate for AI-Assisted Development
- Questions welcome. Everything in this deck traces to CLAUDE.md, feature docs, sprint records, or shipped code.
- `Deck owner`: reh3376
- `Last verified`: (updated at E4 stitch time)
- `Quarterly refresh`: next Q1 (dev-path convention)
- Return-link row:
  - `[⇤ Landing Fork](#landing-fork)` · `[⇤ TOC](#table-of-contents)` · `[README (entry point)](./README.md)` · `[Speaker notes](./mdemg-onboarding-notes.md)` · `[Governance](../features/tutorial-serve.md)`

**Provenance**:
- Mirrors dev-path F10 Thank You slide pattern (shipped by MDEMG-ONBOARDING-DECK-001)

**note**:
- Operator cue: "Pause here for Q&A. This is the longest Q&A anchor in the end-user path."
- End of U17 — return to Landing Fork via the link row, or continue to the dev-path if curious about architecture.
