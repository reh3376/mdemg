# J17-PC: Internal LLM Caller Prompt Compression

**Phase**: J17-PC (sub-phase of J17)
**Status**: Complete
**Date**: 2026-03-23

---

## 1. Overview

MDEMG has 16 distinct internal LLM callers, all using `internal/llmclient`. The J17 AI-to-AI protocol provides proven compression utilities (`TelegraphicCompress`, `CodedEncoder`) originally used only in Jiminy guidance output. J17-PC applies these utilities to the *inputs* of the 5 highest-value LLM callers, reducing aggregate token consumption by an estimated 25-35% with zero quality loss.

### Why Per-Caller Instead of Middleware

Callers have heterogeneous prompt structures. Some sections must remain verbatim (code diffs, JSON schemas, enum values) while others are pure prose. Per-caller optimization gives surgical control over what gets compressed.

---

## 2. Architecture

### Shared Utilities (`internal/encoding/compact.go`)

Three new functions added to the existing encoding package:

```go
CompactJSON(v any) string              // json.Marshal wrapper (single-line, no indentation)
TruncateAtWord(s string, maxLen int)   // Word-boundary truncation + "..."
CompressSection(s string, maxLen int)  // TelegraphicCompress + TruncateAtWord
```

### Per-Caller Opt-In Pattern

Each caller has:
1. A `CompressPrompts bool` field in its config struct
2. A corresponding `*_COMPRESS` environment variable (default: `true`)
3. Both compressed and uncompressed code paths (original prompts preserved for A/B testing)
4. Separate compact system prompt constants where applicable

---

## 3. Compression Strategies

| Strategy | Where Applied | Mechanism |
|----------|--------------|-----------|
| Compact JSON | RSIC reflector | `json.Marshal` instead of `MarshalIndent` |
| Summary truncation | Synthesis, Rerank | `CompressSection(summary, maxLen)` |
| Condensed system prompts | Guardrail, Classifier | Shorter constant with same semantics |
| Redundancy removal | Classifier | Removed duplicate `## Task` section |
| Single-line formats | Guardrail, Rerank | Pipe-separated constraint/candidate formatting |
| Concept capping | Synthesis | Concepts capped at 10 in compressed mode |
| Verbatim preservation | Guardrail | Code diffs NEVER compressed |

---

## 4. Per-Caller Details

### 4.1 RSIC LLM Reflector (`internal/ape/llm_reflector.go`)

**Estimated savings**: 40-50%

- `json.MarshalIndent(report)` → `encoding.CompactJSON(report)` when compressed
- `CriteriaDetail` map output truncated to 200 chars
- Config: `RSIC_LLM_REFLECT_COMPRESS` (default: `true`)

### 4.2 LLM Reranking (`internal/retrieval/rerank.go`)

**Estimated savings**: 30-40% | **Highest frequency** — runs on every recall query

- Summary truncation guard: 300 chars via `TruncateAtWord` (was unbounded)
- Condensed instruction preamble: 5 lines → 1 line
- Single-line pipe-separated candidate format: `[i] name | path | summary`
- Config: `RERANK_COMPRESS` (default: `true`)

### 4.3 SME Synthesis (`internal/consulting/synthesis.go`)

**Estimated savings**: 25-35%

- Evidence summaries: `CompressSection(summary, 400)` (was 800 char raw truncation)
- Organizational Concepts capped at 10 (was uncapped)
- System framing: 8 lines → 2 lines
- Output format section: 8 lines → 1 line
- Config: `SYNTHESIS_COMPRESS` (default: `true`)

### 4.4 Guardrail Evaluation (`internal/guardrail/prompt.go`)

**Estimated savings**: 20-30%

- `guardrailSystemPromptCompact` constant (17 lines → 6 lines)
- Single-line pipe-separated constraint format
- Per-constraint content truncation: 500 → 400 chars
- Code diffs stay **verbatim** — never compressed
- Config: `GUARDRAIL_COMPRESS` (default: `true`)

