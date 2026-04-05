# Upgrading to v0.6.0

This guide covers upgrading from v0.5.x to v0.6.0.

## What Changed

| Area | Change | Impact |
|------|--------|--------|
| SymbolNode storage | MERGE uses natural key `(space_id, name, file_path, symbol_type)` | No more duplicate SymbolNodes |
| V0023 migration | Self-healing dedup before uniqueness constraint | Safe on any graph state |
| Decay formula | Evidence-weighted: `rate/sqrt(evidence_count)`, default 0.02 | Well-evidenced edges decay slower |
| `QUERY_CLASSIFY_ENABLED` | Compose default `false` → `true` | Query classification active by default |
| New commands | `graph repair`, `maintenance`, `embeddings backfill` | Graph health tooling |
| Prune flags | `--match-ignore`, `--include-labels` | Finer-grained orphan control |
| Hidden layer | Batched orphan cleanup | No OOM during L2-L5 consolidation |

## Upgrade Steps

### Step 1: Update the binary

```bash
# Homebrew (recommended)
brew upgrade mdemg

# Or self-update
mdemg upgrade
```

### Step 2: Run graph repair (recommended)

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

### Step 3: Restart services

```bash
docker compose up -d
```

On startup, V0023 migration runs automatically:
- Deduplicates any remaining SymbolNode duplicates (batched, 500 per transaction)
- Creates the uniqueness constraint

### Step 4: Verify

```bash
mdemg data check --pre-campaign --json
```

All 8 checks should pass or warn (no failures).

## What If I Skip Graph Repair?

| Scenario | With `graph repair` first | Without |
|----------|--------------------------|---------|
| Server starts | Immediately | V0023 auto-dedup runs (may take longer on large graphs) |
| Edge weights | Preserved via aggregation | Lost (DETACH DELETE in migration) |
| Vendor nodes | Cleaned up | Remain until manual prune |
| Embeddings | Backfilled | Remain missing until manual backfill |

The migration is self-healing either way — the server will start. But `graph repair` preserves more data fidelity.

## Ongoing Maintenance

Schedule regular maintenance to keep the graph healthy:

```bash
# Weekly maintenance (decay + prune)
mdemg maintenance --space-id <your-space> --dry-run=false

# Or via cron
0 3 * * 0 /usr/local/bin/mdemg maintenance --space-id <your-space> --dry-run=false
```

## Troubleshooting

### Server won't start after upgrade

Check the Neo4j logs for migration errors:

```bash
docker compose logs neo4j
docker compose logs mdemg
```

V0023 is idempotent — restarting is safe. If the constraint creation fails, check for remaining duplicates:

```bash
mdemg graph repair --space-id <your-space> --dry-run
```

### Retrieval quality changed

The decay formula changed from flat-rate to evidence-weighted. If retrieval quality degrades:

```bash
# Check edge statistics
mdemg decay --space-id <your-space> --dry-run
```

Adjust `--decay-rate` (default 0.02) if edges are decaying too fast or too slow.

### Missing embeddings

```bash
mdemg embeddings backfill --space-id <your-space> --dry-run   # Count
mdemg embeddings backfill --space-id <your-space>              # Fill
```
