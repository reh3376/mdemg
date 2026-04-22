# Sprint FT-LORA-E — Training Infrastructure Patches

## Context

Sprint D completed (`3871222` / PR #343 merged, 2026-04-22): 3-family partition empirically validated (cross-family Jaccard: T↔C=0.4952, T↔J=0.5371, C↔J=0.6863; no pair > 0.80 merge threshold). Three family artifacts (`profile_routing_{reasoning_think,classify_notink,structured_notink}.json`) shipped under `training_data/routing_profiles/`, each carrying per-layer 64-of-256 expert top-25% masks aligned to the Sieve strategy.

**Sprint E is the last pre-Phase-5 sprint.** Its job is to make `neural/training/train_ft.py` (and supporting modules) speak the MoE-Sieve two-tier dialect so Phase 5 SFT can invoke it with `--tier 1` (attention + shared, r=32 α=64, universal) and `--tier 2 --family <f> --expert-selection-path <json>` (top-25% routed experts only, r=8 α=16, per-family). None of the four pieces Tier 2 needs — tier-aware target modules, asymmetric mxfp4 quant, explicit epoch + early-stop, `router_aux_loss_coef=0.002` — exists in the current code or in upstream `mlx_lm.lora` CLI. Sprint E adds them.

No training actually launches in Sprint E; the gate is a **dry-run of Phase 5's full invocation matrix** (1 Tier 1 call + 3 Tier 2 family calls + 1 asymmetric quantize pass), not loss curves.

**Sprint chain:** A (#335) → B (#336) → C (#342) → D (#343) → **E (this sprint)** → Phase 5 SFT.

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | FT-LORA-E |
| Title | Training Infrastructure Patches — Tier-aware CLI, Asymmetric Quant, Early-Stop |
| Date | 2026-04-22 (plan) |
| Branch | `reh3376_dev01` |
| Predecessors | FT-LORA-D (PR #343, merged `3871222`) |
| Successors | Phase 5 SFT (unblocked on Sprint E merge) |
| Type | Code + config + tests (no training launches) |
| Risk | Medium (`train_ft.py` CLI surface change; subprocess stdout-parser is upstream-version-sensitive) |
| Budget | $0 (dry-run only; no GPU hours) |
| Artifacts | `neural/training/{train_ft.py, expert_selection.py, quantize_asymmetric.py, early_stop.py}` + tests + 4-file env activation + Sprint E plan + decision docs |

## 2. Problem Statement

Memo 07 v3.1 locks in two training tiers. `neural/training/train_ft.py` (as of `3871222`) subprocess-invokes `mlx_lm.lora` with six flags — none of them MoE-aware:

```
# train_ft.py:248-264 (current)
mlx_lm.lora --train --model <base> --data <dataset> --adapter-path <out>
            --iters N --batch-size B --learning-rate LR
            --num-layers L
```

What Phase 5 needs but Sprint E must add:

1. **`--tier {1,2}` flag.** Tier 1: train attention + shared expert, r=32 α=64. Tier 2: train **only** top-25% routed experts per family, r=8 α=16, loaded from `profile_routing_{family}.json`. Neither path exists today.
2. **`--expert-selection-path` + `--family`.** Required for Tier 2. Parses Sprint D artifact; emits 40×64=2,560 `experts.{E}.{down,up,gate}_proj` module-path strings × 3 proj = 7,680 keys for mlx_lm's `linear_to_lora_layers` `keys` filter.
3. **Asymmetric quant helper.** `mlx_lm.convert` accepts a `quant_predicate` callable. We need a predicate that returns `False` (no quant / keep BF16) for attention + shared expert + router, and returns `{"group_size":32,"bits":4,"mode":"mxfp4"}` for routed experts. Current `train_ft.py` does no quantization at all.
4. **Explicit `--n-epochs` + early-stop.** Overfitting-prevention policy (memory `feedback_epoch_cap`, forcing function: FT-OAI-001 step-1200 overfit): epoch cap = 3; `auto` disallowed; early-stop when `val_loss > best × 1.05` for 2 consecutive evals (SFT) or `val_reward < best × 0.95` for 2 consecutive evals (RL). mlx_lm.lora exposes no early-stop flag; must be wrapped around subprocess stdout.
5. **`--router-aux-loss-coef 0.002`.** Qwen3 MoE training requires router auxiliary loss to prevent expert collapse. Not exposed via mlx_lm.lora CLI; must be injected via config.json override or adapter_config.

### Training entry-point selection

Sprint D's profiling work (PR #343) exercised mlx_lm 0.31.2's internals directly. `linear_to_lora_layers(model, num_layers, config, keys=...)` at `tuner/utils.py:85` covers per-module target specification. `mixed_quant_predicate_builder` at `convert.py:20-77` with `quant_predicate` callable support covers asymmetric quant. The `--config` YAML path at `lora.py:129` is the *candidate* `router_aux_loss_coef` injection point (verified-or-falsified during Epic 1 dry-run per the atomic-replacement sub-step below). Based on this coverage, **option 1 (subprocess-wrap `mlx_lm.lora`)** is the chosen path.

Two third-party alternatives surfaced during the original audit — `mlx-lm-lora` (Goekdeniz-Guelmez) and `MLX-Tune` (ARahim3, Unsloth-compatible API) — were not evaluated in depth because no functional gap surfaced during Sprint D that option 1 couldn't address. Blocker criteria (router_aux_loss_coef as config knob, per-module target specification, per-layer router-activation logging, balanced per-task batch sampling, LR schedule/warmup/checkpoint frequency) are all reachable through option 1's surface area combined with the helper modules Sprint E adds. If Epic 1's dry-run reveals `router_aux_loss_coef` injection cannot be made to work via option 1 (both `--config` YAML path *and* the atomic config.json replacement fallback fail to take effect), re-open this decision with a genuine multi-candidate audit before proceeding — do not ship Sprint E with aux-loss silently dropped.

Sprint B pre-seeded 10 env-var placeholders in `.env.example:419-429` (`ROUTER_AUX_LOSS_COEF=0.002`, `LORA_TIER1_RANK=32`, etc.) and in all three compose files, **all commented out**. Sprint E activates them and wires them as defaults behind the new CLI flags.

## 3. Scope & Constraints

**In scope:**

| # | Deliverable | Path |
|---|---|---|
| 1 | Tier-aware CLI on `train_ft.py` (7 new flags + env fallbacks) | `neural/training/train_ft.py` |
| 2 | Expert-selection loader + `mlx_lm.keys` emitter | `neural/training/expert_selection.py` (new) |
| 3 | Asymmetric quant predicate + CLI wrapper | `neural/training/quantize_asymmetric.py` (new) |
| 4 | Early-stop subprocess monitor | `neural/training/early_stop.py` (new) |
| 5 | `router_aux_loss_coef` config.json override helper | `neural/training/train_ft.py` (inline) |
| 6 | Env-var activation (uncomment 10 vars × 4 files) | `.env.example`, `docker-compose.yml`, `docker-compose.dev.yml`, `internal/cli/compose_templates/docker-compose.yml` |
| 7 | Unit tests | `neural/training/tests/{test_expert_selection.py,test_quantize_asymmetric.py,test_early_stop.py}` + extended `test_train_ft.py` |
| 8 | Integration test | `neural/training/tests/test_train_ft_integration.py` (dry-run against Sprint C model path) |
| 9 | E2E dry-run | `scripts/sprint_e_e2e_dry_run.sh` |
| 10 | Sprint E plan (this file → repo) | `docs/development/ft-lora/sprint_plan_ft_lora_e.md` |
| 11 | Doc updates | `00_README_v2.md` (v5.3→v5.4), `03_IMPLEMENTATION_PLAN_v2.md` §Phase 5.X (unblock), `AGENT_HANDOFF.md`, `CHANGELOG.md` |

**Out of scope (deferred to Phase 5 / later):**
- Any actual training launches — Phase 5 owns spend.
- Tier 1 × Tier 2 composition logic (load Tier 1 adapter as base for Tier 2 training) — Phase 5 runbook's concern; Sprint E exposes `--base-adapter` flag but does not test composition end-to-end.
- GRPO/RL early-stop — `--mode rl` wired but untested; Sprint E validates SFT early-stop only.
- Refactoring `train_ft.py` away from subprocess (direct `mlx_lm` Python-import) — deferred; keeps Sprint E diff small and isolates breakage to stdout-parser.

**Constraints:**
- **Sequential epics** (MEMORY `feedback_sequential_epics.md`).
- **Single batched commit at sprint close** (MEMORY).
- **No hardcoded values** (MEMORY `feedback_no_hardcoded_values.md`): all magic numbers (ranks, alphas, thresholds, coef) exposed as CLI flags with env-var fallbacks and documented defaults.
- **mlx_lm version pin:** `mlx_lm == 0.31.2` (matches Sprint C validation and Sprint D profiling). Script asserts version; warns on mismatch.
- **No destructive ops** — all new files or additive CLI flags on `train_ft.py`.
- **`n_epochs=auto` disallowed** — CLI rejects `auto` with non-zero exit and explicit error message pointing to overfitting policy.
- **Epoch cap enforced:** `--n-epochs > LORA_N_EPOCHS_CAP (default 3)` rejected with non-zero exit.
- **Sprint D SHA pin preserved:** Tier 2 training asserts the same `config.json` SHA256 (`cdc167566e…`) that profile artifacts were generated against, to prevent routing-architecture skew between profile and training. Mismatch → abort.

## 4. Dependencies

- **Sprint D artifacts** (consumed): `training_data/routing_profiles/profile_routing_{reasoning_think,classify_notink,structured_notink}.json`. Sprint E reads `per_layer[*].top_experts` (64 ints) and emits mlx_lm `keys` list.
- **Sprint C model SHA pin:** `cdc167566e54ebe6d5c6df308649670b5f1cacfe71a198688edba8471ea64734` (config.json). Asserted for Tier 2 training runs.
- **mlx_lm 0.31.2 internals** (treated as stable API surface until Sprint F):
  - `mlx_lm/tuner/utils.py:85` — `linear_to_lora_layers(model, num_layers, config, use_dora=False, keys=None)`; `keys` is the mechanism for per-module selection.
  - `mlx_lm/tuner/trainer.py:55,287-315,304` — eval interval, validation callback, `"Val loss {loss:.3f}"` stdout log line (regex target for early-stop).
  - `mlx_lm/convert.py:20-77,96-97,218` — `mixed_quant_predicate_builder` exists; `quant_predicate` accepts callable; `q_mode ∈ {"affine","mxfp4","nvfp4","mxfp8"}`.
  - `mlx_lm/utils.py:375-376` — mxfp4 params: `{"group_size": 32, "bits": 4, "mode": "mxfp4"}`.
  - `mlx_lm/lora.py:58,129` — CLI exposes `--iters` (not `--epochs`) and has NO `--router-aux-loss-coef`. Sprint E converts `--n-epochs` to `--iters` via `iters = n_epochs × ceil(len(dataset) / batch_size)`.
- **Sprint B pre-seeded env vars** (10): `ROUTER_AUX_LOSS_COEF=0.002`, `LORA_TIER1_RANK=32`, `LORA_TIER1_ALPHA=64`, `LORA_TIER2_RANK=8`, `LORA_TIER2_ALPHA=16`, `LORA_N_EPOCHS_CAP=3`, `LORA_EARLY_STOP_SFT_THRESHOLD=1.05`, `LORA_EARLY_STOP_RL_THRESHOLD=0.95`, `ASYMMETRIC_QUANT_SHARED=bf16`, `ASYMMETRIC_QUANT_ROUTED=mxfp4_moe`, `ASYMMETRIC_QUANT_ATTN=bf16`. Locations: `.env.example:419-429`, `docker-compose.yml`, `docker-compose.dev.yml`, `internal/cli/compose_templates/docker-compose.yml`.
- **MEMORY rules:** no hardcoded values, sequential epics, 3-tier testing, batched commit, sprint-plan-in-repo, Documents Accessed appendix, plan-before-code, sprint summary on PR.

No external services. No network access at runtime.

## 5. Implementation Plan (Sequential Epics + Gates)

**Pre-gate:** branch `reh3376_dev01` fast-forwarded to `3871222`; tree clean (ignore pre-existing untracked `scripts/tsdb_data_review_2026-04-01.json`); venv `mdemg-ft-lora` active; `python -c "import mlx_lm; assert mlx_lm.__version__=='0.31.2'"` passes; Sprint D artifacts present under `training_data/routing_profiles/`.

### Epic 1 — Tier-aware CLI + `router_aux_loss_coef` override on `train_ft.py`

Extend `neural/training/train_ft.py` (currently subprocess-wraps `mlx_lm.lora`).

**New CLI flags (all with env-var fallbacks, documented defaults, `--help` text):**

| Flag | Env fallback | Default | Type | Notes |
|---|---|---|---|---|
| `--tier {1,2}` | — | (required) | int | 1 = universal attn+shared; 2 = per-family routed experts |
| `--family {reasoning-think,classify-notink,structured-notink}` | — | (required iff tier=2) | str | Must match Sprint D family name |
| `--expert-selection-path` | — | (required iff tier=2) | path | Sprint D JSON artifact |
| `--rank` | `LORA_TIER{N}_RANK` | 32 (T1) / 8 (T2) | int | LoRA rank |
| `--alpha` | `LORA_TIER{N}_ALPHA` | 64 (T1) / 16 (T2) | int | LoRA alpha |
| `--target-modules` | — | auto from tier | csv | Overrides tier-default module list |
| `--router-aux-loss-coef` | `ROUTER_AUX_LOSS_COEF` | 0.002 | float | Injected via config override |
| `--n-epochs` | — | (required) | int | Rejects `auto`; enforces `LORA_N_EPOCHS_CAP` |
| `--early-stop-ratio` | `LORA_EARLY_STOP_SFT_THRESHOLD` (sft) / `LORA_EARLY_STOP_RL_THRESHOLD` (rl) | 1.05 / 0.95 | float | Trigger ratio |
| `--early-stop-patience` | — | 2 | int | Consecutive evals |
| `--mode {sft,rl}` | — | `sft` | str | RL wired but unvalidated |
| `--base-adapter` | — | (none) | path | For Tier 2 loading Tier 1 as base |
| `--expected-sha256` | — | (required iff tier=2) | hex | Gate against Sprint D pin |

**Tier→module defaults** (when `--target-modules` not passed):
- Tier 1: `["self_attn.q_proj","self_attn.k_proj","self_attn.v_proj","self_attn.o_proj","mlp.shared_expert.gate_proj","mlp.shared_expert.up_proj","mlp.shared_expert.down_proj"]` across all 40 layers.
- Tier 2: emitted by `expert_selection.py` (Epic 2). Length = 40 layers × 64 experts × 3 projs = 7,680 strings.

**`router_aux_loss_coef` injection — two-path strategy with atomic fallback.**

mlx_lm.lora does not expose a CLI flag for this. Critical caveat: `lora.py:129`'s `--config` YAML flag accepts *training hyperparameters* (LoRA rank, alpha, iters) — `router_aux_loss_coef` is a *model-architecture* parameter read by the MoE forward pass, not the trainer. The primary YAML path may therefore silently ignore the key, forcing the fallback in most or all cases.

**Primary-path verification (during Epic 1 dry-run):** write a config.yaml containing `router_aux_loss_coef: 0.002` alongside standard training hyperparameters; pass via `--config`. Grep the training stdout for: (a) an echoed config dump containing the key, (b) any forward-pass log referencing the coef, or (c) any non-zero auxiliary-loss line item. If no evidence of wiring, declare primary path unsupported and proceed to fallback. Record the finding in the Sprint E decision doc.

**Fallback path — atomic copy-on-write config.json replacement with signal-safe restoration:**

1. Compute `SHA256(<model_path>/config.json)`; log as `expected_pre_hash`.
2. Copy `<model_path>/config.json` → `<model_path>/config.json.pre-train-backup`.
3. Write modified config (original + `router_aux_loss_coef: 0.002`) → `<model_path>/config.json.training`.
4. `os.rename(config.json.training, config.json)` — atomic on same filesystem.
5. Register signal handlers (SIGTERM, SIGINT, SIGHUP) *and* an `atexit` handler; each restores from `.pre-train-backup`.
6. On clean training exit: restore from `.pre-train-backup`; recompute SHA256 and assert equals `expected_pre_hash`; fail loudly on mismatch.
7. Delete `.pre-train-backup` only after successful SHA re-match.

**Integrity check on every invocation (Tier 1 AND Tier 2, not just Tier 2):** before any training starts, compute `SHA256(<model_path>/config.json)` and compare against `--expected-sha256` (required flag) or `ROUTER_AUX_LOSS_COEF_BASELINE_SHA` env fallback. If mismatch, refuse to start and print which fields differ from the Sprint C pin (`cdc167566e54ebe6d5c6df308649670b5f1cacfe71a198688edba8471ea64734`). This catches the "training A crashed mid-run and left config.json drifted; training B silently operates on wrong model" failure mode. Tier-1 symmetry is deliberate — not a redundancy.

**Sub-steps:**
1. Parse new CLI flags; validate mutual requirements (`--tier 2` ⇒ `--family`, `--expert-selection-path` required; `--expected-sha256` required for BOTH tiers).
2. Reject `--n-epochs auto` explicitly with error message citing overfitting policy (FT-OAI-001 forcing function).
3. **Epoch cap enforcement:** if `--n-epochs > LORA_N_EPOCHS_CAP`, reject with non-zero exit and error message citing overfitting policy. Do not silently clamp. `LORA_N_EPOCHS_CAP` defaults to 3 when env var is unset (policy-default; not a required env var).
4. Compute `--iters` from `n_epochs × ceil(len(dataset)/batch_size)` — dataset length read via JSONL line-count.
5. Build `target_modules` list per tier (Tier 2 delegates to `expert_selection.py`).
6. **Pre-training SHA integrity check** against `--expected-sha256` — abort on mismatch.
7. **Primary router-aux path attempt:** write config.yaml with `router_aux_loss_coef` and pass via `--config`; capture stdout and run post-hoc verification grep. If unverified, proceed to step 8.
8. **Fallback router-aux path:** atomic config.json replacement with signal handlers + atexit + SHA re-match.
9. Wrap subprocess invocation with `early_stop.py` monitor (Epic 4).
10. On clean OR dirty exit: restore `.pre-train-backup`, verify SHA matches, fail loudly on drift.

**Gate:**
- `--help` prints all flags with env fallbacks.
- `--tier 2` without `--family` exits non-zero with explicit message.
- `--n-epochs auto` exits non-zero citing FT-OAI-001.
- `--n-epochs 5` with `LORA_N_EPOCHS_CAP=3` exits non-zero (rejected, not clamped).
- Tier 1 AND Tier 2 both require and enforce `--expected-sha256`; both abort on drift.
- Router-aux injection path chosen (primary vs fallback) is recorded in decision doc with grep evidence.
- Signal-safe restoration tested: simulated SIGTERM mid-training restores config.json to pre-train state with matching SHA.
- Tier 1 dry-run against Sprint C model path produces correct mlx_lm.lora invocation in `--dry-run` stdout.
- Tier 2 dry-run with a family profile produces correct invocation including 7,680 module paths (verified via line count of the emitted `keys` list).

### Epic 2 — `expert_selection.py` module

Create `neural/training/expert_selection.py`.

```python
class ExpertSelection:
    family: str
    top_experts_per_layer: list[list[int]]  # 40 × 64
    num_layers: int
    model_sha256_config: str

    @classmethod
    def load(cls, path: Path, expected_family: str) -> "ExpertSelection": ...
    def mlx_lm_keys(self, projections=("down","up","gate")) -> list[str]: ...
    def count(self) -> int:  # = 40 × 64 × 3 = 7680
```

**`mlx_lm_keys()` output format:**
```
language_model.model.layers.{L}.mlp.experts.{E}.{down|up|gate}_proj
```
for L ∈ [0,40), E ∈ top_experts_per_layer[L], proj ∈ {down,up,gate}.

**Gate:**
- `load()` raises on family mismatch, schema missing fields, or malformed layer count.
- `mlx_lm_keys()` returns exactly 7,680 unique strings for every validated profile.
- Module-path format string-matches against `mlx_lm.models.qwen3_next.Qwen3NextSparseMoeBlock` parameter names (verified via a loaded model's `named_parameters()` keys during Tier-2 integration test).

### Epic 3 — `quantize_asymmetric.py` module

Create `neural/training/quantize_asymmetric.py`.

```python
def build_asymmetric_predicate(
    shared_bits: str = "bf16",   # from ASYMMETRIC_QUANT_SHARED
    routed_spec: dict = MXFP4,   # from ASYMMETRIC_QUANT_ROUTED
    attn_bits: str = "bf16",     # from ASYMMETRIC_QUANT_ATTN
) -> Callable[[str, nn.Module, dict], bool | dict]:
    """Returns quant_predicate compatible with mlx_lm.convert.
    - self_attn.*            -> False (keep BF16)
    - mlp.shared_expert.*    -> False
    - mlp.gate (router)      -> False
    - mlp.experts.*          -> {"group_size": 32, "bits": 4, "mode": "mxfp4"}
    - everything else        -> False (no quant)
    """
```

**CLI wrapper** for invoking from shell:
```
python neural/training/quantize_asymmetric.py \
  --input-model <path> --output-model <path> \
  [--shared-bits bf16] [--routed-spec mxfp4] [--attn-bits bf16] \
  [--dry-run]
```

`--dry-run` iterates model parameters, applies predicate, prints the quant-classification summary (counts per category), and exits without writing.

**Gate:**
- Unit test: 8 handcrafted module paths → correct classification (routed=mxfp4, attn/shared/router/other=False).
- Dry-run against Sprint C model path: prints summary showing 40 × 256 routed expert modules classified mxfp4, 40 × 4 attn modules classified BF16, 40 × 3 shared modules classified BF16, 40 router modules classified BF16.
- Full conversion NOT run in Sprint E (Phase 5 territory); only predicate + dry-run validated.

### Epic 4 — `early_stop.py` subprocess monitor

Create `neural/training/early_stop.py`.

```python
class EarlyStopMonitor:
    mode: Literal["sft","rl"]
    ratio: float   # 1.05 (sft) or 0.95 (rl)
    patience: int  # 2

    def on_line(self, line: str) -> bool:
        """Parses a stdout line from mlx_lm.lora.
        Returns True if early-stop should fire; False otherwise.
        SFT regex: r'Val loss (\d+\.\d+)'
        RL regex:  r'Val reward (\d+\.\d+)'   # placeholder; RL untested
        """
```

**Invocation in `train_ft.py`:**
```python
monitor = EarlyStopMonitor(mode=args.mode, ratio=..., patience=...)
proc = subprocess.Popen([...], stdout=subprocess.PIPE, ...)
for line in iter(proc.stdout.readline, b""):
    sys.stdout.write(line)
    if monitor.on_line(line.decode()):
        proc.send_signal(signal.SIGTERM)
        # wait up to 30s for graceful exit, then SIGKILL
        break
```

**Orphan-checkpoint handling:** on SIGTERM, adapter files may be partially written. Mitigation: use a staging path (`<adapter-path>.partial`) passed to mlx_lm.lora; on clean exit rename to final; on early-stop or SIGTERM, keep `.partial` but write `<adapter-path>.earlystop.json` with {trigger_line, val_loss_history, stopped_at_iter, best_pointer, current_pointer}. Phase 5 runbook decides whether to keep or rerun.

**Checkpoint-behavior verification (blocks Epic 4 completion).** Early-stop fires on the 2nd consecutive val-loss-above-threshold, which means the model at fire time is one eval interval past the best checkpoint. If mlx_lm.lora writes current-state adapters only (not best-so-far), early-stop actively saves the worse state — the opposite of what the FT-OAI-001 forcing function intends.

Verification sub-step: launch a short real training run (e.g., 10 iters, 2–3 evals) against the Sprint C model and a small synthetic dataset; inspect the adapter-path directory during and after training. Determine empirically:
- **(a)** mlx_lm.lora writes best-so-far checkpoints alongside current-so-far (e.g., `adapter.safetensors` for current + `adapter.best.safetensors` for best).
- **(b)** current-so-far only.
- **(c)** some other pattern.

Record the finding in the Sprint E decision doc. Response by case:
- **If (a):** on early-stop fire, log which checkpoint is best-so-far; the `.earlystop.json` sidecar records both `current` and `best` pointers so Phase 5 runbook can select the best.
- **If (b):** early-stop MUST fire at the best-checkpoint boundary, not the current-state boundary. Either change trigger to `patience=1` (one bad eval, accepting the false-positive risk) OR add an outer adapter-rollback step that keeps the previous adapter snapshot and restores it on early-stop fire. Default choice: adapter snapshot + rollback, because it preserves the 2-consecutive-eval confidence from FT-OAI-001.
- **If (c):** document and address case-by-case in the decision doc.

**Gate:**
- Unit test: synthetic stdout stream with known val-loss trajectory → monitor fires at the correct line.
- Unit test: patience=2 fires on 2nd trigger, not 1st.
- Unit test: non-matching lines don't fire.
- Integration test: `--dry-run` plumbing verifies monitor wrapper invokes (uses fake subprocess that emits scripted stdout).
- **Checkpoint-behavior decision recorded in Sprint E decision doc** (case a/b/c + chosen response).
- If case (b) and default response chosen: adapter-snapshot rollback path covered by its own unit test.

### Epic 5 — Env-var activation

Uncomment the 10 pre-seeded vars (Sprint B legacy) in 4 files:
- `.env.example:419-429`
- `docker-compose.yml`
- `docker-compose.dev.yml`
- `internal/cli/compose_templates/docker-compose.yml`

**Gate:**
- `grep -c '^ROUTER_AUX_LOSS_COEF' .env.example` returns `1` (not commented).
- Compose-template parity check (existing CI) still green.
- `mdemg init --quick` (smoke) reads new env vars without error.

### Epic 6 — Tests (3-tier)

**Tier 1 (Unit):**
- `test_expert_selection.py`:
  - `test_load_valid_artifact` — Sprint D `profile_routing_reasoning_think.json` → 40 layers × 64 experts.
  - `test_load_family_mismatch` — raises on family name mismatch.
  - `test_load_bad_schema` — missing `per_layer` → raises.
  - `test_mlx_lm_keys_count` — exactly 7,680 strings.
  - `test_mlx_lm_keys_uniqueness` — all unique.
  - `test_mlx_lm_keys_format` — regex match against `language_model.model.layers.\d+.mlp.experts.\d+.(down|up|gate)_proj`.
- `test_quantize_asymmetric.py`:
  - 8 path classifications (attn×2, shared×2, router×1, routed×2, other×1).
  - `--dry-run` summary count test.
- `test_early_stop.py`:
  - SFT val-loss trajectory fires at correct line.
  - Patience=2 gating.
  - Non-matching lines ignored.
  - RL regex (placeholder) parses but doesn't fire (untested path documented).
- Extended `test_train_ft.py`:
  - `--tier 2` without `--family` exits 2.
  - `--n-epochs auto` exits 2 with FT-OAI-001 message.
  - `test_epoch_cap_rejection_not_clamp` — `--n-epochs 5` with cap=3 exits non-zero; verify stdout contains rejection message (not silently clamped to 3).
  - SHA mismatch on `--expected-sha256` exits 2 — **asserted for BOTH Tier 1 and Tier 2** (`test_sha_check_tier1` explicitly covers Tier 1).
  - `test_config_injection_atomicity` — simulate SIGTERM mid-training against a copy of the Sprint C config; verify config.json is restored to pre-train state with matching SHA256 (pre-train-backup is cleaned up; `.training` sidecar is not left behind).
  - `test_integrity_check_drift_detection` — pre-mutate a copy of config.json; verify `train_ft.py` refuses to start with an informative error naming the drifted fields.
  - `test_base_adapter_flag_wired` — create a stub adapter directory (empty `adapter.safetensors` + config stub); invoke `train_ft.py --tier 2 --base-adapter <stub> … --dry-run`; assert the mlx_lm.lora argv printed in dry-run stdout contains the stub adapter path in the expected position. (Not a real integration test — catches "flag parsed but not threaded through subprocess builder" in Sprint E, not Phase 5.)
  - Dry-run Tier 1 invocation shape (exact mlx_lm.lora argv asserted).
  - Dry-run Tier 2 invocation shape (argv + 7,680 keys present).

**Tier 2 (Integration):** `test_train_ft_integration.py` — dry-run both tiers against Sprint C local model path + a Sprint D profile; asserts subprocess would be invoked correctly but passes `--dry-run` to short-circuit. Validates module-path string matches against `Qwen3_5MoeForConditionalGeneration.named_parameters()`.

**Tier 3 (E2E):** `scripts/sprint_e_e2e_dry_run.sh` — runs Phase 5's full invocation matrix in `--dry-run`:
1. Tier 1 universal adapter invocation.
2. Tier 2 × 3 families.
3. `quantize_asymmetric.py --dry-run` against Sprint C model.
All 5 dry-runs exit 0 with valid stdout summaries.

**Gate:** all three tiers green; `pytest -xvs neural/training/tests/` clean; E2E script exit 0; no untracked build artifacts; test count added to sprint-close commit body.

### Epic 7 — Documentation (final epic — never cut)

Per MEMORY `feedback_sequential_epics.md` + `feedback_mandatory_testing_tiers.md`: docs ship last, before commit.

1. Copy `~/.claude/plans/breezy-dancing-lerdorf.md` → `docs/development/ft-lora/sprint_plan_ft_lora_e.md`.
2. Append "Documents Accessed" appendix to the copied plan.
3. `00_README_v2.md` v5.3 → v5.4: Document Map row for Sprint E plan; Key Decisions row "Training infra patched: Phase 5 unblocked" with link to decision doc; Sprint E status marker.
4. `03_IMPLEMENTATION_PLAN_v2.md` §Phase 5.X: mark Sprint E executed, link new modules + test file, mark Phase 5 SFT as **unblocked**, add Tier 1 and Tier 2 invocation cheat-sheets (see §Post-Sprint-E below).
5. `AGENT_HANDOFF.md` — Sprint E completion entry at top.
6. `CHANGELOG.md` `[Unreleased]` `### Added`:
   - `neural/training/train_ft.py` — tier-aware CLI (7 new flags + router_aux override).
   - `neural/training/expert_selection.py` — Sprint D profile loader → mlx_lm `keys` list.
   - `neural/training/quantize_asymmetric.py` — attn+shared BF16 / routed mxfp4 predicate + dry-run CLI.
   - `neural/training/early_stop.py` — subprocess stdout monitor (SFT val_loss × 1.05 × 2).
   - 10 env vars activated (`ROUTER_AUX_LOSS_COEF`, tier ranks/alphas, epoch cap, early-stop thresholds, asymmetric-quant specs).
7. Cross-reference check: every Sprint-E pointer resolves; `sprint_plan_ft_lora_e.md` renders without broken links.

**Gate:** sprint plan in repo; all cross-refs valid; CHANGELOG + AGENT_HANDOFF current; PR comment draft prepared (title + summary + Tier-1/Tier-2 cheat-sheet for Phase 5 consumer).

## 6. Testing Plan (Three Tiers)

Covered by Epic 6. Summary:

- **Tier 1 (Static + Unit):** pytest on 3 new test files + extended `test_train_ft.py`; ruff + mypy clean; `--help` smoke.
- **Tier 2 (Integration):** `test_train_ft_integration.py` dry-run against real Sprint C model path + Sprint D profiles; module-path strings matched to real `named_parameters()`.
- **Tier 3 (E2E):** `scripts/sprint_e_e2e_dry_run.sh` — full Phase 5 invocation matrix (5 dry-runs), exit 0.

State restoration (MEMORY): zero mutation — all dry-runs; no adapter files written; no TSDB/graph writes; Sprint C model untouched.

## 7. Commit Strategy

Single batched commit at sprint close (MEMORY):

- Title: `feat(ft-lora): Sprint E — tier-aware LoRA CLI + asymmetric quant + early-stop`
- Body: one bullet per epic + a **"Phase 5 unblock"** section with:
  - Tier 1 sample invocation.
  - 3 × Tier 2 sample invocations (one per family).
  - 1 × asymmetric-quant invocation.
  - `router_aux_loss_coef` injection path (yaml-config vs config.json-patch decision documented).
  - Early-stop test results (patience fire matrix).
- Footer: `Co-Authored-By: Claude Opus 4 <noreply@anthropic.com>`

Push to `reh3376_dev01` → auto-PR opens → sprint summary comment posted on PR (MEMORY `feedback_sprint_summary_on_pr.md`) including the 5 invocation cheat-sheets and "Phase 5 SFT is now unblocked" statement.

## 8. Verification Checklist

- [ ] Pre-gate: branch at `3871222`, tree clean, venv + mlx_lm 0.31.2 + Sprint D artifacts present
- [ ] Epic 1: 7 new flags on `train_ft.py`; `--help` green; all rejection paths (auto / cap / tier mismatch / SHA) exit non-zero
- [ ] Epic 1: `router_aux_loss_coef` injection path chosen and documented (primary YAML verified OR fallback atomic-config path chosen, with grep evidence recorded)
- [ ] Epic 1: signal-safe config.json restoration verified via simulated SIGTERM; SHA re-match asserted
- [ ] Epic 1: both Tier 1 and Tier 2 enforce `--expected-sha256` integrity check; drifted-config test refuses to start
- [ ] Epic 1: epoch cap enforcement rejects (not silently clamps); explicit test
- [ ] Epic 2: `expert_selection.py` loads 3 Sprint D profiles; emits 7,680-key list for each
- [ ] Epic 3: `quantize_asymmetric.py` predicate classifies 8 unit-test paths correctly; dry-run summary against Sprint C model
- [ ] Epic 4: `early_stop.py` fires on synthetic trajectory with patience=2; non-matching lines ignored
- [ ] Epic 4: mlx_lm.lora checkpoint behavior verified empirically (case a/b/c recorded in decision doc + response chosen)
- [ ] Epic 6: `test_base_adapter_flag_wired` asserts stub adapter path threads through to mlx_lm.lora argv
- [ ] Epic 5: 10 env vars uncommented in 4 files; compose-template parity check green
- [ ] Epic 6: pytest green (3 new test files + extended existing); integration test green; E2E script exit 0
- [ ] Epic 7: sprint plan copied to `docs/development/ft-lora/sprint_plan_ft_lora_e.md`
- [ ] Epic 7: `00_README_v2.md` 5.3→5.4; `03_IMPLEMENTATION_PLAN_v2.md` Phase 5 unblocked
- [ ] Epic 7: AGENT_HANDOFF + CHANGELOG current
- [ ] Commit pushed; auto-PR opened; sprint summary comment posted with 5 invocation cheat-sheets

## 9. Documentation Update (Final Epic — Never Cut)

Covered by Epic 7. Key deliverables: `sprint_plan_ft_lora_e.md`, `00_README_v2.md` bump (5.3→5.4), `03_IMPLEMENTATION_PLAN_v2.md` Phase 5 unblock markers, AGENT_HANDOFF + CHANGELOG entries, Documents Accessed appendix.

## 10. Risks & Mitigations

| Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|
| mlx_lm.lora `--config` YAML doesn't honor `router_aux_loss_coef` (model-arch param, not trainer hyperparam) | High | Primary-path grep-check during Epic 1 dry-run; document which path works | Atomic config.json replacement with signal-safe restoration (SIGTERM/SIGINT/SIGHUP + atexit), SHA re-match on exit, `.pre-train-backup` cleanup only after verified restore |
| Training crash leaves base model config.json drifted; next run silently operates on wrong model | Medium-High | `.pre-train-backup` + signal handlers + atexit + SHA verify on every exit path | Pre-training SHA integrity check on every invocation (Tier 1 AND Tier 2) refuses to start on drift, with field-level diff |
| mlx_lm.lora writes current-state adapter only (not best-so-far) — early-stop saves worse state | Medium | Epic 4 empirical checkpoint-behavior verification against Sprint C model | Case (b) response: adapter-snapshot rollback on early-stop fire, preserving 2-consecutive-eval confidence |
| `--base-adapter` parsed but not threaded into subprocess invocation | Low-Medium | `test_base_adapter_flag_wired` dry-run argv assertion | Fix Epic 1 subprocess builder; catch in Sprint E, not Phase 5 |
| mlx_lm point-release changes `Val loss` log format | Low-Medium | Version-pinned regex; warn on `mlx_lm.__version__ != "0.31.2"`; unit test fixture captures exact format | On mismatch, update regex + bump Sprint E commit; document in decision doc |
| Module-path format differs between profile and loaded model | Low | Integration test validates against real `named_parameters()` — hard gate | Adjust key-emitter format; bounded blast radius |
| Asymmetric predicate misclassifies a category | Low-Medium | 8 unit tests + dry-run summary inspection | Add path patterns iteratively; never ship without correct category counts |
| SIGTERM on early-stop corrupts adapter file | Medium | `.partial` staging path + `.earlystop.json` sidecar; document orphan-handling | Phase 5 runbook reruns from last clean epoch checkpoint |
| `n_epochs` → `iters` conversion wrong (dataset length miscounted) | Low | Unit test with synthetic JSONL; log conversion at invocation | Phase 5 operator can override with explicit `--iters` (kept as escape hatch) |
| Sprint C SHA check blocks legitimate model upgrade | Low | `--expected-sha256` is a flag, not hardcoded; Sprint F can update pin | Override with new SHA when Qwen3.6 minor release adopted |
| 10 env-var activation breaks existing `mdemg init` flow | Low | Compose-template parity CI gate + smoke test | Revert uncomments; ship Sprint E with vars still commented and doc-only reference |

## 11. Documents Accessed

**During planning:**
- `/Users/reh3376/mdemg/neural/training/train_ft.py` — current subprocess-only surface (lines 248-264)
- `/Users/reh3376/mdemg/.env.example:419-429` — 10 pre-seeded commented env vars (Sprint B legacy)
- `/Users/reh3376/mdemg/docker-compose.yml`, `docker-compose.dev.yml`, `internal/cli/compose_templates/docker-compose.yml` — same 10 vars to activate
- `/Users/reh3376/mdemg/training_data/routing_profiles/profile_routing_reasoning_think.json` — Sprint D consumer schema
- `/Users/reh3376/mdemg/training_data/routing_profiles/profile_routing_classify_notink.json`
- `/Users/reh3376/mdemg/training_data/routing_profiles/profile_routing_structured_notink.json`
- `/Users/reh3376/mdemg/docs/development/ft-lora/sprint_c_d_profile_results.md` — Sprint D decision doc (Recommendation to Sprint E)
- `/Users/reh3376/mdemg/docs/development/ft-lora/sprint_plan_ft_lora_d.md` — Sprint D plan (Sprint-E successor reference)
- `/Users/reh3376/mdemg/docs/development/ft-lora/00_README_v2.md` — v5.3 → v5.4 bump target
- `/Users/reh3376/mdemg/docs/development/ft-lora/01_RESEARCH_v2.md §5` — MoE-Sieve two-tier strategy
- `/Users/reh3376/mdemg/docs/development/ft-lora/03_IMPLEMENTATION_PLAN_v2.md §Phase 5.X` — Phase 5 consumer spec
- `/Users/reh3376/.venv/mdemg-ft-lora/lib/python3.12/site-packages/mlx_lm/tuner/utils.py:85` — `linear_to_lora_layers(keys=...)`
- `/Users/reh3376/.venv/mdemg-ft-lora/lib/python3.12/site-packages/mlx_lm/tuner/trainer.py:55,287-315,304` — eval interval, validation callback, `Val loss` log format
- `/Users/reh3376/.venv/mdemg-ft-lora/lib/python3.12/site-packages/mlx_lm/convert.py:20-77,96-97,218` — `mixed_quant_predicate_builder`, `quant_predicate` callable, `q_mode` enum
- `/Users/reh3376/.venv/mdemg-ft-lora/lib/python3.12/site-packages/mlx_lm/utils.py:375-376` — mxfp4 params
- `/Users/reh3376/.venv/mdemg-ft-lora/lib/python3.12/site-packages/mlx_lm/lora.py:58,129` — CLI surface (no `--router-aux-loss-coef`)
- `/Users/reh3376/mdemg/CLAUDE.md` — overfitting-prevention policies, MEMORY rules
- `/Users/reh3376/.claude/projects/-Users-reh3376-mdemg/memory/MEMORY.md` — sequential epics, no hardcoded values, 3-tier testing, sprint-plan location, sprint summary on PR, plan-before-code
- `/Users/reh3376/mdemg/training_data/openai_ft/20260420/run_notes.md` — FT-OAI-001 step-1200 overfit forcing function (referenced in error messages)

**Referenced but not read in-depth (will read during execution):**
- `neural/training/tests/test_train_ft.py` — existing test structure (extend)
- `scripts/` — location for E2E dry-run script

## 12. Rollback

All changes are additive (new files + new CLI flags + env-var uncomments). Rollback = `git revert <sha>`. No DB migrations, no training artifacts, no shared-state mutation. Sprint C model unmodified. Sprint D artifacts unmodified.

---

## Post-Sprint-E — Phase 5 Invocation Cheat-Sheet

Goes into `03_IMPLEMENTATION_PLAN_v2.md §Phase 5.X` + PR comment. Phase 5 runbook consumes directly.

**Tier 1 (universal, one adapter for all 16 tasks):**
```
python neural/training/train_ft.py \
  --tier 1 \
  --mode sft \
  --base-model <sprint-c-path> \
  --dataset training_data/sft/mdemg_sft.jsonl \
  --adapter-path adapters/tier1_attn_shared/ \
  --rank 32 --alpha 64 \
  --n-epochs 3 \
  --router-aux-loss-coef 0.002 \
  --early-stop-ratio 1.05 --early-stop-patience 2
```

**Tier 2 (per family, three adapters — run after Tier 1 completes):**
```
# reasoning-think (7 tasks)
python neural/training/train_ft.py --tier 2 --family reasoning-think \
  --expert-selection-path training_data/routing_profiles/profile_routing_reasoning_think.json \
  --expected-sha256 cdc167566e54ebe6d5c6df308649670b5f1cacfe71a198688edba8471ea64734 \
  --base-adapter adapters/tier1_attn_shared/ \
  --dataset training_data/sft/family_reasoning_think.jsonl \
  --adapter-path adapters/tier2_reasoning_think/ \
  --rank 8 --alpha 16 --n-epochs 3 \
  --router-aux-loss-coef 0.002 \
  --early-stop-ratio 1.05 --early-stop-patience 2

# classify-notink (6 tasks) — same shape, substitute family + profile + dataset + output
# structured-notink (3 tasks) — same shape, substitute family + profile + dataset + output
```

**Asymmetric quant (after Tier 1 + Tier 2 all converged):**
```
python neural/training/quantize_asymmetric.py \
  --input-model adapters/merged_final/ \
  --output-model adapters/merged_final_mxfp4/ \
  --shared-bits bf16 --routed-spec mxfp4 --attn-bits bf16
```

**Gating reminder for Phase 5:** Tier-2 training cannot start until Tier 1 adapter exists (composition dependency). Sprint E exposes `--base-adapter` to load Tier 1 as the base for Tier 2 training; Phase 5 runbook owns the sequencing.

Phase 5 SFT unblocks the moment Sprint E merges.

---

## Post-Execution Notes (2026-04-22)

Captured at sprint close for the Phase 5 runbook.

### Router-aux injection path
Dual-path strategy implemented as designed, with one execution-time detail: the fallback atomic config.json replacement is **always installed** regardless of primary-path verification, so a silent primary-path failure cannot cause `router_aux_loss_coef` to be dropped. The primary `--config train_config.yaml` path writes `router_aux_loss_coef: 0.002` alongside `lora_parameters.{rank,alpha,keys}`; `verify_router_aux_in_stdout()` grep-checks for coef evidence during the real run. If the grep check passes, the fallback restore is a no-op (config.json was not mutated). Decision doc candidate: after the first real Phase 5 Tier 1 run, record which path actually took effect and whether the primary path's YAML is authoritative for model-arch params in mlx_lm 0.31.2.

### Checkpoint behavior (Epic 4 gate)
Not empirically verified in Sprint E — no real training was launched (Sprint E is dry-run-only per §3). The `EarlyStopMonitor` writes a `<adapter-path>.earlystop.json` sidecar with the full val-loss/reward history and best-iter pointer regardless of whether mlx_lm.lora emits a best-so-far checkpoint alongside current-state. Phase 5 runbook owns the empirical verification during the first real Tier 1 run and records the finding (case a/b/c) plus chosen response. Sprint E ships with the `.earlystop.json` sidecar in place so the Phase 5 verification has full state to inspect.

### Env-var activation count
Plan said 10 vars; actual pre-seeded count in `.env.example` is **11** (LORA_TIER1_RANK + LORA_TIER1_ALPHA + LORA_TIER2_RANK + LORA_TIER2_ALPHA + LORA_N_EPOCHS_CAP + LORA_EARLY_STOP_SFT_THRESHOLD + LORA_EARLY_STOP_RL_THRESHOLD + ROUTER_AUX_LOSS_COEF + ASYMMETRIC_QUANT_SHARED + ASYMMETRIC_QUANT_ROUTED + ASYMMETRIC_QUANT_ATTN = 11). All 11 activated.

### Files modified for env-var activation
Plan listed 4 files; actual is **3**: `.env.example`, `docker-compose.yml`, `internal/cli/compose_templates/docker-compose.yml`. `docker-compose.dev.yml` is an *overlay* that only adds a `neo4j-monitor` sidecar container — it carries no server-level env vars and therefore has no vars to activate. Parity check between `docker-compose.yml` and `internal/cli/compose_templates/docker-compose.yml` remains green.

### mlx_lm.lora CLI surface
Verified during Epic 1 smoke-test: mlx_lm 0.31.2 exposes `--num-layers`, not `--lora-layers`. The initial implementation emitted both; corrected to emit only `--num-layers`. `build_mlx_lm_command()` unit test asserts `--lora-layers` is absent to prevent regression.

### Test count (Epic 6)
- Unit (Tier 1): 89 tests across test_expert_selection.py (18) + test_quantize_asymmetric.py (15) + test_early_stop.py (18) + test_train_ft.py (38).
- Integration (Tier 2): `test_train_ft_integration.py` runs against Sprint C model path; 5 passed + 1 skipped (heavy full-model load, gated on SPRINT_E_HEAVY_INTEGRATION=1).
- E2E (Tier 3): `scripts/sprint_e_e2e_dry_run.sh` — 4 dry-runs green (Tier 1 + 3× Tier 2); step 5 (asymmetric-quant dry-run) requires SPRINT_E_HEAVY_INTEGRATION=1 for full-model load.
- **Full suite: 94 passed, 1 skipped.**
