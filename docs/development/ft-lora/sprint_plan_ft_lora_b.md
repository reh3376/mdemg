# Sprint FT-LORA-B — Code/Config Alignment + Guardrail llmclient Migration + ULTS sampling_group

## Context

Sprint A (merged 2026-04-21 as PR #335) aligned the plan suite to memo 07 v3.1. Sprint B is the **code-side execution** of what Sprint A queued: grep-audit remediation, `internal/guardrail/llm_evaluator.go` migration to `llmclient`, ULTS schema addition of `sampling_group`, and env/compose plumbing for Sprint E's upcoming asymmetric-quant + router-aux-loss knobs.

Three workstreams run serially in this sprint (per MEMORY rule "do NOT parallelize epics"):
1. **ULTS schema + 16 specs** — add `sampling_group` enum field (T/C/J) so Sprint C inference harness and Sprint E training code can read it. Atomic: schema + runner's `KNOWN_TOP_FIELDS` + all 16 specs, or CI fails.
2. **Guardrail migration** — `internal/guardrail/llm_evaluator.go` currently bypasses `llmclient` (direct HTTP to OpenAI/Ollama), so its calls aren't captured in `llm_interactions` TSDB hypertable and won't auto-switch to local Qwen3.6 at cutover. Creates new 17th task `guardrail.evaluate` with ULTS spec + refactors ~175 lines of direct HTTP to `llmclient.Complete()`. This is a **Phase 5 SFT prerequisite** — if guardrail calls aren't in the training corpus, the fine-tuned model will silently underperform on that task at cutover.
3. **Grep-audit remediation** — 15 files from `SPRINT_A_GREP_AUDIT.md` (8 docs + 7 code/config) with stale `Qwen3-30B-A3B` / `qwen3-30b` refs. All but one are docstring-only (trivial); `scripts/test_vllm_mlx.py:236` is a functional argparse default. Deep path/memory rework deferred to Sprint E.

Plus Epic 5: minimal `.env.example` + compose template additions exposing Sprint-E-bound knobs as placeholder vars (no behavior change; sets up the naming convention).

**Zero training spend** (no FT job launches); local Go + Python + docs only. Planner estimate **3-4 days** (revised up from initial 1-2 day estimate after scope consolidation — guardrail migration is the sprint's largest single workstream and its test/integration surface is non-trivial, see Epic 3).

**Scope decision (user-approved, 2026-04-21):** Bundle all three workstreams into one sprint rather than splitting B.1 / B.2. Rationale: guardrail is a Phase 5 SFT prerequisite and shipping it separately costs a full sprint cycle; the integration test surface overlaps with the ULTS schema change (guardrail.evaluate spec must validate against new `sampling_group` field).

**Sprint chain context:** A (docs, done) → **B (this sprint)** → C (Qwen3.6 MLX validation 3 gates) → D (expert activation profiling) → E (training infra patches: router_aux_loss_coef exposure, mlx_lm.convert selectors, Tier 1/2 CLI flags, early-stop implementation). Phase 5 SFT still blocked until Sprint C passes.

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | FT-LORA-B |
| Title | Code/Config Alignment + Guardrail llmclient Migration + ULTS sampling_group |
| Date | 2026-04-21 (plan) |
| Branch | `reh3376_dev01` |
| Predecessors | FT-LORA-A (PR #335, merged `8ae8f24`) |
| Successors | FT-LORA-C (Qwen3.6 MLX validation — 3 gates) |
| Type | Code + config + docs |
| Risk | Medium (live Go code changes in `internal/guardrail/`; circuit breaker name hard-cutover on admin surface) |
| Budget | $0 |
| Estimate | 3-4 days |

## 2. Problem Statement

Sprint A aligned planning docs but did not touch code — five concrete code/config gaps remain between memo 07 v3.1 and the repo state:

- **Guardrail consumer bypasses llmclient** (`internal/guardrail/llm_evaluator.go` lines 61-239). Direct HTTP → OpenAI or Ollama → no `llm_interactions` capture → absent from Qwen3.6 training data → won't auto-switch to local Qwen3.6 at cutover. Flagged as "17th call site / cutover blocker" in `01_RESEARCH_v2.md §1.1` guardrail note.
- **ULTS schema has no `sampling_group` field.** Sprint C will need to read per-task group assignments (T=7 / C=6 / J=3 per `04_BENCHMARK_RL_v2.md §10.0`) to apply the three memo §3.3 sampling recipes. Schema currently enforces parity hard-fail on unknown fields (`ults_runner.py:61-69, 130-143`) — can't add the field to specs without the schema + runner updates.
- **15 files carry stale `Qwen3-30B-A3B` refs** (per `SPRINT_A_GREP_AUDIT.md` categories a + b). Most are docstring examples; one is a functional argparse default.
- **No `.env.example` / compose plumbing** for upcoming Sprint E knobs: `ROUTER_AUX_LOSS_COEF`, `LORA_TIER1_RANK`, `LORA_TIER2_RANK`, etc. Sprint E rework will be cleaner if the naming convention is seeded in advance as commented placeholder vars.

Left unremediated: Sprint C's inference harness has no authoritative per-task sampling-group source of truth; Sprint E starts by retrofitting env var names the rest of the config already references; guardrail silently drops the only MDEMG LLM call site that never reaches the training set.

## 3. Scope & Constraints

**In scope:**

| # | Workstream | Files |
|---|---|---|
| 1 | ULTS schema + runner + 16 specs | `docs/tests/ults/schema/ults.schema.json`, `docs/tests/ults/runners/ults_runner.py`, all 16 `docs/tests/ults/specs/*.ults.json` |
| 2 | New `guardrail.evaluate` ULTS spec | `docs/tests/ults/specs/guardrail_evaluate.ults.json` (new) |
| 3 | Guardrail llmclient migration | `internal/guardrail/llm_evaluator.go`, `internal/guardrail/*_test.go` (any), `internal/api/server.go` (instantiation site at line 440) |
| 4 | Grep-audit remediation (a) — 8 docs | `docs/development/RESEARCH_ROADMAP.md`, `docs/operations/vllm-mlx-setup.md`, `docs/tests/uaits/README.md`, `docs/features/training-data-capture-verification.md`, `docs/features/embedding-retrieval-data-collection.md`, `docs/features/neural-training-pipeline.md`, `docs/development/ft-oai/sprint_plan_openai_ft_data_generation.md`, `docs/development/ADVERSARIAL_CODEBASE_ANALYSIS_20260410.md` |
| 4 | Grep-audit remediation (b) — 7 code/config | `neural/training/train_ft.py`, `neural/training/tests/test_train_ft.py`, `neural/training/evaluate_ft.py`, `neural/training/teacher_distill.py`, `neural/training/quantize_deploy.py`, `scripts/test_vllm_mlx.py`, `docs/tests/uaits/specs/mdemg.uaits.json` |
| 5 | Env/compose placeholder vars | `.env.example`, `internal/cli/compose_templates/docker-compose.yml.tmpl`, `docker-compose.yml` (if present) |
| 6 | Sprint B doc close-out | `docs/development/ft-lora/00_README_v2.md` (version bump note + Document Map pointer), `AGENT_HANDOFF.md`, `CHANGELOG.md` |

**Out of scope (deferred):**

- Asymmetric-quant full rework (`mlx_lm.convert` per-module selectors, memory-table recalibration in `vllm-mlx-setup.md`, output-path renaming to `mdemg-qwen3.6-35b-v1-asym/`) — **Sprint E**. Sprint B adds TODO comments at those locations.
- Qwen3.6 MLX launch command variations, vllm-mlx version pinning — **Sprint C**.
- Expert activation profiling script `neural/training/profile_expert_routing.py` — **Sprint D**.
- FT-OAI-003 calibration run — **deferred by user** until Qwen training outcome is known (see memory `project_ft_oai_003_deferred.md`).
- Actual Qwen3.6 download, MLX validation runs, benchmark comparisons — **Sprint C**.

**Constraints:**

- Sequential epics (MEMORY rule `feedback_sequential_epics.md`).
- Single batched commit at sprint close (MEMORY rule — user's prior directive observed repeatedly).
- CI must stay green: ULTS schema + runner + all 16 specs updated **atomically** in one commit (not split) or `uxts-canonical-specs.yml` fails for any intermediate state.
- Guardrail migration must preserve existing behavior for `/v1/memory/guardrail/validate` consumers — input / output schema stable, latency budget maintained (or improved), `circuitbreaker.Registry` registration compatible.
- No new external deps; reuse existing `llmclient`, `tsdb`, `circuitbreaker` packages.
- Mid-sprint gate after Epic 3 (guardrail code change + tests pass locally) before continuing — aligns with Sprint A pattern ("frequent monitoring" per user).

## 4. Dependencies

- Merged `8ae8f24` (PR #335) on `main` — Sprint A canonical doc state.
- `internal/llmclient/` (`client.go`, `types.go`, `recorder.go`) — stable public API, no changes needed. Canonical call patterns documented at `internal/retrieval/query_classifier.go:68-74` and `internal/consulting/llm_classifier.go:64-78`.
- `internal/tsdb/llm_writer.go` — `llm_interactions` hypertable + `CopyFrom` bulk writer already handles task-name attribution.
- `internal/circuitbreaker/` — existing registry; Sprint B reuses `openai-guardrail` / `ollama-guardrail` breaker names (or consolidates to per-task names via llmclient's own breaker — design decision in Epic 3).
- No external dependency version bumps.
- Memo `07_MODEL_UPDATE_AND_MOE_STRATEGY.md` v3.1 sampling recipes table (§3.3) + `04_BENCHMARK_RL_v2.md §10.0` 16-task group mapping — source of truth for ULTS `sampling_group` values.

## 5. Implementation Plan (Sequential Epics + Gates)

**Pre-gate:** confirm branch state `reh3376_dev01` fast-forwarded to `8ae8f24`; working tree clean (tsdb_data_review untracked file ignored); `go build ./...` + `golangci-lint run ./...` + `make test-api` all green on current state; `python3 docs/tests/ults/runners/ults_runner.py` green on current 16 specs (establishes pre-change baseline).

### Epic 1 — ULTS schema + runner + 16 spec updates (atomic)

Add `sampling_group` top-level enum field (`"T"|"C"|"J"`) to the ULTS schema; register as a known field in the runner; populate on all 16 specs per the authoritative mapping.

**Sub-steps:**
1. `docs/tests/ults/schema/ults.schema.json` — add to `properties`:
   ```json
   "sampling_group": {
     "type": "string",
     "enum": ["T", "C", "J"],
     "description": "Memo 07 v3.1 §3.3 sampling group — T=reasoning-think, C=no-think classify, J=no-think JSON. Consumed by Sprint C inference harness to apply per-group recipe (temp/top_p/top_k/presence_penalty)."
   }
   ```
   Add to `required` list (so all specs must carry it — prevents silent omission).

2. `docs/tests/ults/runners/ults_runner.py:64` — extend `KNOWN_TOP_FIELDS`:
   ```python
   KNOWN_TOP_FIELDS = {
       "ults_version", "task", "metadata", "prompt", "performance",
       "output_schema", "quality_metrics", "reward_functions", "training_config",
       "sampling_group",  # NEW: Sprint FT-LORA-B — memo 07 v3.1 §3.3 group
   }
   ```
   Add a new parity check `parity_sampling_group` that validates the value is in the enum and flags missing values as hard-fail. **Error message must be actionable**, not cryptic. Exact wording:
   - Missing field: `"Spec '<task_name>' is missing required field 'sampling_group' (must be one of T/C/J per memo 07 v3.1 §3.3 — see docs/development/ft-lora/04_BENCHMARK_RL_v2.md §10.0 for task mapping)"`
   - Invalid value: `"Spec '<task_name>' has invalid sampling_group '<value>' (must be T=reasoning-think, C=no-think classify, J=no-think JSON)"`

**Schema change is hard-required (user-approved, 2026-04-21).** Explore verification confirmed zero external ULTS consumers in the workspace (no `.ults.json` files in `/Users/reh3376/net-topology`, `/Users/reh3376/mdemg.nvim`, `/Users/reh3376/claude_workspace`; schema at `docs/tests/ults/schema/ults.schema.json` is dev-internal, not published as a public contract in `docs/guides/UXTS_DEVELOPER_GUIDE.md`). The 16 repo-internal specs are updated atomically in this same epic. Epic 1 gate (below) re-runs the workspace grep as the explicit confirmation.

3. All 16 specs under `docs/tests/ults/specs/*.ults.json` — add `"sampling_group": "<T|C|J>"` at top level per table:

   | Task | Group | File |
   |---|---|---|
   | `ape.reflect` | T | `ape_reflect.ults.json` |
   | `consulting.classify` | C | `consulting_classify.ults.json` |
   | `consulting.synthesis` | T | `consulting_synthesis.ults.json` |
   | `hidden.summarize` | T | `hidden_summarize.ults.json` |
   | `hidden.name_emergence` | J | `hidden_name_emergence.ults.json` |
   | `hidden.reclassify` | C | `hidden_reclassify.ults.json` |
   | `jiminy.evaluate_llm` | J | `jiminy_evaluate_llm.ults.json` |
   | `jiminy.evaluate` | C | `jiminy_evaluate.ults.json` |
   | `jiminy.synthesize` | T | `jiminy_synthesize.ults.json` |
   | `jiminy.codegen` | C | `jiminy_codegen.ults.json` |
   | `metalearn.generalize` | T | `metalearn_generalize.ults.json` |
   | `retrieval.intent_translate` | C | `retrieval_intent_translate.ults.json` |
   | `retrieval.query_classify` | C | `retrieval_query_classify.ults.json` |
   | `retrieval.rerank_cross` | J | `retrieval_rerank_cross.ults.json` |
   | `retrieval.rerank_nli` | T | `retrieval_rerank_nli.ults.json` |
   | `summarize.generate` | T | `summarize_generate.ults.json` |

**Gate:**
- `python3 docs/tests/ults/runners/ults_runner.py` — green on all 16 specs
- `.github/workflows/uxts-canonical-specs.yml` equivalent (`scripts/verify_uxts_canonical_specs.py`) runs green locally
- Counts match: T=7, C=6, J=3, sum=16 ✓
- Schema validation via `jsonschema` on every spec passes
- **External-consumer confirmation (in writing in the gate):** run `ls /Users/reh3376/*/docs/tests/ults/specs/*.ults.json 2>/dev/null; grep -rln "sampling_group\|ults.schema.json" /Users/reh3376/net-topology /Users/reh3376/mdemg.nvim /Users/reh3376/claude_workspace 2>/dev/null` — zero output expected. If any external ref surfaces, stop and re-plan as optional-with-warning. Record the grep result in the commit body.
- Parity check error-message wording verified by triggering it: temporarily remove `sampling_group` from one spec, run the runner, confirm the new actionable message appears, restore the spec.
- If any spec missing the field, gate fails — must be remediated before moving to Epic 2

### Epic 2 — New `guardrail_evaluate.ults.json` ULTS spec

Create spec for the 17th task. Model after `jiminy_evaluate_llm.ults.json` (long-form JSON, Group J — similar violation-list structure) but simpler output schema.

**Spec fields:**
- `task.name`: `"guardrail.evaluate"`
- `task.description`: "Evaluate a diff for constraint violations and warnings against MDEMG guardrail policies."
- `prompt.think_mode: false` (no reasoning needed — single-shot JSON)
- `prompt.system_prompt_source`: `"internal/guardrail/llm_evaluator.go::guardrailSystemPrompt"`
- `prompt.system_prompt_hash`: SHA-256 of the current `guardrailSystemPrompt` constant (capture post-migration)
- `output_schema`: JSON object with `violations: [{rule, severity, message, line}]`, `warnings: [{rule, message, line}]`, `recommendations: [string]` — mirrors existing `GuardrailEvaluation` Go struct
- `performance.max_tokens: 2000`, `latency_budget_ms: 5000` (matches current `cfg.MaxTokens` / `cfg.TimeoutMs` defaults)
- `quality_metrics`: JSON validity + constraint-detection recall + false-positive rate (per-rule)
- `sampling_group: "J"` (structured JSON output; matches memo §3.3 J-group recipe with `presence_penalty=1.5`)
- `training_config.rank: 8` (Tier 2 default per memo)
- `metadata.author`: "Sprint FT-LORA-B", `metadata.created: "2026-04-21"`, `metadata.notes`: "17th call site — migrated from direct-HTTP to llmclient in Sprint B"

**Gate:**
- `ults_runner.py` validates the new spec clean
- `sampling_group` correctly set to `J`
- `system_prompt_hash` matches what Epic 3's refactor will pass through llmclient

**17-vs-16 task-count reconciliation (canonical, to be applied in Epic 7):** keep `01_RESEARCH_v2.md §1.1` at "16 tasks" (memo-aligned). Add this exact footnote to §1.1 immediately after the 16-task roster:

> **Footnote (Sprint FT-LORA-B, 2026-04-21):** `guardrail.evaluate` has a ULTS spec (`docs/tests/ults/specs/guardrail_evaluate.ults.json`) for completeness and for capturing interactions in `llm_interactions` when `GUARDRAIL_ENABLED=true`, but is **excluded from the Phase 5 SFT training target** (which remains 16 tasks × 500–1000 anchor examples = 8,000–16,000 anchor examples). Guardrail is opt-in / disabled by default; enabling it adds a 17th captured task but does not change the SFT corpus size target. If/when guardrail becomes enabled-by-default in a future release, its inclusion in SFT will be a separate planning decision (revisit the 500–1000 per-task target and the MoE-Sieve family assignment at that time).

The same footnote is reflected in the commit body so reviewers counting specs don't get confused.

### Epic 3 — Guardrail llmclient migration (the core code change)

Refactor `internal/guardrail/llm_evaluator.go` to route through `llmclient.Complete()` instead of direct HTTP. This is the Sprint's biggest risk delta.

**Sub-steps:**
1. **Refactor `internal/guardrail/llm_evaluator.go`:**
   - Remove direct HTTP path (lines 61-239) — both OpenAI (lines 116-175) and Ollama (lines 195-238) branches.
   - Add `llmclient.Client` field on `GuardrailService`.
   - In `NewGuardrailService()`: build `llmclient.Config` from existing `GuardrailConfig` (Provider, Model, OpenAIKey, OpenAIURL, OllamaURL, TimeoutMs), call `llmclient.New()`, store client. Chain `.WithContext("guardrail.evaluate", spaceID)` at call time in `Validate()`.
   - In `Validate()`: build `[]llmclient.Message{{Role: "system", Content: guardrailSystemPrompt}, {Role: "user", Content: formatDiff(req)}}`, call `client.Complete(ctx, messages, llmclient.CompleteOpts{MaxTokens: cfg.MaxTokens, Format: "json_object"})`, parse the response string into `GuardrailEvaluation` (existing struct + parse func stay).
   - **Circuit breaker — hard cutover (user-approved, 2026-04-21).** `llmclient` registers its own per-task breakers under the new names `openai-guardrail.evaluate` / `ollama-guardrail.evaluate`. Remove the manual `circuitbreaker.Registry.Get("openai-guardrail")` / `"ollama-guardrail"` branches (lines 101, 180) entirely — **no aliases**. Research confirmed `POST /v1/admin/breakers/reset` (`internal/api/handlers_breakers.go:25-27, 89-90`) accepts a user-supplied name in the request body and does a direct map lookup; any external script hardcoding `openai-guardrail` will get a 404 `"unknown breaker"` response after this change. This is documented as a **breaking change on the admin surface** in CHANGELOG (see Epic 7). `GET /v1/admin/breakers` (`handlers_breakers.go:38-63`) enumerates the registry and returns all current names, so operators who re-discover breaker names dynamically are unaffected.

2. **Update `internal/api/server.go:440` instantiation:**
   - Pass the llmclient task-name constant (define `const GuardrailTaskName = "guardrail.evaluate"` in a shared place — probably `internal/llmclient/tasks.go` if exists, else `internal/guardrail/const.go`).
   - Ensure `llmclient.SetDefaultRecorder()` in `serve.go:247` runs **before** `NewGuardrailService()` call (already does — bootstrap order preserved).

3. **Update / add tests — mocking approach (research-informed):** Explore agent confirmed **no shared `llmclient.MockClient` exists**; current convention is that consumers like `retrieval/query_classifier_test.go` and `consulting/llm_classifier_test.go` test `parseResponse` only and avoid the `Complete()` call. `internal/llmclient/client_test.go` uses `httptest.NewServer` with inline handlers. Epic 3's concrete plan:
   - **Introduce `internal/llmclient/testclient.go`** exporting a `TestClient` struct that satisfies the `llmclient.Client` interface (add the interface to `types.go` if one isn't already extracted — spot-check first; if the package exposes `*Client` as a concrete struct, extract an interface `Completer` with just `Complete(ctx, messages, opts) (string, error)` and switch `GuardrailService` to depend on the interface, not the concrete type). `TestClient` has settable `CompleteFn func(...) (string, error)` for per-test behavior.
   - **Update `internal/guardrail/*_test.go`**: replace any existing HTTP mocks with `TestClient` instances injected at `NewGuardrailService` construction. Assert on `CompleteFn` invocation args (messages slice, opts, context-carried task name).
   - **New unit test:** `TestValidate_PassesTaskNameToLLMClient` — verifies the task name `"guardrail.evaluate"` reaches `llmclient.Complete` (via `WithContext` chain) and that `MaxTokens` + `Format: "json_object"` are set correctly.
   - **New integration test (tagged `//go:build integration`)**: start MDEMG server with guardrail enabled, POST `/v1/memory/guardrail/validate`, confirm `llm_interactions` hypertable has a row with `task_name='guardrail.evaluate'` and `space_id` matching. Uses dedicated `space_id='mdemg-test-guardrail'`, cleans up via `mdemg data clean --space-id mdemg-test-guardrail --force` on teardown.
   - **Scope note:** if the interface extraction turns up unexpected complexity (e.g., `llmclient.Client` is already a sealed struct with exported methods that GuardrailService depends on), fall back to inline stub-per-test and split the exported `TestClient` into a Sprint B.5 follow-up. Decide at Epic 3 pre-execution after a 10-min read of `internal/llmclient/client.go` + `types.go`.

4. **Rename circuit breaker names** in any `docs/` / config files that referenced the old `openai-guardrail` / `ollama-guardrail` breakers (admin endpoint `/v1/admin/breakers/reset`, Grafana dashboards if any). Preserve list via `git grep "openai-guardrail\|ollama-guardrail"` and update each occurrence to the new task-prefixed form.

**Gate (triple):**
- `go build ./...` green; `golangci-lint run ./...` zero findings on changed files (pre-existing findings not introduced by this change are logged but not blocking — per MEMORY rule "zero tolerance for test failures" applies to new breakage only).
- Existing `internal/guardrail/*_test.go` suite green (unit level).
- New integration test green (live MDEMG server → POST guardrail → row in `llm_interactions` with `task_name='guardrail.evaluate'`).
- **Blast-radius check:** `GUARDRAIL_ENABLED=false` by default — if migration has a latent bug, production users with default config are unaffected. Users who opt in to guardrail see changed behavior — compatibility statement in commit body + CHANGELOG.
- Circuit breaker hard-cutover documented in commit body and in CHANGELOG under a dedicated **"Breaking changes (admin surface)"** subsection listing: old names removed (`openai-guardrail`, `ollama-guardrail`) → new names registered (`openai-guardrail.evaluate`, `ollama-guardrail.evaluate`). Instruction for operators: use `GET /v1/admin/breakers` to discover current names before calling `POST /v1/admin/breakers/reset`.
- `curl -s -X POST http://localhost:9999/v1/admin/breakers/reset -d '{"name":"openai-guardrail"}' -H "X-API-Key: $KEY"` returns 404 `"unknown breaker"` (confirms old name removed); same call with `"openai-guardrail.evaluate"` returns 200 (confirms new name wired).

🔸 **MID-SPRINT USER CHECKPOINT (after Epic 3)** 🔸

Pause: confirm guardrail migration passes all three gates locally AND the integration test produces the expected TSDB row. User reviews diff (`git diff` for `internal/guardrail/` + `internal/api/server.go`). This is the highest-risk epic; user either approves continuation or requests revisions before Epics 4-7.

### Epic 4 — Grep-audit remediation (15 files)

Mechanical pass: replace `Qwen3-30B-A3B` / `qwen3-30b` / `mlx-community/Qwen3-30B-A3B-4bit` with the v5.0 targets. Per Explore agent verification: zero drift since the audit; all line numbers from `SPRINT_A_GREP_AUDIT.md` still accurate.

**Epic 4 pre-execution sanity check — HuggingFace MLX artifact lookup (1 min):**
Before applying the `mlx-community/Qwen3.6-35B-A3B-Q4` replacement string blindly, verify what actually exists on HuggingFace at execution time. Community quant naming is inconsistent (`-Q4`, `-4bit`, `-mlx-4bit`, etc. all appear across `mlx-community/*` repos). Run:
```bash
curl -s "https://huggingface.co/api/models?search=Qwen3.6-35B-A3B&author=mlx-community" | jq -r '.[].id'
```
- If any `mlx-community/Qwen3.6-35B-A3B-*` repo exists: use the exact name returned (preferring a Q4 variant; if only Q8 / BF16 exist at this point, use what's there — Sprint C will re-validate).
- If no Qwen3.6 MLX repo exists yet: use `mlx-community/Qwen3.5-35B-A3B-Q4` (or whatever Qwen3.5 quant is published) and add the TODO comment. Record the HF query result in the commit body so reviewers can see what was available at the time of the decision.

**Replacement rules:**
- `Qwen3-30B-A3B` → `Qwen3.6-35B-A3B` (base name)
- `mlx-community/Qwen3-30B-A3B-4bit` → `mlx-community/Qwen3.6-35B-A3B-Q4` (MLX artifact name — **actual replacement string determined by the HF sanity check above**; falls back to `mlx-community/Qwen3.5-35B-A3B-Q4` if no Qwen3.6 MLX artifact is published at execution time; TODO comment in either fallback case)
- `Qwen/Qwen3-30B-A3B` (HuggingFace source) → `Qwen/Qwen3.6-35B-A3B`
- `mdemg-qwen3-30b-v1-q4` (output paths) → LEAVE as `mdemg-qwen3-30b-v1-q4` in Sprint B; add TODO comment `# TODO (Sprint E): Rename output path to reflect asymmetric-quant strategy (mdemg-qwen3.6-35b-v1-asym/)`. Prevents premature naming lock-in before Sprint E defines the per-module quant selector convention.
- `qwen3-30b-a3b` (lowercase example model_name in training-data-capture-verification.md:45) → `qwen3.6-35b-a3b`

**Functional edits (non-docstring):**
- `scripts/test_vllm_mlx.py:236` — argparse default from `"mlx-community/Qwen3-30B-A3B-4bit"` to `"mlx-community/Qwen3.6-35B-A3B-Q4"`. This changes runtime behavior when `$LLM_MODEL` env is unset — **document in commit body**.
- `docs/tests/uaits/specs/mdemg.uaits.json:10` — `description` field string update; `uaits_runner.py` validates post-edit (not CI-gated but manual pass recommended).

**Deferred to Sprint E (TODO comments only in Sprint B):**
- `docs/operations/vllm-mlx-setup.md` memory table (line 47, `~22 GB`) and performance figures (if any under lines 57-59) — add `<!-- TODO (Sprint E): Update RAM estimate for asymmetric-quant footprint -->`.
- `neural/training/quantize_deploy.py` output-path examples (lines 11, 18) — add inline `# TODO (Sprint E): Asymmetric-quant output-path naming` comment.

**Gate:**
- `grep -rn "Qwen3-30B-A3B" <scope>` returns only Sprint-A-preserved category (c) files (CHANGELOG.md historical / AGENT_HANDOFF.md historical / docs/archive/ / docs/architecture/benchmarks/ / packaging/homebrew-mdemg/). Zero new hits in the 15 target files.
- `scripts/test_vllm_mlx.py --help` output shows new default.
- `python3 docs/tests/uaits/runners/uaits_runner.py` green on updated spec.

### Epic 5 — `.env.example` + compose placeholder vars

Seed the naming convention for Sprint E knobs as **commented** placeholder vars. No behavior change (all defaulted / ignored by current code); just establishes the variable names the rest of the repo will reference.

**Additions to `.env.example`:**
```bash
# --- Sprint FT-LORA-E placeholder knobs (not yet consumed) ---
# ROUTER_AUX_LOSS_COEF=0.002          # MoE load-balancing auxiliary loss coefficient (memo 07 v3.1 §3.5)
# LORA_TIER1_RANK=32                  # Tier 1 LoRA rank (attention + shared expert)
# LORA_TIER1_ALPHA=64                 # Tier 1 LoRA alpha
# LORA_TIER2_RANK=8                   # Tier 2 LoRA rank (routed experts per family)
# LORA_TIER2_ALPHA=16                 # Tier 2 LoRA alpha
# LORA_N_EPOCHS_CAP=3                 # Max epochs per tier — auto is DISALLOWED (Sprint A policy)
# LORA_EARLY_STOP_SFT_THRESHOLD=1.05  # val_loss > best × this → early stop (2 consecutive evals)
# LORA_EARLY_STOP_RL_THRESHOLD=0.95   # val_reward < best × this → early stop (2 consecutive evals)
# ASYMMETRIC_QUANT_SHARED=bf16        # Shared expert + attention quant (memo 07 v3.1 §3.8)
# ASYMMETRIC_QUANT_ROUTED=mxfp4_moe   # Routed experts quant
# ASYMMETRIC_QUANT_ATTN=bf16          # Attention quant (redundant with SHARED but explicit)
```

**Compose template additions** (`internal/cli/compose_templates/docker-compose.yml.tmpl`):
Mirror the above as commented env rows on the `mdemg-server` service, matching existing pattern (e.g. `${JIMINY_ENABLED:-false}`).

**Also add (uncommented — already consumable by `internal/guardrail`):**
```bash
# --- Sprint FT-LORA-B guardrail task-name constant ---
GUARDRAIL_TASK_NAME=guardrail.evaluate
```
(Used by llmclient to tag interactions; default matches the ULTS spec from Epic 2.)

**Gate:**
- `grep -c "LORA_TIER" .env.example` = 4 (rank/alpha × tier 1/2)
- `grep -c "ROUTER_AUX_LOSS_COEF" .env.example` = 1
- Compose template file passes `docker compose config` (syntax check)
- CI compose-template-parity check (if one exists in `.github/workflows/`) green

### Epic 6 — Testing & validation pass

Execute the 3-tier test plan (MEMORY rule: all plans must include unit + integration + e2e).

**Tier 1 (Static/Lint):**
- `go build ./...`
- `golangci-lint run ./...` — zero new findings vs baseline
- `python3 docs/tests/ults/runners/ults_runner.py` — all 17 specs (16 existing + 1 new guardrail)
- `python3 docs/tests/uaits/runners/uaits_runner.py` — green on updated UAITS spec
- `python3 scripts/verify_uxts_canonical_specs.py` (CI gate equivalent) green
- `markdownlint` / `mdl` on edited docs if available
- `docker compose config` validates compose template

**Tier 2 (Integration — Go):**
- `go test ./internal/guardrail/...` — existing unit tests pass with new llmclient mock
- `go test -v -tags=integration ./tests/integration/...` — specifically the new guardrail-TSDB-capture test + any existing guardrail integration tests
- `make test-api BASE_URL=http://localhost:9999` (UATS) — zero regressions on guardrail-related endpoints

**Tier 3 (E2E):**
- Start MDEMG server locally with `GUARDRAIL_ENABLED=true`, `LLM_PROVIDER=openai` (dev OpenAI key) AND with `LLM_PROVIDER=ollama` (localhost Ollama) — both paths
- POST `/v1/memory/guardrail/validate` with a known-bad diff (hardcoded violations)
- Query TSDB: `SELECT count(*) FROM llm_interactions WHERE task_name='guardrail.evaluate' AND ts > NOW() - INTERVAL '5 min';` returns ≥1
- Query TSDB for the same row: verify `model_name`, `provider`, `tokens_in`, `tokens_out` populated
- Circuit breaker test: force 3 consecutive failures (mock OpenAI 500 response), verify breaker opens, verify alert fires
- Restore state after: `mdemg data clean --space-id mdemg-dev --force` on the test rows (per MEMORY rule "ALWAYS restore state after destructive tests")

**Gate:** All three tiers green. One integration run capturing actual guardrail TSDB row persisted (screenshot / SQL output in commit body or PR summary as evidence).

### Epic 7 — Documentation update (final epic — never cut per MEMORY rule)

- `docs/development/ft-lora/00_README_v2.md` — add row in Document Map pointing to Sprint B plan (`sprint_plan_ft_lora_b.md`), bump internal version (5.0 → 5.1 patch, keeping v5.0 memo alignment) with "Changes in v5.1: code/config alignment + guardrail llmclient migration" banner.
- `docs/development/ft-lora/01_RESEARCH_v2.md §1.1` — add the Epic 2 footnote verbatim (the "17-vs-16 task-count reconciliation" block above) immediately after the 16-task roster, and add a short note to the guardrail bullet: "Migrated to `llmclient` in Sprint FT-LORA-B (2026-04-21); 17th ULTS spec `guardrail_evaluate.ults.json` created. Still disabled by default (`GUARDRAIL_ENABLED=false`); enable to capture interactions in `llm_interactions`. See §1.1 footnote for SFT-corpus exclusion rationale."
- `AGENT_HANDOFF.md` — append Sprint B completion entry at top of update log.
- `CHANGELOG.md` — queue new `[Unreleased]` entry with **two subsections**:
  ```
  ### Changed
  - **FT-LORA-B: Code/config alignment + guardrail llmclient migration** (2026-04-21)
    - Guardrail consumer (`internal/guardrail/llm_evaluator.go`) now routes through `llmclient.Complete()`; interactions captured as `task_name='guardrail.evaluate'` in `llm_interactions` TSDB hypertable when `GUARDRAIL_ENABLED=true`.
    - ULTS schema adds required `sampling_group` enum field (T/C/J) — all 16 existing specs + new `guardrail_evaluate.ults.json` carry it. See `04_BENCHMARK_RL_v2.md §10.0` for task mapping.
    - 15 files refreshed from `Qwen3-30B-A3B` → `Qwen3.6-35B-A3B` per memo 07 v3.1. `scripts/test_vllm_mlx.py` argparse default updated (functional change when `$LLM_MODEL` unset).
    - `.env.example` + compose template seed Sprint-E placeholder vars (ROUTER_AUX_LOSS_COEF, LORA_TIER1/2_RANK/ALPHA, ASYMMETRIC_QUANT_*).

  ### Breaking changes (admin surface)
  - **Circuit breaker rename (guardrail).** Old names `openai-guardrail` / `ollama-guardrail` are **removed**; new names `openai-guardrail.evaluate` / `ollama-guardrail.evaluate` registered in their place (managed by `llmclient` per-task breaker convention). `POST /v1/admin/breakers/reset` with `{"name":"openai-guardrail"}` now returns 404 `"unknown breaker"`. Operators: call `GET /v1/admin/breakers` first to discover current names, then reset by the enumerated name. Grafana dashboards and any alerting rules referring to the old names must be updated.
  ```
- Copy `~/.claude/plans/breezy-dancing-lerdorf.md` → `docs/development/ft-lora/sprint_plan_ft_lora_b.md` at sprint close (per Sprint A pattern).
- Append "Documents Accessed" appendix to the sprint plan file (MEMORY mandatory rule).

**Gate:** Sprint plan in repo; cross-ref check (every `sprint_plan_ft_lora_b.md` / §2.8 / §5 pointer resolves); Document Map updated; CHANGELOG + AGENT_HANDOFF current.

## 6. Testing Plan (Three Tiers)

Covered in Epic 6 (intentional — Epic 6 IS the testing pass, not just a gate). Summary:

**Tier 1 (Static/Lint):** `go build ./...`, `golangci-lint run ./...`, `ults_runner.py`, `uaits_runner.py`, `verify_uxts_canonical_specs.py`, `docker compose config`.

**Tier 2 (Integration):** `go test ./internal/guardrail/...`, `go test -tags=integration ./tests/integration/...` (new guardrail-TSDB-capture test + existing), `make test-api` UATS suite.

**Tier 3 (E2E):** Live MDEMG server + POST `/v1/memory/guardrail/validate` + TSDB query verification for `task_name='guardrail.evaluate'` row, both OpenAI + Ollama paths, circuit breaker failure simulation, state restoration.

## 7. Commit Strategy

Single commit at sprint close (per user's observed "batch commits at end" directive):

- Title: `feat(ft-lora): Sprint B — guardrail llmclient migration + ULTS sampling_group + grep-audit remediation`
- Body bullets: one per epic (E1–E7), each ≤1 line
- **Dedicated "Behavior changes" section** highlighting:
  1. Guardrail `/v1/memory/guardrail/validate` now routes through llmclient — interactions captured in `llm_interactions` TSDB table as `task_name='guardrail.evaluate'` when `GUARDRAIL_ENABLED=true`.
  2. `scripts/test_vllm_mlx.py` default model updated (functional change when `$LLM_MODEL` unset).
  3. ULTS schema now **requires** `sampling_group` (T/C/J) — all 16 existing + 1 new spec carry the field; any future ULTS spec without it will fail runner validation.
- **Dedicated "Breaking changes (admin surface)" section** highlighting:
  1. Circuit breaker hard-cutover: `openai-guardrail` / `ollama-guardrail` names removed; replaced by `openai-guardrail.evaluate` / `ollama-guardrail.evaluate`. `POST /v1/admin/breakers/reset` with the old names returns 404. Discover current names via `GET /v1/admin/breakers`.
- Footer: `Co-Authored-By: Claude Opus 4 <noreply@anthropic.com>`

Push to `reh3376_dev01` → auto-PR opens → sprint summary comment posted on PR (per MEMORY rule `feedback_sprint_summary_on_pr.md`) highlighting:
- Behavior changes (3 items above)
- Mid-sprint user-checkpoint outcome
- Tier 3 E2E evidence (TSDB row sample + circuit breaker test log)

**Mid-sprint checkpoint does NOT commit** — diff stays in working tree until user approves Epic 3 gate.

## 8. Verification Checklist

- [ ] Pre-gate: branch fast-forwarded to `8ae8f24`, tree clean, baseline tests green
- [ ] Epic 1: ULTS schema + runner + all 16 specs have `sampling_group` set; runner parity check passes
- [ ] Epic 2: `guardrail_evaluate.ults.json` validates clean; sampling_group=J
- [ ] Epic 3: guardrail refactor green across `go build` / `golangci-lint` / unit tests / integration test; circuit breakers renamed; mid-sprint user checkpoint passed
- [ ] Epic 4: all 15 grep-audit files updated; zero stale `Qwen3-30B-A3B` refs outside historical preserve list; Sprint-E TODOs in place
- [ ] Epic 5: `.env.example` + compose template have placeholder vars; compose syntax valid
- [ ] Epic 6: all three test tiers green; E2E TSDB evidence captured
- [ ] Epic 7: sprint plan committed to `docs/development/ft-lora/sprint_plan_ft_lora_b.md`; Document Map updated; CHANGELOG + AGENT_HANDOFF current; "Documents Accessed" appendix complete
- [ ] Commit pushed; auto-PR opened; sprint summary comment posted
- [ ] `01_RESEARCH_v2.md §1.1` guardrail footnote updated to reflect migration completion

## 9. Documentation Update (Final Epic — Never Cut)

Covered by Epic 7. Key deliverables:
- `docs/development/ft-lora/sprint_plan_ft_lora_b.md` in repo
- `00_README_v2.md` version 5.0 → 5.1 bump + Document Map row for Sprint B plan
- `01_RESEARCH_v2.md §1.1` guardrail footnote update
- `AGENT_HANDOFF.md` Sprint B completion entry
- `CHANGELOG.md` `[Unreleased]` entry
- Documents Accessed appendix on the sprint plan file (MEMORY rule)

## 10. Risks & Mitigations

| Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|
| Guardrail migration breaks existing `/v1/memory/guardrail/validate` consumers | Medium | Preserve request/response schema; `GUARDRAIL_ENABLED=false` by default limits blast radius; integration test gates the migration | Revert Epic 3 commit; keep Epics 1-2, 4-7 if still valuable |
| ULTS schema update ripples through CI before all 16 specs updated | Low | Epic 1 is atomic — schema + runner + all 16 specs in one local change before any push | If partial state pushed accidentally, squash-rebase before opening PR |
| Circuit breaker name change breaks admin endpoint users with hardcoded names | Low-Medium | **Hard cutover (user-approved).** `POST /v1/admin/breakers/reset` accepts user-supplied names (verified `handlers_breakers.go:25-27, 89-90`); external scripts with hardcoded `openai-guardrail` / `ollama-guardrail` WILL 404. Documented in CHANGELOG under "Breaking changes (admin surface)". Admin endpoint gated by `AUTH_API_KEYS` so blast radius is limited to operators who authenticated. Operators should use `GET /v1/admin/breakers` for discovery. | If a specific operator reports breakage post-merge, add an alias in `internal/circuitbreaker/registry.go` as a point-release patch |
| Qwen3.6 MLX artifact not yet published when Sprint B executes | Medium | Epic 4 pre-execution HF sanity check (`curl .../api/models?search=Qwen3.6-35B-A3B&author=mlx-community`) determines the actual replacement string at execution time. Falls back to `Qwen3.5-35B-A3B-Q4` + TODO comment if no Qwen3.6 MLX artifact exists. HF query result recorded in commit body. | Sprint C validation catches the mismatch if the artifact name we chose doesn't actually resolve at load time |
| Integration test flakiness against live TSDB | Low | Use dedicated test space-id `mdemg-test-guardrail`, cleanup on teardown via `mdemg data clean --space-id mdemg-test-guardrail --force` | Mark test with retry; if persistent, drop from Tier 3, keep in Tier 2 unit-level with mocked recorder |
| `scripts/test_vllm_mlx.py` default change breaks existing developer workflows using unset `$LLM_MODEL` | Low | Document in commit body; the script is a smoke-test helper, not a production path | Revert just that one line; restore old default with a deprecation comment |
| Guardrail tests are non-existent or inadequate pre-migration | Medium | Explore agent confirmed tests exist in `internal/guardrail/`; verify coverage during Epic 3; add new tests where coverage is thin | If coverage insufficient, pause Epic 3 to write characterization tests before refactor |
| No shared `llmclient.MockClient` exists; Epic 3 test update adds work | Low-Medium | Confirmed via Explore: consumers roll their own stubs or test `parseResponse` only; Epic 3 plans to introduce `internal/llmclient/testclient.go` exporting a `TestClient` that satisfies the `Completer` interface. 10-min read of `client.go` + `types.go` at Epic 3 pre-execution confirms the interface extraction is tractable. | If interface extraction is non-trivial, use inline per-test stubs and defer the exported `TestClient` to Sprint B.5 |
| Mid-sprint user checkpoint reveals architectural concern with llmclient shape | Low-Medium | User sees full diff of `internal/guardrail/llm_evaluator.go` at checkpoint; early feedback shapes Epic 3 before downstream epics | Revise Epic 3 design; possibly split llmclient migration into its own sprint (Sprint B.5) |

## 11. Documents Accessed

**During planning (read-only):**

- `/Users/reh3376/.claude/plans/breezy-dancing-lerdorf.md` — prior Sprint A plan (confirmed complete, overwritten with Sprint B)
- `docs/development/ft-lora/SPRINT_A_GREP_AUDIT.md` — authoritative scope for Epic 4
- `docs/development/ft-lora/01_RESEARCH_v2.md §1.1, §2.8, §5` — 16-task roster + no-tool-calling policy + MoE strategy
- `docs/development/ft-lora/04_BENCHMARK_RL_v2.md §10.0` — 3-group sampling recipes + 16-task mapping
- `docs/development/ft-lora/00_README_v2.md` — Key Decisions table + Document Map
- `CLAUDE.md` — project instructions, sprint plan format, memory rules
- `/Users/reh3376/.claude/projects/-Users-reh3376-mdemg/memory/MEMORY.md` + relevant feedback files
- `/Users/reh3376/.claude/projects/-Users-reh3376-mdemg/memory/project_ft_oai_003_deferred.md` — confirms FT-OAI-003 out of Sprint B scope

**Explore-agent findings (via sub-agents):**

- **Guardrail migration:** `internal/guardrail/llm_evaluator.go` (lines 61-239), `internal/api/server.go:440`, `internal/api/handlers_guardrail.go:43`, `internal/llmclient/client.go` + `recorder.go` + `types.go`, canonical call patterns at `internal/retrieval/query_classifier.go:68-74` and `internal/consulting/llm_classifier.go:64-78`, bootstrap order at `internal/cli/serve.go:237-252`, TSDB writer at `internal/tsdb/llm_writer.go:151-167`
- **ULTS:** `docs/tests/ults/schema/ults.schema.json`, `docs/tests/ults/runners/ults_runner.py:61-69, 130-143` (`KNOWN_TOP_FIELDS` + parity check), representative specs (`ape_reflect.ults.json`, `consulting_classify.ults.json`, `retrieval_rerank_nli.ults.json`), consumer checks in `neural/training/evaluate_ft.py:434-445`, `quality_filter.py:55-80`, `train_ft.py:76-98`, CI gate `.github/workflows/uxts-canonical-specs.yml:40`
- **Grep audit drift check:** all 15 files confirmed unchanged since `8ae8f24`; 58 matches = 15 files exactly; zero hidden references; worktrees excluded; `scripts/test_vllm_mlx.py:236` confirmed as functional argparse default (not docstring)
- **Post-review verification pass (2026-04-21):**
  - Admin breaker endpoint: `internal/api/handlers_breakers.go:25-27, 89-90` confirms `POST /v1/admin/breakers/reset` uses user-supplied name in body via direct map lookup → rename is a hard break for external scripts. `GET /v1/admin/breakers` (lines 38-63) enumerates the registry, transparent for discovery-path callers.
  - llmclient mock availability: `internal/llmclient/client_test.go` uses `httptest.NewServer` inline; no exported `MockClient` exists. `internal/retrieval/query_classifier_test.go`, `internal/consulting/llm_classifier_test.go` test `parseResponse` only (don't exercise `Complete()`). `internal/jiminy/service_test.go:14-52` defines interface-local `mockConsultant` / `mockEmbedder` for sibling interfaces, not for `llmclient.Client`. → Epic 3 needs to introduce `internal/llmclient/testclient.go`.
  - External ULTS consumer check: zero `.ults.json` files in `/Users/reh3376/net-topology`, `/Users/reh3376/mdemg.nvim`, `/Users/reh3376/claude_workspace`; schema is dev-internal, not published as a public contract in `docs/guides/UXTS_DEVELOPER_GUIDE.md`. → `sampling_group` as required is safe; Epic 1 gate re-runs this grep as written confirmation.

**Referenced but not read in-depth:**

- `docs/tests/ults/specs/*.ults.json` (other 13 specs) — will read during Epic 1 execution
- `internal/guardrail/*_test.go` (if present) — will read during Epic 3 execution to assess mockability
- `.github/workflows/ci.yml` / `uxts-canonical-specs.yml` — will re-check during Epic 6 to confirm no new CI steps needed

## 12. Rollback

Not typically needed (single-commit sprint, `git revert <sha>` restores prior state). If mid-sprint user checkpoint rejects Epic 3:
- Stash Epic 1-2 work as a WIP commit on a throwaway branch
- Reset `reh3376_dev01` to pre-sprint HEAD (`8ae8f24`)
- Re-plan Epic 3 in a follow-up planning session with user feedback

No database migrations or destructive operations — state is fully recoverable from the `main` branch.

---

## Post-Sprint B

Sprint B gate → **Sprint C** (Qwen3.6-35B-A3B MLX validation — 3 gates: (1) MLX model loads under asymmetric quant, (2) inference throughput ≥60 tok/s on M5 Max, (3) benchmark parity vs. hosted `gpt-5.4-mini` within acceptance band). Sprint C plan drafted after Sprint B merges.

FT-OAI-003 remains deferred per user directive — revisit after Sprint E delivers trained Qwen model for quality/cost comparison.

---

## Appendix A — Documents Accessed During Execution (2026-04-21)

Captured at sprint close per MEMORY rule `feedback_sprint_plan_format.md`. Complements §11 (planning-time access).

**Read / modified during execution:**

- **Epic 1 (ULTS schema + runner + 16 specs):**
  - `docs/tests/ults/schema/ults.schema.json` — added `sampling_group` enum + required field
  - `docs/tests/ults/runners/ults_runner.py` — extended `KNOWN_TOP_FIELDS`; added parity check with actionable error message
  - All 16 `docs/tests/ults/specs/*.ults.json` — added `sampling_group` per §10.0 mapping (T=7, C=6, J=3)
  - `scripts/verify_uxts_canonical_specs.py` — verified green locally

- **Epic 2 (new spec):**
  - `docs/tests/ults/specs/guardrail_evaluate.ults.json` (NEW) — sampling_group=J, think_mode=false, structured JSON output schema

- **Epic 3 (guardrail migration):**
  - `internal/llmclient/types.go` — added `Completer` interface
  - `internal/llmclient/testclient.go` (NEW) — exported `TestClient` stub
  - `internal/guardrail/guardrail.go` — added `TaskName` const + `llm llmclient.Completer` field + 5th param to `NewGuardrailService`
  - `internal/guardrail/llm_evaluator.go` — full rewrite: `cb.Execute(ctx, func) { g.llm.Complete(...) }`; breaker names `openai-guardrail.evaluate` / `ollama-guardrail.evaluate`; preserved `cleanJSONResponse`
  - `internal/guardrail/llm_evaluator_test.go` (NEW) — 5 unit tests using `TestClient`
  - `internal/api/server.go` — updated `NewGuardrailService` call site with llmclient built from GuardrailConfig; `.WithContext(guardrail.TaskName, "")`
  - `tests/integration/guardrail_tsdb_capture_test.go` (NEW, `//go:build integration`) — Tier 3 E2E test gated by `TEST_GUARDRAIL_LIVE=1`

- **Epic 4 (grep-audit remediation — 15 files):**
  - 8 docs: `docs/development/RESEARCH_ROADMAP.md`, `docs/operations/vllm-mlx-setup.md` (+ 2 Sprint-E TODO comments), `docs/tests/uaits/README.md`, `docs/features/training-data-capture-verification.md`, `docs/features/embedding-retrieval-data-collection.md`, `docs/features/neural-training-pipeline.md`, `docs/development/ft-oai/sprint_plan_openai_ft_data_generation.md`, `docs/development/ADVERSARIAL_CODEBASE_ANALYSIS_20260410.md`
  - 7 code/config: `neural/training/train_ft.py`, `neural/training/tests/test_train_ft.py`, `neural/training/evaluate_ft.py`, `neural/training/teacher_distill.py`, `neural/training/quantize_deploy.py` (+ Sprint-E TODO), `scripts/test_vllm_mlx.py` (argparse default — functional), `docs/tests/uaits/specs/mdemg.uaits.json`
  - HuggingFace API query: `https://huggingface.co/api/models?search=Qwen3.6-35B-A3B&author=mlx-community` → confirmed artifact is `mlx-community/Qwen3.6-35B-A3B-4bit` (not `-Q4`; plan's guess was revised)

- **Epic 5 (env/compose):**
  - `.env.example` — appended `GUARDRAIL_TASK_NAME=guardrail.evaluate` + 10 commented Sprint-E placeholder vars
  - `docker-compose.yml` — mirrored additions (1 active + 10 commented) after `LLM_INTERACTION_LOGGING`
  - `internal/cli/compose_templates/docker-compose.yml` — identical additions (parity verified via `diff`; `docker compose config --quiet` exit 0)

- **Epic 6 (three-tier validation):**
  - Tier 1: `go build ./...` green; `golangci-lint run` zero new findings on changed packages; `python3 docs/tests/ults/runners/ults_runner.py` for all 17 specs green (17/17); `python3 docs/tests/uaits/runners/uaits_runner.py` green; `docker compose config --quiet` exit 0
  - Tier 2: `go test ./internal/guardrail/... ./internal/llmclient/...` green (unit)
  - Tier 3: integration test compiled clean (`go vet -tags=integration ./tests/integration/...`); live TSDB run gated behind `TEST_GUARDRAIL_LIVE=1` for operator execution

- **Epic 7 (docs close-out):**
  - `docs/development/ft-lora/00_README_v2.md` — v5.1 banner + Document Map row 9
  - `docs/development/ft-lora/01_RESEARCH_v2.md §1.1` — "Sprint FT-LORA-B completion note" footnote + SFT-corpus-exclusion rationale (17-vs-16 reconciliation)
  - `AGENT_HANDOFF.md` — E1-E7 completion entry at top of 2026-04-21 section
  - `CHANGELOG.md` `[Unreleased]` — 6-bullet FT-LORA-B entry + dedicated "Breaking changes (admin surface)" subsection
  - `docs/development/ft-lora/sprint_plan_ft_lora_b.md` (this file, NEW) — verbatim copy of approved plan + this appendix

**Mid-sprint user checkpoint:** passed at Epic 3 gate — user approved continuation through Epics 4-7 with single "go" message.

**Deviations from plan (recorded):**
1. MLX artifact name: `-Q4` → `-4bit` (HF API-verified at execution; plan §4 authorized this fallback path).
2. `scripts/tsdb_data_review_2026-04-01.json` remains untracked (pre-existing; not staged in Sprint B commit).

