Save the block below as `MDEMG_FT_LORA_PACKAGING_SPEC.md` and hand it to your planning agent.

# MDEMG FT-LoRA Packaging & Distribution Specification

**Document ID:** MDEMG-FT-PKG-001
**Version:** 1.0
**Date:** 2026-04-24
**Author (source):** Engineering review, based on MDEMG repo state and current PEFT / MLX / Homebrew best practices
**Intended Consumer:** Planning Agent (sprint formalization)
**Status:** Ready for sprint decomposition
**Related Artifacts:**
- MDEMG Risk & Opportunity Analysis (2026-04-23) — specifically R2 (Model Dependency Fragility Post-Pivot)
- `qwen3-14b-mdemg-v1` fine-tuned model (Phase 5 SFT, shipped)
- Homebrew tap: `reh3376/mdemg`

---

## 1. Purpose

This document specifies how to package, distribute, and install the MDEMG LoRA fine-tuned model (built on Qwen3-14B FP4 dense base) as an **optional component** of the `brew install mdemg` flow. It provides the architectural decision, rationale, Homebrew integration pattern, distribution backend, deployment targets, and a sprint-ready task breakdown.

The planning agent should use this as direct input for creating epics, stories, and acceptance-criteria-driven tasks.

---

## 2. Executive Summary

**Decision: Ship the LoRA adapter disaggregated from the base model. Do not bake either the base or a fused model into the Homebrew bottle.** Instead:

1. Publish the LoRA adapter as an independently versioned artifact (Hugging Face Hub recommended).
2. Keep the Homebrew formula weight-free; install only the `mdemg` binary.
3. Introduce a `mdemg model install|list|verify|remove|fuse` CLI subcommand family that fetches the base (if missing) and the adapter on demand, verifying SHAs against the existing manifest system.
4. Offer an optional companion formula (`mdemg-ft-model`) whose `post_install` simply shells out to `mdemg model install qwen3-14b-mdemg-v1` for users who want a one-command experience.
5. Treat fusing (merging adapter into base) as a **local post-install optimization**, not as a shipped artifact, and only if benchmarks justify it.

This path matches how MLX, Hugging Face PEFT, vLLM, llama.cpp, and Ollama all handle LoRA distribution in 2026, preserves cross-platform portability, and keeps the brew install small and fast.

---

## 3. Decision Rationale

### 3.1 Why adapter-only wins over fused/merged shipping

- **Size & bandwidth.** Qwen3-14B at FP4/MXFP4 is ~8–10 GB. A typical LoRA adapter for that base is 50–400 MB depending on rank and target modules. Users who already have the base locally (via Ollama, LM Studio, HF cache, or a prior MDEMG install) pay zero incremental storage; fused shipping forces a full re-download of the base every release.
- **Reproducibility & provenance.** PEFT's `adapter_config.json` is the canonical contract: it records `base_model_name_or_path`, `r`, `lora_alpha`, `target_modules`, and revision. This contract is lost on fusion. MDEMG's existing manifest-SHA strategy aligns naturally with this convention.
- **Interoperability.** A raw adapter is consumable by MLX (`adapters.safetensors` / `adapters.npz`), Hugging Face `peft`, vLLM multi-LoRA serving, llama.cpp (GGUF LoRA), and Ollama (`ADAPTER` directive in Modelfiles). A fused 14B FP4 artifact locks the consumer to a specific quantization format and forces N-variant shipping.
- **Iteration speed.** Retraining the adapter with a pinned base means a small artifact rollover and a manifest bump — not a multi-GB re-release.
- **Bus-factor / disaster recovery.** A small, CDN-hosted adapter is trivially mirror-able. A fused artifact is not.

### 3.2 Where fusing is justified

Fusing is only defensible if benchmarks show material steady-state inference regression from runtime adapter application. Both MLX and vLLM apply LoRA at near-zero cost; llama.cpp has small overhead. If fusion is ever needed, it should be a local, on-device optimization (`mdemg model fuse`), not the shipped artifact.

### 3.3 Why not bake weights into the Homebrew bottle

- Homebrew bottles are CDN-mirrored and designed to be small and cacheable; multi-GB ML weights are an anti-pattern.
- Recent Homebrew versions deprecated `option` / `with-` flags for core formulae, so `brew install --with-ft-model` is not idiomatic. The 2026-correct pattern is caveats + a CLI subcommand.
- Coupling weights to formula releases means every model re-train triggers a formula version bump, breaking the cadence model.

