# Neural Sidecar Staged Rollout Plan

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Epic**: 5 (Pipeline + Rollout)
**Step**: 5.4
**Prerequisite**: All safety infrastructure (circuit breakers, test matrix) verified

---

## Overview

The neural sidecar promotion follows a four-stage rollout, each with explicit entry/exit gates. Progression is manual (operator decision), but demotion is automatic when safety thresholds are violated.

---

## Stage 0: Shadow

**Config**: `J17_SIDECAR_MODE=shadow` (default)

**Purpose**: Collect training data and validate circuit breaker behavior.

**Entry gate**: Sidecar binary deployed, `J17_SIDECAR_URL` set, `J17_ML_TIER_PREDICTION_ENABLED=true`.

**Exit gate**:
- 500+ training records collected in `.mdemg/neural/protocol-data/`
- Circuit breaker has been tested (triggered and recovered at least once)
- Sidecar `/health` consistently returns 200
- No errors in `j17-sidecar:` log lines beyond expected timeouts

**Duration**: Minimum 3 days or 500 training records, whichever comes later.

**Operator actions**:
1. Deploy sidecar alongside MDEMG server
2. Set `J17_SIDECAR_URL=http://localhost:8100`
3. Set `J17_ML_TIER_PREDICTION_ENABLED=true`
4. Set `J17_PROTOCOL_DATA_COLLECTION=true`
5. Monitor logs for `j17-sidecar:` entries
6. Verify data accumulation: `ls -la .mdemg/neural/protocol-data/`

---

## Stage 1: Compare

**Config**: `J17_SIDECAR_MODE=compare`

**Purpose**: Measure agreement rate and comprehension delta without behavioral effect.

**Entry gate**: Stage 0 exit gate met.

**Exit gate**:
- 2000+ guidance cycles with ML predictions
- Agreement rate >= 80% (overall)
- Zero must-level disagreements where ML would under-encode (ML tier < rule tier for must-severity)
- p99 latency < 200ms
- No circuit breaker trips lasting > 60 seconds

**Duration**: Minimum 7 days or 2000 predictions, whichever comes later.

**Monitoring**:
```bash
curl -s http://localhost:9999/v1/jiminy/ready?stats=true | jq '.protocol_metrics.sidecar_metrics'
```

**What to watch**:
- `agreement_rate` trending toward >= 85%
- `errors` count not growing faster than `requests`
- `avg_latency_ms` stable under 50ms (p50 target)

---

## Stage 2: Canary

**Config**: `J17_SIDECAR_MODE=canary`, `J17_SIDECAR_CANARY_PERCENTAGE=25`

**Purpose**: Validate that ML tier selection maintains or improves comprehension for non-critical constraints.

**Entry gate**: Stage 1 exit gate met.

**Ramp schedule**:
1. Start at 25% canary (`J17_SIDECAR_CANARY_PERCENTAGE=25`)
2. After 500 ML-routed predictions with comprehension delta >= 0: ramp to 50%
3. After 500 more: ramp to 100%
4. Hold at 100% for 3 days minimum

**Exit gate** (must hold at 100% canary for full duration):
- Comprehension delta >= 0 for should-level and info-level constraints
- No individual constraint with comprehension drop > 10% vs rule-based
- Override rate < 30% (ML is not constantly disagreeing)
- Circuit breaker open events < 3 per day
- Sidecar error rate < 5%

**Safety rails**:
- High-priority (must-level) items ALWAYS use rule-based in canary mode
- Protected codes (`J17_PRECEDENT_PROTECTED_CODES`) always use rule-based
- Confidence floor (`J17_SIDECAR_CONFIDENCE_FLOOR=0.6`) gates ML predictions

**Rollback triggers** (automatic via circuit breaker):
- Circuit breaker opens → all requests fall back to rule-based
- Manual: set `J17_SIDECAR_MODE=compare` to exit canary

---

## Stage 3: Active

**Config**: `J17_SIDECAR_MODE=active`

**Purpose**: ML tier selection is the primary path for all non-protected constraints.

**Entry gate**: All Stage 2 gates hold for 3+ days at 100% canary.

**Ongoing monitoring**:
- Comprehension delta remains >= 0
- Agreement rate >= 85% (the 15% disagreement is expected — ML should be *improving* on rule-based)
- No new must-level comprehension regressions
- Sidecar health endpoint responsive

**Differences from canary**:
- High-priority items ARE subject to ML tier selection (unlike canary)
- Protected codes still exempt
- Confidence floor still applies — low-confidence predictions fall back to rule-based

---

## Automatic Demotion

| Trigger | Action | Recovery |
|---------|--------|----------|
| Circuit breaker opens | All predictions return (0,0) → rule-based fallback | Automatic: half-open probe after `J17_SIDECAR_CB_TIMEOUT_SEC` |
| Comprehension drop > 10% (detected by RSIC) | RSIC reflection flags `j17_low_comprehension` | Operator: investigate and retrain, or demote to compare |
| Sidecar process crash | HTTP timeout → circuit breaker opens | Restart sidecar; circuit breaker recovers automatically |
| Model regression after retraining | Comprehension delta turns negative | Set `NEURAL_TIER_MODEL=""` to disable ML predictions at sidecar level |

---

## Configuration Quick Reference

| Variable | Stage 0 | Stage 1 | Stage 2 | Stage 3 |
|----------|---------|---------|---------|---------|
| `J17_SIDECAR_MODE` | shadow | compare | canary | active |
| `J17_SIDECAR_CANARY_PERCENTAGE` | - | - | 25→50→100 | - |
| `J17_SIDECAR_CONFIDENCE_FLOOR` | 0.6 | 0.6 | 0.6 | 0.6 |
| `J17_NLI_SCORE_OF_RECORD` | false | false | true | true |
| `J17_SIDECAR_CB_ENABLED` | true | true | true | true |
| `J17_PROTOCOL_DATA_COLLECTION` | true | true | true | true |

---

## Decision Log Template

Each stage transition should be recorded:

```
Date: YYYY-MM-DD
Transition: Stage N → Stage N+1
Metrics at transition:
  - Agreement rate: X%
  - Comprehension delta: +X.XX
  - p99 latency: Xms
  - Training records: N
  - Sidecar uptime: X days
Operator: <name>
Notes: <any observations>
```
