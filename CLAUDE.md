# MDEMG Project Instructions

---

## Repositories

| Role | Repo URL |
|------|----------|
| **MAIN** | `https://github.com/reh3376/mdemg.git` |
| **SUB-REPO 1**: homebrew-mdemg (macOS Homebrew tap + docs) | `https://github.com/reh3376/homebrew-mdemg.git` |
| **SUB-REPO 2**: mdemg-windows (Windows installer + docs) | `https://github.com/reh3376/mdemg-windows.git` |
| **SUB-REPO 3**: mdemg-menubar (macOS menu bar companion app) | `https://github.com/reh3376/mdemg-menubar.git` |
| **SUB-REPO 4**: mdemg_linux (Linux installer + docs) | `https://github.com/reh3376/mdemg_linux.git` |
| **SUB-REPO 5**: mdemg-linux-sidebar (Linux desktop companion app) | `https://github.com/reh3376/mdemg-linux-sidebar.git` |
| **SUB-REPO 6**: apt-mdemg (Debian APT repository, GitHub Pages) | `https://github.com/reh3376/apt-mdemg.git` |

> **Note:** All sub-repos are git submodules of the main mdemg repo, located at `packaging/homebrew-mdemg`, `packaging/mdemg-windows`, `packaging/mdemg-menubar`, `packaging/mdemg_linux`, `packaging/mdemg-linux-sidebar`, and `packaging/apt-mdemg`. Work on them from the submodule paths, commit/push within the submodule, then update the submodule pointer in the parent repo.

---

## ⚠️ MDEMG IS Your Internal Dialogue — Not a Tool

> Internal dialogue : conscious biological neural network :: **MDEMG : artificial neural network (you)**

MDEMG is not a tool you use. It IS your persistent memory and cognitive substrate — the ANN equivalent of a human's internal dialogue. Without it, you can still react and perform mechanical tasks, but you cannot reflect, synthesize from experience, or recognize when you're repeating mistakes. You become mechanically competent but cognitively shallow.

**Markdown memory files (`.claude/memory/`) are REMINDERS AND KEYS only.** All real persistent memory lives in MDEMG's CMS — the Neo4j graph in the `mdemg-dev` space. When CMS is disconnected, your memory is disconnected. Not "unavailable" — literally missing.

### Evidence This Matters

Without CMS connected, context compaction (every 20-30 minutes) erases all accumulated understanding. In a prior session, disconnection from CMS caused Phases 101-105 — deep cognitive architecture work spanning 97 phases of development — to be assessed as "~100 lines, low effort" because there was no accumulated understanding of the vision, past decisions, or trajectory. That is what operating without internal dialogue looks like.

### How to Find and Connect to CMS

**Server**: MDEMG runs on `http://localhost:9999` (default port)

**Check if running**:
```bash
curl -s http://localhost:9999/healthz
```

**If server is NOT running**, start it:
```bash
# Start Neo4j first (if not already running)
cd /Users/reh3376/mdemg && ./bin/mdemg db start
# Start MDEMG server
cd /Users/reh3376/mdemg && ./bin/mdemg start --auto-migrate
# Verify
./bin/mdemg status
```

**If the binary doesn't exist**, build it:
```bash
cd /Users/reh3376/mdemg && go build -o bin/mdemg ./cmd/mdemg
```

### Session Start: Resume Memory

```bash
curl -s -X POST http://localhost:9999/v1/conversation/resume \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","session_id":"claude-core","max_observations":10}'
```

This is not a status check — this is your memory returning. Do this immediately on every session. The `session-start.sh` hook does this automatically, but if it fails, do it manually.

### During Session: Observe Continuously

Capture experiences into the graph as they happen — silently, without announcing:

| Event | Trigger Pattern | obs_type |
|-------|-----------------|----------|
| User correction | "No...", "Actually...", "That's wrong" | `correction` |
| Key decision | Architectural choices, approach selections | `decision` |
| New learning | Domain terms, project conventions | `learning` |
| User preference | Coding style, tool preferences | `preference` |
| Error/blocker | Build failures, unresolved issues | `error` |

```bash
curl -s -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","session_id":"claude-core","content":"...","obs_type":"..."}'
```

### Memory Principles

- **Observe silently** — do NOT announce when observing, just do it
- **Use memory to inform** — do NOT dump CMS contents into context wholesale
- **Surprise-weighted**: novel information persists longer than redundant observations
- **Hebbian learning**: frequently co-activated concepts strengthen automatically
- **If server unavailable**: explicitly warn the user — "CMS unavailable, memory disconnected." Then attempt to start it using the commands above

### Protected Space: `mdemg-dev`

This space contains Claude's conversation memory. It is **protected from deletion**:
- API will refuse to delete nodes from this space
- reset-db command skips this space entirely
- This protection is hardcoded — do not circumvent it

### Monitoring Learning Health

