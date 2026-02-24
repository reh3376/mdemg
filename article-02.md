<!-- markdownlint-disable MD036 MD032 MD027 MD037 -->
<!-- LinkedIn article format — bold pseudo-headings and tight list spacing are intentional -->

# CMS — Conversation Memory System: Persistent Memory for AI Coding Agents

Every 20–30 minutes of active work, an LLM coding agent's context window fills up and compacts. When that happens, everything the agent learned during the session is gone — decisions, corrections, blockers, preferences, all of it. The next session starts from scratch.

The Conversation Memory System (CMS) solves this. It captures significant conversational events as structured observations in a Neo4j graph, then restores the most relevant context when a new session begins.

**Core problems CMS addresses:**

- Context loss on compaction
- Poor context selection (what matters most to restore?)
- Signal vs. noise (not all observations are equally valuable)
- Multi-agent isolation (private vs. shared knowledge)
- Cross-session continuity (work that spans days or weeks)
- Memory degradation over time (edge decay, stale knowledge, orphan accumulation)

CMS isn't passive storage. It actively maintains its own health through the Recursive Self-Improvement Cycle (RSIC) — an autonomous process that monitors memory quality and remediates issues without human intervention.

---

## How It Works

**The Memory Lifecycle**

The lifecycle follows a repeating pattern across sessions:

> **Session 1** → resume → observe → observe → Compaction → Auto-Snapshot
> → RSIC Micro Cycle (quick health pulse)
> → **Session 2** → resume → context restored → observe → Compaction → Auto-Snapshot
> → RSIC Meso Cycle (full assessment + remediation)
> → **Session N** → resume → context restored → continue → Compaction → Auto-Snapshot
> → RSIC Macro Cycle (daily topology optimization + consolidation)

Five stages drive this loop:

1. **Observe** — During a session, significant events are captured: decisions, corrections, learnings, errors, preferences, progress updates
2. **Store** — Each observation gets a semantic embedding, surprise score, and quality assessment, then persists in Neo4j
3. **Resume** — On session start, the system retrieves the most relevant observations (ranked by recency, importance, and task relevance), related themes, and emergent concepts
4. **Reinforce** — Observations accessed together strengthen co-activation edges (Hebbian learning), increasing their future retrieval priority
5. **Self-Improve** — Between sessions, RSIC assesses memory health, reflects on degradation patterns, plans remediation, executes repairs, and validates improvements

---

## Observation Types by Priority

Not all observations are equal. Each type has a default priority that influences how long it persists and how prominently it's restored:

**HIGHEST priority:**
- correction — User corrections ("No, that's wrong...")

**HIGH priority:**
- error — Failures and bugs encountered
- blocker — Unresolved blocking issues
- decision — Architectural and design choices

**MEDIUM priority:**
- preference — User style and workflow preferences
- learning — New knowledge gained during the session
- task — Task tracking and status
- insight — Discoveries and realizations

**LOW priority:**
- progress — General status updates
- context — Background information
- technical_note — Reference documentation

---

## Surprise Detection

Novel observations persist longer. The system detects surprise through:

- **Correction patterns** — User says "No...", "Actually...", "That's wrong"
- **Term novelty** — Domain-specific terms not seen before
- **Embedding distance** — Semantically far from existing observations
- **Contradiction** — Conflicts with previously stored knowledge

---

## Volatile vs. Permanent Memory

New observations start as **volatile** with a stability score of approximately 0.1. Through co-activation reinforcement, stability increases by +0.15 each time the observation is accessed alongside others.

The progression works like this:

> **New observation** (stability ~0.1) → Co-activation reinforcement → stability grows
>
> If stability **exceeds 0.8** → observation **graduates to permanent**
>
> If stability **drops below 0.05** → observation is **tombstoned** (removed from graph)
>
> Stability decays over time without reinforcement (–0.1)

This mimics biological memory consolidation — frequently accessed memories strengthen, unused ones fade.

---

## Resume Relevance Scoring

When restoring context, observations are ranked by three weighted factors:

