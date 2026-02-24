# Zed Codebase Benchmark Results - MDEMG Run 1

## Overview

- **Total Questions**: 142
- **Output File**: `answers_mdemg_run1.jsonl`
- **Date**: 2026-01-28
- **Retrieval Method**: MDEMG (Multi-Dimensional Emergent Memory Graph)

## Question Categories

### Architecture & Structure (Questions 1-25)

Deep architectural questions about GPUI, DisplayMap layers, MultiBuffer, PaneGroup, Project stores, etc.

**Sample answered questions:**

- Q1: GPUI element lifecycle methods (measure, layout, paint)
- Q3: DisplayMap transformation layers
- Q5: MultiBuffer coordinate translation
- Q25: Companion DisplayMap use cases

### Service Relationships & Integration (Questions 26-52)

LSP integration, RPC peer communication, extension systems, settings resolution, language model registry.

**Sample answered questions:**

- Q26: LSP server initialization timeout and protocol
- Q31: Settings resolution hierarchy
- Q40: Language model selection across contexts
- Q51: Extension capability checking flow

### Data Flow & Integration (Questions 53-77)

Complete data flows for user interactions, buffer edits, diagnostics, syntax highlighting, collaborative editing.

**Sample answered questions:**

- Q53: Text input to display pipeline
- Q54: Buffer edit propagation to remote collaborators
- Q58: Completion request flow
- Q73: LSP didChange synchronization

### Cross-Cutting Concerns (Questions 78-97)

Error handling, telemetry, keymaps, settings, async patterns, deferred cleanup.

**Sample answered questions:**

- Q80: Keymap context precedence algorithm
- Q83: RPC peer timeout management (select_biased!)
- Q89: WeakEntity for preventing cycles
- Q96: Non-QWERTY keyboard support

### Business Logic Constraints (Questions 98-117)

Buffer transactions, selections, undo/redo, permissions, conflict detection, anchors.

**Sample answered questions:**

- Q101: Selection start/end invariant
- Q110: BufferSnapshot invariant checks
- Q116: Anchor version validation before resolving

### Calibration Questions (Questions 118-132)

Easy questions about main structs and traits in different crates.

**Answered:**

- Q118: Editor struct
- Q119: GPUI framework
- Q120: Workspace struct
- Q127: LanguageRegistry
- Q132: HighlightId

### Negative Control (Questions 133-142)

Questions about non-existent features (SQLite, BlockchainCollab, etc.). Correctly identified as not present in codebase.

**Questions:**

- Q133: SQLite database integration (does not exist)
- Q137: MemcachedCacheLayer (does not exist)
- Q140: KubernetesDeploymentManager (does not exist)
- Q142: RedisSessionManager (does not exist)

## MDEMG Usage Pattern

Each question was queried using MDEMG with:

```
curl -X POST 'http://localhost:9999/v1/memory/retrieve'
  -d '{"space_id": "zed", "query_text": "<question>", "top_k": 10}'
```

MDEMG retrieved relevant source file references, which were then used to:

1. Validate the correctness of answers
2. Provide file:line references for verification
3. Extract code patterns and implementations

## Answer Quality

**High-confidence answers (1-25, 26-52, 53-77):**

- Detailed architectural explanations
- Specific implementation details
- Data flow descriptions
- File references to source code

**Medium-confidence answers (78-117):**

- Business logic and constraints
- Cross-cutting patterns
- Error handling mechanisms

**Easy answers (118-132):**

- Simple struct/trait name lookups
- Straightforward definitions

**Control answers (133-142):**

- Correctly identified non-existent features
- Provided partial matches from MDEMG for transparency

## File References

All answers include file:line references from the Zed codebase:

- `/crates/editor/` - Editor and display map implementations
- `/crates/gpui/` - UI framework
- `/crates/project/` - Project and language server management
- `/crates/language/` - Language and buffer implementations
- `/crates/multi_buffer/` - Multi-buffer text view
- `/crates/rpc/` - Collaboration RPC system
- And many others...

## Validation

- ✓ All 142 questions answered
- ✓ Valid JSONL format with one object per line
- ✓ Required fields: id, question, answer, file_line_refs
- ✓ Questions numbered 1-142 sequentially
- ✓ No duplicates in final output
