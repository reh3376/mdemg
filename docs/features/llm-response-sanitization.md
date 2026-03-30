# LLM Response Sanitization

## Overview
SanitizeResponse() is the unified response cleaning pipeline for all 16 LLM consumers. It strips think blocks and code fences before JSON parsing, enabling local model deployment with think mode (Qwen3).

## Problem
9 of 16 LLM consumers call json.Unmarshal directly on raw responses. When models use think mode (e.g., Qwen3's `<think>...</think>` blocks), the prepended content breaks all JSON parsers. Additionally, 6 consumers had duplicated StripCodeFence() calls.

## Solution
Three functions in `internal/llmclient/sanitize.go`:

1. **StripThinkBlock(s string) string** — Removes `<think>...</think>` blocks including nested tags, multiline content, and leading whitespace
2. **StripCodeFence(s string) string** — Removes ```json ... ``` wrappers (consolidated from multiple call sites)
3. **SanitizeResponse(s string) string** — Applies both in order, then TrimSpace

## Call Sites (11 in 10 files)

| # | File | Function | Previous | New |
|---|------|----------|----------|-----|
| 1 | internal/jiminy/outcome_classifier.go | Classify() | Inline CutPrefix fence strip | SanitizeResponse() before existing |
| 2 | internal/jiminy/eval_prompt.go | parseEvalResponse() | cleanEvalJSONResponse() | SanitizeResponse() before existing |
| 3 | internal/consulting/llm_classifier.go | parseConstraintClassifierResponse() | StripCodeFence() | SanitizeResponse() |
| 4 | internal/retrieval/query_classifier.go | parseQueryClassifierResponse() | StripCodeFence() | SanitizeResponse() |
| 5 | internal/retrieval/rerank.go | parseScores() | Bracket scanning | SanitizeResponse() before bracket scan |
| 6 | internal/ape/llm_reflector.go | parseLLMReflectResponse() | StripCodeFence() | SanitizeResponse() |
| 7 | internal/hidden/emergence_namer.go | parseEmergenceNamingResponse() | StripCodeFence() | SanitizeResponse() |
| 8 | internal/hidden/reclassifier.go | parseLLMSubCategories() | StripCodeFence() | SanitizeResponse() |
| 9 | internal/metalearn/generalizer.go | parseGeneralizerResponse() | StripCodeFence() | SanitizeResponse() |
| 10 | internal/summarize/service.go | Summarize() | StripCodeFence() | SanitizeResponse() |
| 11 | internal/summarize/service.go | SummarizeBatch() | StripCodeFence() | SanitizeResponse() |

## Think Mode Interaction
When Ollama serves Qwen3 with think_mode enabled (via vllm-mlx reasoning parser), the model prepends `<think>reasoning...</think>` before the JSON output. StripThinkBlock removes this content before any parse attempt.

## System Prompt Hash
InteractionRecord now includes `SystemPromptHash string` — SHA-256 of the system prompt used for each LLM call. This enables:
- Training data curation by prompt version
- Stale data filtering when prompts change
- ULTS spec hash verification

Computed in `recordInteraction()` in `internal/llmclient/client.go`.

## Future Work
- `CompleteJSON` — format retry logic for partially-formed JSON (planned, not yet built)
- Automatic model-specific sanitization profiles

## Documents Accessed
- internal/llmclient/sanitize.go
- internal/llmclient/sanitize_test.go
- internal/llmclient/client.go
- internal/llmclient/recorder.go
- docs/development/ft-lora/ft-lora-dev/MDEMG_FT_PLAN_DEEP_DIVE_ANALYSIS_v2.md