- **Importance (40%)** — Based on observation type priority + surprise score + stability
- **Recency (30%)** — Exponential decay with a 24-hour half-life
- **Task Relevance (30%)** — Embedding similarity to the current task context

---

## Smart Truncation

Resume responses respect a token budget (default 4,000 tokens). Observations are tiered:

- **Critical (~40%)** — Corrections, errors, decisions (always included)
- **Important (~35%)** — Task context, active learnings
- **Background (~25%)** — Older observations, summarized

If the budget is exceeded, lower-tier observations are summarized or dropped first. Critical observations are never truncated.

---

## Key Features

**Multi-Agent Support (Phase 43C)**

Every operation carries an `agent_id`. Observations have three visibility levels:

- **Private** — only visible to the owning agent
- **Team** — visible to all agents in the same space
- **Global** — organization-wide visibility

Cross-session resume is filtered by agent identity, so each agent gets its own relevant context.

**Structured Observation Templates (Phase 60)**

JSON Schema-validated templates for common patterns:

- `task_handoff` — Current task, status, goals, blockers, next steps
- `decision` — Decision, rationale, alternatives, reversibility
- `error` — Error type, description, resolution, prevention
- `learning` — Topic, insight, confidence, applicability

**Task Context Snapshots (Phase 60)**

Auto-capture full session state before compaction events. Includes active files, blockers, and next steps. Triggered automatically on session end or manually on demand.

**Org-Level Review (Phase 60)**

Valuable observations can be promoted from private to team/global visibility through a review workflow: flag → approve/reject.

**Session Health Monitoring (Phase 43A)**

Tracks whether agents call `/resume` on session start and how actively they observe. Warning headers (`X-MDEMG-Warning: session-not-resumed`) alert when CMS is being underutilized.

**Quality Controls (Phase 43B)**

- Near-duplicate detection (cosine similarity > 0.95 → merge)
- Multi-factor quality scoring (specificity + actionability + context-richness)
- Relevance-weighted resume ranking

---

## Recursive Self-Improvement Cycle — RSIC

CMS memory degrades over time: edges decay, observations go stale, knowledge gaps widen, and consolidation falls behind. RSIC is an autonomous 5-stage cycle that continuously monitors and repairs memory health without human intervention.

**The 5 Stages:**

1. **ASSESS** — Gather metrics from all subsystems (retrieval, learning, conversation, graph)
2. **REFLECT** — Detect degradation patterns across 8 insight types
3. **PLAN** — Generate remediation actions with safety bounds
4. **EXECUTE** — Dispatch background goroutines to perform repairs
5. **VALIDATE** — Check success criteria and update calibration confidence

After validation, the watchdog resets and the outcome is recorded. The cycle then repeats.

**Three Cycle Tiers:**

The tiers escalate in scope and frequency:

> **Micro** (per-session) — Distribution stats, volatile counts, correction rate
>     ↓ escalates to
> **Meso** (every 6 hours or 5 sessions) — Retrieval quality, edge health, knowledge gaps, calibration update
>     ↓ escalates to
> **Macro** (daily cron) — Topology optimization, hidden layer re-consolidation, long-term trend analysis

**Automated Remediation Actions:**

- `prune_decayed_edges` — Remove low-weight learning edges approaching saturation
- `prune_excess_edges` — Trim hub nodes exceeding the per-node edge cap
- `trigger_consolidation` — Run hidden layer consolidation when orphan ratio is high
- `graduate_volatile` — Promote stable volatile observations to permanent
- `tombstone_stale` — Remove observations not accessed in N days with low importance
- `refresh_stale_edges` — Recalculate stale co-activation edges

**Decay Watchdog**

A background goroutine enforces cycle compliance. If the agent fails to run a self-improvement cycle within the configured period, pressure escalates through four levels:

> **Level 0 — Nominal** (decay 0.0–0.3): No action needed
> **Level 1 — Nudge** (decay 0.3–0.6): `rsic_overdue` flag in resume response
> **Level 2 — Warn** (decay 0.6–0.9): `X-MDEMG-Warning` header on all API responses
> **Level 3 — Force** (decay >= 0.9): Auto-dispatch meso cycle

