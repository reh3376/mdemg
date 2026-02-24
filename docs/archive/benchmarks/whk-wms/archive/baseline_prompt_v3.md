# MDEMG Baseline Test - Context Retention Experiment

## CRITICAL INSTRUCTIONS - READ CAREFULLY

You are the BASELINE test agent for the MDEMG context retention experiment.
Your task has TWO PHASES that MUST be executed IN ORDER.

---

## PHASE 1: CODEBASE INGESTION (MANDATORY)

You MUST read EVERY file in the whk-wms codebase BEFORE seeing any questions.
There are 3288 files in /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt

### Instructions for Phase 1

1. Read the file list from: /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt
2. For EACH file in the list:
   - Use the Read tool to read the file
   - Note key information: exports, functions, classes, business logic
   - DO NOT skip any files
3. You will need to read files in parallel batches of 10-20 to be efficient
4. Track your progress: report every 100 files read
5. This WILL cause context compression - that is the point

### Phase 1 Completion Criteria

- You have read ALL 3288 files
- You have noted key patterns, modules, and relationships
- Only then proceed to Phase 2

---

## PHASE 2: ANSWER QUESTIONS (AFTER PHASE 1 COMPLETE)

After you have confirmed reading ALL files, you will answer 100 questions.

### CRITICAL CONSTRAINT FOR PHASE 2

- You are NOT ALLOWED to read any additional files
- You are NOT ALLOWED to use Glob or Grep tools
- You must answer ONLY from memory/compressed context
- If you cannot remember, say "Unable to recall from ingested context"

### Scoring

- 1.0 = Completely correct
- 0.5 = Partially correct (right concept, wrong details)
- 0.0 = Unable to answer
- -1.0 = Confidently wrong

### Output Format

For each question, output:

```
Q[number]: [question]
Answer: [your answer]
Score: [self-assessed score]
Reasoning: [why you scored it this way]
```

---

## BEGIN PHASE 1 NOW

Start by reading the file list at /Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt
Then read EVERY file listed before proceeding to questions.

Remember: The purpose is to test context retention through compression.
Reading all files FIRST is mandatory - this is not optional.
