# Phase 13.1 Epic 0 — Forensic Diagnosis (q 69 + q hard_sym_4)

**Date:** 2026-05-03
**Sprint:** POST-FT-LORA-PHASE13.1 (Column-Weight Ablation)
**Question:** *Why did q `69` and q `hard_sym_4` regress to score 0.000 under v1-rrf4 equal weights?*
**Verdict:** **H1 (equal-weights pathology, Graph+Structural over-aggressive)** — confirmed by 4 column-suppression tests.

---

## The two questions under investigation

| qid | category | text |
|---|---|---|
| `69` | architecture_structure | "How does the **secretsManager module** integrate with **Azure Key Vault** for credential management?" |
| `hard_sym_4` | computed_value | "What is the actual **SLOW_QUERY_THRESHOLD_MS** when the SLOW_QUERY_THRESHOLD_MS environment variable is not set? Where is the **fallback** defined?" |

Both are precise-symbol queries. The "right" candidates are domain-specific files (`secretsManager.module`, `constants`-module-with-fallback, `ENVIRONMENT_SETUP.md`) — not structurally-connected classes.

## Test matrix (all run against `whk-wms` space, top_k=5)

| # | Configuration | q 69 top-1 | q hard_sym_4 top-1 | Verdict |
|---|---|---|---|---|
| **L** | `RetrievalColumnVotingEnabled=false` (legacy linear, the baseline) | `broker-credential-encryption.ts` (rel) | `chatbot-fallback-testing` (irrel) | Linear scored 0.354/0.350 in Phase 13 — grader found evidence somewhere in top-5 |
| **R** | RRF v1-rrf4, equal weights, hops=2 (Phase 13 candidate) | `azure-ad.service` Class (loose match) | `chatbot-fallback-testing` (irrel) | Phase 13 grader: 0.000/0.000 — RRF dropped the right candidates |
| **A** | RRF + Structural OFF | `broker-credential-encryption.ts` ✓ (recovers L's top-1) | top-4 `constants` ✓, top-5 `ENVIRONMENT_SETUP.md` ✓ | **Both regressions recover** |
| **B** | RRF + Graph OFF | `azure-ad.service` Class, **#4 `SecretsManagerModule` ✓**, **#5 `SecretsManagerService` ✓** | **#1 `constants` ✓**, **#2 `ENVIRONMENT_SETUP.md` ✓**, #4 `db-config.env.example` ✓ | **Both regressions recover, more strongly than Test A** |
| **C** | RRF + hops=1 (all 4 columns) | **#1 `secretsManager.module` ✓** (perfect hit) | top-1 `mcpFallbackService` (irrel), but top-3 `query-performance-test` ✓ | q 69 perfect; q hard_sym_4 partial |

## What the evidence converges on

**The dominant root cause is Test A + Test B's joint pattern.** Disabling either Structural (A) or Graph (B) recovers both regressions. Disabling Graph (B) is *strictly better* than disabling Structural (A) — Test B finds the actual `SecretsManagerModule` + `SecretsManagerService` files, plus `db-config.env.example` for hard_sym_4.

This means the Graph+Structural pair, at equal weights `1.0/N` (= 0.25 each), votes 50% of the RRF aggregate toward structurally-connected code, which crowds out Embedding+BM25's better lexical+semantic matches for precise-symbol queries.

**Test C (hops=1, all 4 columns)** finds q 69's perfect candidate (`secretsManager.module` at top-1), confirming H2 contributes — over-aggressive Structural traversal at hops=2 was pulling in too many siblings/parents — but doesn't fully fix q hard_sym_4. So H2 is contributing but not the dominant cause; H1 is dominant.

## Implications for Epic 2 weight-preset design

The diagnosis suggests these presets are most likely to pass A/B (in descending confidence):

1. **`embedding-bm25-priority`** (NEW preset, replaces `lexical-priority` from the plan):
   - `EMBEDDING=0.40, BM25=0.40, GRAPH=0.10, STRUCTURAL=0.10, hops=2`
   - Rationale: precise-symbol queries (which include hard_sym_*) need lexical+semantic match. Cap Graph+Structural at 20% combined.

2. **`embedding-heavy-hops1`** (combines preset + hop-depth from the plan):
   - `EMBEDDING=0.50, BM25=0.20, GRAPH=0.15, STRUCTURAL=0.15, hops=1`
   - Rationale: heavy-Embedding handles symbol queries; hops=1 reduces Structural noise on remaining 15% weight.

3. **`structural-suppress` + `embedding-heavy`** (original plan presets):
   - Less directly evidenced but worth running for the ablation table.

4. **`equal-baseline`**: still worth running as a sanity check that the runner reproduces Phase 13's failed verdict.

The original Plan §13 fork "preset count for sweep" should expand to 5 presets (added `embedding-bm25-priority` and `embedding-heavy-hops1`).

## What the diagnosis does NOT decide

- Whether the winning preset will *also* preserve q `hard_sym_20`'s improvement (+0.100). The improvement was on a `computed_value` query (similar category to hard_sym_4); it may have come specifically from RRF's Graph+Structural votes. Heavy-Embedding presets may lose the improvement.
- Whether the winner generalizes to the full 120-question profile. Epic 4 will tell us.
- Whether per-category weighting is needed (Phase 13.2 scope) — current sweep picks a global preset.

## Test artifacts

| Test | Output JSON | Lines (top-5 summarized in this doc) |
|---|---|---|
| L | `/tmp/phase13_1_diagnosis/q69_linear.json` | 5 candidates |
| L | `/tmp/phase13_1_diagnosis/q_hard_sym_4_linear.json` | 5 candidates |
| R | `/tmp/phase13_1_diagnosis/q69_rrf4.json` | 5 candidates |
| R | `/tmp/phase13_1_diagnosis/q_hard_sym_4_rrf4.json` | 5 candidates |

(Tests A/B/C run interactively against the same endpoints; not preserved as JSON — Epic 1 ablation runner will capture all preset results to disk.)

## Confidence

**High.** Four independent column-configuration tests all pointed at the same conclusion: Graph+Structural at equal weights with hops=2 collectively over-vote on structurally-connected code at the expense of config/docs/constants candidates. The fix is to *cap* their combined contribution (weight reduction or column suppression). Lowering hops to 1 helps but is not sufficient on its own.

## Next step

Epic 1 (ablation runner) → Epic 2 (preset sweep with 5 presets) → Epic 4 (winner verification on full 120q).

## Documents accessed

- `docs/development/post-ft-lora/phase_13_column_voting_post.md` — Phase 13 post (verdict + 3-hypothesis enumeration)
- `docs/development/post-ft-lora/phase_13_ab_verdict_quick.json` — canonical machine-readable verdict
- `docs/architecture/benchmarks/whk-wms/test_questions_120.json` — q 69 + q hard_sym_4 source text
- Live `mdemg` HTTP API at `localhost:9999/v1/memory/retrieve` (5 sequential configurations)