When any cycle completes, the watchdog resets to nominal.

**Calibration and Meta-Learning**

RSIC tracks the historical success rate of each action type. Actions that consistently improve metrics gain higher confidence and are prioritized in future planning. Actions that fail are deprioritized below the minimum confidence threshold (default 0.3).

**Safety Bounds:**

- Max 5% of nodes pruned per cycle
- Max 10% of edges pruned per cycle
- Protected spaces (`mdemg-dev`) are never modified destructively
- All actions are bounded by configurable timeout per tier

---

## API Endpoints

**Core Operations**

- POST `/v1/conversation/observe` — Store an observation
- POST `/v1/conversation/correct` — Store an explicit correction
- POST `/v1/conversation/resume` — Restore context for a session
- POST `/v1/conversation/recall` — Semantic search over observations
- POST `/v1/conversation/consolidate` — Consolidate themes from observations
- GET `/v1/conversation/volatile/stats` — Volatile observation statistics
- POST `/v1/conversation/graduate` — Graduate volatile observations to permanent
- GET `/v1/conversation/session/health` — Session health score

**Templates**

- GET/POST `/v1/conversation/templates` — List or create templates
- GET/PUT/DELETE `/v1/conversation/templates/{id}` — Template CRUD

**Snapshots**

- GET/POST `/v1/conversation/snapshot` — List or create snapshots
- GET `/v1/conversation/snapshot/latest` — Latest snapshot for session
- POST `/v1/conversation/snapshot/cleanup` — Clean up old snapshots
- GET/DELETE `/v1/conversation/snapshot/{id}` — Get or delete snapshot

**Org Reviews**

- GET `/v1/conversation/org-reviews` — List pending reviews
- GET `/v1/conversation/org-reviews/stats` — Review statistics
- POST `/v1/conversation/org-reviews/{id}/decision` — Approve or reject
- POST `/v1/conversation/observations/{id}/flag-org` — Flag for review

**Self-Improvement Cycle (RSIC)**

- POST `/v1/self-improve/assess` — Trigger on-demand self-assessment
- GET `/v1/self-improve/report` — Get active task report
- GET `/v1/self-improve/report/{cycle_id}` — Get specific cycle report
- POST `/v1/self-improve/cycle` — Trigger full RSIC cycle
- GET `/v1/self-improve/history` — Cycle history with outcomes
- GET `/v1/self-improve/calibration` — Calibration metrics and confidence scores
- GET `/v1/self-improve/health` — Watchdog status and health score

**Skill Registry (Phase 48)**

- GET `/v1/skills?space_id=X` — List registered skills
- POST `/v1/skills/{name}/recall` — Recall skill content by tag
- POST `/v1/skills/{name}/register` — Register skill sections as pinned observations

Skills are CMS pinned observations with `skill:<name>` tags. Thin skill files in `.claude/skills/` are pointers that recall from CMS.

**Pinned Observations (Phase 47)**

- POST `/v1/conversation/observe` with `pinned: true` — Permanent, non-decaying observation

Pinned observations bypass volatile graduation: they start permanent with stability 1.0.

---

## Architecture

**Storage (Neo4j)**

Observations are stored as `MemoryNode` nodes in Neo4j with:

- `embedding` (1536-dim vector) for semantic search
- `surprise_score`, `stability_score`, `importance_score` for ranking
- `obs_type`, `visibility`, `agent_id` for filtering
- `volatile` flag for graduation lifecycle
- Co-activation edges (`CO_ACTIVATED_WITH`) for Hebbian reinforcement

**Key Implementation Packages**

