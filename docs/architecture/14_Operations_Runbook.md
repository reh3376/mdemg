# Operations Runbook (Neo4j + MDEMG Service)

This runbook sets the **procedural policies** required to operate the system without turning your memory graph into an expensive chaos machine.

Design mantra (operationalized):

- vector index = recall
- graph = reasoning
- runtime = activation physics
- DB writes = only learning deltas

---

## 1) Service-level SLOs (start conservative)

### Retrieval

- **p95 latency**: < 250 ms (warm cache)
- **candidate_k**: default 200 (cap <= 1000)
- **hop_depth**: default 2 (cap <= 3)
- **max_total_edges_fetched**: default 20k (cap <= 50k)

### Ingestion

- **p95 ingestion latency** (excluding embedding compute): < 150 ms
- **embedding throughput**: budget separately; do not let embedding stall ingestion.

### Learning writeback

- **max edges updated per request**: hard cap (default 200)
- **writeback failures**: < 0.1% of requests

---

## 2) Deployment topology

- Neo4j: dedicated host(s), persistent storage, page cache sized for graph hot set.
- Service: stateless Go instances behind a load balancer.
- Optional offline jobs: decay/consolidation run as cron jobs or a separate worker deployment.

### Configuration must be explicit

All tuning knobs must be config-driven (env/config file), not compiled constants.

---

## 3) Backups and restore

Neo4j supports different backup methods depending on edition and deployment. The operational requirement is simple:

- **You must be able to restore to a consistent point-in-time state**.

### 3.1 Policy

- **RPO** (data loss): 24h maximum (start here; tighten later)
- **RTO** (restore time): 2h maximum (start here; tighten later)
- Keep at least **7 daily** backups, **4 weekly**, **6 monthly**.

### 3.2 Community edition (file-based)

Use `neo4j-admin database dump/load`.

Backup:

```bash
neo4j-admin database dump neo4j --to-path=/backups/neo4j/$(date +%F)
```

Restore:

```bash
neo4j-admin database load neo4j --from-path=/backups/neo4j/2026-01-15 --overwrite-destination=true
```

Operational notes:

- Stop Neo4j for restores (or restore to a new DB name, then swap).
- Validate by running `/readyz` (schema version check) and a smoke retrieval.

### 3.3 Enterprise / cluster

Use Neo4j backup tooling appropriate for the cluster deployment. The policy remains: automate + verify restores regularly.

### 3.4 Backup verification (non-negotiable)

Weekly:

1. Restore backup to a staging environment.
2. Run migrations (should be no-ops if schema already correct).
3. Run a smoke suite:
   - vector index exists
   - retrieval returns results
   - learning writeback succeeds

---

## 4) Schema migrations in production

### 4.1 Rules

- Migrations are **append-only** (no editing old versions).
- Each migration is **idempotent**.
- Service startup checks schema version and refuses to serve if it is below `REQUIRED_SCHEMA_VERSION`.

### 4.2 Rollout procedure

1. Apply migrations to staging.
2. Run regression suite (golden retrieval scoring test vectors).
3. Apply migrations to production.
4. Roll service instances (or let them restart) so they pass schema checks.

---

## 5) Rollover, retention, pruning

### 5.1 Observations retention

Observations are append-only logs; they can grow without bound.

Policy:

- Keep full text observations for **N days** (e.g., 90).
- After N days, either:
  - archive externally (object storage), and keep only a pointer + summary in Neo4j, or
  - keep the observation but drop large payload fields.

### 5.2 Edge pruning

Run daily:

- decay weights
- prune edges below threshold if weak AND low evidence AND old

This is mandatory to prevent hub/clique blowups.

### 5.3 Consolidation rollover

Run weekly (or daily once stable):

- detect stable clusters
- create abstraction nodes (layer k+1)
- thin redundant lateral edges among members

---

## 6) Failure modes and alarms

Your goal is to detect “emergence going pathological” early.

### 6.1 DB health

Alarm on:

- Neo4j down / not accepting connections
- page cache thrash (if visible)
- query latency spikes for vector queries

### 6.2 Retrieval health

Alarm on sustained:

- p95 latency breach
- candidate_k/hop_depth above safe bounds (config drift)
- `max_total_edges_fetched` frequently hit (means expansions too big)

### 6.3 Graph pathology alarms

Alarm on:

- **hub explosion**: max degree or degree p99 climbs rapidly
- **clique spam**: CO_ACTIVATED_WITH edge count grows superlinearly
- **over-decay**: mean activation and recall@K collapse over time

### 6.4 Learning writeback alarms

Alarm on:

- writeback error rate
- edges updated per request at cap for sustained period (means learning is trying to explode)

---

## 7) Incident playbooks

### 7.1 Hub explosion

Symptoms:

- one node dominates results regardless of query
Actions:

1. Increase hub penalty `φ`.
2. Tighten expansion caps (neighbors per node, edges fetched).
3. Prune weak edges around the hub (keep pinned/structural).
4. Consider splitting the hub node into more specific nodes (tombstone old, create new, `MERGED_INTO`).