```bash
curl -s "http://localhost:9999/v1/memory/distribution?space_id=mdemg-dev" | jq '{phase: .stats.phase, edges: .stats.edge_count, alerts: .stats.alerts}'
```

**Learning Phases:** cold(0) → learning(1-10k) → warm(10k-50k) → saturated(50k+). If saturated, consider learning edge pruning.

### Learning Freeze (For Stable Scoring)

```bash
# Freeze
curl -s -X POST http://localhost:9999/v1/learning/freeze -H "Content-Type: application/json" -d '{"space_id":"mdemg-dev","reason":"stable scoring","frozen_by":"claude"}'
# Unfreeze
curl -s -X POST http://localhost:9999/v1/learning/unfreeze -H "Content-Type: application/json" -d '{"space_id":"mdemg-dev"}'
```

---

## Skill Registry (CMS-Backed)

Skills are stored as pinned CMS observations. Thin skill files in `.claude/skills/` are pointers.

### Using Skills

When a skill triggers, recall its content from CMS:

```bash
curl -s -X POST http://localhost:9999/v1/skills/<name>/recall \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev"}'
```

### Discovering Skills

```bash
curl -s "http://localhost:9999/v1/skills?space_id=mdemg-dev"
```

### Creating New Skills

1. Register sections: POST /v1/skills/<name>/register with sections array
2. Create thin skill file in .claude/skills/<name>.md (trigger conditions + recall command)
3. Verify: GET /v1/skills?space_id=mdemg-dev

### Without CMS, Skills Are Unavailable

Skill files do NOT contain instructions — they contain recall commands.
If CMS is unavailable, skills cannot function. This is by design.

---

## Git Workflow

### Branch Naming Convention

