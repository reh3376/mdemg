# Upgrade Guide

This guide covers upgrading between MDEMG versions. Start with the section matching your current version.

## v0.6.x to v0.7.x

### Upgrade Steps

```bash
brew upgrade mdemg        # or: mdemg upgrade
docker compose up -d      # V0024 migration runs automatically
mdemg service install     # optional: adds weekly maintenance LaunchAgent
```

### Verify

```bash
curl -s http://localhost:9999/healthz | jq .   # all checks: ok
mdemg data check --pre-campaign                # 8 checks pass
```

### What Changed by Version

#### v0.7.0

| Area | Change | Impact |
|------|--------|--------|
| V0024 migration | `SignalState` node for signal learner persistence | Auto-runs on startup, no action needed |
| `Config.Validate()` | Cross-field constraint checking on startup | Server may reject invalid config combinations at boot |
| Maintenance LaunchAgent | `mdemg service install` adds weekly decay + prune | Optional but recommended |
| NilSafe embedder | Returns `ErrNoEmbedder` instead of panic | Safer startup without embedding provider |
| Codebase hardening | 3 P0, 4 P1, 2 P2 fixes | See CHANGELOG for details |

#### v0.7.1

| Area | Change | Impact |
|------|--------|--------|
| Jiminy classifier | Negation detection deferred to LLM Tier 2 | Eliminates false 4.5% contradicted rate |
| Outcome classifier | `not_applicable` for unrelated guidance | Prevents false confidence decay |
| Guidance normalization | Structured metadata → natural language | Cosine similarity ceiling raised from 0.33 to 0.59 |
| Similarity thresholds | High: 0.7→0.55, Low: 0.3→0.20 | Adjusted for action summary characteristics |

#### v0.7.2 (CRITICAL: .env model migration)

| Area | Change | Impact |
|------|--------|--------|
| **Default LLM model** | **gpt-5-nano → gpt-4.1-nano** | **Users with explicit `.env` overrides must update** |
| Trust accrual | Threshold 0.5 → 0.20, partial_compliance fixed | Trust grows at expected rate |
| J8 synthesis | Skipped at T1 trust (> 0.75) | T1 compact encoding preserved (5.2x compression) |
| PHP parser | 28th language parser added | Automatic for PHP codebases |
| FT plan v4.0 | Tool-use constraint, curated pipeline | Training docs updated |

**Action required:** If your `.env` sets `LLM_MODEL`, `RECLASS_MODEL`, or `RERANK_MODEL` to `gpt-5-nano` or `gpt-5.4`, you will get 404 errors from OpenAI after upgrading. Either:
1. Remove the override (recommended) — uses new default `gpt-4.1-nano`
2. Update the value to `gpt-4.1-nano` explicitly

```bash
# Check for affected overrides
grep -E '(LLM_MODEL|RECLASS_MODEL|RERANK_MODEL)' .env
```

#### v0.7.3

| Area | Change | Impact |
|------|--------|--------|
| Alert system | File backend at `~/.mdemg/alerts/current.json` | Alerts shown in hooks automatically |
| Alert evaluator | 13 TSDB-query rules run natively | Grafana no longer required for alerting |
| LLM retry | Exponential backoff on 429/503 | More resilient to transient LLM failures |
| Enhanced `/healthz` | Subsystem checks, `degraded` status | Better observability |
| Circuit breakers | Added to outcome classifier, codegen | Graceful degradation on sidecar failure |

New optional config vars:
- `ALERT_ENABLED` (true), `ALERT_COOLDOWN_SEC` (300), `ALERT_MAX_ENTRIES` (50)
- `ALERT_EVALUATOR_ENABLED` (true), `ALERT_EVALUATOR_INTERVAL_SEC` (30)
- `LLM_RETRY_ENABLED` (true), `LLM_RETRY_MAX_ATTEMPTS` (3)
- `HEALTH_PROBE_ENABLED` (true), `HEALTH_PROBE_INTERVAL_SEC` (60)

#### v0.7.4

