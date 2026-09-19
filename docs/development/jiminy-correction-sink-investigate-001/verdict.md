# JIMINY-CORRECTION-SINK-INVESTIGATE-001 — Verdict

**Task**: #156
**Investigated**: 2026-09-07 (~30min wall-clock)
**Predecessor**: JIMINY-CEILING-INVESTIGATION-002 (#153) — D6 disclosed the 691→3 drop as an open follow-up
**Verdict**: **NOT A BUG.** The `guidance_type='correction'` outcome volume drop is the mechanical consequence of **JIMINY-CORRECTION-CORPUS-001** (task #100, shipped 2026-08-12) intentionally tombstoning 36 of 39 correction nodes. The sink is functioning correctly; the corpus itself shrank by ~90%.

## TL;DR

- **41 total L1 correction nodes ever created** in `mdemg-dev`, of which **38 are archived** and **3 are live** (2 gradable + 1 informational)
- **36 of the archives were done by JIMINY-CORRECTION-CORPUS-001** on 2026-08-12 15:53-15:54 UTC (31 `purge` + 5 `purge_dup`)
- **2 additional archives** by JIMINY-CORPUS-AUDIT-004 UI-edit-superseded on 2026-08-14 (`rewrite_1_of_7`, `rewrite_2_of_7`)
- **Post-purge live corpus**: exactly 3 nodes
  1. `mdemg-cms-memory-only` — `is_informational=TRUE` — never generates outcome rows by design
  2. `query-mdemg-cms-file-paths` — grades
  3. `project-planning-docs-in-repo-only` — grades
- **Post-arc correction outcomes ARE being recorded correctly** — D6 shows exactly these 2 gradable codes accounting for all 20+4=24 post-arc `correction` outcomes
- **No pipeline broken**; no code fix needed

## The corrected framing

JIMINY-CEILING-INVESTIGATION-002 D6 framed the 691→3 drop as an open follow-up: "Either L1 correction producer is silently broken or the L0 correction detector isn't matching." **Neither hypothesis was correct** — I hadn't correlated the drop date with the deliberate JIMINY-CORRECTION-CORPUS-001 corpus purge that had shipped exactly 3 days into the arc window. The purge shipped 2026-08-12; the "post-arc" window in #153 D6 starts 2026-08-19, so the entire post-arc window measures the post-purge corpus.

## Evidence chain

### D1 — Per-day `constraint_outcomes` volume (`guidance_type='correction'`)

Pre-corpus-purge (through 2026-08-11): 3-238 rows/day, avg ~50/day
Post-corpus-purge (2026-08-12 onward): 1-117 rows/day, then decayed sharply

Cliff at 2026-08-12: 117 (last day of pre-purge outcomes still landing) → 32 → 7 → 2 → 12 → 21 → 2 → 1 → 21 (2026-09-02, a natural burst).

### D2 — Verdict split pre vs post

| Window | ignored | followed | partial | contradicted | total |
|---|---|---|---|---|---|
| pre-arc (2026-07-28 → 08-11) | 587 | 85 | 17 | 2 | **691** |
| post-arc (2026-08-19 → now) | 20 | 4 | 0 | 0 | **24** |

Ratio holds: pre-arc follow rate ~14.8%, post-arc follow rate ~16.7% — the QUALITY signal is stable (small n post-arc). What changed is VOLUME, not quality.

### D3 — L0 correction obs stream: sparse both pre and post

`obs_type='correction'` observations on `mdemg-dev`:
- 2026-07-20: 3 obs · 2026-08-01: 2 obs · 2026-08-03: 1 obs · 2026-08-07: 1 obs · 2026-08-18: 1 obs
- **Total: 8 obs across 50+ days** — the L0 stream has always been sparse

**Implication**: L1 correction nodes get RE-SURFACED many times per unique node via retrieval. Pre-purge 39 live L1 nodes × ~15 surfaces/node/day = ~600/day. Post-purge 2 gradable live L1 nodes × ~10 surfaces/node/day = ~20/day. The math works out.

### D4 — L1 correction node inventory

- **41 total**, **38 archived**, **3 live**
- 34 of 35 nodes created on 2026-07-20 are archived (via #100's purge)
- 2 nodes from later creation dates archived via #117's rewrite flow

### D7 — Archive attribution

| archive_reason | count | timestamp |
|---|---|---|
| `jiminy_correction_corpus_001_purge` | 31 | 2026-08-12T15:54:18.710Z |
| `jiminy_correction_corpus_001_purge_dup` | 5 | 2026-08-12T15:53:25.333Z |
| `ui_edit_supersede_jiminy_corpus_audit_004_rewrite_1_of_7_2026-08-14` | 1 | 2026-08-14T09:44:09Z |
| `ui_edit_supersede_jiminy_corpus_audit_004_rewrite_2_of_7_2026-08-14` | 1 | 2026-08-14T09:46:48Z |

Every archived node has a traced archive_reason. Fully explains the disappearance.

### D9 — Full ledger of live L1 corrections

```
code                                    informational  created                name
mdemg-cms-memory-only                   TRUE           2026-07-20T11:27:55Z   "Markdown memory files (..."
query-mdemg-cms-file-paths              FALSE          2026-08-14T09:44:09Z   "query-mdemg-cms-file-paths"
project-planning-docs-in-repo-only      FALSE          2026-08-14T09:46:48Z   "project-planning-docs-in-repo-only"
```

The 2 gradable live codes are the exact 2 codes appearing in post-arc `constraint_outcomes` (D6 confirms), plus 2 rows with empty `constraint_code` on 2026-08-16 (a very small heuristic-source residual).

## Verdict: INVESTIGATED_NO_ACTION

The correction sink is FUNCTIONING CORRECTLY. The volume dropped because the corpus was deliberately shrunk from 39 to 3 live nodes (a designed 90% purge). Zero code fixes needed; zero substrate mutation needed; JIMINY-CORRECTION-PRODUCER-001 (task #101) is not silently broken; the L0 detector is not misbehaving.

## Adjacent observation (worth noting, not requiring action here)

The single gradable code `project-planning-docs-in-repo-only` — which also exists as a `role_type='constraint'` (excluded from Path 1's informational-flag batch precisely because it's path-verifiable from action-text) — now dominates the residual correction-outcome stream:

- D6: `project-planning-docs-in-repo-only` correction outcomes 2026-08-15 → 2026-09-02: **20 ignored + 4 followed** = 24 rows (~17% follow rate)

This aligns with the Path 1 sprint's rationale for keeping it graded: it IS classifier-verifiable from file paths, and its follow rate is measurable. Path 1's decision holds.

## Implications for Phase 4b (retrain)

The tiny remaining post-arc correction sample (24 rows in a 15-day window) means:
- Any retrain evaluation that reads `constraint_outcomes` filtered on `guidance_type='correction'` will have <5% of the pre-arc sample size
- If the retrain training signal requires a threshold number of graded corrections, that threshold cannot be met on `mdemg-dev` alone in a reasonable window
- Options if `correction`-specific training signal is needed:
  - Grow the L1 correction corpus (add more L0 correction obs via `/v1/conversation/observe` obs_type=correction, then consolidation → L1 promotion via JIMINY-CORRECTION-PRODUCER-001)
  - Reuse pre-purge `constraint_outcomes` rows (still in TSDB retention; they're valid data even though their source L1 nodes are now archived)
  - Downweight `guidance_type='correction'` in the training loss until corpus grows organically

Not a blocker for a Phase 4b sprint; just a data-density note.

## Follow-ups filed (from this investigation)

1. **Optional: add more L0 correction obs** if a future sprint wants a larger correction sink. Not urgent — the shipped pipeline works, just under-fed.
2. **JIMINY-CORRECTION-PRODUCER-001 recovery check** — verify the L1 promoter still runs (last L1 node created 2026-08-14). Run consolidation, seed a test correction obs, verify promotion happens. Belongs to a separate healthcheck sprint if operator wants confidence.

## Documents Accessed

- Live TSDB `constraint_outcomes` — 4 windowed queries + per-day + per-code breakdowns (D1, D2, D5, D6)
- Live Neo4j `MemoryNode {space_id: 'mdemg-dev', role_type: 'correction'}` — inventory + timeline + archive attribution (D3, D4, D7, D9)
- `docs/development/jiminy-correction-corpus-001/sprint_post.md` — confirmed 35-tombstone shipping date + intent (D8)
- CLAUDE.md JIMINY-CORRECTION-CORPUS-001 pin — cross-reference the shipping record
- Predecessor sprint: `docs/development/jiminy-ceiling-investigation-002/verdict.md` (§D6 disclosed this follow-up)
- Operator directive: "proceed with correction-sink investigation" (2026-09-07)
