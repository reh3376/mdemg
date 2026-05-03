# Phase 13.5 Epic 1 — Disqualification Confirmation Pass

**Date:** 2026-05-02
**Scope:** 7-day window: 2026-04-26 through 2026-05-02
**Purpose:** Sanity-check that no upstream release in the last 7 days flips any of the three disqualifications from the synthesis document ([`phase_13_5_mlx_research_synthesis.md`](./phase_13_5_mlx_research_synthesis.md)).

---

## A. mlx-lm Status

### A.1 New releases since v0.31.3 (2026-04-22)?

**Finding:** No new release. The GitHub [releases page](https://github.com/ml-explore/mlx-lm/releases) confirms **v0.31.3 (2026-04-22) remains the latest release** as of 2026-05-02. No release has shipped in the 7-day window.

### A.2 Issue [#615](https://github.com/ml-explore/mlx-lm/issues/615) — `--max-kv-size` for server?

**Finding:** Still **open**. The issue tracker shows status `Open`, with the "Development" section explicitly reading **"No branches or pull requests."** No fix has been linked or merged.

> Note: PR [#1106](https://github.com/ml-explore/mlx-lm/pull/1106) ("Bring back max-kv-size to the batch generator") was merged 2026-04-05 and addressed `mlx_lm.generate()` batch mode only — not `mlx_lm.server`. PR [#884](https://github.com/ml-explore/mlx-lm/pull/884) ("Add `--max-kv-size` support to `mlx_lm.server`") was **closed without merging** on 2026-02-19.

### A.3 PRs in flight for server max-kv-size?

**Finding:** Searching [all PRs matching "max-kv-size"](https://github.com/ml-explore/mlx-lm/pulls?q=is%3Apr+max-kv-size) shows no open PRs targeting the server. The only server-specific PR (#884) is closed/unmerged. **No work in flight.**

### A.4 SERVER.md "not recommended for production" disclaimer still present?

**Finding:** The disclaimer **remains verbatim** in [`SERVER.md`](https://github.com/ml-explore/mlx-lm/blob/main/mlx_lm/SERVER.md):

> *"The MLX LM server is not recommended for production as it only implements basic security checks."*

Additionally, the maintainer's stated position from [Discussion #371](https://github.com/ml-explore/mlx-lm/discussions/371) — *"mostly intended to be used as a local HTTP endpoint"* — remains unchanged per web search results dated May 2026.

### A. Verdict: **STILL DISQUALIFIED**

No new release, no progress on issue #615, no open PRs for server `--max-kv-size`, production disclaimer intact.

---

## B. Ollama Status

### B.1 New releases since v0.22.1 (2026-04-28)?

**Finding:** No new release. The GitHub [releases page](https://github.com/ollama/ollama/releases) confirms **v0.22.1 (2026-04-28) remains the latest release** as of 2026-05-02. No release has shipped in the 7-day window.

### B.2 Issue [#15862](https://github.com/ollama/ollama/issues/15862) — M5 Max / macOS 26.3 matmul2d crash?

**Finding:** Still **open**. The issue tracker shows `Open` status with no linked PRs, no assigned owner, and no resolution comments. The root-cause description — GGML shader instantiating `mpp::tensor_ops::matmul2d` with mismatched `<half, bfloat>` operands rejected by macOS 26.3.1's updated MetalPerformancePrimitives headers — remains unaddressed.

### B.3 Web search: any new fix announcement?

**Finding:** Web search for `Ollama M5 Max macOS 26.3 matmul2d fix May 2026` returns **no fix announcements**. Results surface only additional open issue reports:
- [#15862](https://github.com/ollama/ollama/issues/15862) — MPPTensorOpsMatMul2dImpl static_assert
- [#15594](https://github.com/ollama/ollama/issues/15594) — Metal compilation error on M5 (macOS Tahoe 26.3.1)
- [#15541](https://github.com/ollama/ollama/issues/15541) — MTLLibrary bfloat/half mismatch
- [#15448](https://github.com/ollama/ollama/issues/15448) — 500 Internal Server Error, Metal compiler failed
- [#15496](https://github.com/ollama/ollama/issues/15496) — Ollama 0.20.5 crashes on M5

All issues remain open. The matmul2d static_assert family affects versions 0.20.5 through 0.22.1.

### B. Verdict: **STILL DISQUALIFIED**

No new release, issue #15862 still open, multiple corroborating issues open, no fix announcement found.

---

## C. LM Studio Status

### C.1 Changelog: any open-source licensing change?

**Finding:** The [LM Studio changelog](https://lmstudio.ai/changelog) shows releases 0.4.9–0.4.12 in April 2026 covering model support improvements, bug fixes, and API enhancements. **No licensing or open-source policy announcement appears.**

### C.2 Web search: any licensing/open-source policy shift?

**Finding:** Web search for `LM Studio open source licensing May 2026` finds one meaningful development: LM Studio [announced](https://lmstudio.ai/blog/free-for-work) it is now **free for commercial/work use** without requiring a separate license, effective earlier in 2026. The `lms` CLI tool has always been MIT-licensed.

**Assessment:** This is a *pricing* change (free-at-work), not an *architecture or source-availability* change. The LM Studio application itself remains **closed-source proprietary software**. The disqualification basis was *closed-source operability risk for a cognitive-substrate framework* — specifically the risk of vendor lock-in, inability to audit, patch, or self-host the inference stack independently. Removing the commercial license fee does not change the closed-source nature of the application. The `lms` CLI being MIT-licensed was already known; it does not make the core inference runtime open source.

References: [LM Studio terms](https://lmstudio.ai/app-terms), [lms CLI license](https://github.com/lmstudio-ai/lms/blob/main/LICENSE).

### C. Verdict: **STILL DISQUALIFIED**

The application remains closed-source. The "free for work" pricing change does not address the operability risk basis for disqualification.

---

## Overall Verdict

**ALL DISQUALIFICATIONS HOLD**

No upstream development between 2026-04-26 and 2026-05-02 flips any of the three disqualifications. Epic 1 gate passes. Proceed to bake-off per sprint plan.

---

## Summary (~150 words)

All three disqualifications from the Phase 13.5 synthesis remain intact as of 2026-05-02.

**mlx-lm server (still disqualified):** v0.31.3 is still the latest release; issue [#615](https://github.com/ml-explore/mlx-lm/issues/615) requesting `--max-kv-size` for the server has no linked PRs and no progress — the most important single piece of evidence is that PR #884 (the only attempt to add server max-kv-size support) was closed *without merging* in February 2026 and nothing has reopened since. The "not recommended for production" disclaimer in `SERVER.md` is verbatim unchanged.

**Ollama (still disqualified):** v0.22.1 is still the latest release; the matmul2d static_assert crash on M5 + macOS 26.3 remains open across at least 5 corroborating issues with no fix in sight and no new release shipping.

**LM Studio (still disqualified):** The "free for work" pricing change does not make the application open source; the closed-source operability risk basis is unaffected.
