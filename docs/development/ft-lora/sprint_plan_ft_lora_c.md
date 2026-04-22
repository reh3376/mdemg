# Sprint FT-LORA-C — Qwen3.6-35B-A3B MLX Validation (Runbook, Planning-Only)

## Context

Sprint B (merged 2026-04-21 as PR #336, commit `101cacb`) executed the code/config alignment that Sprint A queued: ULTS `sampling_group` enum (T/C/J) on all 17 specs, guardrail `llmclient` migration (17th captured call site `guardrail.evaluate`), 15 grep-audit files refreshed to `Qwen3.6-35B-A3B`, and Sprint-E placeholder env/compose knobs. Sprint C is the **first sprint that touches the actual Qwen3.6-35B-A3B model** — it is a **validation runbook**, not an execution sprint.

**Critical scope decision:** Sprint C ships a committable runbook document. It does **not** download the model, run MLX, run benchmarks, or capture any live numbers. The runbook's consumer is a **future Claude Code session** that may be invoked days or weeks after Sprint C merges, possibly from a cold cache with no prior conversational context. Every step is copy-pasteable. Every gate is resumable from disk-persisted state.

**Three gates** — each gated by disk stamps, independently resumable across sessions:
1. **Gate 1 — MLX asymmetric-quant load.** Model loads under the memo 07 v3.1 §3.8 asymmetric scheme (shared + attention BF16, routed experts MXFP4_MOE). Fail → halt FT-LORA line on Qwen3.6; replan as **Sprint C'** on Qwen3.5-35B-A3B.
2. **Gate 2 — Structured output / sampling recipe correctness.** ≥95% JSON validity on 100 synthetic J-group prompts at the J recipe (temp=0.7, top_p=0.95, top_k=20, presence_penalty=1.5, **max_tokens=4096** — deliberate deviation from memo §3.3's production 2048 to isolate sampling-param effects from truncation effects; post-sweep supplementary 20-prompt re-test at max_tokens=2048 records a `production_config_truncation_rate` for Sprint E memo-revision review). On default-recipe miss, run a 12-cell sampling-param sweep before halting.
3. **Gate 3 — Throughput AND benchmark parity vs hosted `gpt-5.4-mini`.** Throughput sub-gate ≥60 tok/s on M5 Max; quality sub-gate is three explicit bands (clear pass / middle / catastrophic). Middle band defers the commit-or-fallback decision to a **new Sprint F** (introduced in §Post-Sprint).

**Zero training spend** (same as Sprint B — $0 budget, no FT jobs, no paid baseline replays beyond what Gate 3 explicitly authorizes).

**Non-continuous-execution design (user-imposed constraint #3):** Each gate persists pass/fail/artifacts/timestamps under `~/.mdemg-sprint-c/gateN/` with a stamp file, log file, and evidence directory. A future session reads these stamps to determine "has this gate already run?" before re-executing. The runbook's very first step on every gate is **resume-check** — if a pass stamp exists and the model hash matches the recorded hash, skip to the next gate.

**Sprint chain:** A (done, #335) → B (done, #336) → **C (this — runbook)** → D (expert profiling) → E (training infra patches) → SFT execution → **F (commit-or-fallback checkpoint — NEW, registered in Post-Sprint)**.

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | FT-LORA-C |
| Title | Qwen3.6-35B-A3B MLX Validation Runbook (Planning-Only) |
| Date | 2026-04-21 (plan) |
| Branch | `reh3376_dev01` |
| Predecessors | FT-LORA-B (PR #336, merged `101cacb`) |
| Successors | FT-LORA-D (expert activation profiling) — gated on Gate 1 + 2 + 3-throughput pass; Gate 3-quality result feeds Sprint F |
| Type | Planning document (runbook) |
| Risk | Low for sprint itself (no code execution); Medium-High for future execution session (live MLX, live benchmark, baseline-drift exposure) |
| Budget | $0 (no downloads, no model runs, no baseline API spend in Sprint C itself; execution-time baseline spend capped under §Dependencies) |
| Estimate | 1-2 days (planning + mid-sprint checkpoint + commit) |

## 2. Problem Statement

Sprint B landed the alignment layer but left the actual model validation untouched — we have never loaded Qwen3.6-35B-A3B under the memo 07 v3.1 asymmetric-quant scheme, never measured JSON validity on J-group recipes against a live Qwen3.6, and never measured throughput or quality parity on M5 Max. All three unknowns are blockers for Phase 5 SFT because:

- If asymmetric-quant load fails, the entire MoE-Sieve strategy (memo §5) collapses — Tier 1 merge-then-quantize and Tier 2 per-family quantization both assume the convert path works. We need to know this **before** Sprint E begins patching `mlx_lm.convert` selectors.
- If J-group JSON validity is <95% at the canonical recipe and cannot be recovered by `presence_penalty` tuning, then either (a) Qwen3.6 cannot structurally produce the outputs MDEMG requires, or (b) our sampling recipe table in memo §3.3 is wrong. Either finding reshapes Sprint E's evaluation harness.
- If throughput is <60 tok/s, MDEMG cannot meet its runtime SLOs on local inference — the entire "switch to local Qwen3.6" plan fails economically.
- If quality vs `gpt-5.4-mini` is catastrophically behind (>30% below baseline), fine-tuning a structurally-weaker base is a bad investment — we halt.

Without a runbook that a future session can execute autonomously, each of these questions would require a fresh planning pass. The runbook **is** the deliverable.

**Additional forcing function:** the plan must survive **week-long pauses between gates**. A future session starting at Gate 2 must not need to re-read Sprints A/B or re-derive hardware assumptions — every assumption is inlined. The "Hardware prerequisites" and "Resume protocol" sub-sections exist for this purpose.

## 3. Scope & Constraints

**In scope:**

| # | Workstream | Files |
|---|---|---|
| 1 | Sprint C plan file (the runbook itself) | `docs/development/ft-lora/sprint_plan_ft_lora_c.md` (NEW) |
| 2 | Sprint C close-out doc updates | `docs/development/ft-lora/00_README_v2.md` (Document Map row 10, v5.2 patch note), `AGENT_HANDOFF.md`, `CHANGELOG.md` |
| 3 | Sprint F registration (skeleton only — NOT a detailed plan) | Addition in §Post-Sprint of this file, plus 1-row update in `00_README_v2.md` Sprint Plan table |

**Out of scope (explicitly deferred):**

- Any actual MLX model download, load, or run — **execution time**, not Sprint C.
- Authorship of runbook helper scripts (Gate 2 JSON-validity checker, Gate 3 throughput meter) — described in the runbook; **author at execution time** per user constraint #"Do NOT write any code/runbook helper scripts to disk".
- `mlx_lm.convert` per-module-class selector patches — **Sprint E**. Gate 1's asymmetric-quant load either works against the **published** `mlx-community/Qwen3.6-35B-A3B-mxfp4-moe` artifact (if HF confirms one exists at execution time) OR falls back to the symmetric `mlx-community/Qwen3.6-35B-A3B-4bit` with a **recorded deviation** and a Sprint E re-test marker. The runbook specifies both paths.
- Sprint F detailed plan content — **Sprint F itself** drafts that. Sprint C only **registers** Sprint F as the post-SFT commit-or-fallback checkpoint.
- Sprint D (expert activation profiling) and Sprint E (training infra) plans — unchanged.
- FT-OAI-003 — remains deferred per `project_ft_oai_003_deferred.md`.

**Constraints:**

- **Sequential epics** (MEMORY rule `feedback_sequential_epics.md`). Five epics below, executed in order.
- **Single batched commit at sprint close** — `feat(ft-lora): Sprint C — Qwen3.6-35B-A3B MLX validation runbook (planning-only)`.
- **Mid-sprint user checkpoint** after Epic 3 (runbook draft complete, before Epics 4-5) — user reviews the full runbook before commit, Sprint A/B pattern.
- **Planning-only** — zero model downloads, zero MLX runs, zero benchmark executions within this sprint.
- **Runbook self-contained** — no step says "run the benchmark"; every step is a copy-pasteable command with exact flags, exact file paths (absolute), exact environment variables, and exact pass/fail numeric thresholds. No qualitative language in acceptance criteria.
- **Non-continuous execution** — runbook must survive week-long pauses between gates. Each gate's first action is a resume-check against `~/.mdemg-sprint-c/gateN/` stamp files.
- **Numeric acceptance bands only** — every pass/fail is a specific number. Quality bands: within 10% of baseline = pass, 10-30% below = middle (defer to Sprint F post-SFT), >30% below = halt. Justification in §Risks.
- **Hard cutover of Sprint chain on failure** — Gate 1 fail → halt Qwen3.6, replan as Sprint C' on Qwen3.5. Gate 2 hard fail (sweep cannot reach ≥95%) → halt. Gate 3 throughput <60 → halt. Gate 3 quality catastrophic → halt. Halts always imply a follow-up planning sprint; they do not imply a code rollback (no code changes in Sprint C).

## 4. Dependencies

- Merged `101cacb` (PR #336) on `main` — Sprint B canonical state including `sampling_group` on all 17 ULTS specs and guardrail `task_name='guardrail.evaluate'` llmclient migration.
- **Hardware (execution-time prerequisite, not Sprint C prerequisite):** Apple M5 Max, 128GB unified memory, macOS (Darwin 25.3.0 per current `uname -a` on user's machine), ≥200GB free on internal SSD (model cache + logs + evidence artifacts).
- **Software (execution-time):**
  - Python 3.11+ (current repo default).
  - `mlx-lm` (via `uv tool install mlx-lm` or `pip install mlx-lm`). **Version pinned at execution-time Epic 1 pre-gate** — runbook step captures `mlx_lm --version` output to the cross-gate `versions.json`. If the Sprint E MR adds a pin to `requirements.txt` before Sprint C runs, runbook reads that pin; if not, runbook pins the observed version into the evidence file.
  - `vllm-mlx` (per `docs/operations/vllm-mlx-setup.md` — `uv tool install git+https://github.com/waybarrios/vllm-mlx.git`). Version captured same way.
  - `huggingface-cli` for downloads and API queries.
  - `jq` for JSON manipulation (commonly already installed; runbook's resume-check asserts it).
- **Hosted API access (Gate 3 only):** `gpt-5.4-mini` via user's existing `OPENAI_API_KEY`. **Hard budget cap: $25 for the Gate 3 baseline capture** (covers the 120-question benchmark × 1 run at typical `gpt-5.4-mini` rates + a ≤20% headroom for token-count estimation error). The runbook enforces this by (a) estimating cost pre-run via `tiktoken` on the prompt set and halting with a user-approval prompt if the estimate exceeds $25, and (b) tracking cumulative spend in `~/.mdemg-sprint-c/gate3/openai_cost.json` during execution and aborting if the running total crosses $25 regardless of remaining questions.
- **24h same-window constraint (Gate 3 baseline ↔ Qwen3.6 runs):** the hosted `gpt-5.4-mini` baseline capture and the Qwen3.6 benchmark pass **must complete within a rolling 24h window** of each other to avoid OpenAI model-revision drift contaminating the quality-gap measurement. Runbook step at Gate 3 start reads the prior baseline stamp (if any) and: (a) if baseline `timestamp_iso8601` is ≤24h old and `openai_model_id` matches, reuse it; (b) else re-capture the baseline fresh. Qwen3.6 benchmark stamp records a `baseline_age_hours` field; >24h at comparison time = re-run baseline before computing the gap. Gate 3 remains resumable across arbitrary pauses *between* gates (week-long inter-gate pauses are fine), but the two halves of Gate 3 itself (baseline + Qwen runs) must pair up within a 24h rolling window; if the pause between them exceeds 24h, resume = re-capture baseline before computing the gap.
- **Benchmark artifacts:** `docs/architecture/benchmarks/run_benchmark_v4.py`, `docs/architecture/benchmarks/grader_v4.py`, `docs/architecture/benchmarks/whk-wms/test_questions_120.json` (note: the actual benchmark questions live under `whk-wms/`, not `docs/benchmarks/` — runbook references the verified path).
- **Smoke test:** `scripts/test_vllm_mlx.py` — already points at `mlx-community/Qwen3.6-35B-A3B-4bit` by default after Sprint B. Used as warm-up in Gate 2 and as the per-task driver for Gate 2's synthetic J-prompts.
- **Source-of-truth docs (runbook links to these, not copies them):** `01_RESEARCH_v2.md §2.8` (no-tool-calling), `§3` (model), `§5` (MoE-Sieve), `04_BENCHMARK_RL_v2.md §10.0` (sampling recipes + 16-task group mapping), `02_M5MAX_HARDWARE_v2.md §2-3` (memory budget, throughput target).
- **No external dependency version bumps in Sprint C itself** (planning-only).

## 5. Implementation Plan (Sequential Epics + Gates)

**Pre-gate:** confirm branch state `reh3376_dev01` fast-forwarded to `101cacb`; working tree clean (`scripts/tsdb_data_review_2026-04-01.json` untracked file ignored per Sprint B); no existing `docs/development/ft-lora/sprint_plan_ft_lora_c.md` in the repo (would indicate a prior attempt; stop and reconcile before proceeding).

### Epic 1 — Runbook skeleton + cross-gate state directory spec

Author sections 1-4 (Header, Problem, Scope, Dependencies) and the cross-gate state spec (§Resume Protocol below) of the runbook file. Goal: lock the runbook's metadata and execution-environment assumptions before writing gate bodies, so Epics 2/3 can reference a stable §Resume Protocol.

**Sub-steps:**
1. Create `docs/development/ft-lora/sprint_plan_ft_lora_c.md` with the 12-section v1.0 format header, sections 1-4 populated, and a `§Resume Protocol` pre-amble that specifies:
   - **Cross-gate state directory:** `~/.mdemg-sprint-c/` (absolute path expanded at runtime).
   - **Subdirectory layout:**
     ```
     ~/.mdemg-sprint-c/
     ├── versions.json              # mlx_lm, vllm-mlx, huggingface_hub, mdemg git SHA, macOS, python
     ├── model.json                 # HF repo, SHA256 of config.json, download timestamp, disk path
     ├── gate1/
     │   ├── passed_<ISO8601>.json      # EXISTS only on pass; contains measured values
     │   ├── failed_<ISO8601>.json      # EXISTS only on fail; contains failure reason + captured values
     │   ├── load.log                    # full stdout/stderr of mlx_lm load attempt
     │   └── ram_footprint.json          # peak RSS / VM snapshot
     ├── gate2/
     │   ├── passed_<ISO8601>.json
     │   ├── failed_<ISO8601>.json
     │   ├── prompts_100.jsonl           # synthetic J-group prompts used (fixed seed)
     │   ├── responses_default.jsonl    # responses at canonical J recipe
     │   ├── sweep_matrix/              # per-cell response sets if sweep needed
     │   │   └── pp<value>_tp<value>.jsonl
     │   └── validity_report.json       # per-prompt JSON validity verdict + summary stats
     └── gate3/
         ├── passed_<ISO8601>.json
         ├── failed_<ISO8601>.json
         ├── middle_band_<ISO8601>.json  # EXISTS only on middle-band quality result
         ├── throughput.json             # warm-up + measurement tokens, elapsed, tok/s
         ├── qwen_answers.jsonl          # local Qwen3.6 answers to test_questions_120
         ├── gpt54mini_answers.jsonl     # fresh baseline capture (same wall-clock window)
         ├── grades_qwen.json            # grader_v4 output
         └── grades_gpt54mini.json       # grader_v4 output
     ```
   - **Stamp file schema** (per-gate `passed_*.json`):
     ```json
     {
       "gate": "gate1" | "gate2" | "gate3",
       "result": "pass" | "fail" | "middle_band",
       "timestamp_iso8601": "...",
       "model_sha256_config": "...",
       "mlx_lm_version": "...",
       "vllm_mlx_version": "...",
       "mdemg_git_sha": "...",
       "measured_values": { /* gate-specific — see each gate's "acceptance criteria" block */ },
       "notes": "optional free-form; deviations recorded here"
     }
     ```
   - **Resume check (every gate's first action):** `jq -e` over the stamp file to assert `.result == "pass"` AND `.model_sha256_config == <current model config SHA>`. On match → skip this gate and proceed. On mismatch (different model SHA) → re-run this gate. On no stamp → run this gate. On `"fail"` stamp → consult §Risks for halt-vs-replan disposition and stop until user acknowledges.
   - **Never delete stamps automatically** — human-only via `rm ~/.mdemg-sprint-c/gateN/*.json` with explicit user go-ahead. (This protects against accidental re-execution after a previous halt.)
2. Inline hardware/software prerequisites table so the runbook is self-sufficient — a future session does not need to open `02_M5MAX_HARDWARE_v2.md` to start:
   - M5 Max, 128GB unified memory; ≥200GB free internal SSD.
   - macOS version: capture `sw_vers -productVersion` into `versions.json` at first execution.
   - Python: `python3 --version` into `versions.json`.
   - `mlx_lm`: `python3 -c 'import mlx_lm; print(mlx_lm.__version__)'` into `versions.json`.
   - `vllm-mlx`: `vllm-mlx --version` into `versions.json`.
   - `huggingface_hub`: `python3 -c 'import huggingface_hub; print(huggingface_hub.__version__)'` into `versions.json`.
   - mdemg repo SHA: `git -C /Users/reh3376/mdemg rev-parse HEAD` into `versions.json`.

**Gate (Epic 1 self-check):**
- File exists at `docs/development/ft-lora/sprint_plan_ft_lora_c.md` with sections 1-4 + §Resume Protocol authored.
- All placeholder numbers from user constraint #4 (10%/30% quality bands, 60 tok/s throughput, 95% JSON validity, 100 synthetic prompts) appear in §Scope/§Dependencies and are consistent with values used later in gate bodies.
- No hidden dependencies on external files — anyone reading the file standalone can identify the pre-execution environment check.

### Epic 2 — Gate 1 runbook (asymmetric-quant MLX load)

Author the Gate 1 body in the runbook file. Gate 1 answers: **can the memo 07 v3.1 §3.8 asymmetric-quant scheme (shared + attention BF16, routed experts MXFP4_MOE) load on M5 Max without OOM, within a specific RAM and load-time ceiling?**

**Gate 1 runbook structure (authored in the plan file — this is the content the future session will copy-paste):**

1. **How to resume / check if already run** — the first sub-section of Gate 1:
   ```bash
   # Resume check
   if [ -f ~/.mdemg-sprint-c/gate1/passed_*.json ] 2>/dev/null; then
     STAMP=$(ls -t ~/.mdemg-sprint-c/gate1/passed_*.json | head -1)
     STAMP_SHA=$(jq -r .model_sha256_config "$STAMP")
     CURRENT_SHA=$(sha256sum ~/.cache/huggingface/hub/models--mlx-community--Qwen3.6-*/snapshots/*/config.json | awk '{print $1}')
     if [ "$STAMP_SHA" = "$CURRENT_SHA" ]; then
       echo "Gate 1 already passed at $(jq -r .timestamp_iso8601 "$STAMP") on matching model SHA — skipping to Gate 2."
       exit 0
     fi
   fi
   # else proceed to step 2
   ```
   The future session copy-pastes this block verbatim; its behavior (skip vs run) is deterministic.

2. **HF artifact availability re-check** — mitigates the week-long-pause risk that artifact names change between plan-time and execution-time:
   ```bash
   mkdir -p ~/.mdemg-sprint-c/gate1
   curl -s "https://huggingface.co/api/models?search=Qwen3.6-35B-A3B&author=mlx-community" \
     | jq '.[] | {id, downloads, lastModified}' \
     | tee ~/.mdemg-sprint-c/gate1/hf_artifact_query.json
   ```
   Expected output includes `mlx-community/Qwen3.6-35B-A3B-4bit` (confirmed in Sprint B). Runbook records actual IDs. If an asymmetric-quant variant exists (candidates: `mlx-community/Qwen3.6-35B-A3B-mxfp4-moe`, `mlx-community/Qwen3.6-35B-A3B-asym`), **prefer it**. If only `-4bit` exists, proceed with it and record a `deviation: "symmetric_4bit_used_pending_sprint_E"` in the stamp file.
   
   **Fallback protocol if neither Qwen3.6 artifact exists (Qwen3.6 was unpublished/pulled):**
   - Record failure in `failed_<timestamp>.json` with reason `"qwen36_artifact_unavailable"`.
   - Halt Sprint C; file a Sprint C' follow-up plan targeting `Qwen3.5-35B-A3B`. Do NOT silently fall back — the fallback is the halt; the user decides to replan.

3. **Download command** (expected ~17GB):
   ```bash
   huggingface-cli download mlx-community/Qwen3.6-35B-A3B-4bit \
     --local-dir ~/.cache/huggingface/hub/models--mlx-community--Qwen3.6-35B-A3B-4bit \
     2>&1 | tee ~/.mdemg-sprint-c/gate1/download.log
   ```
   (Runbook swaps the repo ID to whichever was selected in step 2.)

4. **Record model hash** (shared across gates — downloaded once):
   ```bash
   CONFIG_PATH=$(find ~/.cache/huggingface/hub/models--mlx-community--Qwen3.6-35B-A3B-4bit -name config.json | head -1)
   sha256sum "$CONFIG_PATH" | awk '{print $1}' > ~/.mdemg-sprint-c/gate1/model_config_sha256.txt
   jq -n --arg sha "$(cat ~/.mdemg-sprint-c/gate1/model_config_sha256.txt)" \
         --arg repo "mlx-community/Qwen3.6-35B-A3B-4bit" \
         --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
         --arg path "$(dirname "$CONFIG_PATH")" \
         '{repo: $repo, sha256_config: $sha, downloaded_at: $ts, local_path: $path}' \
     > ~/.mdemg-sprint-c/model.json
   ```

5. **Asymmetric-quant load command** — **WITH EXPECTED-TO-FAIL CAVEAT** per user's Risk-4 direction (the Sprint-E TODO marker in `vllm-mlx-setup.md` signals the selector syntax is not yet finalized upstream):
   
   **Path A — if HF published an asymmetric-quant artifact** (preferred): direct load, no convert call:
   ```bash
   /usr/bin/time -l python3 -c "
   from mlx_lm import load
   model, tokenizer = load('$(jq -r .local_path ~/.mdemg-sprint-c/model.json)')
   print('LOADED_OK')
   import mlx.core as mx
   # Force one forward pass to materialize weights
   out = model(mx.array([[1, 2, 3]]))
   mx.eval(out)
   print('FORWARD_OK')
   " 2>&1 | tee ~/.mdemg-sprint-c/gate1/load.log
   ```
   `/usr/bin/time -l` on macOS reports peak RSS.
   
   **Path B — if only `-4bit` exists and we want to attempt per-module asymmetric convert**: runbook documents the **expected** `mlx_lm.convert` flags per memo §3.8:
   ```bash
   # ATTEMPT only — upstream syntax not finalized as of plan time (Sprint-E TODO in vllm-mlx-setup.md).
   # If this command errors with "unknown flag" or similar, STOP, record the error, and fall through
   # to Path C below. Do NOT invent syntax.
   python3 -m mlx_lm.convert \
     --hf-path mlx-community/Qwen3.6-35B-A3B-4bit \
     --mlx-path ~/.cache/mlx/Qwen3.6-35B-A3B-asym \
     --quantize \
     --q-bits-shared 16 --q-bits-attention 16 --q-bits-routed 4 \
     2>&1 | tee ~/.mdemg-sprint-c/gate1/convert_attempt.log
   ```
   
   **Path C — fallback: symmetric 4-bit load** (always works; accepted deviation):
   ```bash
   /usr/bin/time -l python3 -c "from mlx_lm import load; m, t = load('$(jq -r .local_path ~/.mdemg-sprint-c/model.json)'); print('LOADED_SYMMETRIC_4BIT_OK')" \
     2>&1 | tee ~/.mdemg-sprint-c/gate1/load.log
   ```
   Record `deviation: "symmetric_4bit_used_pending_sprint_E_convert_patch"` in the stamp file's `notes` field. Gate 1 still **passes** on Path C provided the RAM/load-time numeric criteria (below) are met; the asymmetric variant becomes a Sprint E deliverable.

6. **RAM footprint measurement** — parse `/usr/bin/time -l` output:
   ```bash
   grep "maximum resident set size" ~/.mdemg-sprint-c/gate1/load.log \
     | awk '{print $1}' > ~/.mdemg-sprint-c/gate1/rss_bytes.txt
   # macOS reports in bytes; convert to GB with 2 decimals
   python3 -c "print(f'{int(open(\"$HOME/.mdemg-sprint-c/gate1/rss_bytes.txt\").read().strip())/1e9:.2f} GB')" \
     | tee ~/.mdemg-sprint-c/gate1/ram_footprint_gb.txt
   ```
   For cross-check, also capture macOS `vmmap` output mid-load (optional; resume-skippable):
   ```bash
   # In a second terminal while Path A/B/C is running its forward pass:
   vmmap $(pgrep -f "from mlx_lm import load" | head -1) > ~/.mdemg-sprint-c/gate1/vmmap_snapshot.txt
   ```

7. **Numeric acceptance criteria (Gate 1)** — all four must hold for PASS:
   - **Load completes without OOM or Python exception.** `LOADED_OK` (or `LOADED_SYMMETRIC_4BIT_OK` on Path C) present in `load.log`; `FORWARD_OK` present (Path A); exit code 0.
   - **Peak RAM ≤ 24 GB** for inference-only load. Justification: `02_M5MAX_HARDWARE_v2.md §3 Inference Mode` estimates 21 GB for the asymmetric-quant variant and 22 GB for symmetric 4-bit; 24 GB is a +10% headroom ceiling. `vllm-mlx-setup.md` current ~22GB figure (with Sprint-E TODO marker) is the upper reference. If measured RAM ≤ 24 GB → pass; 24-30 GB → middle band → FLAG but still pass Gate 1 (record `ram_band: "high"`), warn downstream that Tier 2 concurrent inference headroom will shrink. >30 GB → FAIL (halt; replan sizing).
   - **Load time ≤ 90 s** on M5 Max cold cache. Justification: Qwen3-30B-A3B Q4 loads in ~45 s on M4 Max per community benchmarks; Qwen3.6-35B-A3B has ~17% more parameters; M5 Max has ~12% higher memory bandwidth. 90 s is a 2× margin on the derived ~52 s estimate. >90 s → FAIL; >180 s → halt (likely swap or I/O fault, not a model issue).
     - **First-load vs warm-load:** the ≤90 s ceiling applies to **first-load from disk after cold page-cache** (reboot or long idle — runbook forces this with `sudo purge` before measurement). Warm-load (immediate re-load after a first-load, weights still resident in page cache) typically completes in 10-20 s and is **not** gate-qualifying; runbook records warm-load separately as a telemetry field but does not gate on it.
     - **SSD tier sensitivity:** first-load time is I/O-bound for the ~20 GB quant artifact. M5 Max internal NVMe (≥4 GB/s sequential read) is the assumed baseline. If the model cache lives on an **external SSD (USB 3.2 ~1 GB/s or Thunderbolt-attached NVMe ~2-3 GB/s)**, multiply the ceiling proportionally: USB 3.2 → allow ≤180 s (still pass), halt threshold 360 s; TB3/4 → allow ≤135 s, halt 270 s. Runbook step 7 captures `df -h` + `diskutil info /` on the model-cache mount and records `storage_tier` in the stamp so future gate comparisons normalize correctly.
   - **Forward pass (3-token input) completes in ≤ 30 s** (warm weights, cold KV). >30 s → FAIL.

8. **Stamp write on pass:**
   ```bash
   jq -n \
     --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
     --arg sha "$(cat ~/.mdemg-sprint-c/gate1/model_config_sha256.txt)" \
     --arg ram "$(cat ~/.mdemg-sprint-c/gate1/ram_footprint_gb.txt)" \
     --arg path "$(cat ~/.mdemg-sprint-c/gate1/load.log | grep -oE 'LOADED[_A-Z4]*_OK' | head -1)" \
     --arg mlx "$(python3 -c 'import mlx_lm; print(mlx_lm.__version__)')" \
     '{gate: "gate1", result: "pass", timestamp_iso8601: $ts, model_sha256_config: $sha,
       mlx_lm_version: $mlx,
       measured_values: {peak_ram_gb: $ram, load_path: $path}}' \
     > ~/.mdemg-sprint-c/gate1/passed_$(date -u +%Y%m%dT%H%M%SZ).json
   ```

9. **Halt protocol on fail:**
   - Write `failed_<timestamp>.json` with captured values.
   - **Hard halt the FT-LORA line on Qwen3.6.** The future session stops and surfaces a message to the user: `Gate 1 FAILED — Qwen3.6-35B-A3B asymmetric-quant load on M5 Max did not meet criteria. Halt FT-LORA; next action is to file Sprint C' targeting Qwen3.5-35B-A3B as base.`
   - Do not automatically try Qwen3.5 in Gate 1 — halt is a **user decision**, per constraint #5.

**Epic 2 self-check (plan-authoring gate, not execution gate):**
- All four numeric thresholds present with justification.
- Resume-check at step 1 is copy-pasteable.
- Fallback path (B → C) preserves Gate 1 pass with a recorded deviation.
- Halt disposition is explicit, not implicit.

### Epic 3 — Gate 2 runbook (structured-output / sampling-recipe correctness)

Author the Gate 2 body. Gate 2 answers: **does Qwen3.6 at the canonical J-group sampling recipe produce ≥95% schema-valid JSON on 100 synthetic J-group prompts? If not, can a sampling-param sweep recover ≥95%?**

**Gate 2 runbook structure:**

1. **Resume check** — analogous to Gate 1:
   ```bash
   if [ -f ~/.mdemg-sprint-c/gate2/passed_*.json ] 2>/dev/null; then
     STAMP=$(ls -t ~/.mdemg-sprint-c/gate2/passed_*.json | head -1)
     STAMP_SHA=$(jq -r .model_sha256_config "$STAMP")
     CURRENT_SHA=$(cat ~/.mdemg-sprint-c/gate1/model_config_sha256.txt)
     [ "$STAMP_SHA" = "$CURRENT_SHA" ] && { echo "Gate 2 already passed — skipping"; exit 0; }
   fi
   # Gate 1 must have passed
   [ -f ~/.mdemg-sprint-c/gate1/passed_*.json ] || { echo "Gate 1 must pass first"; exit 2; }
   mkdir -p ~/.mdemg-sprint-c/gate2
   ```

2. **Synthetic prompt generation — N=100, fixed seed.** N chosen per user constraint #5 ("pick N — suggest 100"). Justification: three J-group tasks × ~33 prompts each gives robust per-task validity statistics at 95% ± 4.4% binomial confidence; the 100 total strikes a balance between statistical power and ~30-60 min wall-clock generation time. Prompts are **drawn from the existing ULTS specs' example outputs**, not invented at plan time:
   - J-group task list (from `04_BENCHMARK_RL_v2.md §10.0`): `hidden.name_emergence`, `jiminy.evaluate_llm`, `retrieval.rerank_cross`. Note: the 17th task `guardrail.evaluate` also has `sampling_group=J` (Sprint B addition); runbook includes it as a **4th J-group source** → 25 prompts per task × 4 tasks = 100. This aligns with the "17th guardrail" footnote in `01_RESEARCH_v2.md §1.1`.
   - **Prompt construction protocol (authored in runbook, NOT in a script file — deferred to execution):**
     - For each of the 4 J-group ULTS specs, read the `prompt.user_template` and `examples` fields.
     - Populate the template with 25 varied fillers drawn from the spec's `examples[].input` or generated by mutating the fillers (length-varied, content-varied — ULTS specs include worked examples).
     - Seed: Python `random.seed(20260421)` (sprint-start date). Same seed → same prompts on every run → reproducible.
     - Write out as `~/.mdemg-sprint-c/gate2/prompts_100.jsonl` — one JSON object per line with `{task, prompt_text, system_prompt, expected_schema_keys, seed_index}`.
   - **Runbook instruction:** "At Gate 2 execution time, author a 30-line Python helper that reads the 4 ULTS spec JSON files and produces `prompts_100.jsonl`. Do NOT commit this helper — it is an execution artifact. If the helper's JSON prompts differ across two execution attempts because of spec changes, record the differing prompt set SHA in the Gate 2 stamp file (`prompts_100_sha256`)."

3. **Canonical J-recipe inference loop:**
   ```bash
   # Assumes vllm-mlx is up on :8100 against the model from Gate 1
   # If not, start it:
   vllm-mlx --model $(jq -r .local_path ~/.mdemg-sprint-c/model.json) --port 8100 &
   VLLM_PID=$!
   # Wait for /v1/models to respond
   until curl -sf http://localhost:8100/v1/models > /dev/null; do sleep 2; done

   # Per-prompt generation — J recipe from 04_BENCHMARK_RL_v2.md §10.0 with
   # max_tokens BUMPED to 4096 (from memo's 2048 production default) to isolate
   # sampling-param effects from truncation effects. Supplementary 2048 re-test
   # runs after sweep (see step 6.5).
   #   temperature=0.7 top_p=0.95 top_k=20 presence_penalty=1.5 max_tokens=4096
   # Think mode: OFF (J = no-think)
   while IFS= read -r line; do
     task=$(echo "$line" | jq -r .task)
     system=$(echo "$line" | jq -r .system_prompt)
     user=$(echo "$line" | jq -r .prompt_text)
     seed_idx=$(echo "$line" | jq -r .seed_index)
     curl -sS http://localhost:8100/v1/chat/completions \
       -H "Content-Type: application/json" \
       -d "$(jq -n \
         --arg m "$(jq -r .local_path ~/.mdemg-sprint-c/model.json)" \
         --arg s "$system" --arg u "$user" \
         '{model:$m, messages:[{role:"system",content:$s},{role:"user",content:$u}],
           temperature:0.7, top_p:0.95, top_k:20, presence_penalty:1.5, max_tokens:4096}')" \
       | jq --arg t "$task" --arg si "$seed_idx" \
           '{task:$t, seed_index:$si, content:.choices[0].message.content}'
   done < ~/.mdemg-sprint-c/gate2/prompts_100.jsonl \
     > ~/.mdemg-sprint-c/gate2/responses_default.jsonl
   ```

4. **JSON validity grading** — runbook describes the grader; **authorship deferred to execution**:
   - **Proposed execution-time location:** `neural/validation/gate2_json_validity.py` (per user guidance — but **do NOT commit**; execution-time artifact).
   - **Grader behavior** (authored verbatim in the runbook for the future session to write):
     1. Load each spec's `output_schema` (JSON Schema).
     2. For each response line: strip `<think>...</think>` via a regex safety net (should be absent on J-group no-think, but defensive).
     3. Try `json.loads()` on the raw content. If fails, try extracting from ````json` / ``` ` code fences (mirror `scripts/test_vllm_mlx.py:106-120`).
     4. If parsed, validate against the spec's JSON Schema using `jsonschema.Draft202012Validator`.
     5. Output per-prompt `{task, seed_index, status: "pass"|"fail", failure_reason}` to `~/.mdemg-sprint-c/gate2/validity_report.jsonl`.
     6. Summary line: `{total: 100, pass: N, pass_rate: P, per_task: {hidden.name_emergence: ..., jiminy.evaluate_llm: ..., retrieval.rerank_cross: ..., guardrail.evaluate: ...}}`.

5. **Numeric acceptance (default recipe):**
   - **≥95% overall pass rate on 100 prompts** → **pass Gate 2**. Stamp with `measured_values.pass_rate_default`.
   - <95% → run sweep (step 6).

6. **Sampling-param sweep matrix** — runs only if step 5 fails. **Matrix concentrates on `presence_penalty` and `temperature`** (the two levers most likely to affect J-group JSON validity per memo §3.3; `top_p`/`top_k` held at canonical 0.95/20). **Matrix spec authored as a nested loop in the runbook:**
   - `presence_penalty ∈ {1.0, 1.25, 1.5, 1.75, 2.0}` (5 cells — brackets the memo's 1.5 default with ±0.5)
   - `temperature ∈ {0.5, 0.7}` (2 cells — canonical 0.7 + a lower variant; hypothesis: if JSON-typical determinism improves validity, 0.5 should surface that signal)
   - Plus **2 control cells** (user constraint #Risk item on tokenizer/chat-template sensitivity):
     - `no_chat_template` — same canonical recipe but bypassing the model's chat template (raw prompt in the user message).
     - `json_mode_on` — canonical recipe + `"response_format": {"type": "json_object"}` in the request body.
   - Total: **5 × 2 + 2 = 12 cells**. Both main levers get multi-point coverage (5 × pp, 2 × temp); no wall-clock waste on `top_p`/`top_k` variants that §10.0 already fixes. Each cell re-runs the full 100 prompts. Per-cell responses in `~/.mdemg-sprint-c/gate2/sweep_matrix/<cell_id>.jsonl`. Per-cell validity in `sweep_matrix/<cell_id>_validity.json`. Aggregate summary in `sweep_summary.json` ordered by pass rate descending.
   - **Budget:** ~38 min per cell (100 prompts × ~23 s avg with J max_tokens=4096 — the bumped Gate 2 ceiling, see step 2's deviation note) × 12 cells ≈ 7.5 h. Document that sweep is a single-session commitment. Wall-clock may run shorter when prompts self-terminate early on `</JSON>` / end-of-schema tokens — the 4096 ceiling is an upper bound, not a per-prompt target.

6.5. **Supplementary truncation-rate verification (non-blocking telemetry)** — runs after step 6 sweep completes, regardless of pass/fail outcome. Purpose: isolate how much of any validity gap was attributable to the 4096→2048 ceiling reduction that production Sprint E will actually use (memo §3.3).
   - **Protocol:** take the winning cell's recipe (or the canonical recipe if step 5 passed outright) and re-run **20 prompts** (5 per J-group task — `hidden.name_emergence`, `jiminy.evaluate_llm`, `retrieval.rerank_cross`, `guardrail.evaluate`) at `max_tokens=2048` (the memo §3.3 production default) on seeds 0-4.
   - **Metric:** `production_config_truncation_rate` = fraction of responses where `finish_reason="length"` AND the response fails JSON validity (true truncation — parseable length-terminated responses don't count).
   - **Output:** `~/.mdemg-sprint-c/gate2/production_config_retest.jsonl` (per-prompt) + `production_config_summary.json` (aggregate).
   - **Disposition:** **non-blocking**. Stamp annotates `{production_config_truncation_rate: X, flag_to_sprint_e: bool}` where `flag_to_sprint_e=true` iff rate >5%. If flagged, the Sprint E planning session should revisit memo §3.3's max_tokens=2048 decision for J-group tasks (candidate options: raise to 3072, adopt per-task ceilings, or accept the truncation as rare-case tolerable).
   - **Budget:** ~8 min (20 prompts × ~23 s at the canonical recipe average). Runs inside the same Gate 2 session as the sweep.

7. **Acceptance (post-sweep):**
   - Best cell ≥ 95% → **pass Gate 2 with recipe-update note.** Stamp records `{pass_rate: X, winning_cell: {...}, note: "recipe revision candidate for memo 07 v3.1 §3.3 — flag to Sprint E"}`. Proceed to Gate 3.
   - Best cell 90-94.9% → **middle band for Gate 2.** Per user constraint #5, Gate 2 has no middle-band defer — the instruction is "halt only if sampling-param tuning cannot recover ≥95%". So 90-94.9% = halt, but with a softer disposition: runbook writes `failed_*.json` with `failure_mode: "sampling_tuning_insufficient"` and user message: `Gate 2 FAILED — best sweep cell pp=X tp=Y reached Z% validity (<95%). Investigate tokenizer/chat-template issues or consider Qwen3.5 fallback.`
   - Best cell <90% → halt; stamp with `failure_mode: "structural_json_failure"`; same user message.

8. **Stamp write on pass:**
   ```bash
   jq -n \
     --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
     --arg sha "$(cat ~/.mdemg-sprint-c/gate1/model_config_sha256.txt)" \
     --arg rate "$PASS_RATE" \
     --arg recipe "$WINNING_RECIPE_JSON" \
     '{gate: "gate2", result: "pass", timestamp_iso8601: $ts, model_sha256_config: $sha,
       measured_values: {pass_rate: ($rate|tonumber), recipe: ($recipe|fromjson)}}' \
     > ~/.mdemg-sprint-c/gate2/passed_$(date -u +%Y%m%dT%H%M%SZ).json
   ```

**Epic 3 self-check (plan-authoring):**
- N=100 justified.
- 4 J-group tasks (3 original + guardrail.evaluate) explicitly enumerated with 25/task split.
- Sweep matrix is 12 cells with explicit per-cell params; "no-chat-template" and "json_mode_on" control cells present.
- Step 6.5 supplementary 20-prompt re-test at `max_tokens=2048` present; `production_config_truncation_rate` flagged to Sprint E if >5%.
- Halt disposition for all three outcome bands.
- No code committed; grader location noted as execution-time artifact.

### Epic 4 — Gate 3 runbook (throughput + benchmark parity)

Author the Gate 3 body. Gate 3 answers: **does Qwen3.6 on M5 Max sustain ≥60 tok/s decode? And within what quality band does it fall vs `gpt-5.4-mini` on the 120-question benchmark?**

**Gate 3 runbook structure:**

1. **Resume check** — analogous, asserting Gate 1 + Gate 2 passed stamps exist with matching model SHA.

2. **Throughput sub-gate — protocol:**
   - **Warm-up:** 200 tokens on a fixed prompt (`"Hello, please generate a 200-token continuation about hardware architecture."`) — discarded from measurement.
   - **Measurement window:** 5 consecutive generations of 500 decode tokens each on a fixed T-group-like reasoning prompt (seeded), at the canonical T recipe (temp=0.6, top_p=0.95, top_k=20, max_tokens=500). Total measured = 2500 tokens.
   - **Token counting:** use `response["usage"]["completion_tokens"]` from vllm-mlx's OpenAI-compatible endpoint. Wall time from Python `time.monotonic()` bracketing each generation call.
   - **Metric:** tok/s = sum(completion_tokens) / sum(wall_time_seconds) across the 5 runs. Record median and mean; **use median** for the gate.
   - **Command block:**
     ```bash
     python3 - <<'PY' | tee ~/.mdemg-sprint-c/gate3/throughput.log
     import json, time, urllib.request
     MODEL = json.load(open(f"{__import__('os').path.expanduser('~')}/.mdemg-sprint-c/model.json"))["local_path"]
     # warm-up
     # ... (send one 200-token request, discard)
     # 5 × 500-token measured runs
     times = []; tokens = []
     for _ in range(5):
         body = json.dumps({...canonical T recipe...}).encode()
         t0 = time.monotonic()
         resp = json.loads(urllib.request.urlopen(..., data=body, timeout=120).read())
         t1 = time.monotonic()
         times.append(t1-t0); tokens.append(resp["usage"]["completion_tokens"])
     median_tps = sorted([t/w for t,w in zip(tokens,times)])[2]
     json.dump({"median_tps": median_tps, "mean_tps": sum(tokens)/sum(times),
                "per_run": list(zip(tokens,times))}, open(f"{__import__('os').path.expanduser('~')}/.mdemg-sprint-c/gate3/throughput.json","w"))
     print(f"median_tps={median_tps:.2f}")
     PY
     ```
     (Runbook includes the fully-fleshed version; abbreviated here.)
   - **Acceptance:** median ≥ 60 tok/s → pass throughput sub-gate. <60 → **halt FT-LORA line** (user constraint #5). Numeric justification: `02_M5MAX_HARDWARE_v2.md §2` derives ≥60 tok/s on M5 Max from M4 Max Qwen3-30B-A3B measurements (64-88 tok/s) plus Qwen3.6's ~17% larger param count offset by M5 Max's ~12% higher bandwidth; 60 tok/s is the deliberately-chosen floor that makes local inference SLO-viable for MDEMG.

3. **Benchmark parity sub-gate — baseline capture protocol:**
   - **Same wall-clock window** — user-raised risk on baseline drift. Runbook mandates that `gpt-5.4-mini` baseline is captured **within the same Gate 3 session**, back-to-back with the Qwen3.6 run. No reuse of prior result files.
   - **Question set:** `docs/architecture/benchmarks/whk-wms/test_questions_120.json` (120 questions, CLAUDE.md testing-tier canonical).
   - **Two-run protocol:**
     1. Qwen3.6 run (local, vllm-mlx on :8100):
        ```bash
        cd /Users/reh3376/mdemg
        LLM_BASE_URL=http://localhost:8100/v1 \
        LLM_MODEL=$(jq -r .local_path ~/.mdemg-sprint-c/model.json) \
        python3 docs/architecture/benchmarks/run_benchmark_v4.py \
          --questions docs/architecture/benchmarks/whk-wms/test_questions_120.json \
          --master docs/architecture/benchmarks/whk-wms/test_questions_120.json \
          --output-dir ~/.mdemg-sprint-c/gate3/qwen_run \
          --codebase /Users/reh3376/whk-wms \
          --space-id whk-wms \
          --model qwen36 \
          2>&1 | tee ~/.mdemg-sprint-c/gate3/qwen_run.log
        cp ~/.mdemg-sprint-c/gate3/qwen_run/answers_*.jsonl ~/.mdemg-sprint-c/gate3/qwen_answers.jsonl
        ```
     2. `gpt-5.4-mini` baseline run (hosted, fresh — same session, minutes later):
        ```bash
        LLM_BASE_URL=https://api.openai.com/v1 \
        LLM_MODEL=gpt-5.4-mini \
        OPENAI_API_KEY=$OPENAI_API_KEY \
        python3 docs/architecture/benchmarks/run_benchmark_v4.py \
          --questions docs/architecture/benchmarks/whk-wms/test_questions_120.json \
          --master docs/architecture/benchmarks/whk-wms/test_questions_120.json \
          --output-dir ~/.mdemg-sprint-c/gate3/gpt54mini_run \
          --codebase /Users/reh3376/whk-wms \
          --space-id whk-wms \
          --model gpt-5.4-mini \
          2>&1 | tee ~/.mdemg-sprint-c/gate3/gpt54mini_run.log
        cp ~/.mdemg-sprint-c/gate3/gpt54mini_run/answers_*.jsonl ~/.mdemg-sprint-c/gate3/gpt54mini_answers.jsonl
        ```
     (Note: exact `run_benchmark_v4.py` CLI flags to be validated at execution-time Epic 4 pre-run via `python3 .../run_benchmark_v4.py --help`. Runbook includes the `--help` command as a pre-flight step. This is the one place where a ≤5-min pre-flight validation is built in — Sprint B's default model change to `scripts/test_vllm_mlx.py` proved the surface moves.)

4. **Grading both runs:**
   ```bash
   python3 docs/architecture/benchmarks/grader_v4.py \
     ~/.mdemg-sprint-c/gate3/qwen_answers.jsonl \
     docs/architecture/benchmarks/whk-wms/test_questions_120.json \
     ~/.mdemg-sprint-c/gate3/grades_qwen.json

   python3 docs/architecture/benchmarks/grader_v4.py \
     ~/.mdemg-sprint-c/gate3/gpt54mini_answers.jsonl \
     docs/architecture/benchmarks/whk-wms/test_questions_120.json \
     ~/.mdemg-sprint-c/gate3/grades_gpt54mini.json
   ```

5. **Quality band computation** — the `grader_v4.py` output includes a `mean_score` field (CLAUDE.md testing section: success criterion `Mean score >= 0.85 (matching baseline)`). Band logic:
   ```bash
   QWEN_MEAN=$(jq .mean_score ~/.mdemg-sprint-c/gate3/grades_qwen.json)
   GPT_MEAN=$(jq .mean_score ~/.mdemg-sprint-c/gate3/grades_gpt54mini.json)
   # Relative gap = (GPT_MEAN - QWEN_MEAN) / GPT_MEAN
   GAP=$(python3 -c "print((${GPT_MEAN} - ${QWEN_MEAN}) / ${GPT_MEAN})")
   ```

6. **Three numeric quality bands (justified):**
   - **Clear pass: gap ≤ 10% (Qwen ≥ 90% of gpt-5.4-mini's mean score).** Justification: a fine-tuning pass (Sprints E + SFT) can realistically lift a 10%-behind base model past parity on in-distribution tasks (FT-OAI-001 and OSS MoE LoRA literature both suggest 5-15% lift is typical for specialized tasks). 10% gap is the "clearly recoverable by planned SFT" threshold.
   - **Middle band: 10% < gap ≤ 30% (Qwen 70-90% of gpt-5.4-mini).** Justification: a 10-30% gap is the range where SFT outcomes are uncertain — recoverable on some task families (classify/J-structured where LoRA tight-fits), less recoverable on reasoning (T). **Proceed to Sprints D and E anyway**, but do not auto-commit at post-SFT time — defer commit-or-fallback decision to **Sprint F** (new, see §Post-Sprint). Middle-band stamp file `middle_band_<timestamp>.json` explicitly includes `sprint_f_required: true`.
   - **Halt: gap > 30% (Qwen < 70% of gpt-5.4-mini).** Justification: >30% base-model gap implies structural capability mismatch (not specialization gap); SFT on a structurally-weaker base is a bad $/hr investment. Write `failed_*.json` with `failure_mode: "catastrophic_quality_gap"`. Stop FT-LORA line.

7. **Stamp semantics:**
   - Throughput + quality `clear pass` → write `passed_<ts>.json` with both `measured_values.median_tps` and `measured_values.quality_gap`.
   - Throughput pass + quality `middle band` → write `middle_band_<ts>.json`. This is a **non-halt, non-pass** state. Sprints D + E still proceed. Sprint F is triggered post-SFT.
   - Throughput fail OR quality `halt` → write `failed_<ts>.json`. Halt.

**Epic 4 self-check:**
- Throughput protocol has explicit warm-up, measurement window, token-count method.
- Baseline captured fresh, same wall-clock window.
- Three bands at 10% / 30% with justification.
- Middle band explicitly registers Sprint F requirement in the stamp file.

### Epic 5 — Plan close-out (sections 6-12 + Documents Accessed + Sprint F registration + Post-Sprint)

Author the remaining 12-section material:

1. **§6 Testing Plan (three tiers — applied to the plan's OWN validation, not gate execution)** per user constraint #12:
   - Tier 1 (static validation): markdownlint (if configured) on the plan file; anchor check that every `§`-reference inside the plan resolves; `jq` round-trip on the authored stamp-file schemas; verify all code blocks have a language tag.
   - Tier 2 (integration): confirm the runbook's step 2 HF API query URL returns HTTP 200 with a non-empty JSON array (no-op, read-only; a real session would execute this to reduce drift risk — runbook documents the check).
   - Tier 3 (E2E rehearsal): **DO NOT EXECUTE.** The rehearsal is conceptual — user-run desk-check of the runbook, end-to-end, reading each step aloud to confirm a future session could execute without additional planning input. This IS the mid-sprint checkpoint.

2. **§7 Commit Strategy:**
   - Single commit at sprint close: `feat(ft-lora): Sprint C — Qwen3.6-35B-A3B MLX validation runbook (planning-only)`
   - Body: one bullet per epic (E1-E5), plus a dedicated "Behavior changes" section stating **no behavior changes** (planning-only) and a dedicated "Future-work registrations" section listing Sprint F as newly introduced.
   - Footer: `Co-Authored-By: Claude Opus 4 <noreply@anthropic.com>`
   - Push to `reh3376_dev01` → auto-PR → sprint summary comment per MEMORY rule.
   - **Mid-sprint checkpoint does NOT commit** — working tree holds the plan until user approves.

3. **§8 Verification Checklist:**
   - [ ] Pre-gate: branch on `101cacb`, tree clean (tsdb untracked ignored)
   - [ ] Epic 1: sections 1-4 + §Resume Protocol authored; stamp schema defined; version-capture commands inlined
   - [ ] Epic 2: Gate 1 runbook complete with 3 paths (A/B/C), 4 numeric thresholds, resume check, halt protocol
   - [ ] Epic 3: Gate 2 runbook complete with N=100 justification, 4 J-group tasks enumerated, 12-cell sweep matrix with control cells, 3-outcome disposition
   - [ ] Epic 4: Gate 3 runbook complete with throughput protocol, fresh baseline capture, three quality bands with justification, Sprint F trigger on middle band
   - [ ] Epic 5: §6-12 authored, Sprint F registered in §Post-Sprint and in `00_README_v2.md` Sprint Plan table, Documents Accessed populated
   - [ ] Mid-sprint user checkpoint passed
   - [ ] Single batched commit pushed to `reh3376_dev01`, auto-PR open, summary posted

4. **§9 Documentation Update (final epic — never cut):**
   - `docs/development/ft-lora/sprint_plan_ft_lora_c.md` (NEW — this file)
   - `docs/development/ft-lora/00_README_v2.md`:
     - Version bump 5.1 → 5.2 with banner: `Changes in v5.2 (Sprint FT-LORA-C — 2026-04-21): Qwen3.6-35B-A3B MLX validation runbook committed (planning-only; zero execution artifacts). Sprint F registered as post-SFT commit-or-fallback checkpoint.`
     - Document Map row 10: `10 | sprint_plan_ft_lora_c.md | Sprint FT-LORA-C v1.0-format plan — 3-gate MLX validation runbook + Sprint F registration | ~14`
     - Sprint Plan table: update Sprint C row status to ✅ Plan committed; add Sprint F row (see §Post-Sprint).
   - `AGENT_HANDOFF.md` — append Sprint C completion entry.
   - `CHANGELOG.md` `[Unreleased]` — new entry:
     ```
     ### Added
     - **FT-LORA-C: Qwen3.6-35B-A3B MLX validation runbook** (2026-04-21)
       - `docs/development/ft-lora/sprint_plan_ft_lora_c.md` — 3-gate runbook designed for non-continuous execution (week-long pauses survive via `~/.mdemg-sprint-c/` disk stamps).
       - Gate 1: asymmetric-quant load (≤24 GB RAM, ≤90 s load); 3 path options (A=published asym, B=convert attempt, C=symmetric 4-bit with Sprint-E deviation).
       - Gate 2: ≥95% JSON validity on 100 synthetic J-group prompts; 12-cell sampling-param sweep fallback including 2 control cells (no-chat-template, json_mode).
       - Gate 3: ≥60 tok/s throughput + three quality bands vs `gpt-5.4-mini` (≤10% pass, 10-30% middle→Sprint F, >30% halt).
     - **Sprint FT-LORA-F registered** as post-SFT commit-or-fallback checkpoint (skeleton only; detailed plan drafted at Sprint F start).
     ```
   - **Documents Accessed** appendix — see §11.

5. **§10 Risks & Mitigations:**

| Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|
| MLX library version drift between plan-time and execution-time (week+ pause) | Medium | §Resume Protocol captures `mlx_lm.__version__` in `versions.json` at first execution. Runbook step 1 of Gate 1 re-reads and compares to any existing stamp; if drifted, forces re-execution of Gate 1. | Pin via `requirements.txt` in Sprint E; until then, runbook records observed version as authoritative. |
| HF artifact availability / naming change (`Qwen3.6-35B-A3B-4bit` rename, pull, or new asymmetric variant publication) | Medium | Gate 1 step 2 is a **mandatory** `curl ...api/models?search=...` re-check with `jq` parsing before download; result captured in `hf_artifact_query.json`. | If Qwen3.6 artifacts vanish entirely: halt Sprint C, file Sprint C' targeting Qwen3.5-35B-A3B. If an asymmetric variant appears: prefer it via Path A. |
| Asymmetric-quant `mlx_lm.convert` selector syntax not supported at execution time (Sprint-E TODO markers in `vllm-mlx-setup.md` suggest not finalized upstream) | High | Gate 1 specifies 3 paths (A/B/C) with explicit fallthrough. Path C (symmetric 4-bit) is always-executable and passes Gate 1 provided RAM/load-time criteria met; deviation recorded for Sprint E. | Sprint E owns the convert patch; Gate 1 re-run on Path A after Sprint E lands. |
| Benchmark baseline drift — `gpt-5.4-mini` responses change between any prior baseline capture and Gate 3 execution | Medium | Gate 3 mandates fresh baseline capture **in the same wall-clock window** as the Qwen3.6 run (steps 3.1 and 3.2 back-to-back); no reuse of stale baseline files. | If OpenAI deprecates `gpt-5.4-mini` before Gate 3 executes, runbook halts and user selects successor model (likely `gpt-5.4` per `.env` defaults); stamp records the substitution. |
| J-group JSON validity sensitive to tokenizer/chat-template differences between MLX build and reference implementation | High | Sweep matrix includes `no_chat_template` and `json_mode_on` control cells (Gate 2 step 6). If a control cell wins, Sprint E gets a tokenizer/template investigation deliverable. | Fall back to `response_format: json_object` in production if `json_mode_on` wins; recipe delta flagged for memo 07 v3.1 update in Sprint E. |
| Future session encounters stale Gate 1 pass stamp with mismatched model SHA | Low | Resume-check compares stamp's `model_sha256_config` to current on-disk SHA; mismatch forces re-run. | Documented; future session cannot accidentally skip a gate against a different model. |
| 100-prompt synthetic set drifts if ULTS specs change between runs | Low | Gate 2 stamp records `prompts_100_sha256`; if re-executed against changed specs, generates fresh prompt set and logs the SHA delta. | Stamp's `notes` field captures prompt-set deltas; user adjudicates whether to accept the new set or replan. |
| $25 Gate 3 baseline budget exceeded at execution time | Low | Runbook explicit hard cap; pre-run token-count estimation pauses for user approval if >$25; running total tracked in `openai_cost.json` and aborts mid-run if cumulative spend crosses $25. | User approves higher budget or swaps benchmark size down (e.g., 40 questions instead of 120); deviation recorded. |
| Baseline `gpt-5.4-mini` drifts between capture and Qwen run (OpenAI silent model revision) | Medium | 24h same-window constraint (§Dependencies); runbook re-captures baseline if `baseline_age_hours > 24`; `openai_model_id` recorded in stamp for post-hoc audit. | If re-capture triggers repeatedly, cache baseline responses locally for the 120-question set as a short-horizon reference (known stale, but reproducible). |
| Sprint C runbook itself proves incomplete at execution time (hand-wave surfaces despite plan) | Medium | Mid-sprint checkpoint (§7 Commit Strategy) has the user desk-check every gate end-to-end before commit; Epic 5 Tier 3 "conceptual rehearsal" is the forcing function. | Amend plan before commit; never commit a hand-wave. |
| Middle-band Gate 3 result mishandled (auto-commits to D/E without Sprint F trigger) | Low | `middle_band_<ts>.json` stamp schema explicitly includes `sprint_f_required: true` (Gate 3 step 7); Sprint F registration in §Post-Sprint is the human-facing reminder. | If Sprint F slips off the backlog, its omission surfaces at post-SFT decision point and user re-reads this runbook. |

6. **§11 Documents Accessed** — see end-of-file appendix.

7. **§12 Rollback:**
   - No code changes in Sprint C. Rollback = `git revert` the single commit; restores prior state with no side effects.
   - Stamp files under `~/.mdemg-sprint-c/` are user-local and not tracked; rollback does not delete them (which is correct — future session's resume-check still honors them). User manually `rm -rf ~/.mdemg-sprint-c/` only on explicit replan.

---

## Post-Sprint C

Sprint C gate (PLAN-MERGE) → unlocks execution of the runbook in a future session. Execution-time outcomes feed the sprint chain:

- **All three gates pass (Gate 3 = clear pass band)** → Sprint D (expert activation profiling) proceeds; eventually Sprint E + SFT execution → direct commit at post-SFT (no Sprint F needed; but Sprint F registration remains as a safety checkpoint).
- **Gate 1 fails** → halt FT-LORA line on Qwen3.6. File Sprint C' targeting Qwen3.5-35B-A3B as base; re-execute the full runbook against the fallback model.
- **Gate 2 hard-fails (sweep cannot reach ≥95%)** → halt. Investigate tokenizer/chat-template or consider Qwen3.5 fallback.
- **Gate 3 throughput fails (<60 tok/s)** → halt. Economic non-viability of local inference.
- **Gate 3 quality is middle band (10% < gap ≤ 30%)** → Sprints D + E **still proceed**; post-SFT commit decision deferred to **Sprint F**.
- **Gate 3 quality halt (>30%)** → halt.

### Sprint FT-LORA-F (new — registered here; detailed plan drafted at Sprint F start)

**Role:** post-SFT commit-or-fallback checkpoint — the explicit decision gate for middle-band Gate 3 results.

**Trigger:** (a) Sprint C Gate 3 left a `middle_band_*.json` stamp under `~/.mdemg-sprint-c/gate3/`, AND (b) Phase 5 SFT execution has produced a trained adapter set (Tier 1 + at least one Tier 2 family).

**Scope preview (NOT a plan — just registration):**
1. Re-run the 120-question benchmark against the **trained** Qwen3.6 (Tier 1 + Tier 2 adapters loaded) vs. `gpt-5.4-mini` fresh baseline (same wall-clock window protocol as Sprint C Gate 3).
2. Compute post-SFT quality gap.
3. **Commit** if gap ≤ 10% (Qwen clearly recovered to parity / surpassed baseline on MDEMG tasks).
4. **Fall back to Qwen3.5-35B-A3B** if gap still 10-30% post-SFT (diminishing returns on further SFT).
5. **Halt FT-LORA line entirely** if gap still >30% post-SFT (structural limit; non-recoverable).

Sprint F is **skeleton-only** in this plan — Sprint C registers it so the middle-band disposition is explicit, but Sprint F itself drafts its full 12-section plan only if/when triggered.

**Updated sprint chain:**
```
A (#335, done) → B (#336, done) → C (this plan, to commit)
  → D (expert profiling)
  → E (training infra patches)
  → Phase 5 SFT (Tier 1 + Tier 2 execution)
  → F (commit-or-fallback checkpoint — ONLY if Sprint C Gate 3 was middle band)
```

---

## Appendix A — Runbook-Execution State Reference (quoted inline for convenience)

Reproduced here so a future session landing on Appendix A can re-orient without re-reading the full plan:

- **Cross-gate state root:** `~/.mdemg-sprint-c/` (expanded from `$HOME`).
- **Model identity:** `~/.mdemg-sprint-c/model.json` — contains `repo`, `sha256_config`, `downloaded_at`, `local_path`.
- **Version pin:** `~/.mdemg-sprint-c/versions.json` — contains mlx_lm, vllm-mlx, huggingface_hub, mdemg SHA, python, macOS versions captured at first execution.
- **Per-gate pass stamp:** `~/.mdemg-sprint-c/gateN/passed_<ISO8601>.json` — its existence + matching `model_sha256_config` = skip gate.
- **Per-gate fail stamp:** `~/.mdemg-sprint-c/gateN/failed_<ISO8601>.json` — its existence = halt (user decision to replan).
- **Gate 3 special state:** `~/.mdemg-sprint-c/gate3/middle_band_<ISO8601>.json` — non-halt, non-pass, triggers Sprint F post-SFT.

---

### Documents Accessed (during Sprint C planning, read-only)

- `/Users/reh3376/mdemg/docs/development/ft-lora/sprint_plan_ft_lora_b.md` (full — Sprint B template pattern, 12-section format, mid-sprint checkpoint convention, Appendix A convention)
- `/Users/reh3376/mdemg/docs/development/ft-lora/00_README_v2.md` (full — Document Map, Key Decisions table, Sprint Plan table, version banner convention)
- `/Users/reh3376/mdemg/docs/development/ft-lora/01_RESEARCH_v2.md` (targeted — §3 model selection/fallback, §5/§5.4 MoE-Sieve + asymmetric-quant policy, `mlx_lm.convert` per-module selectors, Qwen3.6 risk disposition)
- `/Users/reh3376/mdemg/docs/development/ft-lora/02_M5MAX_HARDWARE_v2.md` (targeted — §2 inference throughput target derivation, §3 memory budget / 21 GB asymmetric estimate, §4 vllm-mlx, `mxfp4-moe` artifact reference)
- `/Users/reh3376/mdemg/docs/development/ft-lora/04_BENCHMARK_RL_v2.md` (targeted — §10.0 three-group sampling recipes + 16-task group table + presence_penalty=1.5 mandate)
- `/Users/reh3376/mdemg/docs/operations/vllm-mlx-setup.md` (full — install commands, current ~22GB memory table with Sprint-E TODO markers, serving commands, smoke-test usage, troubleshooting)
- `/Users/reh3376/mdemg/scripts/test_vllm_mlx.py` (full — argparse defaults `mlx-community/Qwen3.6-35B-A3B-4bit`, chat-template handling, JSON-extraction fallback logic at lines 106-120)
- `/Users/reh3376/mdemg/docs/architecture/benchmarks/run_benchmark_v4.py` (partial — CLI flags, runner entry point, validator/answer_generator dependencies)
- Git state: `git log --oneline -15` (confirmed `101cacb` as HEAD, PR #336 merged), `git status` (clean except `scripts/tsdb_data_review_2026-04-01.json` untracked)
- Glob/Grep probes: HF model ID variants (`mxfp4-moe` found in `02_M5MAX_HARDWARE_v2.md:161`, `.env.example:428`, compose templates); `gpt-5.4-mini` references (FT-OAI-003 plan, CLI reference); `mlx_lm.convert` (RESEARCH §5.4 and hardware doc only — no scripts yet)
