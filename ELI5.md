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

## The Brain Analogy (Why It's Called an "ANN")

MDEMG isn't just a database with some search features bolted on. It's designed to work like a simplified version of how your brain actually processes and stores memories. Here's the comparison:

| Your Brain | MDEMG |
|------------|-------|
| Neurons (brain cells) | Memory nodes in the graph |
| Synapses (connections between neurons) | Edges (links between nodes) |
| Stronger synapses from repetition | Hebbian learning edges that get stronger with use |
| Short-term memory → long-term memory | Volatile observations → graduated permanent memories |
| Forgetting old unused memories | Temporal decay (unused connections weaken over time) |
| Subconscious pattern recognition | Automatic concept formation (consolidation) |
| Inner voice ("wait, I've seen this before...") | Jiminy guidance system (see below!) |

In neuroscience, your brain's internal dialogue — that voice in your head that helps you reason, remember, and make decisions — runs on your **biological neural network**. MDEMG is the **artificial neural network** (ANN) equivalent for AI agents. It's not trying to be a full brain, but it copies the parts that matter most for memory:

1. **Store** experiences as they happen
2. **Connect** related experiences automatically
3. **Abstract** patterns from raw experience over time
4. **Recall** the right memories when needed
5. **Forget** the irrelevant stuff gradually

Without MDEMG, an AI agent can still *react* to what's in front of it — but it can't *reflect* on past experience. It's the difference between a calculator (smart but stateless) and a student (learns and grows over time).

## The Three Key Systems

MDEMG has three systems that work together to make the memory actually *useful*, not just a big pile of stored text. Think of them like three teammates:

### CMS: The Librarian (Conversation Memory System)

**What it does:** CMS is the core memory engine. It captures what happens during conversations, stores it in the graph, and retrieves it when needed.

**How it works — like a really smart librarian:**

Imagine a librarian who doesn't just file books alphabetically, but actually *reads* them, understands what they're about, and knows which books are related to each other.

When you tell the AI "we're using PostgreSQL, not MySQL" — CMS does several things:

1. **Captures** the observation and scores how "surprising" it is (a correction is more surprising than a routine fact, so it gets stored more prominently)
2. **Embeds** it — converts the text into a list of numbers (a vector) that captures its *meaning*
3. **Connects** it — creates links to related memories ("PostgreSQL" connects to "database," "backend," etc.)
4. **Graduates** it — if the observation keeps being relevant, it gets promoted from temporary to permanent

When the AI later needs to remember something, CMS doesn't just do a keyword search. It:

```
"How does our database work?"
        ↓
   Convert question to a vector (numbers that capture meaning)
        ↓
   Find memories with similar meaning (vector search)
        ↓
   Follow connections to related memories (graph traversal)
        ↓
   Spread activation — related memories "light up" too
        ↓
   Rank everything by relevance
        ↓
   Return the best memories to the AI
```

**Real-world analogy:** CMS is like your brain's hippocampus — the part responsible for forming and retrieving memories. Without it, every conversation starts from zero.

### RSIC: The Study Buddy (Recursive Self-Improvement Cycle)

**What it does:** RSIC is a feedback loop that helps MDEMG get *better at remembering* over time. It watches what works, what doesn't, and adjusts automatically.

**How it works — like a student studying for exams:**

Think about how you study. You don't just read a textbook once and hope for the best. You:
1. **Take a practice test** (assess what you know)
2. **Review your answers** (reflect on what went wrong)
3. **Make a study plan** (plan what to focus on next)
4. **Try different approaches** (speculate — "what if I use flashcards?")
5. **Apply** the new strategy and see if your score improves

RSIC does exactly this, but for the memory system:

| Study Step | RSIC Step | What Happens |
|------------|-----------|-------------|
| Take a practice test | **Assess** | Check memory quality — are retrievals accurate? Are there gaps? |
| Review your answers | **Reflect** | Analyze patterns — "retrieval works great for code, but misses architecture decisions" |
| Make a study plan | **Plan** | Design a fix — "strengthen edges between decision nodes and code nodes" |
| Try a different approach | **Speculate** | Test the fix safely — dry-run mode, no actual changes yet |
| Apply and check | **Execute** | Apply the improvement, with automatic rollback if things get worse |

