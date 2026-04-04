---
created: 2026-03-23
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: J17-PC
---

# J17-PC: Internal LLM Caller Prompt Compression

## Summary

**Feature**: J17 Prompt Compression
**Summary**: Applies J17 AI-to-AI protocol compression utilities to the inputs of the 5 highest-value internal LLM callers, reducing aggregate token consumption by an estimated 25-35% with zero quality loss.

## Vision & Goals

MDEMG has 16 internal LLM callers. Each consumes tokens for system prompts, context, and input formatting. J17-PC takes the proven compression utilities from the J17 protocol (`TelegraphicCompress`, `CodedEncoder`) — originally used only for Jiminy guidance output — and applies them to LLM *inputs*. Per-caller optimization gives surgical control: prose sections compress aggressively while code diffs, JSON schemas, and enum values remain verbatim.

## Current State

### Architecture

**Shared Utilities** (`internal/encoding/compact.go`):

```go
CompactJSON(v any) string              // json.Marshal wrapper (single-line, no indentation)
TruncateAtWord(s string, maxLen int)   // Word-boundary truncation + "..."
CompressSection(s string, maxLen int)  // TelegraphicCompress + TruncateAtWord
```

**Per-Caller Opt-In Pattern** — each caller has a `CompressPrompts bool` config field, a `*_COMPRESS` env var (default: `true`), and both compressed/uncompressed code paths for A/B testing.

### Workflow

**5 High-Value Callers with Compression Strategies:**

| Caller | Savings | Strategy |
|--------|---------|----------|
| RSIC LLM Reflector | 40-50% | CompactJSON replaces MarshalIndent, criteria truncation |
| LLM Reranking (highest frequency) | 30-40% | Summary truncation (300 chars), condensed preamble, pipe-separated candidates |
| SME Synthesis | 25-35% | CompressSection(400), concepts capped at 10, shorter framing |
| Guardrail Evaluation | 20-30% | Compact system prompt (17->6 lines), pipe-separated constraints, code diffs verbatim |
| Outcome Classification | 20-30% | Compact system prompt (14->3 lines), removed redundant Task section |

### Configuration

See Configuration Reference table below. All default to `true`. Set to `false` for debugging or A/B testing.

## Notes

### Known Limitations

- Compression is prose-only — code diffs, JSON schemas, enum values are never compressed
- Savings estimates are based on prompt analysis, not runtime measurement

### Risks & Gaps

None identified — all compression targets confirmed safe with 14 new tests.

### Future Improvements

- Runtime token measurement to validate estimated savings
- Adaptive compression (compress more when context is tight)

## API Endpoints

None — compression is internal to LLM callers, not exposed via API.

## CLI Commands

None — compression controlled via env vars.

## Configuration Reference

| Env Var | Default | Description |
|---------|---------|-------------|
| `RSIC_LLM_REFLECT_COMPRESS` | `true` | Compress RSIC reflection prompts |
| `RERANK_COMPRESS` | `true` | Compress rerank candidate prompts |
| `SYNTHESIS_COMPRESS` | `true` | Compress SME synthesis prompts |
| `GUARDRAIL_COMPRESS` | `true` | Compress guardrail eval prompts |
| `JIMINY_CLASSIFY_COMPRESS` | `true` | Compress outcome classification prompts |

## Dependencies

| Feature | Relationship |
|---------|-------------|
| J17 AI-to-AI Protocol | Requires — compression utilities from J17 encoding package |
| LLM Client Infrastructure | Requires — all 5 callers use internal/llmclient |
| RSIC Reflector | Enhances — compressed reflection prompts |
| Retrieval Reranking | Enhances — compressed candidate formatting |
| Jiminy Guidance | Enhances — compressed outcome classification |

## Related Files

- `internal/encoding/compact.go` - CompactJSON, TruncateAtWord, CompressSection
- `internal/ape/llm_reflector.go` - RSIC LLM reflector compression
- `internal/retrieval/rerank.go` - LLM reranking compression
- `internal/consulting/synthesis.go` - SME synthesis compression
- `internal/guardrail/prompt.go` - Guardrail prompt builder compression
- `internal/jiminy/outcome_classifier.go` - Outcome classifier compression
- `internal/config/config.go` - 5 `*_COMPRESS` config fields