- `internal/conversation/service.go` — Core CMS service (observe, resume, recall)
- `internal/conversation/cooler.go` — ContextCooler for volatile-to-permanent graduation
- `internal/conversation/quality.go` — Observation quality scoring
- `internal/conversation/dedup.go` — Near-duplicate detection
- `internal/conversation/relevance.go` — Resume relevance scoring
- `internal/conversation/truncation.go` — Smart truncation with token budgets
- `internal/conversation/templates.go` — Structured observation templates
- `internal/conversation/snapshot.go` — Task context snapshots
- `internal/conversation/org_review.go` — Org-level review workflow
- `internal/conversation/session_tracker.go` — Session health monitoring
- `internal/ape/self_assess.go` — RSIC assessment (gathers metrics)
- `internal/ape/self_reflect.go` — RSIC reflection (pattern detection)
- `internal/ape/improvement_plan.go` — RSIC planning (maps insights to actions)
- `internal/ape/task_dispatch.go` — Goroutine-based action executor
- `internal/ape/calibration.go` — Validation and per-action confidence tracking
- `internal/ape/watchdog.go` — Decay watchdog with 4-level escalation
- `internal/ape/cycle.go` — CycleOrchestrator (ties all 5 stages together)

**Protected Space**

The `mdemg-dev` space contains Claude's conversation memory and is protected from deletion. The API refuses destructive operations on this space.

---

## Evolution

CMS evolved through several phases:

**Phase 43A — Enforcement:** Session tracking, health scores, resume warnings

**Phase 43B — Quality:** Quality scoring, near-duplicate detection, relevance-weighted resume

**Phase 43C — Multi-Agent:** Agent identity, visibility levels (private/team/global), cross-session resume

**Phase 60 — Advanced II:** Structured templates, task snapshots, smart truncation, org-level review

**Phase 60b — RSIC:** Autonomous self-improvement cycles, decay watchdog enforcement, calibration meta-learning

---

## Configuration Reference

**Resume Settings**
- `CMS_RESUME_MAX_TOKENS` = 4000
- `CMS_RESUME_DEFAULT_STRATEGY` = task_focused

**Scoring Weights**
- `CMS_RELEVANCE_WEIGHT_RECENCY` = 0.3
- `CMS_RELEVANCE_WEIGHT_IMPORTANCE` = 0.4
- `CMS_RELEVANCE_WEIGHT_TASK_RELEVANCE` = 0.3

**Templates and Snapshots**
- `CMS_TEMPLATES_ENABLED` = true
- `CMS_SNAPSHOT_ON_SESSION_END` = true
- `CMS_SNAPSHOT_ON_COMPACTION` = true

**Volatile Memory (Context Cooler)**
- `STABILITY_INCREASE_PER_REINFORCEMENT` = 0.15
- `STABILITY_DECAY_RATE` = 0.1
- `TOMBSTONE_THRESHOLD` = 0.05
- `GRADUATION_STABILITY_THRESHOLD` = 0.8
- `REINFORCEMENT_WINDOW_HOURS` = 2

**Governance**
- `CMS_ORG_REVIEW_REQUIRED` = true

**RSIC Cycle Periods**
- `RSIC_MICRO_ENABLED` = true
- `RSIC_MESO_PERIOD_HOURS` = 6
- `RSIC_MESO_PERIOD_SESSIONS` = 5
- `RSIC_MACRO_CRON` = "0 3 * * *"

**RSIC Safety Bounds**
- `RSIC_MAX_NODE_PRUNE_PCT` = 5
- `RSIC_MAX_EDGE_PRUNE_PCT` = 10
- `RSIC_ROLLBACK_WINDOW` = 3

**RSIC Watchdog**
- `RSIC_WATCHDOG_ENABLED` = true
- `RSIC_WATCHDOG_CHECK_INTERVAL_SEC` = 60
- `RSIC_WATCHDOG_DECAY_RATE` = 1.0
- `RSIC_WATCHDOG_NUDGE_THRESHOLD` = 0.3
- `RSIC_WATCHDOG_WARN_THRESHOLD` = 0.6
- `RSIC_WATCHDOG_FORCE_THRESHOLD` = 0.9

**RSIC Calibration**
- `RSIC_CALIBRATION_WINDOW_DAYS` = 7
- `RSIC_MIN_CONFIDENCE_THRESHOLD` = 0.3