---

## 4. Target Architecture

Three artifacts, three delivery paths, one CLI surface:

| Artifact | Size | Shipped by MDEMG? | Delivery Path |
|----------|------|-------------------|---------------|
| Base model (Qwen3-14B FP4) | ~8–10 GB | No | Detect existing copy (HF cache, Ollama, LM Studio) or fetch from HF on first run |
| LoRA adapter (`qwen3-14b-mdemg-v1`) | ~100–400 MB | **Yes** | Versioned, SHA-pinned, hosted on HF Hub (or S3 + CDN) |
| Inference sidecar (MLX / llama.cpp / vLLM client) | Small (MB) | Yes | Homebrew bottle |

### 4.1 On-disk layout (user machine)

```
~/.mdemg/
├── models/
│   └── qwen3-14b-mdemg-v1/
│       ├── manifest.json            # SHAs, versions, base pointer
│       ├── adapter_config.json      # PEFT-standard config
│       ├── adapter_model.safetensors
│       └── (optional) fused/        # produced by `mdemg model fuse`
└── cache/
    └── base/qwen3-14b-fp4/          # symlink or copy of base, shared across adapters
```

### 4.2 Runtime resolution order for the base model

The `mdemg model install` command resolves the base in this order:

1. `$MDEMG_BASE_MODEL_PATH` (explicit override).
2. `~/.mdemg/cache/base/qwen3-14b-fp4/`.
3. `~/.cache/huggingface/hub/models--Qwen--Qwen3-14B*` (if present and revision matches manifest).
4. Ollama model store (if Ollama is installed and the base is present).
5. LM Studio model directory.
6. If none present: fetch from HF Hub into `~/.mdemg/cache/base/`.

---

## 5. Homebrew Integration

### 5.1 Pattern A — Single formula + CLI subcommand (recommended)

The formula installs only the binary. All model management is delegated to `mdemg model`.

```ruby
class Mdemg < Formula
  desc "Multi-Dimensional Emergent Memory Graph"
  homepage "https://github.com/reh3376/mdemg"
  # url / sha256 / license as usual

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(output: bin/"mdemg"), "./cmd/mdemg"
  end

  def caveats
    <<~EOS
      MDEMG is installed without ML weights.

      To enable the local fine-tuned model:
        mdemg model install qwen3-14b-mdemg-v1

      This will:
        • Locate or fetch the Qwen3-14B FP4 base (~9 GB)
        • Fetch the MDEMG LoRA adapter (~200 MB)
        • Verify SHAs against the MDEMG manifest
        • Install under ~/.mdemg/models/

      To list / verify / remove installed models:
        mdemg model list | verify | remove <name>
    EOS
  end

  test do
    system "#{bin}/mdemg", "version"
  end
end
```

### 5.2 Pattern B — Companion formula (optional convenience)

```
brew install reh3376/mdemg/mdemg               # core
brew install reh3376/mdemg/mdemg-ft-model      # optional; one-command FT setup
```

The `mdemg-ft-model` formula is weight-free; its `post_install` executes:

```ruby
def post_install
  system "#{HOMEBREW_PREFIX}/bin/mdemg", "model", "install", "qwen3-14b-mdemg-v1", "--non-interactive"
end
```

`brew uninstall mdemg-ft-model` becomes a no-op at the formula level but can optionally call `mdemg model remove qwen3-14b-mdemg-v1` via a `uninstall_postflight` or documentation.

### 5.3 Why not `resource` blocks for the weights

Homebrew's `resource` block technically supports declaring additional URLs with SHA256 pinning, but:

- Download resume is limited.
- Retry semantics are not tuned for multi-hundred-MB or multi-GB artifacts.
- HF authentication (for gated bases) is not supported.
- Progress UX is poor for long-running downloads.

Defer to the `mdemg` CLI — it can use `huggingface_hub` semantics, resume partial downloads, handle auth, and integrate with MDEMG's existing manifest SHA strategy.

---

## 6. Distribution Backend

### 6.1 Primary: Hugging Face Hub

- **Repo:** `reh3376/qwen3-14b-mdemg-lora-v1`
- **Contents:** `adapter_config.json` (base pointer + LoRA hyperparameters), `adapter_model.safetensors`, `README.md` (training card, eval results, license), `tokenizer*.json` if any tokenizer deltas exist, `MDEMG_MANIFEST.json` (MDEMG-specific SHA + compatibility matrix).
- **Access mode:** Public by default; gated if licensing requires it.
- **Advantages:** Free CDN, resumable downloads via `huggingface_hub`, canonical PEFT layout, discoverability, ecosystem tooling compatibility.

