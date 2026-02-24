# Benchmark Agent Rules - DEFACTO STANDARD

**Version:** 1.0
**Applies to:** All codebase benchmark testing

---

## MANDATORY CONSTRAINTS (Violation = Disqualification)

### 1. File Access Restrictions

**ALLOWED:**

- Files within the target repository ONLY
- For blueseer: `/Users/reh3376/repos/blueseer/**/*`

**FORBIDDEN (Immediate Disqualification):**

- ❌ Web search (WebSearch, WebFetch)
- ❌ Files outside target repo
- ❌ Other repositories
- ❌ System files
- ❌ MDEMG source code (for baseline runs)

### 2. Tool Restrictions

**Baseline Agents - ALLOWED TOOLS:**

- `Glob` - File pattern matching within repo
- `Grep` - Content search within repo
- `Read` - Read files within repo
- `Bash` - ONLY for `ls`, `wc`, `head`, `tail` within repo
- `Write` - ONLY for writing answer file to `/tmp/`

**Baseline Agents - FORBIDDEN TOOLS:**

- ❌ `WebSearch`
- ❌ `WebFetch`
- ❌ `Task` (no sub-agents)
- ❌ Any MDEMG/curl commands

**MDEMG Agents - ALLOWED TOOLS:**

- All Baseline tools PLUS:
- `Bash` - For MDEMG curl commands to localhost:9999 ONLY

### 3. Completion Requirements

- **ALL 140 questions MUST be answered**
- No skipping questions
- No partial runs
- Answer format: JSONL with required fields

### 4. Answer Format (MANDATORY)

```json
{"id": 1, "question": "...", "answer": "...", "file_line_refs": ["file.java:123"]}
```

Required fields:

- `id`: Question ID (integer)
- `question`: Full question text
- `answer`: Comprehensive answer with file:line citations
- `file_line_refs`: Array of specific file:line references

### 5. Question ID Integrity (CRITICAL)

**YOU MUST:**

- Use the EXACT question ID from the input JSON for each answer
- Copy the EXACT question text from the input JSON
- Maintain 1:1 mapping between input question ID and output answer ID
- Process questions in order (1, 2, 3, ... 140)

**DO NOT:**

- ❌ Invent your own question IDs
- ❌ Renumber questions
- ❌ Skip question IDs
- ❌ Answer questions out of order
- ❌ Duplicate question IDs

**Example - CORRECT:**

```
Input:  {"id": 47, "question": "How does X work?", ...}
Output: {"id": 47, "question": "How does X work?", "answer": "...", "file_line_refs": [...]}
```

**Example - WRONG (will invalidate results):**

```
Input:  {"id": 47, "question": "How does X work?", ...}
Output: {"id": 1, "question": "How does X work?", ...}  ← WRONG ID!
```

**Verification:** After completion, output file must have exactly 140 lines with IDs 1-140 matching input.

---

## MONITORING REQUIREMENTS

### Orchestrator Responsibilities

1. **Run agents in FOREGROUND** (not background)
2. **Monitor every tool call** for:
   - Forbidden tool usage
   - Out-of-scope file access
   - Web search attempts
3. **Track progress**: Questions answered / 140
4. **Track tool usage**: Count per tool type
5. **Immediate termination** if rules violated

### Disqualification Triggers

| Violation | Action |
|-----------|--------|
| WebSearch/WebFetch call | STOP immediately, mark run DISQUALIFIED |
| File read outside repo | STOP immediately, mark run DISQUALIFIED |
| Accessing other repos | STOP immediately, mark run DISQUALIFIED |
| Incomplete answers (<140) | Mark run INCOMPLETE |
| Missing file_line_refs | Deduct points in grading |

---

## AGENT PROMPT TEMPLATES

### Baseline Agent Prompt

```
You are answering benchmark questions about the BlueSeer Java ERP codebase.

## STRICT RULES - VIOLATION = DISQUALIFICATION
1. You may ONLY access files within: /Users/reh3376/repos/blueseer/
2. You may NOT use WebSearch or WebFetch
3. You may NOT access any other repositories or directories
4. You MUST answer ALL 140 questions
5. You MUST include file:line references in every answer

## CRITICAL: Question ID Integrity
- Use the EXACT question ID from the input JSON - DO NOT renumber
- Copy the EXACT question text from the input JSON
- Each answer ID must match its corresponding input question ID
- Process questions in order: 1, 2, 3, ... 140
- Output file must have IDs 1-140 matching the input file exactly

## Your Task
Answer all questions in {QUESTION_FILE} using ONLY file search within the blueseer repo.

## Output Format
Write answers to {OUTPUT_FILE} in JSONL format (one line per answer):
{"id": <SAME_ID_FROM_INPUT>, "question": "<EXACT_QUESTION_FROM_INPUT>", "answer": "...", "file_line_refs": ["file.java:123"]}

## Allowed Tools
- Glob: Search for files by pattern
- Grep: Search file contents
- Read: Read file contents

## Begin
Read the question file and answer each question systematically, preserving question IDs exactly.
```

### MDEMG Agent Prompt

```
You are answering benchmark questions about the BlueSeer Java ERP codebase.

## STRICT RULES - VIOLATION = DISQUALIFICATION
1. You may ONLY access files within: /Users/reh3376/repos/blueseer/
2. You may NOT use WebSearch or WebFetch
3. You may NOT access any other repositories or directories
4. You MUST answer ALL 140 questions
5. You MUST include file:line references in every answer
6. You MUST use MDEMG retrieval for EVERY question

## CRITICAL: Question ID Integrity
- Use the EXACT question ID from the input JSON - DO NOT renumber
- Copy the EXACT question text from the input JSON
- Each answer ID must match its corresponding input question ID
- Process questions in order: 1, 2, 3, ... 140
- Output file must have IDs 1-140 matching the input file exactly

## Your Task
Answer all questions in {QUESTION_FILE} using MDEMG retrieval + file reading.

## MDEMG Query Pattern
For EVERY question, first query MDEMG:
curl -s -X POST "http://localhost:9999/v1/memory/retrieve" \
  -H "Content-Type: application/json" \
  -d '{"space_id": "blueseer-erp", "query_text": "<question>", "top_k": 10}'

Then read the relevant files from the retrieval results.

## Output Format
Write answers to {OUTPUT_FILE} in JSONL format (one line per answer):
{"id": <SAME_ID_FROM_INPUT>, "question": "<EXACT_QUESTION_FROM_INPUT>", "answer": "...", "file_line_refs": ["file.java:123"]}

## Begin
Read the question file and answer each question systematically using MDEMG, preserving question IDs exactly.
```

---

## EXECUTION CHECKLIST

Before each run:

- [ ] Verify agent prompt includes all STRICT RULES
- [ ] Verify output file path is correct
- [ ] Run in FOREGROUND (not background)
- [ ] Monitor tool usage in real-time
- [ ] Stop immediately on violation

After each run:

- [ ] Verify 140 answers in output file
- [ ] Check no disqualification events
- [ ] Record tool usage counts
- [ ] Record wall time
- [ ] Grade answers

---

*This is the DEFACTO STANDARD for all MDEMG benchmark testing.*
