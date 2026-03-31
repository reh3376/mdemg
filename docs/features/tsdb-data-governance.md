# TimescaleDB Data Governance

## Overview

TimescaleDB is MDEMG's time-series storage layer for operational metrics, LLM interaction logs, embedding events, and retrieval pipeline traces. This document covers the collection architecture, configuration dependencies, data flow, and governance model.

## Collection Architecture

### Data Flow

```
HTTP request → metrics middleware → MetricsRecorder (in-memory buffer)
                                         │
                                    flush interval (60s default)
                                         │
                                         ▼
                              TimescaleDB (metric_samples hypertable)

LLM call → llm_writer.go → batch buffer → CopyFrom → llm_interactions
Embedding call → embedding_writer.go → batch buffer → CopyFrom → embedding_events
Retrieval call → retrieval_writer.go → batch buffer → CopyFrom → retrieval_events
```

All writers use PostgreSQL `CopyFrom` for high-throughput batch inserts. Buffers flush on the configured interval (`TSDB_FLUSH_INTERVAL_SEC`, default 60s) or when the buffer reaches capacity.

### Tables

| Table | Hypertable | Purpose | Writer |
|-------|-----------|---------|--------|
| `metric_samples` | Yes (7-day chunks) | Operational metrics (HTTP latency, Neo4j pool, RSIC scores) | `internal/tsdb/writer.go` |
| `llm_interactions` | Yes (7-day chunks) | LLM call logs (26 columns: prompts, tokens, latency, RAFT context) | `internal/tsdb/llm_writer.go` |
| `embedding_events` | Yes (7-day chunks) | Embedding call logs (23 columns: text, dimensions, cache hit) | `internal/tsdb/embedding_writer.go` |
| `retrieval_events` | Yes (7-day chunks) | Retrieval pipeline traces (22 columns: recall, BM25, rerank stages) | `internal/tsdb/retrieval_writer.go` |
| `ft_training_cycles` | No | Fine-tuning cycle metadata | — |
| `ft_model_versions` | No | Model version registry | — |
| `ft_benchmarks` | No | Benchmark results | — |
| `ft_hitl_decisions` | No | Human-in-the-loop decisions | — |
| `tsdb_schema_meta` | No | Schema version tracking | `internal/tsdb/client.go` |

### Continuous Aggregates

| Aggregate | Source | Interval | Retention |
|-----------|--------|----------|-----------|
| `metrics_hourly` | `metric_samples` | 1 hour | 365 days (`TSDB_HOURLY_RETENTION_DAYS`) |
| `metrics_daily` | `metric_samples` | 1 day | 365 days |

Raw `metric_samples` data is retained for 90 days (`TSDB_RAW_RETENTION_DAYS`).

## Configuration Dependency Chain

The master gate for all TSDB data collection is `TSDB_ENABLED`. When `false` (the default), no writers are initialized and no data is captured, even if TimescaleDB is running and connected.

```
TSDB_ENABLED=true
  │
  ├── TSDB connection established (host, port, user, password, database)
  │     │
  │     ├── TSDB_AUTO_MIGRATE=true → run schema migrations on startup
  │     │
  │     └── TSDB_REQUIRED_SCHEMA_VERSION=7 → refuse to start if schema is behind
  │
  ├── metric_samples writer (always active when TSDB connected)
  │     └── TSDB_FLUSH_INTERVAL_SEC=60 → batch flush cadence
  │
  ├── LLM_INTERACTION_LOGGING=true → llm_interactions writer
  │
  ├── EMBEDDING_EVENT_LOGGING=true → embedding_events writer
  │
  ├── RETRIEVAL_EVENT_LOGGING=true → retrieval_events writer
  │
  └── TSDB_BACKUP_ENABLED=true → backup scheduler
        ├── TSDB_BACKUP_INTERVAL_HOURS=24
        └── TSDB_BACKUP_RETENTION_COUNT=14
```

### Docker Compose Defaults

`docker-compose.yml` overrides three critical defaults for Docker deployments:

| Flag | Code Default | Docker Override | Why |
|------|-------------|-----------------|-----|
| `TSDB_ENABLED` | `false` | `true` | TimescaleDB runs as a compose service — always enable collection |
| `TSDB_AUTO_MIGRATE` | `false` | `true` | Schema must be created on first startup without manual intervention |
| `TSDB_OPTIONAL` | `true` | `true` | Server should start even if TimescaleDB is still initializing |

The `TSDB_AUTO_MIGRATE` env var is handled specially: it is not a Config struct field. Instead, `internal/cli/serve.go` checks `os.Getenv("TSDB_AUTO_MIGRATE")` at startup and merges it with the `--auto-migrate` CLI flag. This accommodates Docker ENTRYPOINT commands that cannot pass CLI flags.

## Schema Migrations

Migrations live in `internal/tsdb/migrations/` and are applied in order:

| File | Version | Creates |
|------|---------|---------|
| `001_metrics_schema.sql` | 1 | `metric_samples` hypertable, `llm_interactions`, `tsdb_schema_meta` |
| `002_ft_schema.sql` | 2 | `ft_training_cycles`, `ft_model_versions`, `ft_benchmarks`, `ft_hitl_decisions` |
| `003_metric_types.sql` | 3 | `metric_type` enum column, type validation |
| `004_aggregate_policies.sql` | 4 | `metrics_hourly`, `metrics_daily` continuous aggregates + refresh policies |
| `005_interaction_enrichment.sql` | 5 | RAFT context columns on `llm_interactions` |
| `006_embedding_retrieval_events.sql` | 6 | `embedding_events`, `retrieval_events` hypertables |
| `007_raft_context.sql` | 7 | Additional RAFT training context columns |

Current required version: **7** (`TSDB_REQUIRED_SCHEMA_VERSION`).

## Privacy and Scrubbing

LLM interaction logs pass through `internal/llmclient/scrubber.go` before being written to TSDB. The scrubber applies 5 regex patterns to remove:
- API keys
- Email addresses
- File paths containing usernames
- Bearer tokens
- Base64-encoded credentials

`embedding_events.query_text` is intentionally NOT scrubbed (by design — query text is needed for retrieval quality analysis).

## Data Quality Monitoring

### CLI Commands

- `mdemg data status` — TSDB connection health and table row counts
- `mdemg data inspect` — Sample recent rows from each table
- `mdemg data stats` — Aggregate statistics per table
- `mdemg data quality` — Data quality checks (NULL rates, gap detection)
- `mdemg data audit` — Privacy audit (scrub pattern detection)

### Grafana Dashboards

The mdemg-overview Grafana dashboard includes TSDB panels sourced from the `timescaledb` datasource, showing metric trends, collection rates, and writer health.

## Documents Accessed

- `internal/config/config.go` — TSDB config flag definitions and defaults (lines 778-790, 3076-3109)
- `internal/cli/serve.go` — TSDB initialization, auto-migrate env var support (lines 76-177)
- `internal/tsdb/client.go` — TSDB client, migration runner, schema version check
- `internal/tsdb/writer.go` — metric_samples batch writer
- `internal/tsdb/llm_writer.go` — LLM interaction logger
- `internal/tsdb/embedding_writer.go` — Embedding event logger
- `internal/tsdb/retrieval_writer.go` — Retrieval event logger
- `internal/tsdb/migrations/` — Schema migration SQL files (001-007)
- `internal/llmclient/scrubber.go` — Privacy scrubber patterns
- `internal/api/server.go` — TSDB client wiring (SetTSDBClient)
- `docker-compose.yml` — Docker TSDB environment overrides
- `docs/features/docker-deployment.md` — TSDB config flags table
