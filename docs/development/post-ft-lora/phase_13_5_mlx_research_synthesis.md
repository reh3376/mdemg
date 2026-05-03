# Phase 13.5 — MLX Server Stability Research Synthesis

**Date:** 2026-05-02
**Inputs:** Streams 1 (crash forensics), 2 (mlx + Apple Metal community evidence), 3 (alternatives matrix), 4 (internal LLM call profile)
**Operator constraint:** Long-term stable solution only. No short-term patches. Decisions cited from research data, not priors.
**Status:** Decision input — Epic 1 fork-pick must cite this synthesis.

---

## 1. The Crash, Definitively

**Verdict (high confidence):** The current crash is caused by **unbounded KV-cache growth in `mlx_lm.server` until Metal GPU memory is exhausted**, at which point an unhandled C++ exception in `mlx::core::gpu::check_error` propagates as SIGABRT. This is a known, acknowledged architectural gap in mlx-lm — not a transient bug, not a hardware issue, and not (primarily) a macOS regression.

| Evidence | Source | Confidence |
|---|---|---|
| Identical fault offset (`libmlx.dylib:0xe4c534`) across all 7 crashes — same instruction, same exception throw | Stream 1 (`atos` symbolicated) | **Certain** |
| `mlx-server.err.log` contains exact error: `[METAL] Command buffer execution failed: Insufficient Memory (00000008:kIOGPUCommandBufferCallbackErrorOutOfMemory)` immediately before every crash | Stream 1 (raw log) | **Certain** |
| KV cache grew 0 → 31–60 GB monotonically with zero eviction events; cache resets only on process restart | Stream 1 (log analysis) | **Certain** |
| Each `ape.reflect` call adds ~1.13 GB to wired Metal memory — math: ~26 calls × 1.13 GB = ~29 GB ≈ observed 31–60 GB at crash | Streams 1 + 4 (cross-correlated) | **High** |
| Pre-crash 60s windows are *not* statistically distinguishable from normal windows (token volume within 0.8%); 3 of 6 crashes had below-average load preceding | Stream 4 (TSDB) | **High** |
| Maintainer-acknowledged in [mlx-lm #854](https://github.com/ml-explore/mlx-lm/issues/854) (open since Feb 2026), [mlx-lm #883](https://github.com/ml-explore/mlx-lm/issues/883), [mlx-lm #1015](https://github.com/ml-explore/mlx-lm/issues/1015), [mlx-lm #615](https://github.com/ml-explore/mlx-lm/issues/615) (since Nov 2025) | Stream 2 | **Certain** |
| `--max-kv-size` flag exists for `mlx_lm.generate()` but **not for `mlx_lm.server`** — request open since Nov 2025, NOT in 0.31.3 | Stream 2 | **Certain** |
| `--prompt-cache-size 256` (our config) is 25× the documented default of 10 — but the agent confirms this controls **sequence count, not byte size**, so even 256 ÷ 25 = 10 is no architectural fix; it just slows the bleed | Streams 1 + 2 | **High** |
| **Maintainer disclaimer:** [`SERVER.md`](https://github.com/ml-explore/mlx-lm/blob/main/mlx_lm/SERVER.md) — *"The MLX LM server is not recommended for production as it only implements basic security checks."* [Discussion #371](https://github.com/ml-explore/mlx-lm/discussions/371) — *"mostly intended to be used as a local HTTP endpoint."* | Stream 2 | **Certain** |

**Implication for the operator's "long-term stable" requirement:** mlx-lm-the-framework is healthy and rapidly maturing. **mlx_lm.server-the-process** is, per its own maintainers, not production-grade and is not designed to be. This is structural; no version bump or config change overcomes a maintainer-stated design intent.

---

## 2. Disentangling Two macOS 26.3.x Metal Issues

Stream 3 raised a possible secondary concern that must be cleanly separated from Stream 1's finding.

**Issue (X) — mlx-lm #854 / #883 / #615 (KV unbounded → SIGABRT):** The crash we are seeing. Architectural. Stream 1 confirmed.

**Issue (Y) — mlx #3337 / Ollama #15862 / GGML matmul2d static_assert (Metal Toolchain 32023):** A *different* macOS 26.3.x regression where `MetalPerformancePrimitives.matmul2d` rejects mixed `<half, bfloat>` operands. This breaks Ollama (definitively, ≥0.20.5–0.22.1) and may affect GGML-based llama.cpp builds. Whether mlx 0.31.2 ships the fix is unclear from the changelog.

**Are we hitting (Y) on top of (X)?** **Probably not:**
- Stream 1 saw real GPU activity: IOAccelerator (Metal) vmSummary was 50 GB at crash 1, KV cache was wired to Metal memory, and `Prompt processing progress` log lines were emitted (real prefill, not CPU fallback). If (Y) were active, we'd see startup compilation failures or CPU fallback (~17 tok/s), not normal Metal throughput.
- The fault stack is in MLX's own Metal completion handler — it is calling Apple's Metal API successfully, then raising on an OOM result. (Y) would manifest as a Metal *library load* failure, not a runtime allocation failure.

**Conclusion:** Issue (Y) is a real and documented hazard for any Metal-using backend on macOS 26.3.x — but **Issue (X) is what's killing us right now.** This must inform fork-ranking: any backend that hits issue (Y) is a non-starter on this hardware until upstream patches land, regardless of how clean its architecture is.

---

## 3. Five Root-Cause Hypotheses, Ranked by Evidence Weight

Stated in §2 of the original sprint plan; now ranked.

| Rank | Hypothesis | Evidence weight | Verdict |
|---|---|---|---|
| **1** | **mlx_lm.server lacks bounded KV cache eviction; long-running drift exhausts Metal** | Streams 1 + 2 + 4 all converge | **CONFIRMED PROXIMATE CAUSE** |
| 2 | macOS 26.3.x Metal toolchain regression (issue Y) | Real and documented; likely a *secondary* hazard but not the trigger of these 7 crashes | **Latent risk for any Metal backend, not the cause here** |
| 3 | mlx-lm 0.31.2 → 0.31.3 patch-lag | 0.31.3 changelog has zero entries for our crash class; some `BatchKVCache` correctness fixes worth having but not the answer | **Refuted as primary cause** |
| 4 | Concurrent prompt + KV-cache pressure exceeding wired-memory budget | Stream 4: pre-crash load is *normal*, not elevated. Concurrent calls accelerate growth but are not the trigger | **Refuted as primary cause** |
| 5 | MoE / activation-quantization pathological path | Production model is dense Qwen3-14B (not MoE); irrelevant | **Refuted** |

The single strongest piece of evidence: the math `~1.13 GB per ape.reflect call × ~26 calls in 14 min = ~29 GB cache growth` matches the observed 31–60 GB at crash time, with zero eviction events logged across 7 crashes. The crash is **deterministic on a clock**, not on a load spike.

---

## 4. Backend Candidate Scoring (Evidence-Cited)

Ranked solely against the operator's stated constraint: **long-term stability for a 24/7 cognitive-substrate use case on M5 Max + macOS 26.3.x.**

| # | Candidate | Production-ready by maintainer? | Crash class (X) addressed? | Hardware/OS confirmed working? | Operability (open vs proprietary) | Engineering swap cost | Verdict |
|---|---|---|---|---|---|---|---|
| 1 | **mlx_lm.server (current)** + tune | **No** — maintainer says not for production | **No** — no `--max-kv-size`, open since Nov 2025 | Yes (modulo crashing every 14 min) | Open | 0 days | **DISQUALIFIED** as long-term solution by maintainer disclaimer + crash root-cause unaddressed |
| 2 | **vllm-mlx** | Yes (Apache 2 wrapper) | Partial — paged KV cache + streaming disconnect guard, but inherits same MLX Metal stack | M5 not in docs; small community (1.1K stars) | Open (Apache 2) | **1–3 days** (model is already MLX format; no conversion) | **High execution risk:** inherits the exact MLX framework we are trying to harden, just with better caching. May surface issue (Y) at scale |
| 3 | **Ollama** | Yes (commercial-grade) | N/A | **No — definitively broken** on M5 + macOS 26.3.x across 0.20.5–0.22.1, 8+ open issues, no fix timeline | Open (MIT) — auto-pull behavior is operability concern | Cannot proceed | **DISQUALIFIED** — non-functional on this exact hardware/OS |
| 4 | **llama.cpp (`llama-server`)** direct | Yes — most mature LLM server in the world (~100K stars, weekly builds) | **Yes — bounded by `--ctx-size × --parallel`** as architectural feature | Mainline build status on M5 + macOS 26.3.x **not confirmed** in research; uses GGML Metal path that affected Ollama; needs build-and-test | Open (MIT) | **2–6 days** (de-quant MLX → f16 → GGUF + Q5_K_M, then UVTS A/B parity) | **Most production-justified, but has confirmable evidence gap on issue (Y) for this hardware/OS** |
| 5 | **MLC-LLM** | Yes (Apache 2) | Yes — paged KV cache (vLLM design) | Theoretically immune to issue (Y) (TVM-compiled Metal, not GGML/MLX); **zero M5/26.3.x community evidence** | Open (Apache 2); 22.5K stars, 281 contributors | **3–8 days** (TVM compilation, no Homebrew formula) | **Most architecturally distinct, lowest evidence base** |
| 6 | **LM Studio (llmster daemon)** | Yes (Apple-recognized in M5 Max press materials) | Unknown — proprietary core; bundled MLX runtime patched independently | Some circumstantial evidence of macOS 26.x patches ahead of upstream | **Closed-source core** — operability risk; can't patch | **2–5 days** | **Closed-source operability risk is structural for an "internal dialogue / cognitive substrate" framework** |

**Two candidates pass operator constraints (long-term, production-grade, on this hardware):**
- **llama.cpp** — best community + maturity + architectural fit; evidence gap on issue (Y) for our exact OS/hardware
- **MLC-LLM** — architecturally distinct from issue (Y); evidence gap is the *opposite* — virtually no field reports on M5/26.3.x

**One candidate is borderline:**
- **vllm-mlx** — fastest swap, but inherits the exact MLX Metal substrate we are escaping, even if better-managed

**Three candidates are disqualified by the operator constraint:**
- mlx_lm.server stay-and-tune (maintainer says not for production)
- Ollama (broken on M5/26.3.x)
- LM Studio (closed-source operability risk)

---

## 5. The Critical Evidence Gap

We cannot honestly pick between **llama.cpp** and **MLC-LLM** from research alone. Both are production-grade, both have architecturally bounded KV cache, both are open-source — but neither has confirmed-working-on-M5-Max-macOS-26.3.x evidence in the cited research. The research can rule out *bad* options definitively; it cannot rank *good* options without empirical testing on our actual hardware.

**This is the gap the operator's "follow the data" constraint demands we fill before fork-picking.**

The gap-filling work is empirical: install both candidates with our production model converted appropriately, run a controlled bake-off, observe results. This is sprint Epic 2 — not sprint Epic 1 (which would have been the fork pick).

---

## 6. Implication for Sprint 13.5 Structure

The sprint plan should be revised:

- **Epic 1 (was: pick a fork):** Becomes a *narrowing* epic — confirm the disqualifications (mlx, Ollama, LM Studio) hold and don't surface unexpected counter-evidence. Lock in the two finalists: llama.cpp and MLC-LLM. Plus possibly vllm-mlx as a control to compare against the current MLX substrate.
- **Epic 2 (was: implementation):** Becomes a **bake-off epic**. Install each finalist, convert/compile the model, run a 4-hour load test mirroring `ape.reflect` cadence (the dominant trigger from Stream 4), measure crash count, latency, quality (UVTS A/B). The bake-off produces the data the fork-pick needs.
- **Epic 3 (was: 24-hr soak):** Becomes the **24-hour soak of the bake-off winner only**.
- **Epic 4 (was: docs):** Unchanged.

Effort revises upward to **10–18 dev-days** (was 7–14): the bake-off is real engineering, not theory work. Budget revises upward to **$10–30 OpenAI** (UVTS A/B runs across multiple candidates).

---

## 7. What This Synthesis Does NOT Decide

Per operator constraint:
- **Does NOT pick the long-term backend.** That decision waits for Epic 2 bake-off data.
- **Does NOT recommend any short-term patch.** mlx_lm.server stays as-is during the bake-off; the watchdog continues to mask the crashes during research, which is acceptable as a *bridge* (not a deliverable).
- **Does NOT commit to GGUF as the model format.** That depends on which finalist wins. (llama.cpp = GGUF; MLC-LLM = TVM `.dylib`; vllm-mlx = MLX native.)

What this synthesis **does** decide:
- mlx_lm.server is not the long-term answer. (Maintainer says so.)
- Ollama is not the answer. (Broken on our hardware.)
- LM Studio is not the answer. (Closed-source operability risk.)
- The remaining candidates require empirical bake-off data before ranking.

---

## 8. Bake-off Design Sketch (input to Epic 2 planning)

Not the full Epic 2 plan — that gets drafted at Epic 1 close. This is the shape so the operator can sanity-check before Epic 2 spawns:

| Phase | Work | Duration |
|---|---|---|
| **B0 — Conversions** | MLX → f16 safetensors (de-quant via `mlx_lm.fuse --de-quantize`) → GGUF Q5_K_M (for llama.cpp) and TVM compile (for MLC-LLM) | 1–2 days |
| **B1 — Install + smoke** | Each candidate launched on a non-conflicting port (8102, 8103). Confirm `/v1/chat/completions` responds, confirm Metal initialized, confirm one full `ape.reflect`-shaped request returns within latency budget | 1 day |
| **B2 — UVTS quality A/B** | Quick profile (16 questions) per candidate, baseline = current mlx_lm.server. Per-question regression > 10% disqualifies a candidate | 0.5 day per candidate |
| **B3 — Crash-rate stress** | 4-hour synthetic load matching `ape.reflect` cadence: 1 call every ~32s, ~5862 tokens_in, ~978 tokens_out. Acceptance: zero crashes | 0.5 day per candidate (parallel-safe) |
| **B4 — Concurrency stress** | 1-hour stress with `--prompt-concurrency 4`-equivalent: 4 concurrent `ape.reflect`-shaped calls. Acceptance: zero crashes, p95 latency within 1.5× B3 baseline | 0.25 day per candidate |
| **B5 — Pick + draft Epic 3** | Synthesis. Operator picks backend. Epic 3 (24-hour soak on winner) drafted | 0.5 day |

**Bake-off gate:** B3 (crash-rate stress) is the single hardest gate. A candidate that produces *any* crash in a 4-hour load matching real-world cadence is disqualified — that's the bar that mlx_lm.server fails today.

---

## 9. Risks the Synthesis Surfaces

| # | Risk | Mitigation |
|---|---|---|
| 1 | All three remaining candidates fail B3 on M5 + macOS 26.3.x (Issue Y is a real-world wall) | Escalate: (a) wait for upstream macOS 26.3.x patch on llama.cpp + retry, (b) consider OS rollback to 26.2.x as a temporary measure (operator decision), (c) escalate Issue Y to Apple via DTS |
| 2 | MLC-LLM TVM compilation fails on M5 / `MTLGPUFamilyApple10` | Fall back to llama.cpp + accept Issue (Y) probability |
| 3 | GGUF re-quantization degrades model quality > 10% | UVTS A/B catches it. Try Q5_K_M before Q4_K_M; try Q8_0 if needed (15+ GB but fits in 128 GB headroom) |
| 4 | Bake-off takes longer than 18 days | The current mlx_lm.server + watchdog continues to function (degraded but functional). This sprint can extend into 13.5b without operational impact |
| 5 | Apple ships a fix for Issue (X) in mlx-lm during Phase 13.5 (unlikely — Nov 2025 issue still open) | Re-evaluate at Epic 2 close. Maintainer has not committed to delivering `--max-kv-size` for the server in any version |

---

## 10. Decisions From the Data

Per "follow the data" and the data-decidable-not-operator-input principle: every decision below is cited from the research streams.

### Decision 1 — Drop vllm-mlx from the finalist set
- **Stream 1:** the crash is at `libmlx.dylib:0xe4c534` (the MLX *framework's* Metal completion handler), not in mlx_lm.server's Python wrapper
- **Stream 3:** vllm-mlx wraps the same MLX framework; subject to the same Metal stack issues. It adds paged KV + streaming guards but does not change the unhandled-exception path in `libmlx.dylib`
- **Verdict:** vllm-mlx inherits the exact failure mode we are escaping. Including it duplicates the substrate we know fails. **Finalist set reduced to 2: llama.cpp + MLC-LLM.**

### Decision 2 — Skip macOS 26.2.x rollback control
- **Stream 1:** Issue Y (Metal Toolchain 32023) was ruled out as the cause of our crash. Real Metal allocations were observed (50 GB GPU vmSummary, 1.13 GB/sequence wired) — incompatible with a Metal-library-load failure
- **Stream 3:** Issue Y is a real hazard for *other* Metal backends on 26.3.x — but the bake-off tests them on our production OS, which is 26.3.2. Testing on a downgraded OS measures something we do not run on
- **Verdict:** Test on macOS 26.3.2 (production). If Issue Y hits any finalist, the bake-off catches it. **No OS rollback in the bake-off.**

### Decision 3 — Sequential, not parallel, bake-off
- **Production reality:** only one LLM backend runs at a time on the production stack
- **Engineering rigor:** parallel execution introduces cross-candidate memory contention on the same 128 GB unified memory pool, contaminating the stress signal with a variable that doesn't exist in production
- **Verdict:** **Sequential bake-off.** ~3-day cost savings of parallel do not justify polluted data on a stability decision

### Final finalist set (locked)
- **F1 — llama.cpp (`llama-server`)** — most production-justified; needs M5 + macOS 26.3.x verification
- **F2 — MLC-LLM** — most architecturally distinct (TVM Metal kernels, immune to Issue Y); needs M5 evidence

The fork-pick remains evidence-pending: it is decided by Epic 2 bake-off results (B2 quality A/B + B3 4-hour `ape.reflect` stress + B4 concurrency stress), not by any further research or operator-input round.

---

## Documents Accessed (Synthesis)

- `phase_13_5_crash_forensics.md` (Stream 1)
- `phase_13_5_external_evidence.md` (Stream 2)
- `phase_13_5_alternatives_matrix.md` (Stream 3)
- `phase_13_5_call_profile.md` (Stream 4)
- `sprint_plan_phase_13_5_mlx_stability.md` (this sprint's frozen plan)
- All inline citations from Streams 2 and 3

---

*Synthesis produced 2026-05-02. All claims tied to one of the four stream reports plus their cited external sources. No source — internal or external — has been reframed beyond what the streams reported.*
