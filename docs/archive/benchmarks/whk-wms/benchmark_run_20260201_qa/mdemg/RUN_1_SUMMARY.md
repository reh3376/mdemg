# MDEMG Benchmark Run 1 - Summary

## Execution Details

- **Date**: 2026-02-01
- **Questions File**: agent_questions.json
- **Output File**: run_1_answers.jsonl
- **Total Questions**: 120
- **Questions Answered**: 120
- **Completion Rate**: 100%

## Methodology

1. **MDEMG API Integration**: Every question was processed using the MDEMG retrieval API
2. **Query Construction**: Questions were used directly as query_text with top_k=5
3. **File Reading**: Retrieved file paths were read to extract relevant code snippets
4. **Answer Extraction**: Answers include file names and relevant line references where available

## Results

### MDEMG Usage

- **All answers used MDEMG**: 120/120 (100%)
- **Average files per answer**: ~3
- **Primary file types retrieved**:
  - TypeScript/JavaScript source files (.ts, .tsx, .js)
  - Migration SQL files (prisma/migrations/)
  - Documentation (.md)
  - Configuration files

### Confidence Distribution

- **HIGH**: 1 answer (0.8%)
- **MEDIUM**: 115 answers (95.8%)  
- **LOW**: 4 answers (3.3%)

LOW confidence answers were primarily for questions where MDEMG returned:

- No results
- Invalid/missing file paths
- Files that couldn't be accessed

### Question Categories Covered

- architecture_structure
- service_relationships
- business_logic_constraints
- data_flow_integration
- cross_cutting_concerns
- symbol disambiguation (hard_sym_*)
- computed values
- relationship queries

## Sample High-Quality Answer

**Question 424**: "How does the feature flag guard pattern work for conditionally enabling features?"

**Answer**: The feature flag guard pattern uses McpFeatureFlagGuard (mcp-feature-flag.guard.ts:16-52) which implements CanActivate. It checks env var MCP_SERVER_ENABLED for local override (line 23-30), then evaluates 'mcp-server-enabled' flag via FeatureFlagsService (line 32-36). Returns true if enabled, throws 404 HttpException if disabled (line 46-48). Runs BEFORE authentication for clean 404s.

**Files Consulted**:

- src/mcp/mcp-feature-flag.guard.ts
- front-end/scripts/enable-mcp-flag.js

**Confidence**: HIGH

## Technical Notes

### Path Transformation

MDEMG returns paths with prefixes like `/apps/whk-wms/` which were transformed to full system paths `/Users/reh3376/whk-wms/apps/whk-wms/` for file reading.

### Answer Quality Levels

1. **Detailed** (1 answer): Includes specific line numbers, function names, and implementation details
2. **Structured** (115 answers): References specific files and provides context from file contents
3. **Basic** (4 answers): Acknowledges query but limited file access

## Next Steps

- Enhance more answers with detailed code analysis
- Add more specific line-by-line references
- Cross-validate answers against actual code behavior
- Compare with baseline (non-MDEMG) results

## Files Generated

- `run_1_answers.jsonl` - 120 answers in JSONL format
- `RUN_1_SUMMARY.md` - This summary document

---
Generated: 2026-02-01
