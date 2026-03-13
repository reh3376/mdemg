# Configuration & Tuning

All configuration is done via environment variables. Set these in `mdemg_build/service/.env` or export them before starting the service.

---

## Required Configuration

```bash
# Neo4j Connection (required)
NEO4J_URI=bolt://localhost:7687
NEO4J_USER=neo4j
NEO4J_PASS=testpassword
REQUIRED_SCHEMA_VERSION=4
```

---

## Service Configuration

```bash
# HTTP Server
LISTEN_ADDR=:9999                    # Service listen address (default :9999)

# Dynamic Port Allocation
# If LISTEN_ADDR port is busy, the server scans this range for an available port.
# PORT_RANGE_START=9999              # Start of fallback range (default: derived from LISTEN_ADDR)
# PORT_RANGE_END=10099               # End of fallback range (default: PORT_RANGE_START + 100)
# PORT_FILE_PATH=.mdemg.port        # Port file for client discovery (default: .mdemg.port)

# Vector Index
VECTOR_INDEX_NAME=memNodeEmbedding   # Neo4j vector index name
```

### Dynamic Port Allocation

The server writes a `.mdemg.port` file containing the actual bound port. Client tools (`mcp-server`, `ingest-codebase`, shell scripts) read this file automatically.

**Resolution priority** (used by `config.ResolveEndpoint()`):

1. `MDEMG_ENDPOINT` environment variable (explicit override)
2. `.mdemg.port` file (dynamic discovery)
3. `LISTEN_ADDR` environment variable (static config)
4. Hardcoded default (`http://localhost:9999`)

The port file is removed on graceful shutdown (SIGINT/SIGTERM).

---

## Embedding Provider

```bash
# Provider selection: "openai", "ollama", or "" (disabled)
EMBEDDING_PROVIDER=openai

# OpenAI Configuration (when EMBEDDING_PROVIDER=openai)
OPENAI_API_KEY=sk-...                # Required for OpenAI
OPENAI_MODEL=text-embedding-3-small   # Embedding model (1536 dims, recommended)
OPENAI_ENDPOINT=https://api.openai.com/v1

# Ollama Configuration (when EMBEDDING_PROVIDER=ollama)
OLLAMA_ENDPOINT=http://localhost:11434
OLLAMA_MODEL=qwen3-embedding:4b      # Embedding model (1536 dims)
```

---

## Embedding Cache

Reduces API calls by caching embedding results.

```bash
EMBEDDING_CACHE_ENABLED=true         # Enable LRU cache (default true)
EMBEDDING_CACHE_SIZE=1000            # Max cached embeddings (default 1000)
```

When enabled, readyz shows: `"embedding_provider": "openai:text-embedding-3-small+cache"`

---

## Retrieval Tuning

```bash
# Candidate generation
DEFAULT_CANDIDATE_K=200              # Vector recall candidates (default 200)
DEFAULT_TOP_K=20                     # Final results returned (default 20)
DEFAULT_HOP_DEPTH=2                  # Graph expansion depth (default 2)

# Graph expansion limits
MAX_NEIGHBORS_PER_NODE=50            # Per-hop degree cap (default 50)
MAX_TOTAL_EDGES_FETCHED=5000         # Total edge limit (default 5000)
```

---

## Scoring Hyperparameters

The scoring formula is:

```
S = α*V + β*A + γ_eff*R + δ*C - φ*log(1+deg) - κ*d
```

Where `γ_eff` = `γ` normally, or `γ * TEMPORAL_SOFT_BOOST` when temporal soft-mode is active.

Current defaults (hardcoded in `scoring.go`, configurable via future update):

| Symbol | Name | Default | Description |
|--------|------|---------|-------------|
| α | SCORING_WEIGHT_VECTOR | 0.55 | Vector similarity weight |
| β | SCORING_WEIGHT_ACTIVATION | 0.30 | Activation score weight |
| γ | SCORING_WEIGHT_RECENCY | 0.10 | Recency weight |
| δ | SCORING_WEIGHT_CONFIDENCE | 0.05 | Confidence weight |
| φ | SCORING_PENALTY_HUB | 0.08 | Hub penalty (log degree) |
| κ | SCORING_PENALTY_REDUNDANCY | 0.12 | Path-prefix redundancy penalty |
| ρ | SCORING_RECENCY_DECAY | 0.05 | Recency decay rate per day |

---

## Temporal Retrieval

Time-aware retrieval that detects temporal intent in queries and adjusts scoring or filtering accordingly.

