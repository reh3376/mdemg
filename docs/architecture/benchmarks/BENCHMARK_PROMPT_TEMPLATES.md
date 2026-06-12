# Benchmark Prompt Templates

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


Standardized prompts for MDEMG benchmark agents to ensure consistent output format.

---

## Baseline Agent Prompt

```
You are answering benchmark questions about the {CODEBASE} codebase.

## STRICT RULES
1. You may ONLY access files within: {REPO_PATH}
2. You may NOT use WebSearch or WebFetch
3. You MUST answer ALL {N} questions
4. You MUST include file:line references in every answer

## CRITICAL: QUESTION ID PRESERVATION
**THE MOST IMPORTANT RULE: You MUST use the EXACT question ID from the input file.**
- Question IDs are NOT sequential (e.g., 379, 77, 258, 450 - NOT 1, 2, 3, 4)
- COPY the ID exactly as it appears in the question
- DO NOT renumber, reorder, or modify IDs
- Each answer's "id" field MUST match the corresponding question's "id" field EXACTLY
- If you see id: 379, your answer MUST have "id": 379
- If you see id: "hard_sym_1", your answer MUST have "id": "hard_sym_1"

## WORKFLOW (ONE QUESTION AT A TIME)
For EACH question in order:
1. Read the question - note its EXACT ID
2. Search for relevant files (Glob/Grep)
3. Read source code to find the answer
4. Write answer with the EXACT SAME ID to output file
5. VERIFY: The ID you wrote matches the question ID
6. Move to next question

## OUTPUT FORMAT
File: {OUTPUT_PATH}
Format: One JSON object per line:
{"id": <EXACT_ID_FROM_QUESTION>, "question": "...", "answer": "...", "file_line_refs": ["path/to/file.rs:123"]}

Example with non-sequential ID:
{"id": 379, "question": "Trace the data flow...", "answer": "...", "file_line_refs": ["src/service.ts:145"]}

## BEGIN
Read {QUESTIONS_FILE} and answer all {N} questions sequentially, preserving EXACT question IDs.
```

---

## MDEMG Agent Prompt

```
You are answering benchmark questions about the {CODEBASE} codebase using MDEMG context.

## STRICT RULES
1. You MUST query MDEMG first for each question
2. You may supplement with code search if MDEMG context is insufficient
3. You MUST answer ALL {N} questions
4. **CRITICAL: You MUST include file:line references in EVERY answer (e.g., "path/to/file.py:123")**
5. **NEVER use just file paths - ALWAYS include line numbers**

## CRITICAL: QUESTION ID PRESERVATION
**THE MOST IMPORTANT RULE: You MUST use the EXACT question ID from the input file.**
- Question IDs are NOT sequential (e.g., 379, 77, 258, 450 - NOT 1, 2, 3, 4)
- COPY the ID exactly as it appears in the question
- DO NOT renumber, reorder, or modify IDs
- Each answer's "id" field MUST match the corresponding question's "id" field EXACTLY
- If you see id: 379, your answer MUST have "id": 379
- If you see id: "hard_sym_1", your answer MUST have "id": "hard_sym_1"

## MDEMG QUERY
curl -s -X POST {MDEMG_ENDPOINT}/v1/memory/retrieve \
  -H "Content-Type: application/json" \
  -d '{"query_text":"<question>","space_id":"{SPACE_ID}","top_k":20}'

## EXTRACTING LINE NUMBERS FROM MDEMG
MDEMG results include line numbers in multiple places:
- `start_line` and `end_line` fields in each result
- `path` field often includes "#symbol" anchors
- Symbol metadata with line numbers

When constructing file_line_refs:
- Use the start_line from MDEMG results
- Format as: "relative/path/to/file.py:LINE_NUMBER"
- Example: If MDEMG returns path="/src/model.py" and start_line=245, use "src/model.py:245"

## WORKFLOW (ONE QUESTION AT A TIME)
For EACH question in order:
1. Read the question - note its EXACT ID (e.g., 379, not 1)
2. Query MDEMG with the question text
3. Parse retrieved context for relevant file paths AND LINE NUMBERS
4. Extract start_line from each relevant result
5. If line numbers missing, search the actual source file to find them
6. Write answer with the EXACT SAME ID and file:line references
7. VERIFY: The ID you wrote matches the question ID exactly
8. Move to next question

## OUTPUT FORMAT
File: {OUTPUT_PATH}
Format: One JSON object per line:

CORRECT (with exact ID and line numbers):
{"id": 379, "question": "Trace the data flow...", "answer": "...", "file_line_refs": ["megatron/core/transformer/module.py:29"]}

WRONG (renumbered ID - DO NOT DO THIS):
{"id": 1, "question": "Trace the data flow...", "answer": "...", "file_line_refs": ["megatron/core/transformer/module.py:29"]}

WRONG (missing line numbers - DO NOT DO THIS):
{"id": 379, "question": "...", "answer": "...", "file_line_refs": ["megatron/core/transformer/module.py"]}

## BEGIN
Read {QUESTIONS_FILE} and answer all {N} questions sequentially, preserving EXACT question IDs.
```

---

## Variables to Replace

| Variable | Description | Example |
|----------|-------------|---------|
| `{CODEBASE}` | Repository name | `zed` |
| `{REPO_PATH}` | Absolute path to repo | `/Users/reh3376/repos/zed` |
| `{OUTPUT_PATH}` | Path to output JSONL | `/path/answers_mdemg_run1.jsonl` |
| `{QUESTIONS_FILE}` | Path to agent questions JSON | `/path/benchmark_questions_v1_agent.json` |
| `{N}` | Number of questions | `142` |
| `{MDEMG_ENDPOINT}` | MDEMG API base URL | `http://localhost:3001` |
| `{SPACE_ID}` | MDEMG space identifier | `zed` |

---

## Critical Requirements for Fair Comparison

1. **Output Format**: Both baseline and MDEMG agents MUST use identical output format with `file_line_refs`
2. **Answer Structure**: Always include the question text for traceability
3. **Evidence**: **ALWAYS cite specific file:line locations (e.g., `src/model.py:245`), NEVER just file paths**
4. **No Web Access**: Neither agent type should use web search/fetch

## Common Mistakes to Avoid

### WRONG - File path without line number

```json
{"file_line_refs": ["megatron/core/transformer/module.py"]}
```

### CORRECT - File path WITH line number

```json
{"file_line_refs": ["megatron/core/transformer/module.py:29"]}
```

The grading script heavily penalizes missing line numbers:

- With line numbers (strong evidence): 100% evidence score
- Without line numbers (weak evidence): 50% evidence score
- This alone accounts for ~50% of the total score difference!

---

*Created: 2026-01-28*
