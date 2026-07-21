# DOC-CURRENCY-002 — Sprint Post

**Dates:** 2026-07-21 (audit + fix, same day)
**Branch:** `reh3376_dev01` | **Format:** sprint plan v1.0
**Parent:** operator request — "dispatch agents to review all existing repo
documentation … consolidate the findings into a fix sprint"

## What shipped

### E0–E1 — audit → 6 findings work orders → 5 parallel fixer agents

The 181-finding audit (5 read-only agents) was consolidated into 6 work-order
files (`findings_*.md`), executed by 5 fixer agents on disjoint file sets with
a re-verify-against-code-before-editing contract, plus orchestrator-reserved
heavies. Every agent reported per-fix verification; 4 findings were **skipped
because the finding itself was wrong** (README `.mcp.json` path — code still
writes `.claude/mcp.json`; `JIMINY_EVALUATE_LLM_ENABLED` is real at
config.go:2787; rsic-feedback's 0.15 stability weight matches code; UVTS
`--apply-tsdb` is UBENCH-only). Skips-with-reasons are the audit working as
designed — docs were NOT blindly "fixed" toward a wrong finding.

Areas (one commit each): root docs · features a–j (30 findings) · features
k–z (24 files) · user/guides/operations (12 files) · tests/sidecar (6 files).
Note: the k–z agent's edits were swept into the a–j commit (it finished
editing before its notification arrived); the "remainder" commit carries only
the final residue. Disclosed here rather than rewriting history.

### E2 — reserved heavies (orchestrator)

- **CLAUDE.md** — 15 fixes (honest aggregate 0.9188 annotations, schema v31,
  24+ alert rules, superseded defaults annotated in place, stale pre-flip
  sparse-gate paragraph deleted, `RETRIEVAL_CTX_STRICT_THRESHOLD` real name).
- **CMS.md** — the config block documented `CMS_*`/`STABILITY_*` variables
  that **never existed** (setting them silently no-ops). Rewritten to the
  real `COOLER_*` family + real `RSIC_*` names/defaults from config.go.
- **VISION.md** training section (dense `mdemg-llm-v1` shipped state; MoE
  strategy as history), **SECURITY.md** version table.
- **UXTS_FRAMEWORK_MATRIX.md** §2/§3/§5 — counts + per-step merge-blocking
  vs soft-fail from ci.yml ground truth; UVTS reinstated as fully functional
  / live-gated (the "spec-only" demotion was 14 months stale); UBENCH rows.
- **prometheus-observability-monitoring.md** — full rewrite as the canonical
  TSDB-native metrics access model doc (the scrape-investigation verdict);
  Prometheus-era fixes preserved as a historical appendix.
- **INGEST_CODEBASE_API.md** — full rewrite against the real
  `/v1/memory/ingest/{trigger,status,cancel,jobs,files}` surface; the
  documented `/v1/memory/ingest-codebase` nested schema never shipped.
  Documented honestly that `include_md`/`include_ts`/`include_py`/
  `archive_deleted` are accepted by the model but dropped by the trigger
  handler (code-level gap, disclosed not papered over).

### E3 — prevention: `scripts/verify_doc_env_vars.py`

Heuristic doc↔code env-var drift checker: UPPER_SNAKE tokens in living docs
(CLAUDE.md, CMS.md, README, docs/features, docs/user) must exist in the code
corpus (Go, templates, workflows, python, shell). Brace-family expansion
(`ALERT_RETRIEVE_{P95,P99}_MS`), commented allowlist for legitimate prose,
**advisory** continue-on-error CI step (deliberate exception to the
no-soft-fail-tests rule: this is a linter with possible false positives, not
a contract; `--strict` locally).

**It caught 5 real drifts during its own build sprint**, two in text written
that same hour:

1. `RETRIEVAL_CTX_STRICT_THRESHOLD` — CLAUDE.md carried the never-existed
   `RETRIEVAL_CONTEXT_STRICT_THRESHOLD` form.
2. `RSIC_{NUDGE,WARN,FORCE}_THRESHOLD` — my own CMS.md rewrite had carried
   over `RSIC_WATCHDOG_*`-prefixed forms from the old (wrong) block.
3. `J17_TRUST_EMA_ALPHA` — j17-ai2ai-protocol.md documented a
   `JIMINY_`-prefixed form.
4. Four fabricated `SYNERGY_*` rows + 8 fabricated cli-reference rows
   (`GUARDRAIL_HOOK_*`, `CONSTRAINT_DECAY_*`, `ORPHAN_CLEANUP_*`, …).
5. A **code-comment bug**: `neural_sidecar/config.py` told operators to set
   `SIDECAR_HOST`; pydantic-settings `env_prefix="NEURAL_"` makes the real
   var `NEURAL_HOST`. Own fix-commit (`fix(neural)`), per the
   surprise-fix rule.

Checker limitation documented in the allowlist: prefix-constructed names
(pydantic `env_prefix` + field) never appear literally — `NEURAL_TIER_MODEL`
is real but allowlisted with the derivation explained.

### E4 — verification

`go build ./...` 0 · `golangci-lint` 0 issues · 63 neural benchmark tests
green · ci.yml valid · checker final state **OK: 92 docs scanned, 0 unknown
tokens** · class-pattern greps clean (no live mlx_lm.server/:8101/old
benchmark paths outside deliberate historical mentions).

### Also committed

`neural/benchmarks/persist.py` psycopg2-fallback — applied during
BENCH-SIDECAR-APPLY-001's live smoke but never staged; committed as its own
`fix(bench-sidecar-apply-001)` so the repo matches tested behavior.

## Numbers

| | |
|---|---|
| Findings in audit | 181 |
| Applied | ~170 (across 6 areas) |
| Skipped, finding-wrong (each cited to code) | 4 |
| Deferred by design (historical docs describe eras) | remainder |
| Real drifts caught by the NEW checker during the sprint | 5 classes |
| Files changed | ~105 across 9 commits |
| Living docs now drift-checked per CI run | 92 |

## Follow-ups (disclosed, not started)

- Neural-sidecar operator doc (none exists; noted in the audit).
- `unified-cli.md` full per-command sections (index table shipped).
- Trigger-handler forwarding gap: `include_md`/`include_ts`/`include_py`/
  `archive_deleted` accepted by `IngestTriggerRequest` but dropped before the
  CLI args (documented honestly in INGEST_CODEBASE_API.md; wiring them is a
  small code sprint if ever needed).
- Checker DOC_GLOBS could grow (docs/guides, docs/operations) once their
  token noise is triaged.

## Documents Accessed

`docs/development/doc-currency-002/findings_*.md` (6 work orders);
`internal/config/config.go` (every env-var verification);
`internal/api/{server,handlers}.go` + `internal/models/models.go` (ingest
surface); `.github/workflows/ci.yml` (gate ground truth);
`neural/neural_sidecar/config.py`; `docs/development/
prometheus-scrape-investigation-001/{investigation,post}.md`; on-disk spec
counts under `docs/api/api-spec/`, `docs/tests/`, `docs/lang-parser/`.