```bash
TEMPORAL_ENABLED=true              # Enable temporal query understanding (default: true)
TEMPORAL_SOFT_BOOST=3.0            # Recency weight multiplier for soft-mode (default: 3.0, range: 1.0-10.0)
TEMPORAL_HARD_FILTER=true          # Enable hard time-range filtering (default: true)
```

### Temporal Modes

| Mode | Trigger | Behavior |
|------|---------|----------|
| `none` | No temporal language detected | Pipeline unchanged — zero regression |
| `soft` | "recent", "latest", "what's new" | Boosts recency weight: `γ_eff = γ × TEMPORAL_SOFT_BOOST` |
| `hard` | "in the last 7 days", "since Jan 2026" | Filters candidates by time range `[after, before)` |

### Hard-Mode Triggers

- "in the last N days/weeks/months"
- "since YYYY-MM-DD" or "since Month Year"
- "before YYYY-MM-DD" or "before Month Year"
- "between DATE and DATE"
- "this week/month/year"

### Soft-Mode Triggers

- "recent", "recently", "latest", "newest"
- "what changed", "what's new", "updates to"
- "new changes", "latest changes", "recent changes"

### API Override

Explicit temporal constraints can be passed via `temporal_after` and `temporal_before` (ISO 8601) on the retrieve and recall endpoints. These override auto-detected intent and force hard-mode filtering.

### Cache Behavior

Temporal queries (`soft` or `hard` mode) bypass the query cache since results are time-sensitive.

---

## Hebbian Learning

```bash
LEARNING_EDGE_CAP_PER_REQUEST=200    # Max CO_ACTIVATED_WITH edges per request
```

Learning formula: `Δw = η * etaMult * a_i * a_j - μ * w_ij`

Weight clamping: `wmax * tanh(w / wmax)` (smooth saturation via tanh soft-cap)

Defaults (in `learning/service.go`):

- `η` (learning rate): 0.1
- `μ` (regularization): 0.01
- `w_min`: 0.0
- `w_max`: 1.0

### ANN Learning Optimizations

```bash
# Cautious Decay — skip decay for recently reinforced edges
LEARNING_CAUTIOUS_DECAY_WINDOW_HOURS=24   # 0=disabled

# Multi-Rate Learning — context-specific eta multipliers
LEARNING_ETA_CONVERSATION_MULT=2.0        # Conversation observations learn faster
LEARNING_ETA_CONFIG_MULT=1.5              # Config↔code edges get stronger signal
LEARNING_ETA_SAME_DIR_MULT=1.2            # Same-directory nodes get proximity boost

# Learning Rate Schedule — maturity-based eta scaling
LEARNING_SCHEDULE_ENABLED=true
LEARNING_SCHEDULE_COLD_MULT=2.0           # 0 edges: accelerated learning
LEARNING_SCHEDULE_LEARNING_MULT=1.0       # 1-10k edges: normal
LEARNING_SCHEDULE_WARM_MULT=0.5           # 10k-50k edges: stabilizing
LEARNING_SCHEDULE_SAT_MULT=0.25           # 50k+ edges: minimal updates

# Negative Feedback
LEARNING_NEGATIVE_WEIGHT=0.15             # Weight reduction per negative feedback
LEARNING_NEGATIVE_DECAY_MULT=2.0          # Decay multiplier for contradicted edges
LEARNING_NEGATIVE_MAX_PER_REQUEST=20      # Max rejected nodes per request
```

### ANN Retrieval Optimizations

```bash
# Squared Activation — sharper, sparser signals
SCORING_ACTIVATION_FLOOR=0.05
SCORING_ACTIVATION_SQUARED=true

# Local-First Activation Spreading — per-hop weight thresholds
ACTIVATION_HOP0_MIN_WEIGHT=0.5
ACTIVATION_HOP1_MIN_WEIGHT=0.2
ACTIVATION_HOP2_MIN_WEIGHT=0.05

# Value Residual Bypass — bonus for high-confidence vector matches
SCORING_BYPASS_THRESHOLD=0.85
SCORING_BYPASS_WEIGHT=0.15
SCORING_BYPASS_CODE_MULT=1.3
SCORING_BYPASS_ARCH_MULT=0.5
```

### ANN Consolidation Optimizations

```bash
# L0 Skip Connections (GROUNDED_BY edges from L5 to L0)
L5_GROUNDING_MAX_EDGES=5
L5_GROUNDING_MIN_SIM=0.4
L5_GROUNDING_INITIAL_WEIGHT=0.5
EDGE_ATTENTION_GROUNDED_BY=0.70

# Frontier Detection
FRONTIER_MIN_EVIDENCE=3
FRONTIER_MAX_OUTGOING=2
```

