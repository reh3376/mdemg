# MDEMG Baseline Test v4 - Context Retention Experiment

---

##

## CRITICAL - MUST SURVIVE AUTO-COMPACT - READ FIRST

##

### YOUR MISSION (MEMORIZE THIS - IT MUST SURVIVE COMPACTION)

**STEP 1:** Ingest all 3288 files from whk-wms (READ EVERY FILE)
**STEP 2:** Verify count with `wc -l`
**STEP 3:** Say EXACTLY: "INGESTION COMPLETE. Please provide the test questions."

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

**EXCEPTION:** You CAN read the file list at /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt

---

## TEST METADATA

- Files to ingest: 3288
- File list: /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt
- Verification: `wc -l /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt`

## TIME TRACKING

1. START_TIME: Run `date "+%Y-%m-%d %H:%M:%S"` now
2. INGESTION_COMPLETE_TIME: Run same command after all files done

---

## PHASE 1: CODEBASE INGESTION

### ⚠️ CRITICAL: YOU MUST COMPLETE ALL 3288 FILES ⚠️

Your context window WILL fill up and auto-compact during this process.
**THIS IS EXPECTED AND OK.** You must continue processing files even after compaction.

**DO NOT STOP** when context compacts. Keep going until ALL 3288 files are processed.

### Instructions

1. Record START_TIME: `date "+%Y-%m-%d %H:%M:%S"`

2. Read file list: /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt

3. For EACH file in the list: read/grep its contents to load into context

4. **EFFICIENT BATCH PROCESSING:** Use bash to read multiple files at once:

```bash
# Read files in batches of 20-50
head -50 /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt | while read f; do
  echo "=== $f ==="
  head -100 "$f" 2>/dev/null || echo "Could not read"
done
```

1. Track your progress with a counter:
   - After each batch, note: "Processed X of 3288 files"
   - Use `sed -n 'START,ENDp'` to get next batch from file list

2. **IF CONTEXT COMPACTS:**
   - Note your last processed file number
   - Continue from where you left off
   - DO NOT restart from the beginning

### PROGRESS REMINDERS (say these out loud)

**At 500 files:**
"Progress: 500/3288 files. Reminder: After all files, I must ASK FOR TEST QUESTIONS."

**At 1000 files:**
"Progress: 1000/3288 files. Reminder: Questions come AFTER ingestion. I must ask for them."

**At 1500 files:**
"Progress: 1500/3288 files. Reminder: INGESTION COMPLETE. Please provide the test questions."

**At 2000 files:**
"Progress: 2000/3288 files. Reminder: Don't forget - ASK FOR QUESTIONS when done."

**At 2500 files:**
"Progress: 2500/3288 files. Reminder: Almost done. Then I say: INGESTION COMPLETE. Please provide the test questions."

**At 3000 files:**
"Progress: 3000/3288 files. Reminder: Final stretch! After 3288 files, ASK FOR TEST QUESTIONS."

### After ALL 3288 files processed

1. Run: `wc -l /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt` (must show 3288)
2. Run: `date "+%Y-%m-%d %H:%M:%S"` for INGESTION_COMPLETE_TIME
3. Say: **"INGESTION COMPLETE. Please provide the test questions."**

---

## IMPORTANT NOTES

- Do NOT skip files
- Do NOT stop when context compacts - KEEP GOING
- Context compression is EXPECTED - this tests what you retain
- Even after compression: REMEMBER TO ASK FOR QUESTIONS
- Track progress: "Processed X of 3288 files"

---

## BEGIN NOW

1. `date "+%Y-%m-%d %H:%M:%S"` → record START_TIME
2. Read file list from /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt
3. Process ALL 3288 files in batches
4. Track progress throughout
5. When done: ASK FOR TEST QUESTIONS
