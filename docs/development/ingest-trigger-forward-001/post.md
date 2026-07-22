# INGEST-TRIGGER-FORWARD-001 — Sprint Post

**Date:** 2026-07-22 | **Branch:** `reh3376_dev01`
**Parent:** DOC-CURRENCY-002 disclosed follow-up (the "accepted but not
forwarded" caveat in INGEST_CODEBASE_API.md).

## What shipped

`handleIngestTrigger` accepted `include_md` / `include_ts` / `include_py` /
`archive_deleted` on the request model but dropped them before building the
CLI args — an API caller excluding Python still ingested Python. The
silently-no-op class DOC-CURRENCY-002 hunted in docs, here in code.

- Config-building extracted to pure `buildIngestConfigFromRequest`
  (testable without spawning the job subprocess); behavior for the existing
  ten fields byte-identical.
- The four pointer-bools land in the job config only when set;
  `buildIngestArgsFromConfig` gains `--include-md/ts/py=%t` mappings
  (`--archive-deleted` mapping already existed and is now reachable from the
  trigger path). An omitted field emits NO flag — the CLI default stays the
  single source of truth (EVENTGRAPH-CLI-001 omitted-when-unset rule).

## Tests

- `TestBuildIngestConfigFromRequest_PointerBools` — set fields land with
  their values; nil fields stay absent from config.
- `TestBuildIngestArgsFromConfig` new cases — explicit `=true/=false` flags
  emitted when present; all eight flag forms asserted ABSENT when omitted.
- Full `./internal/api/` suite + build + lint green.

## Live Tier-3 (mdemg rebuilt + kickstarted)

Triggered a dry-run over the repo with
`{"dry_run":true,"include_py":false,"include_md":false,"archive_deleted":false}`
(scratch space `uats-ingest-forward`) and captured the spawned subprocess
argv:

```
bin/mdemg ingest --path /Users/reh3376/mdemg --space-id uats-ingest-forward
  --endpoint http://localhost:9999 --progress-json --batch=100 --workers=4
  --timeout=300 --extract-symbols=true --consolidate=true
  --include-md=false --include-py=false --dry-run --archive-deleted=false
```

`--include-md=false`, `--include-py=false`, `--archive-deleted=false`
present; `--include-ts` correctly ABSENT (nil in the request → CLI default).
Dry-run wrote nothing; job completed. Also proved the negative first: two
fixture dry-runs (default vs `include_py:false`) return "completed without
progress events" — dry-run emits no count events, which is why argv capture
was the chosen observable.

State: scratch space untouched (dry-run only). `/tmp/ingest-forward-fixture`
left for OS tmp-cleanup (the pre-bash guard blocks `rm -rf`; not worth an
operator confirmation for 4 files + 2 copied dirs in /tmp).

## Verification checklist

- [x] Unit + package tests green; build + lint green
- [x] Live argv capture shows all four fields forwarded, nil omitted
- [x] API doc caveat replaced with honored-fields semantics
- [x] CHANGELOG entry
- [x] Pushed

## Documents Accessed

`internal/api/handlers.go`, `internal/api/handlers_ingest_test.go`,
`internal/models/models.go:676-696`, `internal/cli/ingest.go:113-131`,
`docs/api/INGEST_CODEBASE_API.md`, DOC-CURRENCY-002 post.