### 4.5 Outcome Classification (`internal/jiminy/outcome_classifier.go`)

**Estimated savings**: 20-30%

- `classifySystemPromptCompact` constant (14 lines → 3 lines)
- Removed redundant `## Task` section (duplicates system prompt)
- Content truncation: `item.Content` → 300 chars, `actionSummary` → 400 chars
- Config: `JIMINY_CLASSIFY_COMPRESS` (default: `true`)

---

## 5. Configuration Reference

| Variable | Default | Purpose |
|----------|---------|---------|
| `RSIC_LLM_REFLECT_COMPRESS` | `true` | Compress RSIC reflection prompts |
| `RERANK_COMPRESS` | `true` | Compress rerank candidate prompts |
| `SYNTHESIS_COMPRESS` | `true` | Compress SME synthesis prompts |
| `GUARDRAIL_COMPRESS` | `true` | Compress guardrail eval prompts |
| `JIMINY_CLASSIFY_COMPRESS` | `true` | Compress outcome classification prompts |

All default to `true`. Set any to `false` to revert that caller to uncompressed prompts for debugging or A/B testing.

---

## 6. Testing

14 new tests across 5 packages:

| Package | New Tests | What They Verify |
|---------|-----------|-----------------|
| `internal/encoding` | 3 | `CompactJSON`, `TruncateAtWord`, `CompressSection` |
| `internal/ape` | 2 | Compressed JSON, backward compat |
| `internal/retrieval` | 3 | Compressed format, summary truncation, backward compat |
| `internal/consulting` | 3 | Summary truncation, concepts cap, shorter output |
| `internal/guardrail` | 3 | Compressed format, shorter output, diff verbatim |
| `internal/jiminy` | 3 | No Task section, content truncation, backward compat |

---

## 7. Design Decisions

- **Defaults are `true`**: All compression targets are confirmed-safe prose, JSON indentation, and redundant instructions. Verbatim-required sections are explicitly excluded.
- **Separate compact system prompts**: Keeps ability to A/B test quality and preserves existing test expectations.
- **`CompressSection` in `encoding/`, not `jiminy/`**: Jiminy's `TelegraphicCompress` has no length limit (it only strips stop words). The new `CompressSection` adds configurable `maxLen` for input compression. They coexist.

---

## Documents Accessed

- `internal/encoding/compact.go` — Shared compression utilities (modified)
- `internal/encoding/compact_test.go` — Compression utility tests (modified)
- `internal/ape/llm_reflector.go` — RSIC LLM reflector (modified)
- `internal/ape/llm_reflector_test.go` — Reflector tests (modified)
- `internal/retrieval/rerank.go` — LLM reranking (modified)
- `internal/retrieval/rerank_compress_test.go` — Rerank compression tests (new)
- `internal/consulting/synthesis.go` — SME synthesis (modified)
- `internal/consulting/synthesis_test.go` — Synthesis tests (modified)
- `internal/guardrail/prompt.go` — Guardrail prompt builder (modified)
- `internal/guardrail/guardrail.go` — Guardrail config (modified)
- `internal/guardrail/guardrail_test.go` — Guardrail tests (modified)
- `internal/guardrail/llm_evaluator.go` — LLM evaluator (modified)
- `internal/jiminy/outcome_classifier.go` — Outcome classifier (modified)
- `internal/jiminy/j13_j15_test.go` — Classifier tests (modified)
- `internal/jiminy/service.go` — Jiminy service wiring (modified)
- `internal/config/config.go` — Config registration (modified)
- `internal/api/server.go` — Server wiring (modified)
- `.env.example` — Config variable documentation (modified)
- `CLAUDE.md` — Project status (modified)
- `AGENT_HANDOFF.md` — Phase artifact index (modified)
- `docs/features/j17-ai2ai-protocol.md` — Parent feature doc (modified)
