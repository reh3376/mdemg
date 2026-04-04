---
created: 2026-04-02
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: FT-INFRA
---

# LLM Response Sanitization

## Summary

**Feature**: LLM Response Sanitization
**Summary**: `SanitizeResponse()` is the unified response cleaning pipeline for all 16 LLM consumers. Strips think blocks and code fences before JSON parsing, enabling local model deployment with think mode (Qwen3).

## Vision & Goals

Local model deployment (Qwen3 via vllm-mlx with think mode) prepends `<think>...</think>` blocks before JSON output, breaking all 16 LLM consumers' JSON parsers. A unified sanitization pipeline ensures any model's output format works consistently across the entire codebase, eliminating duplicated cleanup code and enabling future model-specific profiles.

## Current State

### Architecture

Three functions in `internal/llmclient/sanitize.go`:

1. **StripThinkBlock(s string) string** — Removes `<think>...</think>` blocks including nested tags, multiline content, and leading whitespace
2. **StripCodeFence(s string) string** — Removes ````json ... ```` wrappers (consolidated from 6 duplicated call sites)
3. **SanitizeResponse(s string) string** — Applies both in order, then TrimSpace

### Workflow

**11 Call Sites in 10 Files:**

| # | File | Function |
|---|------|----------|
| 1 | `internal/jiminy/outcome_classifier.go` | Classify() |
| 2 | `internal/jiminy/eval_prompt.go` | parseEvalResponse() |
| 3 | `internal/consulting/llm_classifier.go` | parseConstraintClassifierResponse() |
| 4 | `internal/retrieval/query_classifier.go` | parseQueryClassifierResponse() |
| 5 | `internal/retrieval/rerank.go` | parseScores() |
| 6 | `internal/ape/llm_reflector.go` | parseLLMReflectResponse() |
| 7 | `internal/hidden/emergence_namer.go` | parseEmergenceNamingResponse() |
| 8 | `internal/hidden/reclassifier.go` | parseLLMSubCategories() |
| 9 | `internal/metalearn/generalizer.go` | parseGeneralizerResponse() |
| 10-11 | `internal/summarize/service.go` | Summarize(), SummarizeBatch() |

**System Prompt Hash** — `InteractionRecord` now includes `SystemPromptHash string` (SHA-256) enabling training data curation by prompt version and stale data filtering.

### Configuration

No configuration — sanitization is always applied. Future: automatic model-specific profiles.

## Notes

### Known Limitations

- Think block regex assumes `<think>...</think>` format — other think block formats not supported
- No partial JSON recovery (`CompleteJSON` planned but not built)

### Risks & Gaps

None identified.

### Future Improvements

- `CompleteJSON` — format retry logic for partially-formed JSON
- Automatic model-specific sanitization profiles

## API Endpoints

None — internal infrastructure, not exposed via API.

## CLI Commands

None — automatic, internal to all LLM consumers.

## Configuration Reference

None — always active, no configurable parameters.

## Dependencies

| Feature | Relationship |
|---------|-------------|
| LLM Client (`internal/llmclient`) | Requires — sanitization applied in response parsing |
| All 16 LLM Consumers | Feeds into — every consumer benefits from unified sanitization |
| Training Data Pipeline | Enhances — SystemPromptHash enables prompt-version curation |

## Related Files

- `internal/llmclient/sanitize.go` - StripThinkBlock, StripCodeFence, SanitizeResponse
- `internal/llmclient/sanitize_test.go` - Sanitization unit tests
- `internal/llmclient/client.go` - SystemPromptHash computation in recordInteraction()
- `internal/llmclient/recorder.go` - InteractionRecord with SystemPromptHash field
