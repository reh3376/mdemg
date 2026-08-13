# SCRUB-IDEMPOTENT-001 — Sprint Post

**Date:** 2026-08-13
**Branch:** `reh3376_dev01`
**Trigger:** Operator morning triage discovered the export-auto scheduled job was firing HIGH alerts in a loop (6 pending on the alerts channel; 4 fresh failures in ~10 minutes). Root cause: scrubber non-idempotency + 2 pre-existing early-returns in the shipped scrub CLI that made all 3 export-table branches unreachable.

## Problem

The `env_secret` scrub pattern (`(?i)(PASSWORD|SECRET|TOKEN|API_KEY|PRIVATE_KEY)\s*[=:]\s*["']?([^\s"',;]+)["']?`) had a value class `[^\s"',;]+` that included closing punctuation `)` and `]`. On real content like:

- `password = neo4jPass()` — scrubs to `password=[REDACTED])` (drops `(` but leaves `)`)
- `api_key = os.getenv("OPENAI_API_KEY")` — scrubs to `api_key=[REDACTED]OPENAI_API_KEY")` (consumes past the string literal's OPENING quote)

Each subsequent scrub pass consumes more characters. Since intake scrub (`llmclient.Scrub` on the recorder path) runs exactly ONE pass, the STORED value never converges to a fixed point of `ScrubStringExcluding`. The export scan-gate at `internal/tsdb/exporter.go:427-435` compares `stored == Scrub(stored)` and — because those never match — blocks export-auto forever with a HIGH alert. This had been alarming continuously on mdemg-dev throughout the morning triage window.

The `[REDACTED]` placeholder itself was a secondary victim: `api_key=[REDACTED]OPENAI_API_KEY")` on a fresh pass matches `[REDACTED]OPENAI_API_KEY` as the value (`]` was allowed in the value class), gets replaced with `api_key=[REDACTED])`, and only reaches a fixed point after 4 passes total.

## Root cause (2 pieces)

1. **Scrubber regex**: `envSecretPattern`'s value class allowed closing brackets that syntactically belong to enclosing scope (function calls, string literals, list indexing).
2. **CLI early-returns**: `scrub-export-tables` had 2 places where an empty dirty set on an EARLIER branch (guidance_training_rows, retrieval_events) returned from the whole handler before the LATER branches (retrieval, llm) could run. Same class also on `--dry-run` — a single guidance dirty short-circuited previewing the other 2 tables.

## Shipped

**`internal/llmclient/scrubber.go`**:
- `envSecretPattern` value class narrowed from `[^\s"',;]+` to `[^\s"',;)\]]+`. Now stops at `)` and `]`. Keeps `{` `}` `<` `>` in the class so SCRUB-ENV-REF-001's `${VAR}` / `${VAR:-default}` shell-ref preservation continues to work (over-tightening broke those tests during development — the narrow exclusion is the right cut).
- New helper `isRedactedPlaceholder(v)` — belt-and-suspenders defense returning true iff the captured value begins with `[REDACTED`. Called from `scrubEnvSecret` immediately after `isShellEnvVarRef`. Preserves matches whose value is already a scrub placeholder, even if the outer regex later widens for a legitimate reason. Prior art: mirror shape of `SCRUB-ENV-REF-001`'s `isShellEnvVarRef`.

**`internal/llmclient/scrubber_test.go`**:
- New `TestScrub_IsIdempotent` — 18-input pin including the 2 documented failure classes verbatim: `password=neo4jPass()`, `api_key = os.getenv("OPENAI_API_KEY")`, plus real-world composites (shell env refs, multi-secret lines, placeholder-preserved forms, clean text). Asserts `Scrub(s) == Scrub(Scrub(s))` for every input.
- New `TestScrub_PlaceholderPreserved` — 4-case pin locking in the belt-and-suspenders defense: `password=[REDACTED]`, `API_KEY=[REDACTED]`, `token=[REDACTED_KEY]`, `SECRET=[REDACTED_something]` all round-trip unchanged.

**`internal/cli/data_scrub_guidance.go`**:
- Replaced 2 `if len(dirties) == 0 { ... return nil }` early-returns with `if len(dirties) > 0 { ... }` else-branch print. Guidance and retrieval branches now share the flow with the new llm branch.
- Removed 2 `if dryRun { ... return nil }` early-returns from the guidance branch; dry-run now walks ALL three branches.
- Added `llm_interactions.user_prompt` as the 3rd scrub branch. Same shape as retrieval — SELECT/scan/dirty/preview/dry-run/UPDATE. Uses `ScrubStringExcluding(up, nil)` (no skip patterns — matches exporter `llmInteractionsSpec.textFields[6] = nil`).
- Added `SET timescaledb.max_tuples_decompressed_per_dml_transaction = 0` before the llm UPDATE loop — historical llm_interactions chunks are compressed and older ones exceed the per-txn cap under normal setting. Best-effort; per-row UPDATE errors still surface.

## Live Tier-3 (mdemg-dev, 2026-08-13)

Before the fix (this morning's operator triage):
- `guidance_training_rows.action_summary` dirty: 7 rows across 3 sweeps
- `llm_interactions.user_prompt` dirty: 32 rows (across 2 apply passes, needed compression-cap raise for 3 rows on older chunks)
- Export-auto: 10 → 5 → 5 → 3 violations across successive attempts; each attempt fired HIGH `scheduled-job` alert

After the fix:
- CLI dry-run walks all 3 tables cleanly:
  - `guidance_training_rows`: scanned 13583, dirty 0
  - `retrieval_events`: scanned 2877, dirty 0
  - `llm_interactions`: scanned 32157, dirty 0
- `mdemg data export-auto --space-id mdemg-dev` completes cleanly: **19426 rows, 0 privacy violations, 1 old archive pruned**
- Server binary kickstarted at 09:41 EDT; `/healthz` returns `{status: ok, checks: {all subsystems ok}}`
- Full test suite green: `go test ./internal/llmclient/ ./internal/cli/` PASS (61s)
- Lint: `golangci-lint run ./internal/llmclient/ ./internal/cli/` — 0 issues

## Two arch rules pinned (CLAUDE.md)

1. **Privacy-scrub patterns MUST be idempotent under multiple passes** — the `TestScrub_IsIdempotent` pin asserts `Scrub(s) == Scrub(Scrub(s))` across a realistic corpus. Intake runs ONE pass; the export scan-gate compares `stored == Scrub(stored)` — a non-idempotent scrub silently blocks export-auto forever on the affected row class. When adding a new scrub pattern, extend the pin with representative inputs (real-world composite strings, not synthetic minimal cases) and ensure the pattern's replacement output does NOT re-trip its own regex.

2. **Multi-branch CLI handlers that walk independent per-table workloads MUST NOT early-return when an earlier branch is clean** — the shipped `scrub-export-tables` had 2 pre-existing sites (guidance-empty + guidance-dryrun) that returned from the whole handler before retrieval/llm branches could run. Same class pattern to check on any future CLI that iterates a fixed set of stages: an `if empty { return nil }` mid-flow is the smell. Prefer `if !empty { ...work... }` fall-through OR a final block-level `return` only after every branch has had a chance to run. Also applies to dry-run: dry-run should preview EVERY branch, not the first.

## Follow-ups disclosed

- **Migrate the throwaway `dev-probes/scrubprobe` pattern into a first-class CLI subcommand** with proper flags + tests. The one-shot script I wrote to clear the llm_interactions blockers this morning is now shipped as this sprint's CLI extension (superseded).
- **Add a `sync-scrub-predicate` CI check** that pin-tests intake-scrub and export-scan-gate use the SAME predicate signature (`ScrubStringExcluding(s, textFields[col])` on both sides). Silent divergence would create a repeat of this class where intake claims clean, export claims dirty. Deferred — no drift observed today; both sides currently use `ScrubStringExcluding`.
- **Property-based fuzz test for the scrubber** — the 18-input pin catches known regressions but not novel classes. A `go test -fuzz` corpus would harden further. Deferred; pin is sufficient for the known failure surfaces.

## Documents Accessed

- `internal/llmclient/scrubber.go` — the fix site (regex + `isRedactedPlaceholder`)
- `internal/llmclient/scrubber_env_ref_test.go` — the SCRUB-ENV-REF-001 pin that caught my initial over-tightening (`}` in exclusion class broke `${VAR}` refs)
- `internal/llmclient/scrubber_test.go` — extended with 2 new pins
- `internal/cli/data_scrub_guidance.go` — CLI extension + early-return fixes
- `internal/tsdb/exporter.go` (§llmInteractionsSpec + §privacy scan predicate) — verified the CLI predicate matches the exporter's `textFields[6] = nil` (full-scrub) contract
- CLAUDE.md pins: EXPORT-SCRUB-INTAKE-001, SCRUB-ENV-REF-001 (the two prior scrub-touching sprints)
- Live: TSDB SQL diagnostics, `mdemg data scrub-export-tables` dry-run + apply, `mdemg data export-auto` clean run, `launchctl kickstart` server restart, `/healthz` post-restart probe
