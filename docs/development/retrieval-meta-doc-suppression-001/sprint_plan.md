# RETRIEVAL-META-DOC-SUPPRESSION-001 — Sprint Plan

**Task**: #143
**Origin**: MDEMG-DOCS-INGEST-001 ⚠️ MIXED verdict (task #142 verdict.md) + operator ratification 2026-08-24 (Option 1 — targeted per-path score suppression).

## 1. Header & Metadata

- **Sprint**: RETRIEVAL-META-DOC-SUPPRESSION-001
- **Category**: dev / retrieval intervention
- **Est. duration**: 1-2 days
- **Date**: 2026-08-24
- **Author**: Claude (opus 4.7) + operator `reh3376`

## 2. Problem Statement

MDEMG-DOCS-INGEST-001 (task #142) achieved 100% substrate coverage (1,221 nodes ingested, 0/10 not-found) but only **6/10 (60%) top-3** on MDEMG-usage probe queries because 3 specific hub nodes systematically over-score on nearly every MDEMG query:

| Node | Path | Score on "mdemg data export" | edges in | edges out |
|---|---|---|---|---|
| `n_389d04632e5bf44075dc` | `/.goreleaser.yaml` | 0.100 (rank #1) | 21 | 34 |
| `n_7f4d972379d1b1c1b089` | `/CHANGELOG.md` | 0.400 (rank #2) | 104 | 531 |
| `n_fc2686a88657906c6277` | `/CLAUDE.md` | 0.800 (rank #3) | 59 | 367 |

**Mechanism** (empirically verified this session): NOT `activation_confidence` (all 3 have `act_conf=0.5` default). Driver is **BM25 + short whole-file content containing "MDEMG"** repeatedly — every MDEMG-usage query matches these lexically at short-document-normalization boost.

**Archive is unsafe** — these are heavy graph hubs (531+ outgoing edges each) anchoring L1 Hidden clusters. Archiving would orphan cluster memberships + disrupt many downstream traversals.

**Jiminy rerank is not the fix** — A/B test this session showed jiminy_enabled=true made things WORSE (top-3 6→4, 3 new not-found; LLM rerank is nondeterministic).

## 3. Scope & Constraints

**In scope**:
- New config fields `RetrievalSuppressPaths []string` + `RetrievalSuppressFactor float64` in `internal/config/config.go` (env: `RETRIEVAL_SUPPRESS_PATHS` comma-sep, `RETRIEVAL_SUPPRESS_FACTOR` default 0.3)
- New pure helper `SuppressCandidatesByPath(cands, paths, factor)` in `internal/retrieval/suppress.go`
- Hook in `Service.Retrieve` right after RRF fusion (post-fusion pre-seed-extraction pre-rerank)
- 5 Tier 1 pin tests (empty list no-op, matching → multiplied + re-sorted, non-matching untouched, factor=0 → dropped-to-zero score, factor=1 → no-op)
- Live re-run of task #142's 10 probes with the 3 offending paths in the suppress list; verify ≥8/10 top-3

**Out of scope** (each disclosed in §11):
- Automatic detection of "meta-doc" patterns (operator-specified paths only for now)
- Reranker-side blacklist (jiminy layer)
- Any UVTS A/B (this is opt-in default-off; ⚠️ verdict metric IS the verification)
- Any archive or corpus surgery
- Broader retrieval-scoring rework (RRF weight tuning, etc.)

**Constraints**:
- Default OFF (empty path list); operator opts in via env
- Applied at exact-path match (no regex) — most auditable, least surprising
- Non-destructive to substrate — no graph mutation
- Reversible via env unset (no restart-safe issue; config re-reads on server restart)
- Idempotent — running suppression twice on same cands is a no-op after re-sort
- No hardcoded values per `never-hardcode-config`

## 4. Dependencies

- **Task #142 shipped** — provides the substrate content + the 10-probe verification harness ✅
- **`/v1/memory/retrieve` shipped** — the hook point exists in `Service.Retrieve` post-fusion ✅
- **mdemg server running** — for live smoke ✅

## 5. Implementation Plan (sequential epics + gates)

### Epic 1 — Config fields + wiring
- Add 2 fields to `Config` struct with comment linking to this sprint
- Wire in `FromEnv`: parse `RETRIEVAL_SUPPRESS_PATHS` (comma-sep), `RETRIEVAL_SUPPRESS_FACTOR` (default 0.3)
- **Gate**: `go build` clean; `TestConfigLoad_SuppressPathsAndFactor` green (parse env vars + defaults)

### Epic 2 — Helper `internal/retrieval/suppress.go`
- Pure function `SuppressCandidatesByPath(cands []Candidate, suppressPaths []string, factor float64) []Candidate`
- Convert `suppressPaths` to a `map[string]struct{}` for O(1) lookup
- For each cand, if `Path` is in the map, multiply `RRFScore` by factor
- Re-sort by RRFScore descending (stable sort)
- Return modified slice (in-place OK for perf)
- **Gate**: 5 Tier 1 pin tests green

### Epic 3 — Hook in `Service.Retrieve`
- Add call right after fusion (line ~655) + before the empty-check
- Guarded by `if len(s.cfg.RetrievalSuppressPaths) > 0`
- Also plumb `RetrievalSuppressPaths`/`Factor` into `scorerVersion()` (cache key namespace) so opt-in flip flushes cache
- **Gate**: `go build` clean, `golangci-lint` clean

### Epic 4 — Live smoke + verdict
- Set `RETRIEVAL_SUPPRESS_PATHS=/.goreleaser.yaml,/CHANGELOG.md,/CLAUDE.md` in `.env` (or launchctl setenv) + kickstart mdemg server
- Re-run task #142's 10 probes via `/tmp/mdemg_probe.py` (already exists)
- Compare top-3 count: baseline 6/10 → target ≥8/10

**Verdict rubric** (evidence-decided):
- ✅ ≥8/10 top-3 AND 0 regressions on 4 previously-passing probes → ship as default `.env` update
- ⚠️ 7-8/10 → analyze which probes still fail; may need broader suppress list OR factor tuning
- ❌ ≤6/10 → suppression not the mechanism; investigate further (fall to Alt 2 broader BM25 weight tuning)

### Epic 5 — Sprint post + CHANGELOG + task update
- verdict.md, sprint_post.md with arch rules pinned
- feature doc `docs/features/retrieval-suppress-paths.md` per `mandatory-feature-docs`
- CHANGELOG entry
- Task #143 completed; task #144 status confirmed unblocked

## 6. Testing Plan (3 tiers)

### Tier 1 — Unit tests (5 pins)
- `TestSuppressCandidatesByPath_EmptyList` — no-op
- `TestSuppressCandidatesByPath_MatchingPathsMultiplied` — factor applied; other cands untouched
- `TestSuppressCandidatesByPath_ResortsCorrectly` — post-mult sort by RRFScore desc
- `TestSuppressCandidatesByPath_ZeroFactor` — matching cands drop to score 0 but still present
- `TestSuppressCandidatesByPath_IdempotentReRun` — running twice with same inputs = same output

### Tier 2 — Integration
- Config parse: `TestConfigLoad_SuppressPathsAndFactor` (env → Config.RetrievalSuppressPaths correct list + factor)
- Cache namespace: verify `scorerVersion()` includes the suppress config → cache flushes on flag flip

### Tier 3 — Live e2e
- Re-run task #142's 10 probes with suppress enabled
- Baseline: 6/10 top-3 (from task #142 verdict.md)
- Target: ≥8/10 top-3

## 7. Commit Strategy

Single commit: config + helper + tests + hook + verdict + sprint post + CHANGELOG. All in one PR since the intervention is small + tightly-coupled.

## 8. Verification Checklist

- [ ] Config fields added + wired in FromEnv
- [ ] Helper written + 5 pin tests green
- [ ] Hook added in Service.Retrieve + build/lint clean
- [ ] `scorerVersion()` includes suppress config
- [ ] Live smoke: re-run 10 probes; capture ranks before/after
- [ ] Verdict = one of {✅/⚠️/❌}
- [ ] Sprint post + feature doc + CHANGELOG
- [ ] Task #143 completed

## 9. Documentation Update

- Epic 5's feature doc `docs/features/retrieval-suppress-paths.md` per `mandatory-feature-docs`
- Sprint post with 2-3 arch rules candidate for CLAUDE.md

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Suppression tanks other query types (retrieval.query_classify, jiminy.evaluate) | MEDIUM | HIGH | Default OFF; opt-in via env; the 3 suppressed paths are semantically low-value for ANY task query (they're not concept content, they're project meta-files) |
| Meta-docs still surface via graph traversal from their many out-edges | LOW-MEDIUM | LOW | Score suppression cascades — lower-scored seeds don't get selected, so their expansion frontier shrinks; if problem persists, follow-up sprint can extend suppression to seed-eligibility gate |
| Operator adds a valid-content path to the suppress list by mistake | LOW | MEDIUM | Feature doc documents each of the 3 initial paths + why they're safe to suppress; add a `--dry-run` mode to CLI (out of scope but noted) |
| Cache-key inclusion missed → operator flips config but sees stale results | MEDIUM | LOW | Explicit `scorerVersion()` inclusion pinned by test |
| Factor=0 drops score to 0 = effectively-archive semantics | LOW | LOW | Documented; operator's choice; factor=0.1 is more conservative for "downweight but keep discoverable" |

## 11. Non-Goals (explicit)

- **Automatic meta-doc detection** — heuristic like "if a node has >100 outgoing edges + short content" could work but adds complexity; operator-specified list is auditable
- **Reranker-side blacklist** — jiminy layer changes affect the strict-mode enforcement + guidance surfacing; higher blast radius
- **Suppression at ingest time** — retrieval-side is reversible; ingest-side changes require re-ingest
- **UVTS A/B** — this is a narrow ⚠️-branch intervention with a clear verdict rubric; UVTS would be over-engineering
- **Regex path matching** — surprising; exact-match is safer

## 12. Documents Accessed

- `internal/retrieval/service.go:450-1220` (Service.Retrieve hook point at line 655)
- `internal/retrieval/service.go:type Candidate` (Path + RRFScore fields available)
- `internal/config/config.go:781-795` (RetrievalDiversityEnabled et al. — wiring pattern reference)
- `docs/development/mdemg-docs-ingest-001/verdict.md` (this sprint's forcing function; probe evidence)
- `docs/development/mdemg-docs-ingest-001/live_verify_report.md` (10-probe baseline)
- `/tmp/mdemg_probe.py` (probe harness — will re-use for live smoke)
- Live Neo4j queries via `docker exec cypher-shell` (edge counts + node characterization)
- Live `curl /v1/memory/retrieve` runs to identify the 3 offending node_ids
- Deep-dive workflow `wf_b389463a-61b` A2 investigation (predicted this class)
- CLAUDE.md pins: HEBB-ETA-001 (why activation_confidence is default 0.5), RRF-SCALE-001 (fused score contract)
- Operator ratification 2026-08-24: Option 1 (targeted per-path suppression, code sprint)
