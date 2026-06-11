# 07 — Forward-Forward Shallow Heads

**Sprint ID**: FF-SHALLOW-HEADS
**Date**: 2026-04-21 (plan authored)
**Branch**: TBD
**Scope**: Introduce Forward-Forward (FF)-trained shallow classifiers for three small decision heads in MDEMG: the promotion classifier (should this L_n node be promoted?), the trust-bin classifier (what source-trust bin does this observation land in?), and the context-appropriateness classifier (does this retrieval result match the query context?). FF is used **only** for these shallow heads over **frozen LLM embeddings** — the known sweet spot from the literature. Backprop remains untouched for the neural sidecar's deep components.

**Upstream**: [White Paper Review](mdemg-white-paper-review.md) Papers 3 (Hinton FF), 4 (Ororbia & Mali — FF underperforms at scale; use where scale doesn't matter).

---

## Sprint Framing

Forward-Forward's original claim — "replace backprop entirely" — has not held up at scale (Paper 4). But the paper 4 literature identifies a concrete sweet spot: **shallow networks, frozen input representations, online/streaming data, continual-learning-friendly contexts.** MDEMG has three decision heads that fit this profile exactly:

1. **Promotion classifier**: "given this L_n node's features, should it be promoted to L_{n+1}?" Binary. 2-3 layers max. Input: frozen LLM embedding + co-activation features. Training data: historical promotions + RSIC-labeled "this promotion was a mistake" cases.
2. **Trust-bin classifier**: "given this observation's source metadata, what trust bin?" 3-way (trusted/normal/untrusted). Currently rule-based (Sprint 01 Phase 2.1). FF replacement learns from user overrides.
3. **Context-appropriateness classifier**: "given query context fingerprint and candidate observation, is this candidate appropriate?" Binary. Trained on click-through / effectiveness data from RSIC.

All three are currently either rule-based, heuristic-counting, or absent. FF gives a principled, online-trainable, local-credit alternative without requiring a monolithic training pipeline.

**Important caveat** (from Paper 4): do not extend this to deep components. The cross-encoder re-ranker (`train.py`) and tier predictor (`train_protocol.py`) stay on backprop. This sprint is explicitly bounded to shallow heads.

---

## Gap Summary

| Category | Critical | High | Medium | Low | Total |
|----------|----------|------|--------|-----|-------|
| FF Training Infrastructure (Python) | 0 | 4 | 1 | 0 | **5** |
| Head 1: Promotion Classifier | 0 | 3 | 1 | 0 | **4** |
| Head 2: Trust-Bin Classifier | 0 | 2 | 1 | 0 | **3** |
| Head 3: Context-Appropriateness | 0 | 2 | 1 | 0 | **3** |
| Integration (Go ↔ sidecar) | 0 | 2 | 1 | 0 | **3** |
| Observability | 0 | 1 | 1 | 0 | **2** |
| Testing & Verification | 0 | 3 | 1 | 0 | **4** |
| Mandatory Documentation Phase | 0 | 4 | 2 | 0 | **6** |
| **Total** | **0** | **21** | **9** | **0** | **30** |

---

## Phase 1: FF Training Infrastructure

### 1.1 FF trainer module (HIGH)

**Gap**: No FF implementation exists in the neural sidecar.

**Fix** — New file `neural/training/ff_trainer.py`:

```python
"""Forward-Forward trainer for shallow classification heads.

Implements the canonical FF algorithm from Hinton (2022):
- Two forward passes: positive data and negative data
- Per-layer "goodness" objective (sum of squared activities)
- Local credit assignment; no backward pass
- Online-friendly — one example at a time if desired
"""
import torch
import torch.nn as nn
from dataclasses import dataclass

@dataclass
class FFConfig:
    hidden_sizes: list[int]        # e.g., [128, 64] for 2-layer shallow head
    goodness_threshold: float = 2.0 # θ in the goodness function
    lr: float = 1e-3
    epochs: int = 10
    batch_size: int = 32

class FFLayer(nn.Module):
    def __init__(self, d_in, d_out, cfg: FFConfig):
        super().__init__()
        self.linear = nn.Linear(d_in, d_out)
        self.opt = torch.optim.Adam(self.parameters(), lr=cfg.lr)
        self.threshold = cfg.goodness_threshold

    def forward(self, x):
        # Normalize input (detaches from prior layer's gradient path)
        x = x / (x.norm(dim=1, keepdim=True) + 1e-4)
        return torch.relu(self.linear(x))

    def train_step(self, x_pos, x_neg):
        g_pos = self.forward(x_pos).pow(2).mean(1)
        g_neg = self.forward(x_neg).pow(2).mean(1)
        # Local goodness loss: positives should exceed threshold, negatives fall below
        loss = torch.log(1 + torch.exp(
            torch.cat([-g_pos + self.threshold, g_neg - self.threshold])
        )).mean()
        self.opt.zero_grad()
        loss.backward()  # layer-local only
        self.opt.step()
        return loss.item()

class FFClassifier(nn.Module):
    def __init__(self, d_in, cfg: FFConfig):
        super().__init__()
        sizes = [d_in] + cfg.hidden_sizes
        self.layers = nn.ModuleList(
            [FFLayer(sizes[i], sizes[i+1], cfg) for i in range(len(sizes)-1)]
        )

    def predict(self, x):
        # Classification via goodness — no final softmax
        activations = []
        h = x
        for layer in self.layers:
            h = layer.forward(h)
            activations.append(h.pow(2).mean(1))
        return sum(activations)  # aggregate goodness across layers
```

**Files**: `neural/training/ff_trainer.py` (new), `neural/training/ff_trainer_test.py` (new)

---

### 1.2 Negative sample generation from RSIC (HIGH)

**Fix** — `neural/training/ff_negatives.py`: pulls RSIC-flagged "ineffective" outcomes and constructs negative training examples for each head:

- Promotion classifier: nodes that were promoted but later demoted → negatives
- Trust-bin classifier: observations reclassified by admin override → negatives
- Context-appropriateness: retrieval results marked unhelpful via RSIC → negatives

**Files**: `neural/training/ff_negatives.py`

---

### 1.3 Training pipeline (HIGH)

**Fix** — `neural/training/train_ff_heads.py`: a single entrypoint that:
1. Reads training data from TSDB (positives) and RSIC failure stream (negatives)
2. Generates frozen LLM embeddings (uses existing embedder; no gradients)
3. Trains each head via FFClassifier
4. Exports ONNX / torchscript for the Go sidecar to load

**Files**: `neural/training/train_ff_heads.py`, `neural/training/README_ff.md`

---

### 1.4 Online updates (HIGH)

**Gap**: The whole point of FF here is online learning — the heads should update continuously, not just in batch training.

**Fix** — Sidecar endpoint `POST /v1/sidecar/ff/update_online` that accepts a (positive, negative) pair and does one FF step per layer. Rate-limited to prevent runaway updates.

**Files**: `neural/sidecar/server.py`, `neural/sidecar/ff_online.py`

---

### 1.5 Model checkpointing (MEDIUM)

**Fix** — Each head's weights are checkpointed every hour to `neural/checkpoints/ff_<head_name>_<version>.pt`. Enables rollback and A/B vs fixed snapshots.

**Files**: `neural/sidecar/server.py`

---

## Phase 2: Head 1 — Promotion Classifier

### 2.1 Feature vector definition (HIGH)

**Fix** — For each L_n node considered for promotion:
- Node's frozen embedding (LLM-generated) — 1536 dims
- Co-activation count, mean edge weight, mean endpoint confidence
- Surprise statistics over node's observations (mean, variance, max)
- Time since last reinforcement
- Layer of the node (L_n)

Concatenated to ~1550 dims. Input to FFClassifier with hidden_sizes=[128, 64].

**Files**: `neural/training/features_promotion.py`

---

### 2.2 Shadow-mode deployment (HIGH)

**Gap**: Do not replace the existing evidence-counting promotion rule outright.

**Fix** — Sidecar exposes `POST /v1/sidecar/ff/promotion/score`. Go calls it alongside the existing rule, logs both decisions, metric gauges the agreement rate.

**Files**: `internal/api/handlers_promotion.go` (modify), `neural/sidecar/server.py`

---

### 2.3 Live promotion gate (HIGH)

**Fix** — After 2+ weeks of shadow mode with ≥80% agreement rate and ≥60% of disagreements resolved in favor of FF by manual review, flip `PROMOTION_FF_AUTHORITATIVE=true`. Evidence-counting rule becomes shadow.

**Files**: `internal/config/config.go`, documentation

---

### 2.4 Interaction with Sprint 03 (MEDIUM)

**Note**: Sprint 03's prediction-error promotion and this sprint's FF promotion classifier are both candidates for replacing evidence-counting. Plan: FF head consumes the prediction-error score as an input feature. No conflict; they compose.

---

## Phase 3: Head 2 — Trust-Bin Classifier

### 3.1 Feature vector definition (HIGH)

Authenticated? Scope count? Source string? Has user provided corrections in past? Rate of observations per hour? → ~20 hand-crafted features + frozen embedding of the observation content.

**Files**: `neural/training/features_trust.py`

### 3.2 Replaces rule in Sprint 01 Phase 2.1 (HIGH)

**Fix** — `ClassifySourceTrust` in `internal/conversation/source_trust.go` calls FF head via sidecar. Fallback to rule-based if sidecar unavailable.

**Files**: `internal/conversation/source_trust.go`

### 3.3 Admin override feedback loop (MEDIUM)

**Fix** — When admin manually reclassifies an observation's trust, emit a negative training signal to the FF head. Closes the loop.

**Files**: `internal/api/handlers_quarantine.go` (Sprint 01 addition)

---

## Phase 4: Head 3 — Context-Appropriateness Classifier

### 4.1 Feature vector (HIGH)

Query context fingerprint (Sprint 05) + candidate fingerprint + embedding similarity + co-activation strength → ~520 dims.

**Files**: `neural/training/features_context.py`

### 4.2 Integration with retrieval ranking (HIGH)

**Fix** — If sparse-retrieval-ranking or column-voting is active, FF-head score is an additional ranking signal (or its own column in Sprint 04 terms).

**Files**: `internal/retrieval/column_ff_context.go` (new)

### 4.3 A/B benchmark against whk-wms (MEDIUM)

---

## Phase 5: Integration — Go ↔ Sidecar

### 5.1 gRPC/HTTP contract for FF heads (HIGH)

**Fix** — Extend neural sidecar protobuf or HTTP API:

```protobuf
service FFHeads {
  rpc ScorePromotion(PromotionFeatures) returns (ScoreResponse);
  rpc ScoreTrust(TrustFeatures) returns (TrustBinResponse);
  rpc ScoreContext(ContextFeatures) returns (ScoreResponse);
  rpc UpdateOnline(TrainingExample) returns (UpdateResponse);
}
```

**Files**: `neural/sidecar/proto/ff.proto`, generated Go bindings

### 5.2 Graceful degradation (HIGH)

If sidecar is down or slow, Go falls back to pre-FF logic (evidence counting / rule-based). Never blocks a user request on FF.

**Files**: all sidecar-calling code paths

### 5.3 Health checks (MEDIUM)

Sidecar `/healthz` reports per-head readiness (loaded, last-trained-at).

**Files**: `neural/sidecar/server.py`

---

## Phase 6: Observability

### 6.1 Prometheus metrics (HIGH)

```
mdemg_ff_head_latency_seconds{head, space_id} - histogram
mdemg_ff_head_unavailable_fallback_total{head} - counter
mdemg_ff_head_agreement_rate{head} - gauge (vs legacy rule, where applicable)
mdemg_ff_head_training_loss{head} - gauge
```

**Files**: `internal/metrics/registry.go`, `neural/sidecar/server.py`

### 6.2 Grafana dashboard (MEDIUM)

New `mdemg-ff-heads.json` with per-head latency, agreement rate, training progress.

**Files**: `deploy/grafana/dashboards/mdemg-ff-heads.json`

---

## Phase 7: Testing & Verification

### 7.1 Unit tests (HIGH)
- FFLayer goodness computation (goodness high on positive, low on negative)
- FFClassifier prediction monotonicity
- Graceful sidecar-unavailable fallback

### 7.2 Integration test (HIGH)
Shadow-mode promotion agreement rate ≥ 0.8 after training on historical data.

### 7.3 A/B benchmark (HIGH)
Each head benchmarked independently. Merge blocked until benchmarks pass.

---

## Phase 8: Mandatory Documentation Phase

### 8.1 CHANGELOG.md (HIGH)
### 8.2 AGENT_HANDOFF.md (HIGH)
### 8.3 CLAUDE.md — add FF-heads vocabulary (MEDIUM)
### 8.4 `docs/features/ff-shallow-heads.md` (new feature doc) (HIGH)
### 8.5 `docs/architecture/neural-sidecar.md` — add FF section (HIGH)
### 8.6 Homebrew beta testing guide + submodule bump (MEDIUM)

---

## Risk Analysis & Rollback

### R1: FF heads underperform rule-based baseline

**Likelihood**: Medium. This is the empirical question this sprint answers.

**Mitigation**: Shadow mode ≥2 weeks before any head becomes authoritative. Never replace rule if agreement <80%.

**Rollback**: Disable each head's authoritative flag. Fall back to rule.

### R2: Sidecar latency impacts hot paths

**Likelihood**: Medium. Promotion and retrieval ranking are frequent calls.

**Mitigation**: Tight sidecar timeouts (<50ms). Circuit breaker. Fall back to rule if slow.

**Rollback**: As R1.

### R3: Online updates cause catastrophic forgetting

**Likelihood**: Low (FF is more robust than SGD here), but possible.

**Mitigation**: Rate limit online updates. Periodic checkpoint restores. Regression test run daily against fixed eval set.

**Rollback**: Restore previous checkpoint.

### R4: Scope creep into deep components

**Likelihood**: HIGH if not explicitly guarded against. Developers may be tempted to apply FF to the cross-encoder.

**Mitigation**: Sprint scope explicitly excludes deep components. Paper 4's warning surfaced in CLAUDE.md.

---

## Sprint Size Estimate

| Phase | Estimate |
|-------|----------|
| 1. FF Infrastructure | 3 days |
| 2. Promotion Classifier | 2 days |
| 3. Trust-Bin Classifier | 1 day |
| 4. Context-Appropriateness | 1 day |
| 5. Integration | 1.5 days |
| 6. Observability | 0.5 day |
| 7. Testing & Verification | 2 days |
| 8. Mandatory Documentation | 0.5 day |
| Shadow-mode soak | 2 weeks calendar |
| **Total dev time** | **~11.5 days** |
| **Total calendar** | **~4 weeks incl. shadow soak** |

---

## Dependencies

**Blocks**: None.

**Blocked by**: Ideally 01 (Sprint 01's source-trust rule is what Head 2 replaces). 03 may interact with Head 1 but is not a hard block.

---

## Documents Accessed

- `neural/training/train.py`, `train_protocol.py` (existing backprop pipelines — not touched, just referenced)
- `internal/conversation/source_trust.go` (from Sprint 01)
- `internal/hidden/service.go` (promotion logic)
- White paper review Papers 3, 4
