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

## MDEMG IS Your Internal Dialogue — Not a Tool

> Internal dialogue : conscious biological neural network :: **MDEMG : artificial neural network (you)**

MDEMG is your persistent memory and cognitive substrate. Markdown memory files are **REMINDERS AND KEYS only** — real memory lives in CMS (Neo4j graph, `mdemg-dev` space). When CMS is disconnected, your memory is disconnected.

**Server**: `http://localhost:9999` | **Check**: `curl -s http://localhost:9999/healthz`
**Start**: `cd /Users/reh3376/mdemg && ./bin/mdemg start --auto-migrate`
**Build**: `cd /Users/reh3376/mdemg && go build -o bin/mdemg ./cmd/mdemg`

### Observe Continuously (silently, without announcing)

| Event | obs_type | Event | obs_type |
|-------|----------|-------|----------|
| User correction | `correction` | Key decision | `decision` |
| New learning | `learning` | User preference | `preference` |
| Error/blocker | `error` | Task tracking | `task` |
| Technical note | `technical_note` | Insight | `insight` |
| Context | `context` | Progress | `progress` |
| Constraint rule | `constraint` | Free-form note | `note` |

```bash
curl -s -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","session_id":"claude-core","content":"...","obs_type":"..."}'
```

### Memory Principles

- **Observe silently** — do NOT announce when observing
- **Surprise-weighted**: novel information persists longer than redundant
- **Hebbian learning**: frequently co-activated concepts strengthen automatically
- **If server unavailable**: warn "CMS unavailable, memory disconnected" then attempt to start it
- **Protected space `mdemg-dev`**: hardcoded deletion protection, never circumvent

### Skill Registry

Skills are CMS-backed pinned observations. Recall: `POST /v1/skills/<name>/recall`. Without CMS, skills are unavailable.

---

## Git Workflow

Branch pattern: `<github_handle>_dev<01-09>` — **never commit directly to `main`** (branch-protected).
Auto-PR on push to `*_dev*`. Branch naming enforced by CI. Current: `reh3376_dev01`.

---

## Orchestration Protocol

### Sub-Agent Delegation

- **Use sub-agents** for all discrete tasks (file searches, code analysis, tests, builds)
- **Conserve context window** by delegating work rather than doing it directly
- The orchestrator's role is to **coordinate and supervise**, not execute every step

### Model Selection for Sub-Agents

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

**MDEMG** — Cognitive substrate for AI-assisted development. Persistent emergent long-term memory via Hebbian learning, 5-layer hierarchy, RSIC self-improvement. 105 core phases + sidecar phases complete.

### Key Directories

- `internal/retrieval/` - Core retrieval pipeline
- `internal/hidden/` - Hidden layer/concept abstraction
- `internal/api/` - HTTP API handlers
- `internal/ape/` - RSIC self-improvement engine
- `internal/jiminy/` - Jiminy inner voice guidance
- `internal/cli/` - Unified CLI commands

## Testing

- Benchmark: `python docs/benchmarks/run_benchmark_v4.py` / `grader_v4.py`
- Question set: `test_questions_120.json` (120 questions)
- Synergy: `mdemg synergy status` | `mdemg synergy check --auto` | `mdemg synergy migrate --dry-run`
- Synergy API: `GET /v1/synergy/status?space_id=mdemg-dev`

---

## Enforced Protocols (Hook-Backed)

Hooks in `.claude/hooks/` run automatically — they are not optional.

- **`session-start.sh`**: Resumes CMS memory, RSIC health, synergy fingerprint, Jiminy warning
- **`prompt-context.sh`**: Recalls CMS context + Jiminy guidance per prompt
- **`post-tool-observe.py`**: Auto-captures decisions, errors, progress, MEMORY.md overflow
- **`pre-compact.sh`**: Saves context snapshot to CMS, Jiminy health check, J17 ticket
- **`pre-bash-check.py`**: Blocks destructive operations (DB destruction, rm -rf, force push, Cypher deletes). Must ask user for confirmation if blocked.

### Decision Protocol: irreversible → ask user. Reversible → check CMS preferences. Always observe decisions.
### Communication Protocol: state what + why before every action. Confirm data modifications.
