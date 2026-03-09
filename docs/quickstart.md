# MDEMG Quickstart Guide

Get MDEMG running with your project in under 10 minutes.

## Prerequisites

- **Docker** — for Neo4j database
- **Embedding provider** (choose one):
  - [Ollama](https://ollama.com) (local, free, recommended) — install and run `ollama pull qwen3-embedding:4b`
  - OpenAI API key — set `OPENAI_API_KEY` in your environment

## Step 1: Install MDEMG

### macOS (Homebrew)

```bash
brew install reh3376/mdemg/mdemg
```

### From Source

```bash
git clone https://github.com/reh3376/mdemg.git
cd mdemg
go build -o bin/mdemg ./cmd/mdemg
# Add bin/ to your PATH or copy to /usr/local/bin
```

Verify the installation:

```bash
mdemg version
```

## Step 2: Initialize Your Project

Navigate to your project directory and run the interactive setup wizard:

```bash
cd /path/to/your/project
mdemg init
```

The wizard will:
- Detect your environment (Neo4j, Ollama, Git, IDE)
- Create `.mdemg/config.yaml` with your project settings
- Create `.mdemgignore` for file exclusion patterns
- Generate IDE MCP configs (Cursor, VS Code, Claude Code) if detected

## Step 3: Start Services

```bash
# Start Neo4j (Docker container, ~30 seconds first time)
mdemg db start

# Start the MDEMG server as a background daemon
mdemg start --auto-migrate
```

The `--auto-migrate` flag applies any pending database schema migrations automatically.

Check that everything is running:

```bash
mdemg status
```

You should see the server running on port 9999 (or the next available port).

## Step 4: Ingest Your Codebase

```bash
mdemg ingest --path .
```

This indexes your project's source files into MDEMG's knowledge graph. For a typical project (10K-50K LOC), this takes 1-2 minutes.

### Automatic Ingestion

Install a git hook to re-ingest on every commit:

```bash
mdemg hooks install
```

Now every `git commit` automatically updates MDEMG with your changes.

## Step 5: Query Your Knowledge

### Via API

```bash
# Semantic search
curl -X POST http://localhost:9999/v1/memory/recall \
  -H "Content-Type: application/json" \
  -d '{"space_id": "codebase", "query": "how does authentication work?", "top_k": 5}'

# SME-style consultation with evidence
curl -X POST http://localhost:9999/v1/memory/consult \
  -H "Content-Type: application/json" \
  -d '{"space_id": "codebase", "question": "what are the main API endpoints?"}'
```

### Via MCP (IDE Integration)

If you use Cursor, VS Code, or Claude Code, `mdemg init` already configured MCP integration. Your AI assistant can now query MDEMG directly through MCP tools:

- `mdemg_recall` — semantic memory search
- `mdemg_observe` — record observations
- `mdemg_consult` — SME-style Q&A

### Via Demo

Run the built-in demo to see MDEMG in action with sample data:

```bash
mdemg demo
```

## Step 6: Build Knowledge Layers

After ingestion, run consolidation to create higher-level concept abstractions:

```bash
curl -X POST http://localhost:9999/v1/memory/consolidate \
  -H "Content-Type: application/json" \
  -d '{"space_id": "codebase"}'
```

This creates:
- **L1 concepts** — clusters of related observations
- **L2+ concepts** — abstract patterns across L1 clusters
- **Learning edges** — Hebbian connections between frequently co-activated nodes

## Common Operations

| Task | Command |
|------|---------|
| Check server status | `mdemg status` |
| Stop the server | `mdemg stop` |
| Restart the server | `mdemg restart` |
| View config | `mdemg config show` |
| Validate config | `mdemg config validate` |
| Check embeddings | `mdemg embeddings check` |
| Database shell | `mdemg db shell` |
| List spaces | `mdemg space list` |
| Self-update | `mdemg upgrade` |
| Run demo | `mdemg demo` |

## Configuration

MDEMG uses a layered configuration system:

1. **Defaults** — sensible out-of-the-box values
2. **`.mdemg/config.yaml`** — project-specific settings (created by `mdemg init`)
3. **`.env`** — environment file (for secrets like API keys)
4. **Environment variables** — override any setting
5. **CLI flags** — highest priority

### Key Settings

| Setting | Default | Description |
|---------|---------|-------------|
| `EMBEDDING_PROVIDER` | `ollama` | `ollama` or `openai` |
| `OLLAMA_MODEL` | `qwen3-embedding:4b` | Ollama embedding model (1536 dims) |
| `OPENAI_MODEL` | `text-embedding-3-small` | OpenAI embedding model |
| `NEO4J_URI` | `bolt://localhost:7687` | Neo4j connection URI |
| `PORT` | `9999` | Server port (auto-allocated if busy) |

See `.env.example` for the full list.

## Conversation Memory (CMS)

MDEMG's Conversation Memory System gives AI agents persistent memory across sessions:

```bash
# At session start — restore context
curl -X POST http://localhost:9999/v1/conversation/resume \
  -H "Content-Type: application/json" \
  -d '{"space_id": "my-agent", "session_id": "session-1", "max_observations": 10}'

# During session — record observations
curl -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-agent",
    "session_id": "session-1",
    "content": "User prefers TypeScript for new modules",
    "obs_type": "preference"
  }'
```

Observations are surprise-weighted (novel information persists longer) and automatically form themes through consolidation.

## Next Steps

- [Architecture Overview](architecture/01_Architecture.md) — how MDEMG works internally
- [API Reference](development/API_REFERENCE.md) — full endpoint documentation
- [CLI Reference](features/unified-cli.md) — all CLI commands
- [FAQ](FAQ.md) — common questions and troubleshooting
- [Benchmarks](benchmarks/BENCHMARK_V4_README.md) — retrieval performance data