### 6.2 Secondary: S3-compatible object store

Used as a mirror or primary for air-gapped / enterprise customers. `mdemg model install --source s3://bucket/path` must be supported.

### 6.3 Offline / air-gapped path

`mdemg model export qwen3-14b-mdemg-v1 --out mdemg-ft-v1.tar.zst` produces a single archive containing base + adapter + manifest. `mdemg model import mdemg-ft-v1.tar.zst` is the inverse. This path is required for industrial deployments (e.g., Whiskey House use case).

---

## 7. Deployment Targets

The adapter destination must be a parameter, not a hardcoded assumption. Three runtime topologies, one artifact:

| Topology | Adapter location | Loader |
|----------|------------------|--------|
| Local (Apple Silicon) — default | `~/.mdemg/models/qwen3-14b-mdemg-v1/` | MLX in-process sidecar |
| Local (Linux/CUDA) | same path | llama.cpp with `--lora` or Ollama with `ADAPTER` directive |
| Remote inference cluster | adapter path or HF reference pushed to endpoint | vLLM `--enable-lora` + `LoRARequest`, or TGI |

Configuration key: `mdemg.config.yaml → ml.inference.target = {mlx|llama_cpp|ollama|vllm|tgi}` with target-specific endpoint / path parameters.

---

## 8. CLI Surface (to be implemented)

```
mdemg model install <name> [--source hf|s3|file] [--base-model <path|hf-ref>]
                           [--no-base-fetch] [--non-interactive] [--verify]
mdemg model list
mdemg model verify <name>        # re-check SHAs vs manifest
mdemg model remove <name>        # removes adapter; base is preserved unless --with-base
mdemg model fuse <name>          # local optimization; produces fused/ dir
mdemg model export <name> --out <archive>
mdemg model import <archive>
```

Exit codes must be stable for scripting; progress must be machine-readable with `--json`.

---

## 9. Acceptance Criteria (top-level)

The full deliverable is accepted when all of the following hold:

1. `brew install mdemg` installs a binary-only distribution under 200 MB total.
2. `mdemg model install qwen3-14b-mdemg-v1` succeeds on a clean machine with no prior base, producing a working MLX inference path.
3. On a machine with an existing Qwen3-14B FP4 in any of the recognized cache locations, `mdemg model install` detects and reuses it (verifiable via logs and disk-usage delta).
4. `mdemg model verify` returns non-zero iff any SHA mismatches the manifest.
5. At least one non-Apple inference path is documented and smoke-tested (llama.cpp or vLLM with adapter loading).
6. The adapter repo on HF Hub has a valid `adapter_config.json` referencing the exact pinned Qwen3-14B FP4 base and revision.
7. Homebrew formula tests (`brew test mdemg`, `brew audit --strict`) pass.
8. Optional `mdemg-ft-model` companion formula installs and invokes `mdemg model install` successfully in `post_install`.
9. Offline export/import round-trip produces byte-identical manifest SHAs.
10. All artifacts and flows are documented in `docs/ml/ft-lora-packaging.md`.

---

## 10. Sprint-Ready Task Breakdown

### Epic 1 — Adapter publication

- **Story 1.1** Produce PEFT-standard `adapter_config.json` + `adapter_model.safetensors` from the current training artifact.
- **Story 1.2** Create and populate `reh3376/qwen3-14b-mdemg-lora-v1` on HF Hub with model card, license, eval results, and `MDEMG_MANIFEST.json`.
- **Story 1.3** Pin exact base model repo + revision; verify load via `peft.PeftModel.from_pretrained`.
- **Acceptance:** Repo loads via `AutoPeftModelForCausalLM.from_pretrained` on a clean Python environment.

### Epic 2 — `mdemg model` CLI

- **Story 2.1** Implement `install` with source detection (HF cache, Ollama, LM Studio, explicit override) and SHA verification.
- **Story 2.2** Implement `list`, `verify`, `remove`.
- **Story 2.3** Implement `export` / `import` for offline workflows.
- **Story 2.4** Implement `fuse` as a local optimization path (no-op by default, opt-in).
- **Acceptance:** All commands return stable exit codes and `--json` machine-readable output; integration tests cover clean-install, reuse-existing, corrupted-manifest, and offline round-trip.