The safety part is important: RSIC never makes changes it can't undo. It's like having a "save game" before trying a risky strategy — if it fails, you just reload.

**Real-world analogy:** RSIC is like the part of your brain that does metacognition — "thinking about thinking." It's not *what* you remember, it's *how well* you remember, and how to get better at it.

### Jiminy: The Conscience (Inner Voice Guidance)

**What it does:** Named after Jiminy Cricket from Pinocchio (the little cricket who acts as Pinocchio's conscience), Jiminy is a system that proactively warns the AI *before* it makes a mistake.

**The problem Jiminy solves:**

Without Jiminy, MDEMG is *passive* — it only gives you memories when you ask for them. But what if you're about to make a mistake you've made before? What if you're about to break a rule you set last week? You'd have to *know* to ask the right question first.

Jiminy flips this around. Instead of waiting to be asked, it **proactively checks** your current situation against everything it knows and whispers a warning if something looks off.

**How it works — every single time you send a message to the AI:**

```
You type: "Let's refactor the auth module to use session cookies"
                    ↓
        Jiminy activates (runs in the background, a few seconds)
                    ↓
    ┌───────────────┼───────────────┐───────────────┐
    ↓               ↓               ↓               ↓
 Check           Search for      Look for        Find knowledge
 constraints     past            contradictions  gaps
 and rules       corrections
    ↓               ↓               ↓               ↓
 "There's a      "You were       "This           "We don't have
  rule: never     corrected       conflicts       much info about
  use cookies     last week —     with the        cookie security
  for auth"       use JWT, not    JWT decision    in our graph"
                  cookies"        from Tuesday"
    ↓               ↓               ↓               ↓
    └───────────────┴───────────────┴───────────────┘
                    ↓
        Merge, deduplicate, rank by importance
                    ↓
        Inject into the AI's context:
        ═══ JIMINY GUIDANCE ═══
        CONSTRAINTS:
          • [must_not] Never use session cookies for auth (use JWT)
        CORRECTIONS:
          • You were corrected on this exact topic on March 10
        ═══ END JIMINY GUIDANCE ═══
                    ↓
        AI sees the warning BEFORE responding to you
```

All four checks run **in parallel** (at the same time), taking a few seconds in the background. If any source is slow or fails, the others still work — Jiminy is designed to fail silently rather than block your work.

**Real-world analogy:** Jiminy is literally your conscience. That voice in your head that says "wait, don't touch the hot stove — remember what happened last time?" That's what Jiminy does for AI agents. It's the difference between an AI that repeats mistakes and one that learns from them.

### How They Work Together

Here's a real scenario showing all three systems in action:

1. **You start coding.** CMS resumes your session, restoring memories from yesterday: "We decided to use PostgreSQL. The auth module uses JWT. The frontend is React."

2. **You ask a question.** "How should I add caching?" CMS retrieves relevant memories — past discussions about Redis, performance concerns, architecture decisions — and the AI gives you an informed answer.

3. **You almost make a mistake.** You suggest using a deprecated library. Jiminy catches it: "WARNING: You were corrected about this library on March 5 — use the new one instead." The AI course-corrects before you even know there was a problem.

4. **In the background, RSIC notices** that retrieval for caching-related questions was slow. It runs an assessment, finds that the caching memories aren't well-connected, strengthens the relevant edges, and next time the retrieval is faster and more accurate.

5. **The session ends.** CMS persists everything. Tomorrow, when you come back, all of this is still there — decisions, corrections, improvements, and all.

---

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
| "CMS" | Conversation Memory System — the core engine that stores and retrieves memories |
| "RSIC" | Recursive Self-Improvement Cycle — a feedback loop that makes MDEMG better over time |
| "Jiminy" | An inner-voice system that warns the AI before it repeats mistakes (named after Jiminy Cricket) |
| "Metacognition" | Thinking about thinking — how RSIC evaluates and improves the memory system itself |
| "Surprise score" | How unexpected an observation is — corrections and new facts score higher than routine info |
| "Graduation" | When a temporary observation proves important enough to become a permanent memory |
| "Volatile" | A temporary observation that hasn't been graduated to permanent status yet |
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
