# Claude-Mem Setup & Usage Guide

**Version:** Latest (npm)
**Repository:** <https://github.com/thedotmack/claude-mem>
**Status:** Installed and configured for user scope

---

## What is Claude-Mem?

Claude-Mem is a persistent memory system for Claude Code that:

- Automatically captures everything Claude does during coding sessions
- Compresses conversations with AI for efficient storage
- Injects relevant context back into future sessions
- Uses ChromaDB vector storage for semantic search

### Key Benefits

- Never re-explain your project context
- Search past conversations with natural language
- Automatic operation - no manual intervention needed
- Local-only storage for privacy

---

## Installation Status

```
Location: ~/.claude-mem/
Config: ~/.claude.json (project: /path/to/mdemg)
MCP Server: uvx chroma-mcp --client-type persistent
```

**Requires restart of Claude Code to activate.**

---

## How It Works

### Three-Layer Memory Architecture

1. **Raw Conversation Log**
   - Every message saved to SQLite
   - Flat file indexing for instant startup

2. **Compressed Summaries**
   - AI-powered compression of conversations
   - Key decisions, patterns, and context preserved

3. **Vector Embeddings (ChromaDB)**
   - Semantic search across all sessions
   - Hybrid keyword + vector retrieval

### Automatic Lifecycle

| Event | What Happens |
|-------|--------------|
| Session Start | Latest memories loaded automatically |
| During Session | Conversations captured in real-time |
| Session End | Compression + embedding generation |
| Next Session | Relevant context injected |

---

## Slash Commands

### `/save`

Quick save of current conversation overview.

```
/save
```

Use when you want to explicitly preserve the current state.

### `/remember`

Search your saved memories with natural language.

```
/remember how did we set up the Neo4j schema?
/remember what was the fix for the port conflict?
/remember authentication patterns we discussed
```

### `/claude-mem help`

Show all memory commands and features.

```
/claude-mem help
```

---

## Natural Language Search

You can also ask Claude directly to search memories:

```
"Search my memories for database connection issues"
"What do you remember about the MDEMG project?"
"Find previous discussions about embeddings"
```

---

## Web UI

Access the real-time memory stream at:

```
http://localhost:37777
```

Features:

- View all stored memories
- Search and filter
- Memory statistics
- Debug information

---

## Privacy Controls

### Exclude Sensitive Content

Wrap sensitive information in `<private>` tags:

```
<private>
API_KEY=sk-abc123...
DATABASE_PASSWORD=secret
</private>
```

This content will NOT be stored in memory.

### Data Location

All data stored locally:

```
~/.claude-mem/
├── chroma/          # Vector database
├── conversations/   # Raw logs
└── compressed/      # AI summaries
```

---

## Configuration

### View Current Config

```bash
cat ~/.claude.json | grep -A 20 "claude-mem"
```

### Adjust Settings

Edit `~/.claude/settings.json` or use:

```bash
claude-mem config --timeout 300000  # Increase hook timeout
```

### Available Options

| Setting | Default | Description |
|---------|---------|-------------|
| `timeout` | 180000ms | Hook execution timeout |
| `compression` | enabled | AI compression of logs |
| `autoload` | enabled | Load memories at startup |
| `maxContext` | 4000 tokens | Max context injection size |

---

## Troubleshooting

### MCP Server Not Connecting

```bash
# Check if uvx is available
which uvx

# Manually test chroma-mcp
uvx chroma-mcp --client-type persistent --data-dir ~/.claude-mem/chroma

# Check MCP status
claude mcp list
```

### Memories Not Loading

1. Restart Claude Code completely
2. Check `~/.claude-mem/` exists and has data
3. Verify hooks in `~/.claude/settings.json`

### Reset Memory System

```bash
# Backup first!
cp -r ~/.claude-mem ~/.claude-mem.backup

# Clear and reinstall
rm -rf ~/.claude-mem
claude-mem install --user --force
```

---

## Beta Features

### Endless Mode (Experimental)

Biomimetic memory architecture for extended sessions:

```bash
claude-mem config --beta endless
```

Access beta channel:

```bash
npm install -g claude-mem@beta
```

---

## Best Practices

1. **Let it run automatically** - Don't over-manage; the system learns your patterns

2. **Use `/save` for milestones** - After solving a tricky bug or completing a feature

3. **Search before starting** - Use `/remember` to check if you've solved similar problems

4. **Review the Web UI** - Periodically check localhost:37777 to see what's being captured

5. **Use `<private>` tags** - For credentials, tokens, and sensitive data

---

## Integration with MDEMG

Claude-mem and your MDEMG memory graph serve different purposes:

| Aspect | claude-mem | MDEMG |
|--------|-----------|-------|
| Scope | Claude Code sessions | Application data |
| Storage | SQLite + ChromaDB | Neo4j graph |
| Purpose | Context preservation | Knowledge reasoning |
| Control | Automatic | API-driven |

They can complement each other - claude-mem remembers how you work, MDEMG stores what you learn.

---

## Quick Reference

```bash
# Check status
claude mcp list

# View memories (web)
open http://localhost:37777

# Commands in Claude Code
/save                    # Save current context
/remember <query>        # Search memories
/claude-mem help         # All commands

# Manual memory search
"Search my memories for X"
```
