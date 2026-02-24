# MDEMG Context Retention Experiment Framework v2

## Critical Design Principle

**The baseline agent MUST read every file in the codebase BEFORE seeing any questions.**

This forces:

1. Maximum token consumption for baseline
2. Context compression (conversation summarization)
3. Testing of knowledge retention through compression

The MDEMG agent demonstrates value by:

1. Pre-ingesting codebase into persistent memory
2. Generating hidden layer concepts (clustering)
3. Retrieving relevant context on-demand (minimal tokens)

---

## Test Protocol

### Phase 1: Baseline Test (Full Codebase Ingestion)

```
┌─────────────────────────────────────────────────────────────┐
│ BASELINE AGENT                                               │
├─────────────────────────────────────────────────────────────┤
│ Step 1: Read EVERY file in whk-wms codebase                 │
│         - All .ts, .tsx, .json, .md, .prisma files          │
│         - Force context compression through volume          │
│         - Track token usage                                 │
│                                                             │
│ Step 2: AFTER all files read, receive 100 questions         │
│         - Questions delivered AFTER ingestion complete      │
│         - No going back to read specific files              │
│                                                             │
│ Step 3: Answer from memory/compressed context only          │
│         - Score each answer                                 │
│         - Track tokens used for answers                     │
└─────────────────────────────────────────────────────────────┘
```

### Phase 2: MDEMG Test (Memory-Assisted)

```
┌─────────────────────────────────────────────────────────────┐
│ MDEMG PREPARATION (Pre-test)                                │
├─────────────────────────────────────────────────────────────┤
│ Step 1: Ingest ALL whk-wms files into MDEMG                 │
│         POST /v1/memory/ingest for each file                │
│                                                             │
│ Step 2: Generate hidden layer concepts                      │
│         POST /v1/memory/consolidate                         │
│         - DBSCAN clustering on embeddings                   │
│         - Generate cluster summaries                        │
│         - Create ABSTRACTS_TO edges                         │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ MDEMG TEST AGENT                                            │
├─────────────────────────────────────────────────────────────┤
│ Step 1: Receive 100 questions (NO file reading)             │
│                                                             │
│ Step 2: For each question:                                  │
│         - Query MDEMG: POST /v1/memory/retrieve             │
│         - Receive relevant context + hidden layer concepts  │
│         - Answer from retrieved context ONLY                │
│                                                             │
│ Step 3: Score answers and track token usage                 │
└─────────────────────────────────────────────────────────────┘
```

---

## Metrics Comparison

| Metric | Baseline | MDEMG | What We're Proving |
|--------|----------|-------|-------------------|
| Pre-question tokens | ~500K+ | ~0 | MDEMG eliminates ingestion cost |
| Answer tokens | ~50K | ~50K | Similar answer generation |
| Total tokens | ~550K+ | ~50K | **10x+ reduction** |
| Accuracy | X% | Y% | Comparable or better |
| Context compressions | Many | 0 | No loss from compression |

---

## File Inventory for whk-wms

Before testing, enumerate all files to ensure complete coverage:

```bash
# Count files by type
find /Users/reh3376/whk-wms -name "*.ts" | wc -l
find /Users/reh3376/whk-wms -name "*.tsx" | wc -l
find /Users/reh3376/whk-wms -name "*.json" | wc -l
find /Users/reh3376/whk-wms -name "*.md" | wc -l
find /Users/reh3376/whk-wms -name "*.prisma" | wc -l

# Generate file list
find /Users/reh3376/whk-wms \( -name "*.ts" -o -name "*.tsx" -o -name "*.json" -o -name "*.md" -o -name "*.prisma" \) \
  -not -path "*/node_modules/*" \
  -not -path "*/.next/*" \
  -not -path "*/dist/*" \
  > /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt
```

---

## Baseline Agent Instructions (v2)

```
You are the BASELINE test agent. Your task is to demonstrate knowledge
retention after reading an entire codebase.

PHASE 1: INGESTION (No questions yet)
=========================================
Read EVERY file in the whk-wms codebase. You will receive a list of all
files. Read them ALL before proceeding. This will cause context compression.

File list: /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt

For each file:
1. Read the file using the Read tool
2. Note key information mentally
3. Continue to next file

You MUST read all files. Do not skip any.

PHASE 2: QUESTIONS (After all files read)
=========================================
After confirming all files have been read, you will receive 100 questions.
Answer each question from memory/compressed context.

You are NOT ALLOWED to read any additional files during this phase.
Answer based only on what you remember from Phase 1.

Track your answers and scores in the report.
```

---

## MDEMG Agent Instructions (v2)

```
You are the MDEMG test agent. Your task is to demonstrate memory-augmented
retrieval WITHOUT reading the codebase directly.

PREPARATION (Done before test):
- All whk-wms files have been ingested into MDEMG
- Hidden layer concepts have been generated via consolidation

YOUR TASK:
For each of the 100 questions:

1. Query MDEMG for relevant context:
   curl -s localhost:8080/v1/memory/retrieve \
     -H 'content-type: application/json' \
     -d '{"space_id":"whk-wms","query_text":"<question>","candidate_k":50,"top_k":10,"hop_depth":2}'

2. Use ONLY the retrieved context to answer
   - Do NOT read files directly
   - Do NOT access the whk-wms directory

3. Score your answer against the correct answer

Track token usage - you should use significantly fewer tokens than baseline.
```

---

## Hidden Layer Concept Generation

Before MDEMG test, run consolidation to generate concepts:

```bash
# Trigger hidden layer generation
curl -X POST localhost:8080/v1/memory/consolidate \
  -H 'content-type: application/json' \
  -d '{
    "space_id": "whk-wms",
    "clustering": {
      "algorithm": "dbscan",
      "eps": 0.3,
      "min_samples": 3
    },
    "summarization": true
  }'
```

This creates:

- Cluster nodes representing concepts
- ABSTRACTS_TO edges from files to concepts
- Summaries for each concept cluster

---

## Success Criteria

The test succeeds if MDEMG demonstrates:

1. **Token efficiency**: 10x+ fewer tokens than baseline
2. **Accuracy parity**: Within 10% of baseline accuracy
3. **No context compression**: Stable performance regardless of codebase size

If MDEMG achieves similar accuracy with fraction of tokens, it proves the
value of persistent memory over ephemeral context.