### 7.2 Clique spam (CO_ACTIVATED_WITH density blowup)

Actions:

1. Raise activation threshold for learning.
2. Reduce `eta` and increase `mu` regularization.
3. Enforce per-node cap: only keep top-N coactivation neighbors by weight.
4. Prune low-evidence coactivation edges.

### 7.3 “Forgetting everything”

Actions:

1. Reduce decay rates.
2. Pin critical nodes/edges.
3. Increase baseline importance (recency and confidence weights) temporarily.

---

## 8) Observability checklist

Track these time series:

- node/edge counts by label/type
- degree distribution (p50/p90/p99/max)
- edge weight distribution by relationship type
- vector query latency + recall sizes
- expansion edges fetched per request
- learning edges updated per request
- consolidation outputs (abstractions created per period)

---

## 9) Security and policy enforcement

- Store `sensitivity` on nodes/observations.
- Enforce sensitivity filtering **server-side** during retrieval and before returning context.
- Log policy decisions (deny/allow counts) but avoid logging raw sensitive content.

---

## 10) Maintenance cadence (suggested)

- Hourly: lightweight health checks + anomaly detection
- Daily: decay + pruning
- Weekly: consolidation + abstraction generation
- Monthly: restore drill + capacity review

---

## 11) RSIC Observability & Operations (Phase 91)

The Recursive Self-Improvement Cycle (RSIC) subsystem has TSDB-backed metrics (via `/v1/metrics/snapshot`), a Grafana dashboard, and alert rules evaluated by the server-native alert evaluator (SNA-001) with SQL queries against TimescaleDB — Grafana is optional, dashboards-only.

### 11.1 RSIC Health Indicators

| Metric | Healthy Range | Description |
|--------|---------------|-------------|
| `mdemg_rsic_cycle_total` | Steady non-zero rate | Cycles starting and completing |
| `mdemg_rsic_cycle_duration_seconds` | p95 < 300s | End-to-end cycle time |
| `mdemg_rsic_trigger_rejected_total` | < 10% of total triggers | Trigger rejections (cooldown, dedupe, overlap) |
| `mdemg_rsic_action_total` | > 95% success rate | Per-action success/failure |
| `mdemg_rsic_safety_blocked_total` | Near zero | Safety validator blocks |
| `mdemg_rsic_watchdog_decay_score` | < 4.0 | Time-weighted decay since last cycle |
| `mdemg_rsic_watchdog_escalation_level` | 0 (nominal) | Watchdog state (0=nominal, 1=nudge, 2=warn, 3=force) |
| `mdemg_rsic_watchdog_force_total` | < 1/day | Watchdog force triggers |
| `mdemg_rsic_calibration_confidence` | > 0.7 per action | Per-action success rate (0-1) |
| `mdemg_rsic_snapshot_created_total` | Matches destructive actions | Pre-mutation snapshots captured |

### 11.2 Failure Mode Playbooks

#### MDEMGRSICHighFailureRate (>25% cycles not completing)

**Symptoms:** Cycle outcomes show `error` or `low_confidence` predominating.

**Diagnosis:**

```bash
# Check recent cycle history
curl -s http://localhost:9999/v1/self-improve/history?limit=10 | jq '.[] | {cycle_id, tier, error, trigger_source}'

# Check assessment confidence
curl -s -X POST http://localhost:9999/v1/self-improve/assess \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","tier":"meso"}' | jq '{confidence, overall_health}'

# Check Neo4j health
curl -s http://localhost:9999/v1/self-improve/health | jq '.safety'
```

**Root causes:** Neo4j connection issues, insufficient graph data for assessment, min_confidence threshold too high.

**Remediation:** Check Neo4j pool metrics, lower `RSIC_MIN_CONFIDENCE` if data is sparse, verify space has sufficient nodes.

#### MDEMGRSICRepeatedForceTriggers (>0.5 force/hr for 30m)

**Symptoms:** Watchdog repeatedly escalating to force level, cycles completing but decay resetting too slowly.

**Diagnosis:**

```bash
# Check watchdog state
curl -s http://localhost:9999/v1/self-improve/health | jq '.watchdog'

# Check cycle history for meso completions
curl -s "http://localhost:9999/v1/self-improve/history?limit=20&tier=meso" | jq '.[] | {cycle_id, trigger_source, completed_at}'
```

**Root causes:** Meso period too long, watchdog decay rate too aggressive, cycles completing but not resetting watchdog.

**Remediation:** Adjust `RSIC_MESO_PERIOD_HOURS`, lower `RSIC_WATCHDOG_DECAY_RATE`, or enable macro cron (`RSIC_MACRO_CRON`).

#### MDEMGRSICActionFailureSpike (>50% per-action failure)

**Symptoms:** A specific RSIC action type (e.g., `prune_decayed_edges`) is consistently failing.

**Diagnosis:**

```bash
# Check calibration confidence per action
curl -s http://localhost:9999/v1/self-improve/calibration | jq '.'

# Run a dry-run cycle to isolate the issue
curl -s -X POST http://localhost:9999/v1/self-improve/cycle \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","tier":"meso","dry_run":true}' | jq '.deltas'
```

