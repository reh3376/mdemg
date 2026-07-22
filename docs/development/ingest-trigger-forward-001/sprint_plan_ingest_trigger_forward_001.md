# Sprint INGEST-TRIGGER-FORWARD-001 — forward the four dropped IngestTriggerRequest fields

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | INGEST-TRIGGER-FORWARD-001 |
| Owner | Roger Henley |
| Branch | `reh3376_dev01` |
| Format | Sprint plan v1.0 (12-section) |
| Effort | ~0.5 dev-day |
| Parent | DOC-CURRENCY-002 disclosed follow-up: `IngestTriggerRequest` accepts `include_md`/`include_ts`/`include_py`/`archive_deleted` but `handleIngestTrigger` drops them before building the CLI args — a caller setting them silently gets the CLI defaults |

## 2. Problem Statement

`models.IngestTriggerRequest` (models.go:678) carries four documented fields the
API contract promises to honor: `include_md`, `include_ts`, `include_py`
(pointer-bools, CLI default true) and `archive_deleted` (pointer-bool, CLI
default true). `handleIngestTrigger` (handlers.go:2789) copies neither into the
job `config`, so `buildIngestArgsFromConfig` never emits `--include-md/ts/py`
(and its existing `archive_deleted` mapping is dead on this path). An API
caller excluding Python from ingestion still ingests Python. Same
silently-no-op class DOC-CURRENCY-002 hunted in docs — here it's in code.

## 3. Scope & Constraints

**In scope:** forward all four fields (nil = omit from config → CLI default
applies, mirroring the shipped `ExtractSymbols`/`Consolidate` pointer-bool
pattern); add the three `--include-*` mappings to `buildIngestArgsFromConfig`;
unit tests for handler-forwarding + args-building; live Tier-3 dry-run proving
the flag reaches the subprocess; docs (API doc caveat removal, CHANGELOG).
**Out of scope:** new request fields; scheduled-sync path changes (it builds
its own config); UATS spec additions beyond what exists.
**Constraints:** nil pointer-bools MUST NOT emit a flag (CLI defaults are the
single source of truth — no default copies server-side, per the
omitted-when-unset rule from EVENTGRAPH-CLI-001).

## 4. Dependencies

✅ CLI flags exist (`ingest.go:113-131`); ✅ pointer-bool pattern shipped for
`extract_symbols`/`consolidate`; ✅ `handlers_ingest_test.go` conventions.

## 5. Implementation Plan (sequential)

- **E0** this plan.
- **E1** handler: copy `include_md`/`include_ts`/`include_py`/`archive_deleted`
  into config when non-nil. Args builder: add `--include-md=%t` /
  `--include-ts=%t` / `--include-py=%t` (archive_deleted mapping exists).
  Unit tests: set-false forwarded; set-true forwarded; nil omitted (no flag).
- **E2** live Tier-3: server rebuilt + kickstarted; POST
  `/v1/memory/ingest/trigger` with `dry_run:true, include_py:false` on a
  small fixture dir; job status completes; verify the spawned CLI received
  `--include-py=false` (dry-run stats exclude .py / process args in log).
- **E3** docs: INGEST_CODEBASE_API.md caveat → honored-fields note; CHANGELOG;
  post.md.

## 6. Testing Plan

Tier 1: new unit tests. Tier 2: `go test ./internal/api/...` + full build.
Tier 3: E2 live dry-run flag-propagation proof.

## 7. Commit Strategy

`docs(E0)` → `fix(E1)` → `docs(E2 evidence + E3)`.

## 8. Verification Checklist

unit green · build+lint green · live dry-run shows exclusion honored · docs
updated · pushed.

## 9. Documentation Update

`docs/api/INGEST_CODEBASE_API.md` (drop the "accepted but not forwarded"
caveat, document nil-omits-flag semantics); CHANGELOG Fixed; post.md.

## 10. Risks & Mitigations

| Risk | Sev | Mitigation |
|---|---|---|
| Behavior change for callers who set fields expecting them ignored | Low | The fields were documented as honored; forwarding is the contract. Dry-run available |
| Boolean-false vs nil confusion | Low | Pointer-bools already the shipped pattern on this struct; tests pin nil-omits |

## 11. Rollback

Revert the fix commit — fields return to being dropped (pre-sprint behavior).

## 12. Documents Accessed

`internal/api/handlers.go` (handleIngestTrigger, buildIngestArgsFromConfig);
`internal/models/models.go:676-696`; `internal/cli/ingest.go:113-131`;
`internal/api/handlers_ingest_test.go`; DOC-CURRENCY-002 post (disclosure).