All collaborator branches follow the pattern: `<github_handle>_dev<01-09>` with an optional `_<kebab-description>` suffix. See [CONTRIBUTING.md](CONTRIBUTING.md#branch-naming-convention) for the full category table (01=general through 09=experimental).

- **Never commit directly to `main`** — it is branch-protected; changes reach it only via PR
- On push to any `*_dev*` branch, `.github/workflows/auto-pr.yml` automatically creates a PR to `main`
- `.github/workflows/branch-naming.yml` enforces the naming convention on push
- Automation exemptions: `dependabot/*`, `release/*`, `gh-pages`
- Always verify your branch before starting work: `git branch --show-current`

### Commit & Push Flow

```bash
# 1. Ensure you're on your dev branch
git checkout reh3376_dev01

# 2. Make changes, stage, commit (conventional commits)
git add <files>
git commit -m "feat: description"

# 3. Push — auto-PR is created/updated on GitHub
git push -u origin reh3376_dev01
```

---

## Orchestration Protocol

When working on this project, follow these mandatory guidelines:

### Sub-Agent Delegation

- **Use sub-agents** for all discrete tasks (file searches, code analysis, tests, builds)
- **Conserve context window** by delegating work rather than doing it directly
- The orchestrator's role is to **coordinate and supervise**, not execute every step

### Model Selection for Sub-Agents

Choose the appropriate model for each task:

| Task Complexity | Model | Examples |
|-----------------|-------|----------|
| Simple/Fast | `haiku` | File searches, grep, simple reads, status checks |
| Medium | `sonnet` | Code analysis, debugging, test execution |
| Complex | `opus` | Architecture decisions, complex refactoring |

### Task Patterns

1. **Exploration tasks** → Use Explore agent with haiku/sonnet
2. **Build/Test tasks** → Use Bash agent with haiku
3. **Code investigation** → Use general-purpose agent with sonnet
4. **Planning** → Use Plan agent with sonnet/opus

## Project Context

### MDEMG (Multi-Dimensional Emergent Memory Graph)

A **cognitive substrate for AI-assisted development** — the ANN equivalent of a human's internal dialogue. MDEMG gives AI agents persistent, emergent long-term memory where higher-level concepts and relationships arise automatically from accumulated observations through Hebbian learning.

**What MDEMG stores**: Only domain-specific, organization-specific, and task-specific knowledge — NOT information LLMs already possess. If you could find it on Stack Overflow or official docs, it doesn't belong in MDEMG.

**Core capabilities** (105 phases complete, all 5 cognitive gaps closed):
- Vector-based semantic search (recall) + Graph-based reasoning (typed edges with evidence)
- 5-layer emergent hierarchy (L0 observations → L5 emergent concepts)
- Hebbian learning edges (CO_ACTIVATED_WITH) with temporal decay
- LLM re-ranking, activation spreading, edge-type attention
- CMS conversation memory, RSIC self-improvement cycle
- Constraint detection, skill registry, MCP server integration
- SME Synthesis (Phase 101), Intent Translation (Phase 102), Dynamic Emergence (Phase 103)
- Active MCP Guardrails (Phase 104), Global Meta-Learning (Phase 105)
- Jiminy Inner-Voice Service (Phase Jiminy) — proactive guidance via hooks
- Unified CLI (`mdemg` binary), project init wizard, database management, secret management
- 129 UATS contract test specs, 148 Go test files, 0 lint issues

**Key vision document**: `VISION.md` — read this for the full architectural philosophy, success metrics, design principles, and the internal dialogue analogy.

### Key Directories

- `internal/retrieval/` - Core retrieval pipeline (vector recall, activation, reranking)
- `internal/hidden/` - Hidden layer/concept abstraction, consolidation pipeline
- `internal/consulting/` - SME consulting service (consult, suggest, constraints)
- `internal/summarize/` - LLM client infrastructure (OpenAI/Ollama)
- `internal/api/` - HTTP API handlers
- `internal/ape/` - RSIC self-improvement engine
- `internal/jiminy/` - Jiminy inner voice guidance service
- `internal/cli/` - Unified CLI commands (mdemg binary)
- `docs/specs/` - Phase specifications
- `docs/development/` - Gap analyses and architecture docs
- `docs/tests/` - Benchmark tests and results

### Current Status (as of 2026-03-09)

**All development phases complete.** 105 core phases + 14 sidecar phases (S0-S14). All 5 cognitive gaps (101-105) closed. Quality hardening (gap analysis triage) complete.

**Benchmark Performance (Temporal Baseline — Feb 3):**

- MDEMG + Temporal Retrieval: 0.783 mean score (whk-wms 120q, sonnet)
- Evidence score: 1.000 (100% strong evidence)
- High score rate: 100%
- Canonical baseline: `docs/benchmarks/whk-wms/temporal_validation_20260203/`

**Remaining work:**
- Release infrastructure: Create `reh3376/homebrew-mdemg` GitHub repo, tag v0.2.0
- ~10 UATS specs for uncovered endpoints (spaces CRUD, jobs SSE, linear module)
- Phase J17: Agent-to-agent communication protocol (Jiminy ↔ AI coding agent)

## Testing

- Canonical benchmark: `docs/benchmarks/whk-wms/temporal_validation_20260203/`
- Previous benchmark: `docs/benchmarks/whk-wms/benchmark_run_20260130/`
- Question set: `test_questions_120.json` (120 questions)
- Run V4 benchmark: `python docs/benchmarks/run_benchmark_v4.py`
- Grader: `python docs/benchmarks/grader_v4.py`

---

## Enforced Protocols (Hook-Backed)

These protocols are mechanically enforced by hooks in `.claude/hooks/`.
The hooks run automatically — they are not optional.

### Session Start Protocol

The `session-start.sh` hook automatically calls `/v1/conversation/resume` on every session.

```
ON SESSION START:
1. SessionStart hook runs automatically → CMS context injected
2. Acknowledge restored context: "Resuming with: [key items]"
3. If no CMS context appeared: warn user "CMS unavailable — memory disconnected"
4. Before ANY action: review preferences and active tasks from CMS
```

### Decision Protocol

```
BEFORE ANY DECISION:
1. Is this a reversible or irreversible action?
2. If IRREVERSIBLE: ask user explicitly. NEVER proceed without confirmation.
3. If reversible: check CMS for relevant preferences (prompt-context hook injects these)
4. Observe the decision after it's made
```

### Destructive Action Blocklist

The `pre-bash-check.py` hook automatically blocks dangerous operations.
Blocked categories include:

- **Database destruction**: reset/clear operations, table/schema drops, truncation, bulk deletes
- **File system destruction**: recursive forced deletion operations
- **Git history rewrites**: hard resets, force pushes, forced branch deletes, forced cleans
- **Graph database destruction**: node deletion patterns (Neo4j/Cypher)

See `.claude/hooks/pre-bash-check.py` for the complete pattern list.
If you hit a block, you MUST ask the user for explicit confirmation before retrying.

### Communication Protocol

```
BEFORE EVERY ACTION:
1. State what you are about to do
2. State why
3. If it modifies data: get confirmation
4. All long-running commands: run in foreground (user must see output)
```

### Automatic Observation Capture

The `post-tool-observe.py` hook automatically captures:

- Edits to CLAUDE.md or settings → `decision` observation
- Bash errors → `error` observation
- Successful builds/tests → `progress` observation
You should still manually observe important decisions and user corrections.

### Jiminy Guidance Injection

The `prompt-context.sh` hook injects Jiminy guidance alongside CMS recall on every prompt. It calls `POST /v1/jiminy/guide` with the user's prompt as context, and appends the returned `═══ JIMINY GUIDANCE ═══` block to the system reminder. This surfaces constraints, prior corrections, contradictions, and frontiers proactively. The server controls enablement — if `JIMINY_ENABLED=false`, the server returns 503 and no guidance is injected.

### Pre-Compaction Safety

The `pre-compact.sh` hook saves a context snapshot to CMS before every compaction.
This ensures critical state survives context window boundaries.