**Root causes:** Neo4j pool exhaustion, Cypher query timeout, missing service provider (learning/hidden layer not available).

**Remediation:** Check Neo4j pool metrics, verify service wiring, check specific action executor logs.

#### MDEMGRSICSafetyRejectionSpike (>0.1/min safety blocks)

**Symptoms:** Safety validator frequently blocking actions due to protected space or blast radius.

**Diagnosis:**

```bash
# Check safety state
curl -s http://localhost:9999/v1/self-improve/health | jq '.safety'

# Check rollback history
curl -s http://localhost:9999/v1/self-improve/rollback | jq '.'
```

**Root causes:** Blast radius limits too restrictive, unexpected data growth in target space, protected space misconfiguration.

**Remediation:** Review `RSIC_MAX_EDGES_AFFECTED` / `RSIC_MAX_NODES_AFFECTED` limits, check space data volume, verify protected spaces list.

#### MDEMGRSICHighRejectionRate (>50% triggers rejected)

**Symptoms:** Most trigger attempts are being blocked by cooldown, dedupe, or overlap guards.

**Diagnosis:**

```bash
# Check orchestration state
curl -s http://localhost:9999/v1/self-improve/health | jq '.orchestration'
```

**Root causes:** Cooldown period too long, triggers firing too rapidly, active cycles not completing (overlap block).

**Remediation:** Lower `RSIC_TRIGGER_COOLDOWN_SEC`, check for stuck active cycles, verify cycle completion is calling `CompleteCycle`.

#### MDEMGRSICLowConfidence (confidence < 0.3 for 30m)

**Symptoms:** Calibrator reporting persistently low success rate for one or more action types.

**Diagnosis:**

```bash
curl -s http://localhost:9999/v1/self-improve/calibration | jq '.'
```

**Root causes:** Action type has structural issues (bad Cypher, missing service), action depends on data conditions that don't exist.

**Remediation:** Run dry-run cycles to test the action, consider temporarily removing the action from the planner's repertoire.

#### MDEMGRSICHighDecayScore (decay > 8.0 for 10m)

**Symptoms:** Watchdog decay climbing, no recent cycles recorded.

**Diagnosis:**

```bash
curl -s http://localhost:9999/v1/self-improve/health | jq '{watchdog, orchestration}'
```

**Root causes:** All trigger sources disabled, cycles failing before completion, meso period too long.

**Remediation:** Enable at least one trigger source (`RSIC_MICRO_ENABLED=true`), check cycle error logs, trigger a manual cycle.

#### MDEMGRSICCycleDurationSpike (p95 > 300s for 10m)

**Symptoms:** Cycles taking longer than 5 minutes at the 95th percentile.

**Diagnosis:**

```bash
# Check active tasks
curl -s http://localhost:9999/v1/self-improve/health | jq '.active_tasks'

# Check Neo4j performance
curl -s http://localhost:9999/v1/prometheus | grep mdemg_tsdb_pool  # neo4j_pool_* gauges were deleted (no driver pool API); real pool stats are TSDB pgxpool
```

**Root causes:** Neo4j slow queries, large blast radius estimation, consolidation taking too long, network latency.

**Remediation:** Check Neo4j query performance, reduce action scope, increase tier timeout if appropriate.

### 11.3 RSIC Safe Mode

To disable all automatic RSIC triggers and run only manual cycles:

```bash
# Disable all automatic triggers
export RSIC_MICRO_ENABLED=false
export RSIC_MESO_PERIOD_SESSIONS=0
export RSIC_MACRO_CRON=""
export RSIC_WATCHDOG_ENABLED=false
```

In safe mode, only `POST /v1/self-improve/cycle` with `trigger_source: manual_api` will execute cycles. Use this during incidents or when investigating RSIC-related issues.

To re-enable, restore the previous values and restart the server.

### 11.4 RSIC SLOs

| Metric | Target | Measurement |
|--------|--------|-------------|
| Cycle success rate | > 95% | `completed / (completed + error + low_confidence)` over 24h |
| Meso cycle p95 duration | < 5 min | `histogram_quantile(0.95, rsic_cycle_duration_seconds{tier="meso"})` |
| Action success rate | > 95% | `success / (success + failed)` over 24h |
| Trigger rejection rate | < 10% | `rejected / (rejected + started)` over 24h |
| Watchdog force triggers | < 1/day | `increase(rsic_watchdog_force_total[24h])` |
| Safety blocks | < 5/day | `increase(rsic_safety_blocked_total[24h])` |

### 11.5 Grafana Dashboard

The RSIC Operations dashboard (`/d/mdemg-rsic`) provides 16 panels across 4 rows:

- **Overview**: Cycle rate, success rate, rejection rate, watchdog escalation level
- **Cycles**: Duration by tier, cycles by source, rejection reasons, outcome breakdown
- **Actions**: Success/failure table, duration p95, safety blocks, calibration confidence
- **Watchdog**: Decay score, escalation timeline, force triggers, snapshot creation
