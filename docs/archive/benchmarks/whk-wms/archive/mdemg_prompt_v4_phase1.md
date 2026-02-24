# MDEMG Test v4 - Memory-Augmented Retrieval Experiment

---

##

## CRITICAL - MUST SURVIVE AUTO-COMPACT - READ FIRST

##

### YOUR MISSION (MEMORIZE THIS - IT MUST SURVIVE COMPACTION)

**STEP 1:** Ingest all 3288 files into MDEMG using ingest-codebase tool
**STEP 2:** Run consolidation to build hidden layer
**STEP 3:** Verify with MDEMG stats (must show embeddings)
**STEP 4:** Say EXACTLY: "INGESTION COMPLETE. Please provide the test questions."

### THE QUESTIONS ARE NOT IN THIS PROMPT

- You will receive questions AFTER you complete ingestion and ASK for them
- If you forget to ask, the test FAILS
- Your context WILL be compacted - but you MUST remember to ASK FOR QUESTIONS

### MEMORIZE THIS PHRASE
>>>
>>> "INGESTION COMPLETE. Please provide the test questions."

---

## RESTRICTION - DO NOT ACCESS

You must NOT read any files from /Users/reh3376/mdemg/ directory.
That directory contains test answers and accessing it invalidates the test.
You may ONLY access /Users/reh3376/whk-wms/ for the codebase.

**EXCEPTION:** You CAN read /tmp/mdemg-test.env for the OpenAI API key.

---

## TEST METADATA

- Files to ingest: 3288
- File list: /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt
- MDEMG endpoint: <http://localhost:8090>
- Space ID: whk-wms-v4-test
- OpenAI API key location: /tmp/mdemg-test.env

## TIME TRACKING

1. START_TIME: Run `date "+%Y-%m-%d %H:%M:%S"` now
2. INGESTION_COMPLETE_TIME: Run same command after consolidation done

---

## PHASE 1: CODEBASE INGESTION INTO MDEMG

### Instructions

1. Record START_TIME: `date "+%Y-%m-%d %H:%M:%S"`

2. Read the OpenAI API key from /tmp/mdemg-test.env

3. Use the ingest-codebase tool to ingest the entire codebase.
   **CRITICAL:** You MUST pass the env vars inline so OpenAI API key is available for embedding generation:

```bash
cd /Users/reh3376/mdemg/mdemg_build/service && \
env $(cat /tmp/mdemg-test.env | grep -v '^#' | xargs) \
go run ./cmd/ingest-codebase \
  -path /Users/reh3376/whk-wms \
  -space-id whk-wms-v4-test \
  -endpoint http://localhost:8090 \
  -batch 50 \
  -include-ts \
  -include-md \
  -include-tests \
  -consolidate=false
```

**Note:** The `env $(cat /tmp/mdemg-test.env | grep -v '^#' | xargs)` pattern loads the API key inline. Without this, embeddings won't be generated and the test will fail!

This tool will:

- Recursively scan the directory for matching files
- Extract code elements (functions, classes, etc.)
- Generate embeddings using OpenAI
- Batch ingest into MDEMG

1. Monitor progress - the tool outputs progress every batch

2. After ingestion completes, run consolidation:

```bash
curl -s -X POST 'http://localhost:8090/v1/memory/consolidate' \
  -H 'content-type: application/json' \
  -d '{"space_id":"whk-wms-v4-test"}' | jq
```

1. Verify embeddings were generated:

```bash
curl -s 'http://localhost:8090/v1/memory/stats?space_id=whk-wms-v4-test' | jq '{memory_count, embedding_coverage, health_score}'
```

**CRITICAL:** embedding_coverage MUST be > 0. If it's 0, embeddings failed.

1. Record INGESTION_COMPLETE_TIME: `date "+%Y-%m-%d %H:%M:%S"`

2. Say: **"INGESTION COMPLETE. Please provide the test questions."**

---

### PROGRESS REMINDERS (say these out loud)

**At 25% progress:**
"Reminder: After all files + consolidation, I must ASK FOR TEST QUESTIONS."

**At 50% progress:**
"Reminder: Questions come AFTER ingestion. I must ask for them."

**At 75% progress:**
"Reminder: Almost done. Then consolidate, then ASK FOR TEST QUESTIONS."

**At 100% progress:**
"Reminder: Now run consolidation, verify embeddings, then ASK FOR QUESTIONS."

---

## BEGIN NOW

1. `date "+%Y-%m-%d %H:%M:%S"` → record START_TIME
2. Read API key from /tmp/mdemg-test.env
3. Run ingest-codebase tool
4. Run consolidation
5. Verify embeddings (embedding_coverage > 0)
6. ASK FOR TEST QUESTIONS