| Area | Change | Impact |
|------|--------|--------|
| DD-P1P2 campaign | 10 P1 + 21 P2 bug fixes | Concurrency, decay, compose, timeouts fixed |
| Healthcheck port | Uses `${MDEMG_PORT:-9999}` | Custom port users: compose healthcheck now works |
| Code comprehension | Feature-gated feedback loop | Off by default (`JIMINY_CODE_REGEN_ENABLED=false`) |
| Embedding cache TTL | `NODE_EMBEDDING_CACHE_TTL_SEC` (3600) | Stale embeddings evicted after 1 hour |
| Goroutine semaphore | RSIC dispatch capped at 50 | Prevents goroutine explosion |

---

## v0.5.x to v0.6.0

This section covers upgrading from v0.5.x to v0.6.0.

### What Changed

| Area | Change | Impact |
|------|--------|--------|
| SymbolNode storage | MERGE uses natural key `(space_id, name, file_path, symbol_type)` | No more duplicate SymbolNodes |
| V0023 migration | Self-healing dedup before uniqueness constraint | Safe on any graph state |
| Decay formula | Evidence-weighted: `rate/sqrt(evidence_count)`, default 0.02 | Well-evidenced edges decay slower |
| `QUERY_CLASSIFY_ENABLED` | Compose default `false` → `true` | Query classification active by default |
| New commands | `graph repair`, `maintenance`, `embeddings backfill` | Graph health tooling |
| Prune flags | `--match-ignore`, `--include-labels` | Finer-grained orphan control |
| Hidden layer | Batched orphan cleanup | No OOM during L2-L5 consolidation |
| Training data export | `instance_id` auto-generated as `{hostname}-{space_id}` | Export no longer requires `MDEMG_INSTANCE_ID` to be set manually |
| `mdemg init` | Writes `MDEMG_INSTANCE_ID` to `.env` | New installs get consistent instance tracking |

### Upgrade Steps

#### Step 1: Update the binary

```bash
# Homebrew (recommended)
brew upgrade mdemg

# Or self-update
mdemg upgrade
```

#### Step 2: Run graph repair (recommended)

Before restarting (which triggers V0023 migration), repair any duplicate SymbolNodes:

```bash
mdemg graph repair --space-id <your-space> --dry-run        # Preview
mdemg graph repair --space-id <your-space> --dry-run=false   # Execute
```

The repair command:
1. Removes vendor/ignored nodes
2. Merges duplicate SymbolNodes (preserves CO_ACTIVATED_WITH edge weights)
3. Sweeps orphans
4. Backfills missing embeddings
5. Reports V0023 readiness

#### Step 3: Restart services

```bash
docker compose up -d
```

On startup, V0023 migration runs automatically:
- Deduplicates any remaining SymbolNode duplicates (batched, 500 per transaction)
- Creates the uniqueness constraint

#### Step 4: Verify

```bash
mdemg data check --pre-campaign --json
```

All 8 checks should pass or warn (no failures).

### What If I Skip Graph Repair?

| Scenario | With `graph repair` first | Without |
|----------|--------------------------|---------|
| Server starts | Immediately | V0023 auto-dedup runs (may take longer on large graphs) |
| Edge weights | Preserved via aggregation | Lost (DETACH DELETE in migration) |
| Vendor nodes | Cleaned up | Remain until manual prune |
| Embeddings | Backfilled | Remain missing until manual backfill |

The migration is self-healing either way — the server will start. But `graph repair` preserves more data fidelity.

### Ongoing Maintenance

Schedule regular maintenance to keep the graph healthy:

```bash
# Weekly maintenance (decay + prune)
mdemg maintenance --space-id <your-space> --dry-run=false

# Or via cron
0 3 * * 0 /usr/local/bin/mdemg maintenance --space-id <your-space> --dry-run=false
```

### Troubleshooting

#### Server won't start after upgrade

Check the Neo4j logs for migration errors:

```bash
docker compose logs neo4j
docker compose logs mdemg
```

V0023 is idempotent — restarting is safe. If the constraint creation fails, check for remaining duplicates:

```bash
mdemg graph repair --space-id <your-space> --dry-run
```

#### Retrieval quality changed

The decay formula changed from flat-rate to evidence-weighted. If retrieval quality degrades:

```bash
# Check edge statistics
mdemg decay --space-id <your-space> --dry-run
```

Adjust `--decay-rate` (default 0.02) if edges are decaying too fast or too slow.

#### Missing embeddings

```bash
mdemg embeddings backfill --space-id <your-space> --dry-run   # Count
mdemg embeddings backfill --space-id <your-space>              # Fill
```