### Epic 3 — Homebrew formula refactor

- **Story 3.1** Remove any weight-bearing `resource` blocks; shrink bottle to binary-only.
- **Story 3.2** Add caveats pointing to `mdemg model install`.
- **Story 3.3** Create optional `mdemg-ft-model` companion formula with `post_install` hook.
- **Story 3.4** Run `brew audit --strict` and fix findings.
- **Acceptance:** Clean brew install under 200 MB; companion formula results in a working FT model in one command.

### Epic 4 — Cross-platform inference matrix

- **Story 4.1** Smoke-test adapter load via MLX on Apple Silicon (baseline).
- **Story 4.2** Smoke-test adapter load via llama.cpp (convert adapter to GGUF LoRA if required) on Linux/CUDA.
- **Story 4.3** Smoke-test adapter load via vLLM multi-LoRA on Linux/CUDA.
- **Story 4.4** Document compatibility matrix and known caveats.
- **Acceptance:** All three targets produce identical outputs (within quantization tolerance) on a fixed eval prompt set.

### Epic 5 — Documentation & onboarding

- **Story 5.1** Write `docs/ml/ft-lora-packaging.md` covering architecture, install paths, and air-gapped workflow.
- **Story 5.2** Update `README.md` quickstart with the optional FT install step.
- **Story 5.3** Add `FAQ` entries for common failure modes (base not found, SHA mismatch, cache permission issues).
- **Acceptance:** Docs reviewed; external user can complete clean install and first inference in <30 minutes on broadband.

### Epic 6 — Observability & manifests

- **Story 6.1** Emit install / verify / load metrics to the existing TSDB.
- **Story 6.2** Add Grafana panel for model-install latency, cache-hit rate, and SHA verification failures.
- **Acceptance:** Panels populated with data from at least one install on each supported platform.

---

## 11. Dependencies & Sequencing

| Epic | Depends on | Blocks |
|------|-----------|--------|
| 1 — Adapter publication | current training artifact | 2, 3, 4 |
| 2 — CLI | 1 | 3 (companion formula), 5 |
| 3 — Homebrew formula | 2 | 5 |
| 4 — Cross-platform | 1, 2 | 5 |
| 5 — Documentation | 2, 3, 4 | — |
| 6 — Observability | 2 | — (parallelizable) |

Suggested sprint sequencing: Epics 1 + 2 in Sprint N, Epics 3 + 4 in Sprint N+1, Epics 5 + 6 in Sprint N+2 (with 6 ideally running in parallel with 2).

---

## 12. Success Metrics

- `brew install mdemg` median completion time ≤ 60 s on broadband.
- `mdemg model install qwen3-14b-mdemg-v1` median end-to-end time ≤ 10 min on broadband when base must be fetched, ≤ 2 min when base is reused.
- Disk footprint saving ≥ 80% for users with a pre-existing Qwen3-14B base vs. fused-shipping alternative.
- Adapter artifact update cadence decoupled from `mdemg` binary release cadence (independent version numbers).
- Cross-platform parity: eval prompt set outputs match within quantization tolerance on MLX, llama.cpp, and vLLM.

---

## 13. Out of Scope (explicitly deferred)

- Training pipeline changes (covered in MDEMG phase registry; see Phase 5 / Phase 10 / Sprint F tracks).
- Automatic multi-adapter hot-swapping at runtime (possible via vLLM / MLX but not required for v1).
- Signed artifact attestations (sigstore / cosign) — recommended for v2.
- Per-tenant gated model access — recommended for enterprise tier, not required for OSS flow.

---

## 14. Planning Agent Instructions

When decomposing this document:

1. Treat each **Epic** in §10 as a candidate sprint epic; each **Story** as a backlog item.
2. Lift the **Acceptance Criteria** from §9 as the Definition of Done at the sprint level, and the per-epic acceptance criteria as story-level DoD.
3. Respect the **dependency graph** in §11 when sequencing.
4. Track the **Success Metrics** in §12 as the sprint-retrospective evaluation criteria.
5. Treat items in §13 as explicit non-goals; do not auto-expand scope.
6. Cross-reference R2 (Model Dependency Fragility Post-Pivot) in the MDEMG Risk & Opportunity Analysis — this document is the concrete remediation plan for R2.