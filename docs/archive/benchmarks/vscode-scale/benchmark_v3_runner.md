# Benchmark V3 - Hard Multi-File Questions

## Test Design

These 20 questions are designed to:

1. **Require cross-file correlation** - Each question spans 3+ files
2. **Need specific values** - Constants, enum values, string literals
3. **Exceed memorization** - Combinations unlikely in training data
4. **Force file access** - Answers cannot be guessed

## Monitoring Requirements

### Baseline Agent Monitoring

- **NO tool access** - Pure LLM inference
- Track: Every answer attempt and confidence level
- Expected: Low accuracy, many "unknown" responses

### MDEMG Agent Monitoring

Must log:

1. **Every MDEMG query** - Full query text
2. **Every result** - File paths returned, scores
3. **Answer derivation** - Which results informed each answer
4. **Total queries made** - Count per question

### Output Format

Each agent must output:

```json
{
  "question_id": "hard_001",
  "queries_made": [
    {"query": "extension activation flow timeout", "results": ["path1", "path2"], "scores": [0.85, 0.72]}
  ],
  "files_accessed": ["src/vs/.../file1.ts", "src/vs/.../file2.ts"],
  "answer": {
    "method_chain": ["methodA", "methodB", "methodC"],
    "timeout_ms": 60000
  },
  "confidence": "high|medium|low|unknown",
  "evidence_found": true
}
```

## Scoring Criteria

| Level | Criteria | Points |
|-------|----------|--------|
| Full | All parts correct with evidence | 4 |
| Partial | 2-3 parts correct | 2-3 |
| Minimal | 1 part correct | 1 |
| Wrong | Incorrect or no answer | 0 |

**Maximum score**: 80 points (20 questions × 4 points)

## Expected Outcomes

| Agent | Expected Score | Rationale |
|-------|----------------|-----------|
| Baseline | 0-20 (0-25%) | Cross-file questions defeat memorization |
| MDEMG | 50-70 (60-85%) | Graph retrieval enables correlation |

The **delta should be 40-60%** for truly hard multi-file questions.
