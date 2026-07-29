# RETRIEVAL-DIVERSITY-001 — Sprint Post

**Date:** 2026-07-29 | **Branch:** `reh3376_dev01`
**Parent trigger:** RETRIEVAL-QUALITY-AUDIT-001 recommendation #3.

## Verdict

**Shipped default-off in code, flipped ON in `.env` after live smoke.**
Post-rerank exact-name-match dedup with a safety-net back-fill.
Addresses RQA-001 cluster D directly; live smoke on q04 shows the
target pattern (2× pre-bash-check + 2× sql = 4/5 duplicated) resolves
to 3 diverse results.

## What shipped

- **`internal/retrieval/diversity.go`** — pure function
  `ApplyDiversityFilter(results, topK, cfg)` implementing the
  post-rerank dedup with fill-from-skipped safety net
- **`DiversityCfg`** struct with `Enabled`, `MaxPerName`, `MinOutput`
- **Config**: `RETRIEVAL_DIVERSITY_ENABLED` (default false),
  `RETRIEVAL_DIVERSITY_MAX_PER_NAME` (default 1 = strict dedup),
  `RETRIEVAL_DIVERSITY_MIN_OUTPUT` (default 1 = "prefer diverse
  coverage over completeness")
- **Wire** in `internal/retrieval/service.go` right BEFORE the topK
  truncation (correct architectural placement — filter picks from a
  larger candidate pool than the caller will see)
- **7 unit tests** pin exact-name dedup, safety-net minimum output,
  disabled passthrough, empty-name-always-diverse,
  MaxPerName=2 tunable, input-shorter-than-topK dedup, zero-value
  defaults

## Live Tier-3 smoke (mdemg-dev)

**q04** (RQA-001 pattern: 2× pre-bash-check + 2× sql):
- FLAG-OFF: 5 results — `pre-bash-check, pre-bash-check, _sql_builtins, sql, sql` (4/5 duplicate slots)
- **FLAG-ON: 3 results — `pre-bash-check, _sql_builtins, sql`** ✓ 100% diverse, both duplicates dropped

**q14** (RQA-001 pattern: 5 emergent-concepts):
- FLAG-OFF: 5 results — all named `EmergentConcept-*-uuid-cuidv2-*` with DIFFERENT unique suffix numbers (115, 114, 59, 1177, 1907)
- FLAG-ON: 5 results, more diverse mix — the emergent-concept naming
  convention creates unique names per instance, so exact-name-match
  doesn't help here; this pattern needs name-prefix matching or
  embedding-cosine (deferred, `RETRIEVAL-LAYER-BALANCE-001` or a
  future sprint targets it more directly)

**Verdict**: clear win on cluster D's core pattern (exact-name
duplicates); doesn't help name-varied concept clusters (out of scope).

## Design decision — "prefer diverse coverage over completeness"

Original test design assumed fill-from-skipped ALWAYS runs to reach
topK. But that DEFEATS the sprint's purpose — you'd get 5 results
with 2 duplicates back. The RQA-001 design intent was "frees ~11% of
top-5 slots"; that intent needs the output to actually BE shorter
when dedup fires.

**Chosen semantic**: dedup runs; output = up to topK diverse
results; fill-from-skipped is a SAFETY NET that only kicks in when
`MinOutput` (default 1) wouldn't be reached otherwise. Operators
who want strict "never short" back-fill can set
`RETRIEVAL_DIVERSITY_MIN_OUTPUT = topK`.

## Rules pinned

1. **Diversity-filter design intent = "prefer diverse coverage over
   completeness"**. Default `MinOutput=1` means a caller asking for
   topK=5 who gets 3 diverse results is BETTER OFF than one who
   gets 5 with duplicates. This is a semantic choice; the safety-net
   config knob lets operators override if a specific consumer needs
   fixed output size.
2. **Filter runs BEFORE the topK truncation** in `service.go` — the
   filter needs a larger candidate pool than the caller will
   ultimately see; wiring after the truncation would defeat the
   design.
3. **Empty-name results bypass dedup** (always-diverse). Protects
   results that don't carry a name from spurious dedup by empty-key
   collision.

## Follow-ups disclosed

- **Name-prefix diversity** (~1d) — for the q14-shape pattern
  (emergent-concept clusters with unique-suffix names). Would extend
  dedup to also match on `strings.TrimSuffix(name, "-<digits>")`
  patterns. Deferred; the shipped `RETRIEVAL-LAYER-BALANCE-001` (next
  sprint) may address it more directly by scoring L0/L1 concrete
  over L3+ abstract.
- **Embedding-cosine diversity** (~2d) — for the general
  near-duplicate case where names differ but content is
  semantically similar. Deferred as premature — the observed cluster
  D patterns are exact-name and name-prefix-shaped, not content-
  shaped.

## Documents Accessed

- `docs/development/retrieval-quality-audit-001/post.md` (parent
  cluster D)
- `docs/development/retrieval-diversity-001/sprint_plan.md` (this dir)
- `internal/retrieval/service.go` (integration point)
- `internal/models/models.go` (`RetrieveResult.Name` field)
- `internal/config/config.go` (env-var wire pattern)
- Live TSDB queries against mdemg-dev via `/v1/memory/retrieve`
  (q04 + q14 with flag on/off)