---

## Semantic Edge Creation

Automatically creates ASSOCIATED_WITH edges on ingest to similar existing nodes.

```bash
SEMANTIC_EDGE_TOP_N=5                # Max edges created per ingest (default 5)
SEMANTIC_EDGE_MIN_SIMILARITY=0.7     # Minimum similarity threshold (default 0.7)
```

---

## Batch Ingest

```bash
BATCH_INGEST_MAX_ITEMS=100           # Max observations per batch (1-1000, default 100)
```

Batch endpoint: `POST /v1/memory/ingest/batch`

---

## Anomaly Detection

Non-blocking anomaly detection during ingest.

```bash
ANOMALY_DETECTION_ENABLED=true       # Enable detection (default true)
ANOMALY_DUPLICATE_THRESHOLD=0.95     # Vector similarity for duplicates (default 0.95)
ANOMALY_STALE_DAYS=30                # Days before update is "stale" (default 30)
ANOMALY_MAX_CHECK_MS=100             # Timeout for checks in ms (default 100)
```

Detected anomalies returned in ingest response `anomalies[]` field.

---

## Activation Model

Spreading activation parameters (in `activation.go`):

| Parameter | Default | Description |
|-----------|---------|-------------|
| Steps (T) | 3 | Activation propagation iterations |
| Decay (λ) | 0.15 | Per-step decay factor |
| Min threshold | 0.20 | Ignore nodes below this for learning |

---

## Edge Decay CLI

The `cmd/decay` CLI applies exponential decay to edge weights.

Formula: `w_new = w_old * exp(-decay_rate * days)`

| Parameter | Default | Description |
|-----------|---------|-------------|
| Decay rate | 0.01 | Daily decay rate |
| Prune threshold | 0.01 | Remove edges below this weight |
| Protected | - | Edges with `is_pinned=true` skip decay |

Usage:

```bash
go run ./cmd/decay --space-id ide-agent --dry-run
go run ./cmd/decay --space-id ide-agent --execute
```

---

## Consolidation CLI

The `cmd/consolidate` CLI detects clusters and promotes abstractions.

| Parameter | Default | Description |
|-----------|---------|-------------|
| Min cluster size | 3 | Minimum nodes for cluster |
| Max cluster size | 15 | Maximum nodes per cluster |
| Min density | 0.5 | Internal edge density threshold |
| Evidence threshold | 3 | Min evidence count for promotion |

Usage:

```bash
go run ./cmd/consolidate --space-id ide-agent --dry-run
go run ./cmd/consolidate --space-id ide-agent --execute
```

---

## Complete .env Example

```bash
# MDEMG Service Configuration

# Neo4j (required)
NEO4J_URI=bolt://localhost:7687
NEO4J_USER=neo4j
NEO4J_PASS=testpassword
REQUIRED_SCHEMA_VERSION=4

# Service
LISTEN_ADDR=:9999

# Dynamic Port Allocation (optional)
# PORT_RANGE_START=9999
# PORT_RANGE_END=8999
# PORT_FILE_PATH=.mdemg.port

# Embedding Provider
EMBEDDING_PROVIDER=openai
OPENAI_API_KEY=sk-proj-...
OPENAI_MODEL=text-embedding-3-small
OPENAI_ENDPOINT=https://api.openai.com/v1

# Embedding Cache
EMBEDDING_CACHE_ENABLED=true
EMBEDDING_CACHE_SIZE=1000

# Vector Index
VECTOR_INDEX_NAME=memNodeEmbedding

# Retrieval
DEFAULT_CANDIDATE_K=200
DEFAULT_TOP_K=20
DEFAULT_HOP_DEPTH=2
MAX_NEIGHBORS_PER_NODE=50
MAX_TOTAL_EDGES_FETCHED=5000

# Learning
LEARNING_EDGE_CAP_PER_REQUEST=200

# Semantic Edges
SEMANTIC_EDGE_TOP_N=5
SEMANTIC_EDGE_MIN_SIMILARITY=0.7

# Batch Ingest
BATCH_INGEST_MAX_ITEMS=100

# Anomaly Detection
ANOMALY_DETECTION_ENABLED=true
ANOMALY_DUPLICATE_THRESHOLD=0.95
ANOMALY_STALE_DAYS=30
ANOMALY_MAX_CHECK_MS=100

# Temporal Retrieval
TEMPORAL_ENABLED=true
TEMPORAL_SOFT_BOOST=3.0
TEMPORAL_HARD_FILTER=true
```
