# MDEMG - Explain Like I'm 5 (Well, Maybe 12)

## What's the Problem?

Have you ever used an AI chatbot like ChatGPT or Claude? They're really smart — but they have a big weakness: **they forget everything between conversations**.

Imagine if every time you talked to your best friend, they forgot who you were, what you talked about yesterday, and everything you'd ever done together. That's what it's like for AI coding assistants right now. Every time they start a new conversation (or the conversation gets too long), they lose all their memory.

This is especially painful when you're coding. You tell the AI "we decided to use approach X instead of Y because of Z" — and 30 minutes later, it suggests approach Y again because it forgot.

## What Does MDEMG Do?

**MDEMG gives AI assistants a long-term memory.**

Think of it like this:

- **Without MDEMG**: Your AI assistant is like a goldfish — smart in the moment, but no memory of what happened before.
- **With MDEMG**: Your AI assistant is like a colleague who keeps detailed notes — they remember past decisions, learn from mistakes, and build up knowledge over time.

## How Does It Work? (The Fun Part)

MDEMG works kind of like how your brain forms memories — seriously! Here's the simplified version:

### 1. Observations (Raw Memories)

When you're working with an AI assistant, MDEMG captures important moments:
- "The user corrected me — the database is PostgreSQL, not MySQL"
- "We decided to use React for the frontend"
- "This function handles login authentication"

These are like individual memories in your brain.

### 2. Connections (How Memories Link Together)

MDEMG uses something called a **graph database** (Neo4j). A graph is just a bunch of dots (nodes) connected by lines (edges) — like a web or a mind map.

```
[React Frontend] ---uses---> [REST API] ---connects to---> [PostgreSQL DB]
       |                                                          |
       |-------- both part of ------> [Login System] <------------|
```

When two memories keep showing up together (like "React" and "REST API" always being discussed in the same context), MDEMG strengthens the connection between them. This is inspired by how real brains work — it's called **Hebbian learning** (named after a neuroscientist). The simple version: "neurons that fire together, wire together."

### 3. Concepts (Higher-Level Understanding)

Over time, MDEMG automatically groups related memories into bigger concepts. Individual observations about React components, API endpoints, and database queries might get grouped into a higher-level concept like "Authentication System."

This happens in layers:
- **Layer 0**: Raw observations ("UserService.login uses JWT tokens")
- **Layer 1**: Hidden concepts ("Authentication Module")
- **Layer 2**: Broader patterns ("Security Architecture")
- **Layer 3+**: Even more abstract ideas

It's like going from individual puzzle pieces to seeing the whole picture.

### 4. Retrieval (Finding the Right Memory)

When the AI needs to remember something, MDEMG doesn't just do a simple text search (like Ctrl+F). It uses:

- **Vector search**: Converts text into numbers (called embeddings) and finds memories with similar *meaning*, not just matching words. So searching for "how does login work?" would find memories about "authentication flow" even though the words are different.
- **Graph traversal**: Follows the connections between memories to find related information. "Oh, you're asking about login? Let me also grab what I know about JWT tokens and the user database, since those are connected."

## The Tech Stack (What It's Built With)

| Technology | What It Does | Why It's Cool |
|------------|-------------|---------------|
| **Go** | The main programming language | Fast, compiles to a single binary, great for servers |
| **Neo4j** | Graph database | Stores memories as connected nodes — perfect for relationships |
| **Vector indexes** | Semantic search | Finds memories by meaning, not just keywords |
| **Protocol Buffers** | Data format for export/import | Efficient binary format used by Google |
| **REST API** | How other programs talk to MDEMG | Standard HTTP — any programming language can use it |

## Key Vocabulary

Here are some terms from the README translated:

| README Term | What It Actually Means |
|-------------|----------------------|
| "Persistent memory" | Memory that doesn't disappear when the conversation ends |
| "Semantic retrieval" | Finding information by meaning, not exact word matching |
| "Context compaction" | When a conversation gets too long and the AI has to summarize/forget older parts |
| "Hebbian learning" | Connections get stronger when two things are used together (brain-inspired) |
| "Emergent concepts" | Higher-level ideas that form automatically from lots of smaller observations |
| "Hidden layer" | A concept that was created by the system (not directly from user input) |
| "Vector embedding" | A way to represent text as a list of numbers so a computer can measure similarity |
| "Graph database" | A database that focuses on relationships between things (vs. tables in SQL) |
| "Activation spreading" | When you look up one memory, related memories also "light up" — like how thinking about pizza might make you think about cheese |
| "Temporal decay" | Old, unused memories gradually fade — just like how you forget things you haven't thought about in a while |
| "Space" | A container that keeps one project's memories separate from another's |
| "Consolidation" | The process of organizing raw memories into higher-level concepts |
| "UATS" | Automated tests that verify the API works correctly (like a spell-checker for code) |

## Want to Learn More?

If you're interested in the ideas behind MDEMG, here are some rabbit holes:

- **Graph databases**: Try the free [Neo4j sandbox](https://neo4j.com/sandbox/) to play with connected data
- **Vector embeddings**: Search for "word2vec explained" on YouTube — it's the foundation of how AI understands word meaning
- **Hebbian learning**: Look up "neural networks for beginners" — MDEMG borrows ideas from how biological brains learn
- **Go programming**: [Go by Example](https://gobyexample.com/) is a great starting point if you want to read the code
- **REST APIs**: Try making API calls with `curl` — it's how programs talk to each other over the internet
- **Git**: If you're not using version control yet, learn Git first — it's the most important tool in programming

## Can I Try It?

Yes! You'll need:
1. **Docker Desktop** (free) — to run the Neo4j database
2. **Go 1.24+** — to build from source
3. Optionally, an **OpenAI API key** or **Ollama** (free, local) for the AI-powered features

```bash
# Clone the repo
git clone https://github.com/reh3376/mdemg.git
cd mdemg

# Build it
go build -o bin/mdemg ./cmd/mdemg

# Start it up
./bin/mdemg db start
./bin/mdemg start --auto-migrate

# Check it's working
curl http://localhost:9999/healthz
# Should return: {"status":"ok"}
```

Then try storing and retrieving a memory:

```bash
# Store a memory
curl -X POST http://localhost:9999/v1/memory/ingest \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-first-space",
    "name": "hello-world",
    "content": "MDEMG is a memory system for AI agents",
    "path": "/learning/first-memory"
  }'

# You should get back a node_id — that's your memory stored in the graph!
```
