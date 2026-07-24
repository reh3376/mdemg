# RUNE-SAFE-STRINGS-001 — Sprint Post

**Date:** 2026-07-24 | **Branch:** `reh3376_dev01`
**Parent:** TSDB-WRITER-UTF8-001's disclosed follow-up.

## Verdict

**Shipped.** All multi-byte-exposed string truncation now routes through
three shared rune-safe primitives in `internal/sanitize`
(`CutRuneSafe` / `CutRuneSafeSuffix` / `TailRuneSafe`). 34 sites fixed
across 33 files; ~12 scattered local helper bodies now delegate to the one
implementation. Byte-budget semantics preserved everywhere (max still means
"at most max bytes"; the cut backs off ≤3 bytes to a rune boundary) — no
storage/prompt budget or ASCII behavior changes, which is why the full test
suite passed untouched.

## Audit honesty note

The from-memory "8 sites" scope was wrong — and the first audit pass was
too: its `head -60` silently truncated the grep output, hiding the
csharp/java parsers, the **consulting hot-path** (`content[:497]` into the
constraint-classify LLM prompt), retrieval profiling, the skills handler,
and the standalone `cmd/ingest-codebase` binary. A residual re-grep after
the first sweep caught them. ⚠️ Lesson: **an audit grep with a `head` limit
is not an audit** — run unbounded, classify every line.

## What was fixed (by destination class)

- **TSDB poison-row class:** `llm_endpoint_health_writer.ErrorMessage`,
  metrics path labels (`normalizePath` → `metric_samples`), ftloop bench
  output tail (→ jobhealth message), issue-filer fingerprint signature.
- **Neo4j names:** constraint/correction node names (`constraint_nodes` /
  `correction_nodes` / `constraint_detector.extractConstraintName`), L5
  emergent identity name, reclassifier names.
- **LLM prompts:** consulting classify content ×2, summarize content, gaps
  interview patterns, ape reflector ×2, jiminy synthesizer/eval/encoder,
  reclassifier sample summaries, ingest content (CLI + standalone binary).
- **API/logs/CLI:** skills description, strict-classifier denial reason,
  scraper preview, embeddings debug previews, llmclient `TruncateForLog`,
  retrieval profiling, demo/data_clean/model_run display, `titleCase`
  first-rune capitalization, kotlin/cuda/cpp/csharp/java/python parser
  signatures/values/docstrings.

Documented-safe and untouched: hex/SHA/CUID cuts, `[]string` slicing,
ASCII-sanitized strings (hidden `cleaned` loops, gaps/codegen/docker slug
sanitizers), guarded ASCII prefixes, code-gen template literals
(scaffold/plugin templates emit a byte-unsafe truncate into *generated
plugin* code — noted as possible follow-up, not mdemg runtime).

## Testing

- **Tier 1:** `runesafe_test.go` — every-byte-position straddle sweeps for
  2/3/4-byte runes on all three primitives; ASCII outputs pinned identical
  to the historical idioms; no-op conventions (max≤0) pinned.
- **Tier 2:** `go build ./...`, `golangci-lint run ./...` (0 issues),
  `go test ./...` full suite green — zero regressions.
- **Tier 3 (live):** new binary deployed via launchd kickstart; observed a
  193-byte CJK `never delete 生产数据库…` constraint observation on scratch
  space `rune-smoke-001` (crafted so byte 120 lands mid-rune); consolidation
  promoted it; **Neo4j constraint node name landed at 118 bytes, valid
  UTF-8, boundary-clean** — the exact `constraint_nodes.go` fixed line,
  proven end-to-end. Pre-fix the name would have ended in two orphan bytes
  of a split 违.

## Rules pinned

1. **Never hand-roll `s[:n]` on content that can contain multi-byte runes**
   — route through `internal/sanitize` (`CutRuneSafe`/`CutRuneSafeSuffix`/
   `TailRuneSafe`). Byte-index cuts at ASCII search results
   (`IndexAny`/`LastIndex` of ASCII chars) are inherently boundary-safe and
   exempt.
2. Audit greps must run unbounded — a `head`-limited audit hid 10 sites.

## Cleanup / state

Scratch space `rune-smoke-001` (2 nodes) left in place per uats-scratch
precedent — below all ORPHAN-ALERT-001 significance floors. Server runs the
shipped binary (deploy was the smoke prerequisite; no state to restore).

## Documents Accessed

`sprint_plan.md` (this dir); TSDB-WRITER-UTF8-001 context (CLAUDE.md notes,
`internal/llmclient/client.go`, `internal/guardrail/guardrail.go`);
unbounded audit greps (fixed-length / variable-bound / tail patterns); all
33 touched files; live `rune-smoke-001` Neo4j verification output.
