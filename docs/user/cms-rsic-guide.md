# CMS & RSIC Usage Guide for MDEMG

This guide covers the two core runtime systems in MDEMG: the **Conversation Memory System (CMS)** for capturing and retrieving knowledge, and the **Recursive Self-Improvement Cycle (RSIC)** for automated memory maintenance. All examples use `curl` against a running MDEMG server.

> **Dynamic Ports:** MDEMG uses dynamically assigned ports (set during `mdemg init`). Your port is in `.env` (`MDEMG_PORT`) or `.mdemg.port`. All examples use `localhost:9999` as a placeholder — replace with your assigned port.

---

## Table of Contents

1. [What is CMS?](#what-is-cms)
2. [CMS Workflow](#cms-workflow)
   - [Observe](#observe-capture-knowledge)
   - [Correct](#correct-record-mistakes)
   - [Resume](#resume-restore-context-at-session-start)
   - [Recall](#recall-semantic-query)
   - [Consolidate](#consolidate-build-hierarchy)
   - [Graduate](#graduate-lifecycle-management)
3. [Session Health and Constraints](#session-health-and-constraints)
4. [What is RSIC?](#what-is-rsic)
5. [RSIC Workflow](#rsic-workflow)
   - [Assess](#assess-system-health-scoring)
   - [Run a Full Cycle](#run-a-full-cycle)
   - [Monitor Health](#monitor-health)
   - [History and Calibration](#history-and-calibration)
   - [Learning Freeze](#learning-freeze)
   - [Rollback](#rollback)
6. [Jiminy Inner-Voice Guidance](#jiminy-inner-voice-guidance)
7. [Practical Examples](#practical-examples)
   - [Setting Up CMS for a New AI Agent](#setting-up-cms-for-a-new-ai-agent)
   - [Daily Maintenance Workflow](#daily-maintenance-workflow)
   - [Debugging Poor Retrieval Quality](#debugging-poor-retrieval-quality)
   - [Running RSIC on a Schedule](#running-rsic-on-a-schedule)

---

## What is CMS?

The Conversation Memory System is MDEMG's persistent knowledge layer. It gives AI agents the ability to remember across sessions -- capturing observations during work, detecting what is novel, organizing knowledge into themes and concepts, and restoring context when a session starts.

CMS is built on a **5-layer emergent hierarchy** stored in a Neo4j graph:

| Layer | Node Type | Description |
|-------|-----------|-------------|
| L0 | `conversation_observation` | Raw observations captured during sessions |
| L1 | `conversation_theme` | Clusters of related observations (auto-generated) |
| L2-L4 | `hidden` / emergent concepts | Higher-order abstractions built by consolidation |
| L5 | `emergent_concept` | Top-level emergent concepts spanning many themes |

### Core Concepts

**Spaces**: All memory lives within a `space_id` -- a logical namespace. A space might represent a project, a team, or a single agent's memory. The space `mdemg-dev` is protected and cannot be deleted.

**Sessions**: Within a space, `session_id` tracks individual conversation sessions. Observations are tagged with both space and session.

**Surprise Scoring**: Every observation gets a surprise score (0.0-1.0) measuring how novel it is. The score is computed from four factors:

| Factor | Weight | What It Measures |
|--------|--------|------------------|
| Correction detection | 0.40 | Explicit user corrections ("No, that's wrong...") |
| Term novelty | 0.25 | Domain-specific terms (CamelCase, acronyms, snake_case) |
| Embedding novelty | 0.25 | Cosine distance from existing observations |
| Contradiction score | 0.10 | Conflicts with known facts (placeholder for future) |

High-surprise observations persist longer in memory and rank higher during resume and recall.

**Hebbian Learning**: When observations are co-retrieved or co-activated, MDEMG creates `LEARNING_EDGE` relationships between them with weighted strengths that strengthen through use and decay over time -- modeled after biological synaptic plasticity.

**Temporal Decay**: Edge weights and observation relevance decay over time. Observations start as "volatile" and must earn permanence through reinforcement (repeated access, co-activation). A `stability_score` (0.0-1.0) tracks this, with a graduation threshold of 0.8.

---

## CMS Workflow

### Observe (Capture Knowledge)

**Endpoint**: `POST /v1/conversation/observe`

This is the primary way to feed knowledge into CMS. Every significant piece of information -- a decision, a learning, a preference, an error -- gets captured as an observation.

**Observation Types** (the `obs_type` field):

| Type | Importance | When to Use |
|------|-----------|-------------|
| `correction` | 0.9 | User explicitly corrects a mistake |
| `decision` | 0.8 | Architectural or approach choices |
| `blocker` | 0.8 | Issues preventing progress |
| `error` | 0.75 | Build failures, runtime errors |
| `context` | 0.7 | Background information for continuity |
| `task` | 0.7 | Task tracking and work items |
| `insight` | 0.65 | Discoveries and realizations |
| `learning` | 0.6 | New domain knowledge (default if omitted) |
| `preference` | 0.5 | User's coding style, tool preferences |
| `technical_note` | 0.5 | Technical documentation |
| `progress` | 0.4 | Status updates |

**Basic observation**:

```bash
curl -s -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "session_id": "session-001",
    "content": "The API uses JWT tokens with RS256 signing, not HS256 as initially assumed.",
    "obs_type": "learning"
  }' | jq .
```

<details>
<summary>PowerShell equivalent</summary>

```powershell
$body = @{
    space_id   = "my-project"
    session_id = "session-001"
    content    = "The API uses JWT tokens with RS256 signing, not HS256 as initially assumed."
    obs_type   = "learning"
} | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/conversation/observe" -Body $body -ContentType "application/json"
```
</details>

**Response**:

```json
{
  "obs_id": "a1b2c3d4-...",
  "node_id": "e5f6g7h8-...",
  "surprise_score": 0.72,
  "surprise_factors": {
    "term_novelty": 0.35,
    "contradiction_score": 0.0,
    "correction_score": 0.0,
    "embedding_novelty": 0.68
  }
}
```

**With tags and metadata**:

```bash
curl -s -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "session_id": "session-001",
    "content": "Always use structured logging with zap, never fmt.Println in production code.",
    "obs_type": "preference",
    "tags": ["coding-style", "logging"],
    "metadata": {"source": "code-review"},
    "pinned": true
  }' | jq .
```

<details>
<summary>PowerShell equivalent</summary>

```powershell
$body = @{
    space_id   = "my-project"
    session_id = "session-001"
    content    = "Always use structured logging with zap, never fmt.Println in production code."
    obs_type   = "preference"
    tags       = @("coding-style", "logging")
    metadata   = @{ source = "code-review" }
    pinned     = $true
} | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/conversation/observe" -Body $body -ContentType "application/json"
```
</details>

**Pinned observations** (`"pinned": true`) are permanent -- they never decay and are always included in resume. Use them for critical preferences, project rules, and skill definitions.

**Multi-agent and visibility**:

```bash
curl -s -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "shared-project",
    "session_id": "session-001",
    "content": "Database schema uses soft deletes (is_archived flag, not DELETE).",
    "obs_type": "decision",
    "user_id": "alice",
    "agent_id": "agent-claude-1",
    "visibility": "team",
    "refers_to": ["node-id-of-schema-doc"]
  }' | jq .
```

Visibility values: `private` (only the `user_id` owner can see it), `team` (anyone in the space), `global` (visible to all).

**Constraint detection**: When an observation contains constraint-like language (e.g., "NEVER do X", "ALWAYS use Y"), MDEMG automatically detects it and creates constraint nodes. The response will include `detected_constraints` if any are found.

**RSIC micro-trigger**: If `RSIC_MICRO_ENABLED` is set, each observation may trigger a lightweight micro-tier RSIC cycle in the background to maintain memory health.

---

### Correct (Record Mistakes)

**Endpoint**: `POST /v1/conversation/correct`

Use this when the agent made a mistake and the user explicitly corrects it. Corrections always receive a minimum surprise score of 0.9, ensuring they persist and rank highly.

```bash
curl -s -X POST http://localhost:9999/v1/conversation/correct \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "session_id": "session-001",
    "incorrect": "The config file is at /etc/app/config.yaml",
    "correct": "The config file is at /opt/app/config.yaml",
    "context": "Deployment paths were changed in the v2.0 migration"
  }' | jq .
```

<details>
<summary>PowerShell equivalent</summary>

```powershell
$body = @{
    space_id   = "my-project"
    session_id = "session-001"
    incorrect  = "The config file is at /etc/app/config.yaml"
    correct    = "The config file is at /opt/app/config.yaml"
    context    = "Deployment paths were changed in the v2.0 migration"
} | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/conversation/correct" -Body $body -ContentType "application/json"
```
</details>

The correction is stored as a combined observation with `obs_type: correction` and content formatted as `CORRECTION: Incorrect: ... | Correct: ...`. The high surprise score ensures this knowledge dominates when the topic comes up again.

---

### Resume (Restore Context at Session Start)

**Endpoint**: `POST /v1/conversation/resume`

This is the first call an agent should make at the start of every session. It returns the most relevant observations, themes, and emergent concepts from the space -- effectively restoring the agent's memory.

**Basic resume**:

```bash
curl -s -X POST http://localhost:9999/v1/conversation/resume \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "session_id": "session-002",
    "max_observations": 10
  }' | jq .
```

<details>
<summary>PowerShell equivalent</summary>

```powershell
$body = @{
    space_id         = "my-project"
    session_id       = "session-002"
    max_observations = 10
} | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/conversation/resume" -Body $body -ContentType "application/json"
```
</details>

**Response structure**:

```json
{
  "space_id": "my-project",
  "session_id": "session-002",
  "observations": [
    {
      "node_id": "...",
      "obs_type": "correction",
      "content": "CORRECTION: Incorrect: ... | Correct: ...",
      "summary": "Config file at /opt/app/config.yaml, not /etc/app/",
      "surprise_score": 0.9,
      "score": 0.87,
      "tags": ["correction", "session:session-001"],
      "created_at": "2026-03-10T14:22:00Z"
    }
  ],
  "themes": [
    {
      "node_id": "...",
      "name": "Deployment Configuration",
      "summary": "Configuration paths and deployment conventions...",
      "member_count": 5,
      "dominant_obs_type": "decision",
      "avg_surprise_score": 0.65
    }
  ],
  "emergent_concepts": [
    {
      "node_id": "...",
      "name": "Infrastructure Patterns",
      "summary": "Recurring patterns in deployment and infrastructure...",
      "layer": 3,
      "keywords": ["deployment", "config", "docker"],
      "session_count": 4
    }
  ],
  "summary": "Restored 10 recent observations...",
  "jiminy": {
    "rationale": "Prioritized 10 observations with high novelty...",
    "confidence": 0.85,
    "score_breakdown": {
      "surprise_avg": 0.55,
      "surprise_max": 0.9,
      "recency": 0.8,
      "theme_coverage": 1.0,
      "concept_coverage": 0.5
    },
    "highlights": [
      "High-surprise correction: Config file at /opt/app/config.yaml"
    ]
  },
  "memory_state": "healthy"
}
```

**Relevance scoring for resume**: Observations are ranked by a composite score, not just recency:

| Factor | Weight | Description |
|--------|--------|-------------|
| Recency | 0.40 | Exponential decay over hours |
| Surprise | 0.25 | Higher surprise = more important |
| Type priority | 0.20 | Corrections > decisions > errors > learnings > progress |
| Co-activation | 0.15 | Observations frequently retrieved together rank higher |

**Jiminy Rationale**: The `jiminy` field explains WHY specific observations were selected, with confidence scores and highlighted items. It is named after Jiminy Cricket -- the conscience that provides guidance.

**Filtering options**:

```bash
curl -s -X POST http://localhost:9999/v1/conversation/resume \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "include_decisions": true,
    "include_learnings": true,
    "max_observations": 20,
    "agent_id": "agent-claude-1",
    "requesting_user_id": "alice"
  }' | jq .
```

**Anomaly detection**: If meta-cognition is enabled (`METACOG_ENABLED=true`), resume checks for issues:
- `empty-resume`: Space has data but resume returned nothing (possible embedder failure)
- `no-themes`: Observations exist but no themes (needs consolidation)

The `memory_state` field will be `"healthy"`, `"nominal"`, or `"degraded"` based on anomaly findings.

---

### Recall (Semantic Query)

**Endpoint**: `POST /v1/conversation/recall`

Unlike resume (which returns the most relevant recent context), recall answers specific questions by searching across all stored knowledge using vector similarity.

```bash
curl -s -X POST http://localhost:9999/v1/conversation/recall \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "query": "What authentication method does the API use?",
    "top_k": 5,
    "include_themes": true,
    "include_concepts": true
  }' | jq .
```

**Response**:

```json
{
  "space_id": "my-project",
  "query": "What authentication method does the API use?",
  "results": [
    {
      "type": "conversation_observation",
      "node_id": "...",
      "content": "The API uses JWT tokens with RS256 signing...",
      "score": 0.89,
      "layer": 0,
      "metadata": {
        "obs_type": "learning",
        "surprise_score": 0.72,
        "session_id": "session-001"
      }
    },
    {
      "type": "conversation_theme",
      "node_id": "...",
      "content": "Authentication and security patterns...",
      "score": 0.75,
      "layer": 1
    }
  ]
}
```

**Temporal filtering**:

```bash
curl -s -X POST http://localhost:9999/v1/conversation/recall \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "query": "recent deployment changes",
    "temporal_after": "2026-03-01T00:00:00Z",
    "temporal_before": "2026-03-10T23:59:59Z",
    "top_k": 10
  }' | jq .
```

**Tag filtering** (useful for finding pinned skill content):

```bash
curl -s -X POST http://localhost:9999/v1/conversation/recall \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "query": "coding conventions",
    "filter_tags": ["coding-style"]
  }' | jq .
```

---

### Consolidate (Build Hierarchy)

**Endpoint**: `POST /v1/conversation/consolidate`

Consolidation clusters raw observations (L0) into themes (L1), then builds higher-level emergent concepts (L2-L5). This is how MDEMG transforms raw data into structured understanding.

```bash
curl -s -X POST http://localhost:9999/v1/conversation/consolidate \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project"
  }' | jq .
```

**Response**:

```json
{
  "space_id": "my-project",
  "themes_created": 3,
  "concepts_created": 1,
  "duration_ms": 2450
}
```

Consolidation should be run periodically. RSIC can trigger it automatically when it detects stale consolidation (last run > 24 hours ago) or high orphan ratio (> 20% of nodes have no edges).

---

### Graduate (Lifecycle Management)

**Endpoint**: `POST /v1/conversation/graduate`

Graduation manages the lifecycle of volatile observations. New observations start as volatile with a low stability score (0.1). As they are accessed and reinforced, their stability increases. When stability reaches 0.8, they "graduate" to permanent storage.

This endpoint first applies temporal decay to all observations, then processes graduations for those that have earned permanence.

```bash
curl -s -X POST http://localhost:9999/v1/conversation/graduate \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project"
  }' | jq .
```

**Response**:

```json
{
  "space_id": "my-project",
  "graduated": 12,
  "decayed": 5,
  "decay_applied": true
}
```

**Checking volatile stats**:

```bash
curl -s "http://localhost:9999/v1/conversation/volatile/stats?space_id=my-project" | jq .
```

Returns counts of volatile vs. permanent observations and average stability scores.

---

## Session Health and Constraints

### Session Health

**Endpoint**: `GET /v1/conversation/session/health?session_id=X`

Tracks how well CMS is being used during a session:

```bash
curl -s "http://localhost:9999/v1/conversation/session/health?session_id=session-001" | jq .
```

```json
{
  "session_id": "session-001",
  "space_id": "my-project",
  "resumed": true,
  "observations_since_resume": 15,
  "health_score": 0.85,
  "tracked": true,
  "last_resume_at": "2026-03-10T09:00:00Z",
  "last_observe_at": "2026-03-10T10:45:00Z"
}
```

A low health score (< 0.3) with zero observations indicates CMS is not being used. The watchdog may escalate this.

### Session Anomalies

**Endpoint**: `GET /v1/conversation/session/anomalies?session_id=X&space_id=Y`

Aggregated anomaly summary including watchdog state:

```bash
curl -s "http://localhost:9999/v1/conversation/session/anomalies?session_id=session-001&space_id=my-project" | jq .
```

### Constraints

**List constraints**: `GET /v1/constraints?space_id=X`

```bash
curl -s "http://localhost:9999/v1/constraints?space_id=my-project" | jq .
```

Returns constraint nodes automatically detected from observations containing language like "NEVER", "ALWAYS", "MUST". Each constraint has a `constraint_type`, `confidence`, and count of source observations.

**Constraint stats**: `GET /v1/constraints/stats?space_id=X`

```bash
curl -s "http://localhost:9999/v1/constraints/stats?space_id=my-project" | jq .
```

### Guardrail Validation

**Endpoint**: `POST /v1/memory/guardrail/validate`

Validates code changes against learned constraints (Phase 104):

```bash
curl -s -X POST http://localhost:9999/v1/memory/guardrail/validate \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "files_changed": ["internal/config/config.go"],
    "diff": "- password := \"hardcoded\"\n+ password := os.Getenv(\"DB_PASSWORD\")"
  }' | jq .
```

Returns `status: "pass"`, `"warn"`, or `"fail"` with violation details.

### Learning Distribution

**Endpoint**: `GET /v1/memory/distribution?space_id=X`

Shows the learning phase and edge statistics:

```bash
curl -s "http://localhost:9999/v1/memory/distribution?space_id=my-project" | jq '{phase: .stats.phase, edges: .stats.edge_count, alerts: .stats.alerts}'
```

<details>
<summary>PowerShell equivalent</summary>

```powershell
(Invoke-RestMethod -Uri "http://localhost:9999/v1/memory/distribution?space_id=my-project").stats | Select-Object phase, edge_count, alerts
```
</details>

**Learning phases**:
- `cold` (0 edges): No learning data yet
- `learning` (1-10k edges): Actively building connections
- `warm` (10k-50k edges): Healthy, well-connected graph
- `saturated` (50k+ edges): Too many edges -- consider pruning

### Skills Registry

Skills are stored as pinned CMS observations with `skill:<name>` tags.

**List skills**: `GET /v1/skills?space_id=X`

```bash
curl -s "http://localhost:9999/v1/skills?space_id=my-project" | jq .
```

**Register a skill**: `POST /v1/skills/{name}/register`

```bash
curl -s -X POST http://localhost:9999/v1/skills/code-review/register \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "description": "Code review guidelines",
    "sections": [
      {
        "name": "style",
        "content": "Use gofmt, 100-char line limit, no global state.",
        "tags": ["golang"]
      },
      {
        "name": "security",
        "content": "Never hardcode secrets. Always use environment variables or keyring.",
        "tags": ["security"]
      }
    ]
  }' | jq .
```

<details>
<summary>PowerShell equivalent</summary>

```powershell
$body = @{
    space_id    = "my-project"
    description = "Code review guidelines"
    sections    = @(
        @{
            name    = "style"
            content = "Use gofmt, 100-char line limit, no global state."
            tags    = @("golang")
        },
        @{
            name    = "security"
            content = "Never hardcode secrets. Always use environment variables or keyring."
            tags    = @("security")
        }
    )
} | ConvertTo-Json -Depth 3
Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/skills/code-review/register" -Body $body -ContentType "application/json"
```
</details>

**Recall a skill**: `POST /v1/skills/{name}/recall`

```bash
curl -s -X POST http://localhost:9999/v1/skills/code-review/recall \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "section": "security"
  }' | jq .
```

---

## What is RSIC?

The **Recursive Self-Improvement Cycle** is MDEMG's automated memory maintenance system. It continuously monitors memory health and takes corrective action when problems are detected. Think of it as a background process that keeps the knowledge graph clean, connected, and useful.

RSIC runs a **5-stage pipeline**:

```
Assess --> Reflect --> Plan --> Execute --> Validate/Calibrate
```

1. **Assess**: Gather health metrics from the graph (orphan ratio, edge entropy, consolidation freshness, learning phase, volatile backlog)
2. **Reflect**: Analyze the assessment and produce actionable insights (e.g., "30% of edges are below threshold -- prune needed")
3. **Plan**: Convert insights into concrete task specifications with safety bounds
4. **Execute**: Dispatch tasks (prune edges, run consolidation, graduate volatiles, tombstone stale nodes)
5. **Validate/Calibrate**: Check success criteria, update per-action confidence scores

### Tiers

RSIC operates at three granularity tiers:

| Tier | Trigger | Timeout | Purpose |
|------|---------|---------|---------|
| `micro` | Per-observation (automatic) | 30 sec | Lightweight opportunistic maintenance |
| `meso` | Per-session or periodic | 10 min | Regular session-level maintenance |
| `macro` | Scheduled | 30 min | Deep maintenance and restructuring |

### Trigger Sources

| Source | Priority | Allowed Tiers | How It Fires |
|--------|----------|---------------|--------------|
| `watchdog_force` | 0 (highest) | meso | Watchdog decay threshold exceeded |
| `manual_api` | 1 | micro, meso, macro | Direct API call |
| `macro_cron` | 2 | macro | Scheduled task |
| `session_periodic` | 3 | meso | Every N sessions via resume |
| `micro_auto` | 4 (lowest) | micro | After each observation |

### Automatic Actions

RSIC can execute these actions based on reflection insights:

| Action | Trigger Condition | What It Does |
|--------|-------------------|--------------|
| `prune_decayed_edges` | Saturated learning phase, or >30% weak edges | Removes edges with weight below decay threshold |
| `prune_excess_edges` | Low edge weight entropy | Removes excess edges per node to prevent over-connectivity |
| `trigger_consolidation` | >20% orphan ratio, or consolidation >24h stale | Runs full conversation consolidation |
| `graduate_volatile` | >100 volatile observations | Promotes stable volatile observations to permanent |
| `tombstone_stale` | >15% correction rate | Archives observations superseded by corrections |
| `refresh_stale_edges` | >100 edges with average weight <0.2 | Re-activates stale learning edges |

### Safety Enforcement

All RSIC actions pass through a safety validator:
- **Graph-relative bounds**: Max nodes/edges affected computed as a percentage of actual graph size (`RSIC_MAX_NODE_PRUNE_PCT`, `RSIC_MAX_EDGE_PRUNE_PCT`), with a floor of 10
- **Configurable protected spaces**: `RSIC_PROTECTED_SPACES` (comma-separated, default: empty). When empty, RSIC can operate on all spaces including `mdemg-dev`. Set to block destructive actions on specific spaces.
- **Pre-mutation snapshots**: Before any destructive action, a snapshot is captured for potential rollback
- **Dry-run mode**: Preview what actions would do without executing
- **Calibration-aware planner**: Actions with historical success rate below `RSIC_MIN_ACTION_CONFIDENCE` (default: 0.2) are suppressed unless severity is critical
- **Diagnostic classification**: Alert-only actions are excluded from calibration success/failure tracking

---

## RSIC Workflow

### Assess (System Health Scoring)

**Endpoint**: `POST /v1/self-improve/assess`

Run just the assessment stage to see current memory health without taking action:

```bash
curl -s -X POST http://localhost:9999/v1/self-improve/assess \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "tier": "meso"
  }' | jq .
```

**Response**:

```json
{
  "space_id": "my-project",
  "tier": "meso",
  "timestamp": "2026-03-10T10:00:00Z",
  "retrieval_quality": 0.9,
  "task_performance": 0.65,
  "memory_health": 0.8,
  "edge_health": 0.7,
  "overall_health": 0.78,
  "confidence": 0.75,
  "learning_phase": "warm",
  "edge_count": 25000,
  "orphan_count": 120,
  "total_nodes": 1500,
  "orphan_ratio": 0.08,
  "correction_rate": 0.05,
  "consolidation_age_sec": 43200,
  "volatile_count": 45,
  "permanent_count": 1200,
  "avg_edge_weight": 0.45,
  "edges_below_threshold": 3000,
  "edge_weight_entropy": 0.72
}
```

**Sub-score calculation**:

- **Overall Health** = 0.30 x retrieval_quality + 0.25 x memory_health + 0.25 x edge_health + 0.20 x task_performance
- **Retrieval Quality**: Based on learning phase (cold=0.3, learning=0.6, warm=0.9, saturated=0.7)
- **Memory Health**: Starts at 1.0, penalized by high orphan ratio (>10%: -0.1, >20%: -0.3), high correction rate (>15%: -0.2), stale consolidation (>24h: -0.2)
- **Edge Health**: Starts at 1.0, penalized by high ratio of weak edges (>30%: -0.3) and low entropy (<0.5: -0.2)
- **Task Performance**: Ratio of permanent to total observations (proxy for knowledge maturity)
- **Confidence**: Based on data availability (0-4 data points, mapped to 0.1-1.0). Low confidence (<`RSIC_MIN_CONFIDENCE`) causes the cycle to bail early.

---

### Run a Full Cycle

**Endpoint**: `POST /v1/self-improve/cycle`

Run the complete Assess -> Reflect -> Plan -> Execute -> Validate pipeline:

```bash
curl -s -X POST http://localhost:9999/v1/self-improve/cycle \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "tier": "meso"
  }' | jq .
```

**Response**:

```json
{
  "cycle_id": "rsic-meso-a1b2c3d4",
  "tier": "meso",
  "space_id": "my-project",
  "started_at": "2026-03-10T10:00:00Z",
  "completed_at": "2026-03-10T10:00:45Z",
  "actions_executed": 2,
  "success_count": 2,
  "failed_count": 0,
  "metrics_before": {
    "overall_health": 0.65,
    "edge_count": 25000,
    "orphan_ratio": 0.22,
    "volatile_count": 150,
    "correction_rate": 0.05,
    "edge_entropy": 0.45
  },
  "metrics_after": {
    "overall_health": 0.78,
    "retrieval_quality": 0.75,
    "memory_health": 1.0,
    "edge_health": 0.82,
    "orphan_ratio": 0.15,
    "correction_rate": 0.04,
    "edge_weight_entropy": 0.52
  },
  "criteria_met": true,
  "criteria_detail": {
    "orphan_ratio_delta": "met",
    "overall_health_delta": "met"
  },
  "insights": [
    {
      "pattern_id": "high_orphan_ratio",
      "severity": "medium",
      "description": "More than 20% of nodes are orphaned...",
      "recommended_action": "trigger_consolidation",
      "metric": "orphan_ratio",
      "value": 0.22,
      "threshold": 0.2
    }
  ],
  "trigger_source": "manual_api",
  "policy_version": "phase87-v1",
  "safety_version": "phase88-v1",
  "safety_summary": {
    "actions_checked": 2,
    "actions_allowed": 2,
    "actions_rejected": 0,
    "snapshots_created": 2
  }
}
```

**Dry-run mode** (preview without executing):

```bash
curl -s -X POST http://localhost:9999/v1/self-improve/cycle \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "tier": "meso",
    "dry_run": true
  }' | jq .
```

The dry-run response includes `deltas` showing what each action would do:

```json
{
  "dry_run": true,
  "deltas": [
    {
      "action": "prune_decayed_edges",
      "would_execute": true,
      "estimated_affected": 350,
      "safety_limit": 100,
      "within_bounds": false,
      "protected_space_blocked": false,
      "rejection_reason": "estimated 350 exceeds limit 100"
    }
  ]
}
```

**Idempotency** (prevent duplicate cycles):

```bash
curl -s -X POST http://localhost:9999/v1/self-improve/cycle \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "tier": "meso",
    "idempotency_key": "daily-meso-2026-03-10"
  }' | jq .
```

If the same `idempotency_key` is sent again, the response returns the original cycle ID with a `dedupe` field.

**Trigger source** (for audit trail):

```bash
curl -s -X POST http://localhost:9999/v1/self-improve/cycle \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "tier": "macro",
    "trigger_source": "macro_cron"
  }' | jq .
```

Valid trigger sources: `manual_api`, `micro_auto`, `session_periodic`, `macro_cron`, `watchdog_force`.

---

### Monitor Health

**Endpoint**: `GET /v1/self-improve/health`

Returns the full RSIC health status including watchdog, orchestration, persistence, and safety information:

```bash
curl -s http://localhost:9999/v1/self-improve/health | jq .
```

```json
{
  "status": "ok",
  "active_tasks": 0,
  "watchdog": {
    "decay_score": 2.5,
    "escalation_level": 1,
    "last_cycle_time": "2026-03-10T08:00:00Z",
    "next_due": "2026-03-10T14:00:00Z",
    "space_id": "my-project",
    "session_health_score": 0.75,
    "obs_rate_per_hour": 3.2,
    "consolidation_age_sec": 21600
  },
  "orchestration": {
    "concurrent_cycles": {},
    "session_counters": {},
    "recent_triggers": []
  },
  "persistence": {
    "enabled": true,
    "space_id": "my-project"
  },
  "safety": {
    "enforcement_active": true,
    "safety_version": "phase88-v1",
    "bounds": {
      "max_nodes_affected": 100,
      "max_edges_affected": 200,
      "protected_spaces": ["mdemg-dev"]
    },
    "rollback": {
      "window_sec": 3600,
      "snapshots_held": 5,
      "oldest_snapshot_age_sec": 1800
    }
  }
}
```

**Watchdog escalation levels**:

| Level | Name | Meaning |
|-------|------|---------|
| 0 | Nominal | Everything is fine |
| 1 | Nudge | Gentle reminder -- decay score above nudge threshold |
| 2 | Warn | Warning -- decay score above warn threshold |
| 3 | Force | Auto-triggers a meso cycle (watchdog_force trigger) |

The watchdog's `decay_score` increases at `RSIC_WATCHDOG_DECAY_RATE` per hour since the last cycle.

**Active tasks and reports**:

```bash
# List active tasks
curl -s http://localhost:9999/v1/self-improve/report | jq .

# Get reports for a specific task
curl -s http://localhost:9999/v1/self-improve/report/rsic-meso-a1b2c3d4-task-0 | jq .
```

**Signal effectiveness** (Phase 80):

```bash
curl -s http://localhost:9999/v1/self-improve/signals | jq .
```

---

### History and Calibration

**Endpoint**: `GET /v1/self-improve/history`

View past RSIC cycle outcomes:

```bash
# Last 10 cycles (default)
curl -s "http://localhost:9999/v1/self-improve/history" | jq .

# Last 20 cycles
curl -s "http://localhost:9999/v1/self-improve/history?limit=20" | jq .

# Filter by tier and trigger source
curl -s "http://localhost:9999/v1/self-improve/history?tier=meso&trigger_source=manual_api" | jq .

# Filter by space
curl -s "http://localhost:9999/v1/self-improve/history?space_id=my-project&limit=5" | jq .
```

**Endpoint**: `GET /v1/self-improve/calibration`

View per-action confidence scores (success rate over history):

```bash
curl -s http://localhost:9999/v1/self-improve/calibration | jq .
```

```json
{
  "calibration": {
    "prune_decayed_edges": 0.85,
    "trigger_consolidation": 0.92,
    "graduate_volatile": 1.0,
    "tombstone_stale": 0.75
  }
}
```

Calibration scores are updated after every cycle. An action with a low confidence score may indicate it is failing frequently, which the Reflect stage can use to adjust future recommendations.

---

### Learning Freeze

**Freeze**: `POST /v1/learning/freeze`

Temporarily stop creating new Hebbian learning edges. Useful during benchmarking or stable scoring periods.

```bash
curl -s -X POST http://localhost:9999/v1/learning/freeze \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "reason": "stable scoring during benchmark",
    "frozen_by": "operator"
  }' | jq .
```

**Unfreeze**: `POST /v1/learning/unfreeze`

```bash
curl -s -X POST http://localhost:9999/v1/learning/unfreeze \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project"
  }' | jq .
```

**Check freeze status**: `GET /v1/learning/freeze/status`

```bash
# Specific space
curl -s "http://localhost:9999/v1/learning/freeze/status?space_id=my-project" | jq .

# All spaces
curl -s "http://localhost:9999/v1/learning/freeze/status" | jq .
```

---

### Rollback

**List snapshots**: `GET /v1/self-improve/rollback`

Before RSIC executes destructive actions (edge pruning, tombstoning), it captures a snapshot. Snapshots can be used to undo an action within the rollback window (default: 3600 seconds).

```bash
curl -s http://localhost:9999/v1/self-improve/rollback | jq .
```

```json
{
  "snapshots": [
    {
      "snapshot_id": "snap-abc123",
      "cycle_id": "rsic-meso-a1b2c3d4",
      "action_type": "prune_decayed_edges",
      "space_id": "my-project",
      "affected_count": 150,
      "created_at": "2026-03-10T10:00:30Z",
      "expires_at": "2026-03-10T11:00:30Z"
    }
  ],
  "count": 1,
  "rollback_window_sec": 3600
}
```

**Execute rollback**: `POST /v1/self-improve/rollback`

```bash
curl -s -X POST http://localhost:9999/v1/self-improve/rollback \
  -H "Content-Type: application/json" \
  -d '{
    "snapshot_id": "snap-abc123"
  }' | jq .
```

Rollback restores the pre-action state. It will fail if the snapshot has expired (beyond the rollback window).

---

## Jiminy Inner-Voice Guidance

Jiminy is MDEMG's proactive guidance service -- an "inner voice" for AI agents that surfaces constraints, prior corrections, contradictions, and frontier knowledge *before* the agent acts. Instead of relying on the agent to recall the right context, Jiminy pushes relevant warnings into the conversation automatically.

### How It Works

Jiminy fans out to four parallel knowledge sources:

1. **Constraints** -- learned rules from the consulting service (e.g., "never hardcode secrets")
2. **Corrections** -- prior mistakes found via vector search against correction-type observations
3. **Contradictions** -- CONTRADICTS edges near the current context
4. **Frontiers** -- unexplored or under-explored concepts that may be relevant

Results are merged, deduplicated by content similarity, sorted by priority and confidence, and returned as a ranked list of guidance items.

### Standalone API

**Endpoint**: `POST /v1/jiminy/guide`

```bash
curl -s -X POST http://localhost:9999/v1/jiminy/guide \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-project",
    "context": "Refactoring the auth middleware to support OAuth2",
    "session_id": "dev-session-01"
  }' | jq .
```

<details>
<summary>PowerShell equivalent</summary>

```powershell
$body = @{
    space_id   = "my-project"
    context    = "Refactoring the auth middleware to support OAuth2"
    session_id = "dev-session-01"
} | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri "http://localhost:9999/v1/jiminy/guide" -Body $body -ContentType "application/json"
```
</details>

**Response**:

```json
{
  "data": {
    "guidance": [
      {
        "type": "correction",
        "priority": "high",
        "content": "Previous session noted: always validate token expiry server-side, not client-side",
        "confidence": 0.9,
        "source_nodes": ["obs-correction-abc"]
      },
      {
        "type": "frontier",
        "priority": "low",
        "content": "OAuth2 refresh token rotation is a known knowledge gap",
        "confidence": 0.6,
        "source_nodes": ["hidden-frontier-xyz"]
      }
    ],
    "prompt_augmentation": "Based on prior sessions: (1) Validate token expiry server-side...",
    "confidence": 0.78,
    "rationale": "Found 1 correction and 1 frontier related to auth middleware",
    "source_counts": {"constraints": 0, "corrections": 1, "patterns": 0, "conflicts": 0, "frontiers": 1}
  }
}
```

If `JIMINY_ENABLED=false`, the server returns `503 Service Unavailable`.

### Hook Integration

In practice, you rarely call Jiminy directly. The `prompt-context.sh` hook calls `POST /v1/jiminy/guide` automatically on every user prompt, injecting a `=== JIMINY GUIDANCE ===` block into the system context. This means the agent receives relevant warnings, constraints, and corrections without any manual intervention.

The hook passes the user's prompt as `context` and the active session ID, so guidance is always scoped to what the agent is about to do.

To install Claude Code hooks (which include Jiminy integration):

```bash
# Install Claude hooks (includes Jiminy prompt injection)
mdemg hooks install --type claude --space-id my-project

# Or install all hook types at once
mdemg hooks install --type all --space-id my-project

# Verify hooks are installed
mdemg hooks list
```

### Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `JIMINY_ENABLED` | `false` | Must be set to `true` to enable Jiminy. When disabled, the endpoint returns 503 and hooks skip guidance injection. |
| `JIMINY_MAX_ITEMS` | `10` | Maximum guidance items per request |
| `JIMINY_TIMEOUT_MS` | `6000` | Timeout for guidance generation (ms). Jiminy queries four sources in parallel; this caps total wall time. |
| `JIMINY_MIN_CONFIDENCE` | `0.3` | Minimum confidence threshold for inclusion |
| `JIMINY_INCLUDE_FRONTIERS` | `true` | Include frontier suggestions in guidance |
| `JIMINY_FRONTIER_MIN_SIM` | `0.5` | Minimum similarity score for frontier matches |

### Relationship to CMS

Jiminy reads from the same knowledge graph that CMS writes to. Corrections captured via `POST /v1/conversation/observe` (with `obs_type: "correction"`) are the primary source for Jiminy's correction guidance. Constraints detected by the consulting service feed into Jiminy's constraint guidance. The `jiminy` field in resume responses reflects the Jiminy health score for the session.

---

## Practical Examples

### Setting Up CMS for a New AI Agent

**Step 1: Start MDEMG**

#### macOS / Linux

```bash
# Start Neo4j
cd /path/to/mdemg && docker compose up -d neo4j

# Build and start the server
go build -o bin/mdemg ./cmd/mdemg
./bin/mdemg serve --auto-migrate
```

#### Windows

```powershell
# Start Neo4j
cd C:\path\to\mdemg
docker compose up -d neo4j

# Build and start the server
go build -o bin\mdemg.exe .\cmd\mdemg
.\bin\mdemg.exe serve --auto-migrate
```

**Step 2: Verify the server is running**

```bash
curl -s http://localhost:9999/healthz | jq .
# Should return: {"status": "ok", "version": "..."}
```

**Step 3: Initialize a space with a first observation**

```bash
curl -s -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-new-project",
    "session_id": "init-session",
    "content": "This project uses Go 1.22, PostgreSQL 16, and Redis 7. The API follows REST conventions with JSON payloads.",
    "obs_type": "context",
    "pinned": true
  }' | jq .
```

**Step 4: Register a skill with project conventions**

```bash
curl -s -X POST http://localhost:9999/v1/skills/project-conventions/register \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-new-project",
    "sections": [
      {
        "name": "coding-style",
        "content": "Use gofmt. Max line length 120. Prefer table-driven tests. Always handle errors explicitly -- no _ = err.",
        "tags": ["golang", "style"]
      },
      {
        "name": "git-workflow",
        "content": "Branch from main. Conventional commits (feat:, fix:, docs:). Squash merge to main.",
        "tags": ["git"]
      }
    ]
  }' | jq .
```

**Step 5: Begin each session with resume**

```bash
curl -s -X POST http://localhost:9999/v1/conversation/resume \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-new-project",
    "session_id": "work-session-1",
    "max_observations": 10
  }' | jq .
```

**Step 6: Observe continuously during the session**

```bash
# After a key decision
curl -s -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-new-project",
    "session_id": "work-session-1",
    "content": "Chose Chi router over Gorilla mux because Chi has better middleware chaining and is actively maintained.",
    "obs_type": "decision"
  }' | jq .

# After learning something new
curl -s -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-new-project",
    "session_id": "work-session-1",
    "content": "The database connection pool should be configured with max_open_conns=25, max_idle_conns=5 based on the load test results.",
    "obs_type": "learning",
    "tags": ["database", "performance"]
  }' | jq .
```

---

### Daily Maintenance Workflow

Run this sequence once per day (or automate it -- see [Running RSIC on a Schedule](#running-rsic-on-a-schedule)):

```bash
SPACE="my-project"
BASE="http://localhost:9999"

# 1. Check current health
echo "=== Health Assessment ==="
curl -s -X POST "$BASE/v1/self-improve/assess" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\": \"$SPACE\", \"tier\": \"meso\"}" | jq '{overall: .overall_health, confidence: .confidence, phase: .learning_phase, orphan_ratio: .orphan_ratio}'

# 2. Run consolidation if stale
echo "=== Consolidation ==="
curl -s -X POST "$BASE/v1/conversation/consolidate" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\": \"$SPACE\"}" | jq .

# 3. Graduate volatile observations
echo "=== Graduation ==="
curl -s -X POST "$BASE/v1/conversation/graduate" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\": \"$SPACE\"}" | jq .

# 4. Run a dry-run RSIC cycle to preview maintenance actions
echo "=== RSIC Dry Run ==="
curl -s -X POST "$BASE/v1/self-improve/cycle" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\": \"$SPACE\", \"tier\": \"meso\", \"dry_run\": true}" | jq '{insights: [.insights[].pattern_id], deltas: [.deltas[] | {action, would_execute, estimated_affected}]}'

# 5. If dry run looks safe, run the real cycle
echo "=== RSIC Execute ==="
curl -s -X POST "$BASE/v1/self-improve/cycle" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\": \"$SPACE\", \"tier\": \"meso\", \"trigger_source\": \"macro_cron\", \"idempotency_key\": \"daily-$(date +%Y-%m-%d)\"}" | jq '{cycle_id, actions_executed, success_count, failed_count}'

# 6. Check learning distribution health
echo "=== Learning Health ==="
curl -s "$BASE/v1/memory/distribution?space_id=$SPACE" | jq '{phase: .stats.phase, edges: .stats.edge_count, alerts: .stats.alerts}'
```

<details>
<summary>PowerShell equivalent</summary>

```powershell
$Space = "my-project"
$Base = "http://localhost:9999"

# 1. Check current health
Write-Host "=== Health Assessment ==="
$assess = Invoke-RestMethod -Method Post -Uri "$Base/v1/self-improve/assess" `
    -Body (@{ space_id = $Space; tier = "meso" } | ConvertTo-Json) `
    -ContentType "application/json"
$assess | Select-Object overall_health, confidence, learning_phase, orphan_ratio

# 2. Run consolidation if stale
Write-Host "=== Consolidation ==="
Invoke-RestMethod -Method Post -Uri "$Base/v1/conversation/consolidate" `
    -Body (@{ space_id = $Space } | ConvertTo-Json) `
    -ContentType "application/json"

# 3. Graduate volatile observations
Write-Host "=== Graduation ==="
Invoke-RestMethod -Method Post -Uri "$Base/v1/conversation/graduate" `
    -Body (@{ space_id = $Space } | ConvertTo-Json) `
    -ContentType "application/json"

# 4. Run a dry-run RSIC cycle to preview maintenance actions
Write-Host "=== RSIC Dry Run ==="
$dryRun = Invoke-RestMethod -Method Post -Uri "$Base/v1/self-improve/cycle" `
    -Body (@{ space_id = $Space; tier = "meso"; dry_run = $true } | ConvertTo-Json) `
    -ContentType "application/json"
$dryRun.insights | ForEach-Object { $_.pattern_id }
$dryRun.deltas | Select-Object action, would_execute, estimated_affected

# 5. If dry run looks safe, run the real cycle
Write-Host "=== RSIC Execute ==="
$today = Get-Date -Format "yyyy-MM-dd"
Invoke-RestMethod -Method Post -Uri "$Base/v1/self-improve/cycle" `
    -Body (@{ space_id = $Space; tier = "meso"; trigger_source = "macro_cron"; idempotency_key = "daily-$today" } | ConvertTo-Json) `
    -ContentType "application/json" | Select-Object cycle_id, actions_executed, success_count, failed_count

# 6. Check learning distribution health
Write-Host "=== Learning Health ==="
(Invoke-RestMethod -Uri "$Base/v1/memory/distribution?space_id=$Space").stats | Select-Object phase, edge_count, alerts
```
</details>

---

### Debugging Poor Retrieval Quality

When recall returns irrelevant or empty results:

**Step 1: Check the basics**

```bash
SPACE="my-project"
BASE="http://localhost:9999"

# Is the embedding provider working?
curl -s "$BASE/v1/embedding/health" | jq .

# How many observations exist?
curl -s "$BASE/v1/memory/stats?space_id=$SPACE" | jq .

# What is the learning phase?
curl -s "$BASE/v1/memory/distribution?space_id=$SPACE" | jq '{phase: .stats.phase, edges: .stats.edge_count}'
```

**Step 2: Run a health assessment**

```bash
curl -s -X POST "$BASE/v1/self-improve/assess" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\": \"$SPACE\", \"tier\": \"meso\"}" | jq '{
    overall: .overall_health,
    retrieval: .retrieval_quality,
    memory: .memory_health,
    edge: .edge_health,
    orphan_ratio: .orphan_ratio,
    consolidation_age_hours: (.consolidation_age_sec / 3600),
    volatile_count: .volatile_count,
    learning_phase: .learning_phase
  }'
```

**Step 3: Diagnose by assessment results**

| Symptom | Likely Cause | Fix |
|---------|-------------|-----|
| `orphan_ratio > 0.2` | Many nodes have no edges | Run consolidation |
| `consolidation_age_sec > 86400` | Stale themes/concepts | Run consolidation |
| `learning_phase: "cold"` | No learning edges | Make more observations, wait for co-activation |
| `learning_phase: "saturated"` | Too many edges diluting signals | Prune decayed edges |
| `edge_weight_entropy < 0.5` | Edge weights clustered at extremes | Prune excess edges |
| `volatile_count > 100` | Backlog of ungraduated observations | Run graduation |

**Step 4: Fix it**

```bash
# Run consolidation
curl -s -X POST "$BASE/v1/conversation/consolidate" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\": \"$SPACE\"}" | jq .

# Or let RSIC handle it
curl -s -X POST "$BASE/v1/self-improve/cycle" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\": \"$SPACE\", \"tier\": \"meso\"}" | jq '{cycle_id, actions_executed, insights: [.insights[].pattern_id]}'
```

**Step 5: Check for freeze state**

```bash
curl -s "http://localhost:9999/v1/learning/freeze/status?space_id=$SPACE" | jq .
```

If learning is frozen, new edges are not being created, which could degrade retrieval over time. Unfreeze when stable scoring is no longer needed.

**Step 6: Verify improvement**

```bash
# Re-run the assessment
curl -s -X POST "$BASE/v1/self-improve/assess" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\": \"$SPACE\", \"tier\": \"meso\"}" | jq '{overall: .overall_health, retrieval: .retrieval_quality}'

# Test recall again
curl -s -X POST "$BASE/v1/conversation/recall" \
  -H "Content-Type: application/json" \
  -d "{\"space_id\": \"$SPACE\", \"query\": \"your test query here\", \"top_k\": 5}" | jq '.results | length'
```

---

### Running RSIC on a Schedule

#### Option 1: Cron job (macOS / Linux)

Add to your crontab (`crontab -e`):

```cron
# Run meso RSIC cycle every 6 hours
0 */6 * * * curl -s -X POST http://localhost:9999/v1/self-improve/cycle -H "Content-Type: application/json" -d '{"space_id":"my-project","tier":"meso","trigger_source":"macro_cron","idempotency_key":"meso-'$(date +\%Y\%m\%d-\%H)'"}'

# Run macro RSIC cycle daily at 2 AM
0 2 * * * curl -s -X POST http://localhost:9999/v1/self-improve/cycle -H "Content-Type: application/json" -d '{"space_id":"my-project","tier":"macro","trigger_source":"macro_cron","idempotency_key":"macro-'$(date +\%Y\%m\%d)'"}'

# Run consolidation daily at 3 AM
0 3 * * * curl -s -X POST http://localhost:9999/v1/conversation/consolidate -H "Content-Type: application/json" -d '{"space_id":"my-project"}'

# Run graduation daily at 3:30 AM
30 3 * * * curl -s -X POST http://localhost:9999/v1/conversation/graduate -H "Content-Type: application/json" -d '{"space_id":"my-project"}'
```

#### Option 2: Systemd timer (Linux)

MDEMG ships with systemd unit files for scheduled RSIC cycles:

```bash
# Install the systemd units (if not already installed by install.sh)
sudo cp systemd/mdemg-rsic.service /etc/systemd/system/mdemg-rsic@.service
sudo cp systemd/mdemg-rsic.timer /etc/systemd/system/mdemg-rsic@.timer
sudo systemctl daemon-reload

# Enable the RSIC timer (runs daily at 3:00 AM with +/-5min jitter)
sudo systemctl enable --now mdemg-rsic@$USER.timer

# Check timer status
systemctl list-timers | grep mdemg

# View RSIC cycle logs
journalctl -u mdemg-rsic@$USER

# Disable the timer
sudo systemctl disable --now mdemg-rsic@$USER.timer
```

To customize the schedule, override the timer:

```bash
sudo systemctl edit mdemg-rsic@$USER.timer
```

Add your custom schedule:

```ini
[Timer]
OnCalendar=
OnCalendar=*-*-* */6:00:00
```

This runs RSIC every 6 hours instead of daily.

#### Option 3: Windows Task Scheduler

```powershell
# Run meso RSIC cycle every 6 hours
$mesoAction = New-ScheduledTaskAction -Execute "powershell.exe" -Argument '-NoProfile -Command "Invoke-RestMethod -Method Post -Uri http://localhost:9999/v1/self-improve/cycle -Body (@{space_id=\"my-project\";tier=\"meso\";trigger_source=\"macro_cron\";idempotency_key=\"meso-$(Get-Date -Format yyyyMMdd-HH)\"} | ConvertTo-Json) -ContentType application/json"'
$mesoTrigger = New-ScheduledTaskTrigger -Once -At (Get-Date).Date -RepetitionInterval (New-TimeSpan -Hours 6)
Register-ScheduledTask -TaskName "MDEMG-RSIC-Meso" -Action $mesoAction -Trigger $mesoTrigger -Description "MDEMG meso RSIC cycle every 6 hours"

# Run macro RSIC cycle daily at 2 AM
$macroAction = New-ScheduledTaskAction -Execute "powershell.exe" -Argument '-NoProfile -Command "Invoke-RestMethod -Method Post -Uri http://localhost:9999/v1/self-improve/cycle -Body (@{space_id=\"my-project\";tier=\"macro\";trigger_source=\"macro_cron\";idempotency_key=\"macro-$(Get-Date -Format yyyyMMdd)\"} | ConvertTo-Json) -ContentType application/json"'
$macroTrigger = New-ScheduledTaskTrigger -Daily -At 2am
Register-ScheduledTask -TaskName "MDEMG-RSIC-Macro" -Action $macroAction -Trigger $macroTrigger -Description "MDEMG macro RSIC cycle daily at 2 AM"

# Run consolidation daily at 3 AM
$consolAction = New-ScheduledTaskAction -Execute "powershell.exe" -Argument '-NoProfile -Command "Invoke-RestMethod -Method Post -Uri http://localhost:9999/v1/conversation/consolidate -Body (@{space_id=\"my-project\"} | ConvertTo-Json) -ContentType application/json"'
$consolTrigger = New-ScheduledTaskTrigger -Daily -At 3am
Register-ScheduledTask -TaskName "MDEMG-Consolidation" -Action $consolAction -Trigger $consolTrigger -Description "MDEMG daily consolidation at 3 AM"

# Run graduation daily at 3:30 AM
$gradAction = New-ScheduledTaskAction -Execute "powershell.exe" -Argument '-NoProfile -Command "Invoke-RestMethod -Method Post -Uri http://localhost:9999/v1/conversation/graduate -Body (@{space_id=\"my-project\"} | ConvertTo-Json) -ContentType application/json"'
$gradTrigger = New-ScheduledTaskTrigger -Daily -At 3:30am
Register-ScheduledTask -TaskName "MDEMG-Graduation" -Action $gradAction -Trigger $gradTrigger -Description "MDEMG daily graduation at 3:30 AM"
```

To remove scheduled tasks:

```powershell
Unregister-ScheduledTask -TaskName "MDEMG-RSIC-Meso" -Confirm:$false
Unregister-ScheduledTask -TaskName "MDEMG-RSIC-Macro" -Confirm:$false
Unregister-ScheduledTask -TaskName "MDEMG-Consolidation" -Confirm:$false
Unregister-ScheduledTask -TaskName "MDEMG-Graduation" -Confirm:$false
```

#### Option 4: Rely on automatic triggers

If you enable the built-in triggers, RSIC runs itself:

- **Micro auto** (`RSIC_MICRO_ENABLED=true`): Runs a micro cycle after each observation
- **Session periodic**: Runs a meso cycle every N sessions (configurable via `RSIC_MESO_PERIOD_HOURS`)
- **Watchdog**: If no cycle runs for too long, the watchdog escalates and eventually force-triggers a meso cycle

#### Option 5: Mixed approach (recommended)

Let automatic triggers handle routine maintenance, but schedule daily macro cycles for deep maintenance:

1. Set `RSIC_MICRO_ENABLED=true` for lightweight per-observation maintenance
2. Let session-periodic triggers handle meso cycles naturally
3. Schedule a daily macro cycle via cron, systemd timer, or Windows Task Scheduler for comprehensive cleanup
4. Monitor via `GET /v1/self-improve/health` and respond to watchdog warnings

#### Monitoring script

Save and run periodically as a health check:

```bash
#!/bin/bash
HEALTH=$(curl -s http://localhost:9999/v1/self-improve/health)
ESCALATION=$(echo "$HEALTH" | jq -r '.watchdog.escalation_level // 0')
ACTIVE=$(echo "$HEALTH" | jq -r '.active_tasks // 0')

if [ "$ESCALATION" -ge 2 ]; then
  echo "WARNING: RSIC watchdog at escalation level $ESCALATION"
fi

if [ "$ACTIVE" -gt 0 ]; then
  echo "INFO: $ACTIVE RSIC tasks currently active"
fi

echo "Watchdog decay: $(echo "$HEALTH" | jq '.watchdog.decay_score')"
echo "Snapshots held: $(echo "$HEALTH" | jq '.safety.rollback.snapshots_held')"
```

<details>
<summary>PowerShell equivalent</summary>

```powershell
# monitor-rsic.ps1
$Health = Invoke-RestMethod -Uri "http://localhost:9999/v1/self-improve/health"
$Escalation = if ($Health.watchdog.escalation_level) { $Health.watchdog.escalation_level } else { 0 }
$Active = if ($Health.active_tasks) { $Health.active_tasks } else { 0 }

if ($Escalation -ge 2) {
    Write-Warning "RSIC watchdog at escalation level $Escalation"
}

if ($Active -gt 0) {
    Write-Host "INFO: $Active RSIC tasks currently active"
}

Write-Host "Watchdog decay: $($Health.watchdog.decay_score)"
Write-Host "Snapshots held: $($Health.safety.rollback.snapshots_held)"
```
</details>

---

## Quick Reference: All Endpoints

### CMS Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/v1/conversation/observe` | Capture an observation |
| POST | `/v1/conversation/correct` | Record a correction |
| POST | `/v1/conversation/resume` | Restore session context |
| POST | `/v1/conversation/recall` | Semantic query |
| POST | `/v1/conversation/consolidate` | Build themes/concepts |
| POST | `/v1/conversation/graduate` | Graduate volatile observations |
| GET | `/v1/conversation/volatile/stats` | Volatile observation stats |
| GET | `/v1/conversation/session/health` | Session health score |
| GET | `/v1/conversation/session/anomalies` | Session anomaly summary |
| GET | `/v1/constraints` | List constraint nodes |
| GET | `/v1/constraints/stats` | Constraint statistics |
| POST | `/v1/memory/guardrail/validate` | Validate changes against constraints |
| GET | `/v1/memory/distribution` | Learning phase and edge stats |
| GET | `/v1/skills` | List registered skills |
| POST | `/v1/skills/{name}/register` | Register skill sections |
| POST | `/v1/skills/{name}/recall` | Recall skill content |

### RSIC Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/v1/self-improve/assess` | Run health assessment only |
| POST | `/v1/self-improve/cycle` | Run full RSIC cycle |
| GET | `/v1/self-improve/health` | RSIC health + watchdog + safety status |
| GET | `/v1/self-improve/history` | Past cycle outcomes |
| GET | `/v1/self-improve/calibration` | Per-action confidence scores |
| GET | `/v1/self-improve/report` | Active task statuses |
| GET | `/v1/self-improve/report/{taskID}` | Reports for specific task |
| GET | `/v1/self-improve/signals` | Signal effectiveness tracking |
| GET | `/v1/self-improve/rollback` | List available snapshots |
| POST | `/v1/self-improve/rollback` | Execute a rollback |

### Jiminy Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/v1/jiminy/guide` | Get proactive inner voice guidance for current context |

### Learning Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/v1/learning/freeze` | Freeze learning edge creation |
| POST | `/v1/learning/unfreeze` | Resume learning |
| GET | `/v1/learning/freeze/status` | Check freeze state |
| GET | `/v1/learning/stats` | Learning edge statistics |
| POST | `/v1/learning/prune` | Manually prune edges |

## Interpreting J17 / Jiminy Dashboards

Several panels on `mdemg-j17.json` and `mdemg-jiminy.json` legitimately read zero or stale under normal operation. Before filing an incident, check the list below.

### Expected-zero metrics

| Metric | Why it can legitimately be zero |
|--------|--------------------------------|
| `j17_ticket_restore_success_rate` | J17 tickets are restored only when a prior session lost context and the agent opts to resume it. Most sessions never trigger a restore. With DH-004 (v0.7.1) this metric no longer drags Protocol Health when the event count is zero — `TicketRestoreSuccessRate` defaults to 1.0 and `TicketRestoreTotal` distinguishes "no data" from "100% pass." |
| `j17_replay_frequency_per_hour` | Replay only fires when the cooler detects context drift severe enough to warrant a replay. Stable conversations drive this to 0 by design — the health formula already treats 0 replays as a *good* signal (no penalty). |
| `j17_sidecar_*` (inference counts, NLI bias, etc.) | Zero when `J17_NLI_COMPREHENSION_ENABLED=false` or `J17_SIDECAR_URL` is unreachable. Check `mdemg-neural-sidecar-1` container health first. With DH-004 the sidecar timeout was raised 200 → 1000ms; calls that previously fell back now complete. |
| `mdemg_jiminy_latest_age_ms` | Under `/strict` mode (claude-core session), `prompt-context.sh` calls `/v1/jiminy/reformulate` not `/v1/jiminy/latest`, so this metric stops updating. The panel reading "stale" is expected; it is *not* an alert condition under `/strict`. |
| `rsic_health_task` | Ratio of permanent / (volatile + permanent) conversation observations. Low ratio means volatile observations haven't graduated via the Context Cooler. With DH-004 the session-coactivation path now reinforces stability; a new space will take ~5 reinforcements per observation to graduate. |

### Alerts worth investigating

- **"Jiminy Pipeline Critical" repeating** — with DH-004 (v0.7.1), alert dedup is now TOCTOU-safe via `cooldown.TryRecord()`. If you still see entries closer together than `ALERT_COOLDOWN_SEC` (default 300s), `~/.mdemg/alerts/current.json` keeps up to `ALERT_MAX_ENTRIES` (default 50) historical entries — multiple timestamped rows over an hour+ is expected file content, not duplicate alerts.
- **`openai-*` breaker stuck open** — check `GET /v1/admin/breakers`. A single transient timeout used to trip these; DH-004 adds `LLM_RETRY_DEADLINE_ENABLED=true` so one retry is allowed when budget permits. If a breaker is wedged open after the incident, force-close with:

  ```bash
  curl -X POST http://localhost:9999/v1/admin/breakers/reset \
    -H 'X-API-Key: <your-key>' \
    -d '{"name":"openai-constraint-classify"}'
  ```

- **`j17_nli_mean_bias` > threshold** — if the sidecar is running, confirm `J17_SIDECAR_URL` points at it and `J17_SIDECAR_TIMEOUT_MS` ≥ 1000. If you see a high fallback rate with the feature disabled, DH-004 gates fallback counting on `NLIComprehensionScorer.IsOperational()` — a disabled scorer no longer pollutes the bias metric.
