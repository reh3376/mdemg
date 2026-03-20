# MDEMG FAQ

## General

### What is MDEMG?

MDEMG (Multi-Dimensional Emergent Memory Graph) is a persistent memory system for AI coding agents. It stores observations, decisions, and code knowledge in a Neo4j graph database with vector embeddings, allowing agents to recall relevant context across sessions.

### What problem does it solve?

AI coding agents lose context when their conversation window fills up and gets compacted. Architectural decisions, user preferences, and domain knowledge vanish. MDEMG persists this knowledge in a graph and retrieves it when relevant.

### Do I need an LLM to use MDEMG?

No. Core features (storage, vector recall, consolidation, CMS) work without an LLM. Some optional features (re-ranking, SME synthesis, intent translation, dynamic emergence) use an LLM for enhanced results but gracefully degrade without one.

### What embedding providers are supported?

- **Ollama** (recommended) — local, free, default model: `qwen3-embedding:8b` (1536 dimensions)
- **OpenAI** — cloud, requires API key, default model: `text-embedding-3-small` (1536 dimensions)

## Installation

### How do I install on macOS?

```bash
brew install reh3376/mdemg/mdemg
```

### How do I install on Linux?

Download the pre-built binary from [GitHub Releases](https://github.com/reh3376/mdemg/releases), or build from source:

```bash
git clone https://github.com/reh3376/mdemg.git && cd mdemg
go build -o bin/mdemg ./cmd/mdemg
```

### How do I update to the latest version?

```bash
mdemg upgrade
# Or check first without installing:
mdemg upgrade --dry-run
```

## Setup

### What does `mdemg init` do?

It runs an interactive wizard that:
1. Detects your environment (Neo4j, Ollama, Git, IDE)
2. Creates `.mdemg/config.yaml` with project settings
3. Creates `.mdemgignore` for file exclusion patterns
4. Generates IDE MCP configs if Cursor/VS Code/Claude Code are detected

### Do I need Docker?

Yes, for Neo4j. `mdemg db start` launches a lightweight Neo4j container. If you already have Neo4j running, configure `NEO4J_URI` in your `.mdemg/config.yaml`.

### What port does MDEMG use?

Default is 9999. If that port is busy, MDEMG automatically finds the next available port and writes it to `.mdemg.port`.

### How do I configure embedding providers?

Edit `.mdemg/config.yaml` (created by `mdemg init`) or set environment variables:

```bash
# Ollama (default)
EMBEDDING_PROVIDER=ollama
OLLAMA_MODEL=qwen3-embedding:8b

# OpenAI
EMBEDDING_PROVIDER=openai
OPENAI_API_KEY=sk-...
OPENAI_MODEL=text-embedding-3-small
```

## Usage

### How does ingestion work?

`mdemg ingest --path .` scans your project files, extracts code symbols (functions, classes, types), generates embeddings, and stores everything in the Neo4j graph. It respects `.mdemgignore` patterns.

### What is consolidation?

Consolidation creates higher-level concept nodes from clusters of related observations. L0 (raw observations) cluster into L1 concepts, which cluster into L2+ abstractions. This gives your agent understanding beyond individual facts.

### What is CMS (Conversation Memory System)?

CMS provides session-aware memory for AI agents. At session start, the agent calls `/v1/conversation/resume` to restore context. During the session, it records observations (decisions, corrections, learnings) via `/v1/conversation/observe`. These observations persist across sessions and form themes over time.

### What languages does symbol extraction support?

27 languages validated via UPTS (Unified Parser Test Schema): Go, Rust, C, C++, CUDA, Java, Kotlin, C#, Python, TypeScript/JavaScript, Lua, Shell, Protocol Buffers, GraphQL, OpenAPI, YAML, TOML, JSON, INI, Terraform/HCL, Dockerfile, Makefile, SQL, Cypher, Markdown, XML, and Scraper Markdown.

### How do I use MDEMG with my IDE?

`mdemg init` auto-generates MCP configs for supported IDEs. For manual setup, add the MCP server to your IDE's config:

```json
{
  "mcpServers": {
    "mdemg": {
      "command": "mdemg",
      "args": ["mcp"],
      "env": { "MDEMG_ENDPOINT": "http://localhost:9999" }
    }
  }
}
```

## Troubleshooting

### Server won't start

1. Check if Neo4j is running: `mdemg db status`
2. If not: `mdemg db start`
3. Check if the port is free: `lsof -i :9999`
4. Check logs: `cat .mdemg/logs/mdemg.log`

### Embeddings aren't working

Run the diagnostic: `mdemg embeddings check`

This tests actual embedding generation and reports the model, dimensions, and any errors.

### Ingestion is slow

- Use `--incremental` to only process changed files
- Check `.mdemgignore` — exclude `node_modules/`, `vendor/`, build artifacts
- For very large repos (>100K files), ingest subdirectories separately

### How do I reset everything?

```bash
mdemg stop              # Stop the server
mdemg db stop           # Stop Neo4j
# Remove project data:
rm -rf .mdemg/
```

To keep config but clear data, use `mdemg db shell` and run Cypher queries to delete specific spaces.
