# JIMINY-ROLETYPE-ADAPTER-001 — Sprint Post (2026-07-17)

## Summary
Closes the JIMINY-CORPUS-001 disclosed follow-up: retrieval-sourced guidance
items were 100% mis-typed as `learning` because `RetrieveForJiminy` dropped
`role_type`. Fixed end-to-end; live-verified on `mdemg-dev`.

## What shipped
- **E0** — `sprint_plan_jiminy_roletype_adapter_001.md` committed.
- **E1** — additive `RoleType`/`ObsType` fields wired through
  `retrieval.Candidate`, `models.RetrieveResult`, `retrieval.FusedCandidate`,
  `retrieval.BM25Result`, `jiminy.RetrievalResult`. Both `vectorRecall` and
  `BM25Search` Cypher extended with `coalesce(node.role_type,'')` and
  `coalesce(node.obs_type,'')` on `RETURN`. Both scorers (`scoring.go` linear
  + `scoring_rrf.go` RRF) copy the two fields into `models.RetrieveResult`.
  The BM25 merge branch prefers non-empty ontology labels when merging into a
  vector-seeded fused entry. `jiminyRetrievalAdapter.RetrieveForJiminy` copies
  both fields (refactored into `mapRetrieveResultsToJiminyResults` for
  testability).
- **fix-commit** — `internal/retrieval/reasoning.go` extended the
  `originalByID` restore hook (which already preserved `Layer` through the
  proto boundary) to also preserve `RoleType`/`ObsType`. Caught during E4
  live smoke.
- **E2** — `classifyRetrievalItem` now prefers `RoleType == "constraint"|"correction"`
  before the Layer≥2 concept short-circuit and the `ObsType` switch. Default
  fallback preserved for empty `RoleType`.
- **E3** — extended `TestClassifyRetrievalItem` truth table by 6 rows
  (role beats obs; role beats concept short-circuit; unknown role_type falls
  through; defaults preserved). Added
  `TestMapRetrieveResultsToJiminyResults_PropagatesRoleAndObs` and
  `TestMapRetrieveResultsToJiminyResults_EmptyInput` pins. Full `go test ./...`
  green; `golangci-lint run` clean on touched packages.
- **E4** — live Tier-3 evidence in `live_verification.md`.
- **E5** — canonical docs updated (CLAUDE.md architecture note,
  CHANGELOG `[Unreleased] > Fixed` entry,
  `docs/features/jiminy-actionability.md` follow-up flipped from
  disclosed-open to shipped-closed, this post).

## Commits (on `reh3376_dev01`)
1. `docs(jiminy-roletype-adapter-001): E0 — sprint plan`
2. `feat(jiminy-roletype-adapter-001): E1 — propagate role_type + obs_type through retrieval`
3. `feat(jiminy-roletype-adapter-001): E2 — role_type-preferring classifier`
4. `test(jiminy-roletype-adapter-001): E3 — unit + adapter tests`
5. `fix(jiminy-roletype-adapter-001): reasoning module rerank preserved role_type` *(surprise fix-commit)*
6. `docs(jiminy-roletype-adapter-001): E4 — live Tier-3 verification`
7. `docs(jiminy-roletype-adapter-001): E5 — CLAUDE.md/CHANGELOG/feature/post`

## Live evidence highlights
| Signal | Pre-fix | Post-fix |
|---|---|---|
| `/v1/memory/retrieve` `results[].role_type` | (field absent from schema) | `'constraint'` populated on L1 constraint hits |
| `/v1/jiminy/latest` guidance type distribution | 100% `learning` (retrieval path) | 4/5 `constraint`, 1/5 `learning` |
| `constraint_outcomes.guidance_type='constraint'` | never populated via retrieval path | 3 rows (both `tier1` and `llm` sources) |
| Neo4j `GUIDANCE_OUTCOME` edge targeting | dependent on empty `guidance_type` | resolved via matched `constraint_code` |

## Lessons captured
1. **Struct-to-struct mapping seams need every ontology label.** The chain
   was silently empty at 5 seams because each seam looked "shape-preserving"
   but dropped fields that hadn't yet been added upstream. When adding a
   new ontology field, grep for every construction site of the target type
   and audit for missing fields.
2. **Reasoning-module reranks need the same restore hook as scorer output.**
   The default-on `keyword-booster` rebuilds `RetrieveResult` from a proto
   with a smaller shape and used `originalByID` to preserve `Layer`. Any new
   field the proto doesn't carry needs the same treatment. Codified in
   CLAUDE.md.
3. **BM25 is its own sink, not a view.** The RRF path uses virtual columns
   over pre-fetched `cands`, but BM25 runs its own Cypher upstream — merging
   into the fused pool. A non-empty preference on the merge branch avoids a
   first-column-wins race dropping labels.
4. **Live smoke catches what unit tests don't.** All unit tests were green
   post-E3, but E4 discovered the reasoning-module drop the moment the
   real reasoning pipeline ran. `feedback_live_testing_required` still
   binds — do not skip Tier 3 for "the tests passed".
5. **macOS 26 launchd hardening rejects ad-hoc signed binaries.** Dev-loop
   restart via `nohup ./bin/mdemg serve` bypasses launchd's `OS_REASON_CODESIGNING`
   rejection. Production install uses a signed Homebrew binary; this only
   affects dev restarts on the operator's box.

## Non-goals (respected)
- Did not create `role_type='correction'` nodes (still zero in `mdemg-dev`).
  The pipeline can now carry them if a producer exists.
- Did not retune `JIMINY_SURFACE_ACTIONABLE_WEIGHT` or Lever A quotas.
- No schema / migration / compose changes.

## Follow-ups
- A producer for `role_type='correction'` nodes is now the highest-value
  next lever for the actionable surface (probably a Jiminy contradicted-outcome
  → correction bridge, or an operator-authored CLI). Out of scope here.
- Follow-rate re-measurement from JIMINY-CORPUS-001 remains pending on
  accumulating usage days; today's E4 smoke does not backfill it.

## Acceptance criteria — all met
- [x] `POST /v1/memory/retrieve` responses carry `role_type` / `obs_type` on constraint-role hits.
- [x] `classifyRetrievalItem` emits `GuidanceConstraint` for retrieval-sourced constraint nodes.
- [x] `constraint_outcomes` gains at least one `guidance_type='constraint'` row post-fix.
- [x] Full test suite green; lint clean.
- [x] Canonical docs updated.
