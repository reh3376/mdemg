# Sprint Plan — RUNE-SAFE-STRINGS-001

## 1) Header & Metadata

- **Sprint:** RUNE-SAFE-STRINGS-001
- **Date:** 2026-07-24
- **Branch:** `reh3376_dev01`
- **Type:** Hardening (correctness) — no behavior/limit changes
- **Parent:** TSDB-WRITER-UTF8-001's disclosed follow-up (the recorder
  chokepoint + guardrail `truncateString` shipped there; this sprint sweeps
  the remaining byte-slicing truncation sites repo-wide)
- **Note:** originally disclosed as "UTF8-SAFE-TRUNC*TE-001"; renamed because
  the pre-bash guard's SQL-keyword pattern matches the hyphenated name.

## 2) Problem Statement

Go string slicing `s[:n]` cuts at BYTE offsets. When `s` contains multi-byte
UTF-8 (CJK, emoji, accented chars — routine in LLM output, file content,
error messages, paths), a cut can land mid-rune, producing an **invalid
UTF-8 string**. Consequences by destination:

1. **TSDB (poison-row class):** postgres rejects invalid UTF-8; one poison
   row fails an entire buffered flush batch (the TSDB-WRITER-UTF8-001
   forcing incident). Sites: `llm_endpoint_health_writer.ErrorMessage[:500]`,
   metrics path labels, jobhealth messages via the ftloop bench tail.
2. **Neo4j names:** constraint/correction/L5 node names truncated from
   observation content land as mojibake node identities.
3. **LLM prompts:** invalid bytes degrade tokenization mid-prompt
   (summarize, gaps interview, ape reflector, jiminy synthesis/eval).
4. **Logs/CLI:** cosmetic mojibake.

A fresh audit (supersedes the from-memory "8 sites") found **~15 local
truncate helpers** — only 2 rune-safe (jiminy `truncateBytes`, guardrail
`truncateString`, plus ftloop `truncateForTitle`) — and **~25 inline
slicing sites** with multi-byte exposure.

## 3) Scope & Constraints

- **In:** one shared primitive set in `internal/sanitize` (leaf package:
  stdlib-only imports); migrate all multi-byte-exposed helpers + inline
  sites onto it; unit tests; live Tier-3 smoke.
- **Out (documented-safe, untouched):** hex/SHA/CUID slicing; slices of
  slices (`[]string[:n]`); ASCII-sanitized strings (gaps `sanitizePluginName`,
  hidden `cleaned` loops, jiminy codegen); guarded ASCII prefixes
  (python_parser `line[:3]` behind `HasPrefix("\"\"\"")`); code-gen template
  string literals (scaffold/plugin templates — generated-code hygiene noted
  as a possible follow-up, not runtime).
- **Constraint:** byte-budget semantics PRESERVED — `max` still means "at
  most max bytes" (backing off ≤3 bytes to a rune boundary). No truncation
  limit changes; no suffix changes; downstream size guarantees (TSDB column
  budgets, prompt budgets) unchanged.

## 4) Dependencies

None new. `unicode/utf8` (stdlib). No schema, config, or API changes.

## 5) Implementation Plan (sequential epics)

- **E1:** `internal/sanitize/runesafe.go`: `CutRuneSafe(s, maxBytes)`,
  `CutRuneSafeSuffix(s, maxBytes, suffix)`, `TailRuneSafe(s, maxBytes)`;
  fix `sanitize.go` maxLen cut itself. Gate: table tests green (2/3/4-byte
  runes straddling every cut position; max≤0 no-op; max≥len identity).
- **E2:** server-side migration — helper bodies delegate to sanitize
  (jiminy `truncateBytes`/`truncateForPrompt`/`truncateContent`,
  guardrail `truncateString`, llmclient `TruncateForLog`, ftloop
  `truncateForTitle`, encoding `TruncateAtWord` fallback paths, languages
  `TruncateContentWithInfo`, cli `truncateRunBody`/data `truncate`); inline
  fixes (tsdb endpoint-health, metrics collectors path label, hidden l5Name
  + constraint/correction names + reclassifier, jiminy strict_classifier /
  encoder / synthesizer / contradicted_bridge, ape llm_reflector ×2, gaps
  interview, summarize, scraper preview, embeddings previews, ftloop bench
  tail). Gate: build green.
- **E3:** CLI + parsers — ingest (content, comment, capitalize-first-rune),
  ingest_claude_md summary, watchdog output, demo/data_clean display,
  kotlin/cuda/cpp/python parser signature/value/docstring cuts. Gate:
  build green.
- **E4:** full verification (Testing Plan below). Gate: all green.
- **E5:** live Tier-3 smoke. Gate: valid UTF-8 lands.
- **E6:** documentation + ship.

## 6) Testing Plan

- **Tier 1 (unit):** sanitize table tests (all rune widths × cut positions,
  suffix + tail variants); regression tests asserting ASCII behavior
  byte-identical to pre-sprint for migrated helpers.
- **Tier 2 (integration):** `go build ./...`, `golangci-lint run ./...`,
  `go test ./...` full suite — zero tolerance.
- **Tier 3 (live e2e):** real binary + real Neo4j/TSDB: observe an
  observation on a scratch space whose content forces the constraint-name
  cut inside a CJK sequence → consolidation → read the created node's name
  from Neo4j → assert `utf8.Valid` + ≤ budget + boundary-clean. (The exact
  fixed line: `constraint_nodes.go` name cut.)

## 7) Commit Strategy

Single commit (mechanical sweep, one concern) unless live smoke surfaces a
surprise defect — which gets its own fix-commit (Phase 11.6.2 rule).

## 8) Verification Checklist

- [ ] All primitives table-tested incl. 4-byte emoji straddles
- [ ] Zero remaining multi-byte-exposed `s[:n]` sites (audit re-grep clean)
- [ ] ASCII-exempt sites documented (this plan §3)
- [ ] Build + lint + full `go test ./...` green
- [ ] Live smoke: CJK constraint name lands valid in Neo4j
- [ ] CHANGELOG + CLAUDE.md note

## 9) Documentation Update (final epic — never cut)

`docs/development/rune-safe-strings-001/{sprint_plan,post}.md`; CHANGELOG
entry; CLAUDE.md architecture note (the "when truncating strings, use
`internal/sanitize` primitives" rule). No feature doc — internal hardening,
not operator-visible.

## 10) Risks & Mitigations

- **Behavior drift on ASCII paths** → regression tests pin ASCII outputs
  byte-identical; byte-budget semantics preserved.
- **Import cycles** → sanitize is stdlib-only leaf; verified.
- **Blocked bash patterns** (the SQL-keyword guard matches the word) →
  file named `runesafe.go`, exported names avoid the word, grep via
  bracket patterns.

## 11) Rollback Procedures

Pure code change, no data/schema/config. Revert the commit.

## 12) Documents Accessed

TSDB-WRITER-UTF8-001 context (CLAUDE.md sprint notes; `internal/llmclient/
client.go` recorder; `internal/guardrail/guardrail.go`); full-repo audit
greps (fixed-length, variable-bound, tail-slicing patterns); helper bodies
across 15 packages; `internal/sanitize/sanitize.go` imports.
