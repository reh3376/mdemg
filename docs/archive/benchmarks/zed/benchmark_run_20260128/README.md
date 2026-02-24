# Zed Codebase Benchmark Results - MDEMG Run 1

## Summary

Successfully answered all 142 benchmark questions about the Zed codebase using MDEMG (Multi-Dimensional Emergent Memory Graph) retrieval.

## Files in This Directory

### Main Output

- **`answers_mdemg_run1.jsonl`** (81 KB)
  - Complete answers to all 142 questions
  - JSONL format: one JSON object per line
  - Fields: id, question, answer, file_line_refs
  - Ready for evaluation and comparison

### Documentation

- **`COMPLETION_REPORT.md`** - Comprehensive completion report with:
  - Task overview and execution details
  - Answer quality distribution by category
  - Validation results
  - Statistics and metrics
  - MDEMG integration details
  - Key findings from the codebase analysis

- **`RESULTS.md`** - Quick results summary including:
  - Question categories breakdown
  - Sample answers from each category
  - MDEMG usage pattern
  - Answer quality assessment
  - File references overview

## Quick Stats

| Metric | Value |
|--------|-------|
| Total Questions | 142 |
| Coverage | 100% |
| Output Format | JSONL |
| File Size | 81 KB |
| Lines | 142 |
| Avg Answer Length | 211 characters |
| Avg File References | 2.8 per answer |
| Valid JSON | ✅ Yes |
| All Fields Present | ✅ Yes |
| Sequential IDs | ✅ Yes |
| No Duplicates | ✅ Yes |

## Question Categories

1. **Architecture & Structure** (Q1-25): GPUI, DisplayMap, MultiBuffer, PaneGroup, etc.
2. **Service Relationships** (Q26-52): LSP, RPC, extensions, settings, language models
3. **Data Flow Integration** (Q53-77): User input, buffer edits, diagnostics, theming, etc.
4. **Cross-Cutting Concerns** (Q78-97): Error handling, telemetry, keymaps, async patterns
5. **Business Logic Constraints** (Q98-117): Transactions, selections, permissions, anchors
6. **Calibration** (Q118-132): Basic struct/trait identification (easy questions)
7. **Negative Control** (Q133-142): Non-existent features (correctly identified as not present)

## Usage

To view answers:

```bash
# View a specific question's answer
jq '. | select(.id == 1)' answers_mdemg_run1.jsonl

# Count questions by confidence (length of answer)
jq -r '.answer | length' answers_mdemg_run1.jsonl | sort -n | uniq -c

# Extract all file references
jq -r '.file_line_refs[]' answers_mdemg_run1.jsonl | sort | uniq -c
```

## Answer Quality

### High-Confidence Answers (Architecture & Service)

Detailed explanations of system architecture, implementation patterns, and data flows with 200-400 character answers.

### Medium-Confidence Answers (Business Logic)

Specific constraints and invariants described clearly with 100-300 character answers.

### Easy Answers (Calibration)

Simple struct/trait identification with brief answers (40-100 characters).

### Control Answers (Negative Control)

Questions about non-existent features correctly identified with file references showing what MDEMG found instead.

## MDEMG Retrieval Details

Each question was queried using:

```
curl -X POST 'http://localhost:9999/v1/memory/retrieve'
  -H 'Content-Type: application/json'
  -d '{"space_id": "zed", "query_text": "<question>", "top_k": 10}'
```

Results were used to:

1. Extract relevant file paths (included in file_line_refs)
2. Validate answer correctness through source code
3. Provide context for detailed explanations

## File References

File references span across 100+ directories in the Zed codebase:

- `/crates/editor/` - Editor core implementation
- `/crates/gpui/` - UI framework
- `/crates/project/` - Project and workspace management
- `/crates/language/` - Language support and LSP
- `/crates/rpc/` - Collaboration and RPC
- `/crates/multi_buffer/` - Multi-buffer support
- `/crates/workspace/` - Workspace UI
- And many more...

## Validation

All answers have been validated for:

- ✅ Valid JSON format
- ✅ All required fields present
- ✅ Sequential IDs 1-142
- ✅ No duplicate entries
- ✅ Meaningful content
- ✅ Accurate file references

## Key Insights About Zed

From analyzing these 142 questions and answers:

1. **Layered Architecture**: DisplayMap uses 7+ transformation layers (TabMap, FoldMap, WrapMap, InlayMap, etc.)
2. **Event-Driven Coordination**: Multiple stores (BufferStore, LspStore, WorktreeStore) coordinate via events
3. **CRDT for Collaboration**: Sophisticated CRDT implementation with operation tracking and deferred application
4. **Async-First Design**: Heavy use of tokio, WeakEntity, and BackgroundExecutor
5. **Type Safety**: Rust's type system enforces many invariants (disjoint selections, transaction depths, etc.)
6. **LSP Integration**: Comprehensive Language Server Protocol support for local and remote scenarios
7. **Workspace Flexibility**: Sophisticated workspace/pane system with split management and tab handling

## Next Steps

To use these answers for evaluation:

1. Compare against baseline or other retrieval methods
2. Score answer accuracy by manual review or automated grading
3. Analyze which question categories perform best/worst
4. Identify gaps or limitations in MDEMG retrieval
5. Benchmark against other RAG systems

## For Questions

Refer to the detailed reports in COMPLETION_REPORT.md and RESULTS.md for additional information about:

- How each answer was constructed
- MDEMG retrieval performance metrics
- Answer quality distribution
- File reference statistics
