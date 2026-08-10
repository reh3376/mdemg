# Beta Bundle Import + Janitor

Feature doc for **B5** — the receiver counterpart to `mdemg beta-share`. Ships in v0.11.0-beta.4+.

## What it does

Closes the loop from `mdemg beta-share` (opt-in tester submission, shipped v0.11.0-beta.2) to the local retrain corpus. Testers produce a scrubbed bundle → attach to a GitHub issue → maintainer downloads → `mdemg beta-import <bundle.tar.gz>` verifies integrity + privacy contracts + imports the guidance corpus rows to a per-tester synthetic space.

Two companion commands:

- `mdemg beta-import <bundle>` — the verifier + importer
- `mdemg beta-janitor {delete,sweep}` — 30-day retention janitor

## Trust model

Operator manually invokes `mdemg beta-import <bundle>` against a bundle they downloaded from a GH issue attachment. **Not automated** — no GH webhook or auto-processor. Same operator-intent trust level as `mdemg module validate <path>` or `go run <path>`.

Two defense-in-depth gates run BEFORE any DB write:

1. **Per-JSONL SHA-256 vs manifest** — proves the JSONL wasn't modified in transit from tester → attachment → download.
2. **Privacy re-scrub** — every text-field value re-runs through the shipped scrubber (`llmclient.ScrubStringExcluding`) with the same per-field skip patterns the exporter used. Any diff = REJECT. Catches PII that would survive a SHA-matching but hand-crafted bundle.

Any gate failure REJECTS the bundle before touching the DB.

## Attribution model

Every imported row's `space_id` + `instance_id` is remapped to `beta-tester-<submission_id>` (a CUIDv2-suffixed synthetic space). The tester's original `space_id` (which might be `mdemg-dev` on their side!) is DROPPED. This gives:

- **Zero collision** with the operator's own space (e.g. `mdemg-dev`).
- **Deletion keying** — the janitor deletes by `space_id LIKE 'beta-tester-%'`. No `submission_id` column needed on `guidance_training_rows`.
- **30-day retention enforceable** by cron / launchd running `mdemg beta-janitor sweep --older-than-days 30 --yes` periodically.

Row `time` is preserved (window-based deletion works). `row_id` is re-minted per row via `cuid2.Generate()` — the exporter drops `row_id` from projection to avoid `(time, row_id)` PRIMARY KEY collisions across independently-produced bundles.

## Schema-version compatibility

Bundle `manifest.schema_version >= 9` required for corpus import (B5a landed schema 9 in v0.11.0-beta.4 with `guidance_training_rows` added to the exporter's projection).

Bundles from v0.11.0-beta.1 / .2 / .3 carry schema 8 (no guidance rows) — beta-import runs SHA + privacy gates on them (still meaningful for integrity checks) but skips corpus import with a `⚠ Lite mode` notice. No error, no crash — the operator just knows there's nothing to import.

## Command reference

### `mdemg beta-import <bundle.tar.gz>`

```
Flags:
  --dry-run             Verify the bundle + report row counts without writing
  --yes                 Skip the interactive opt-in prompt (script-friendly)
  --space-suffix STR    Override the space suffix (default: submission_id from receipt)
```

Example flow:

```bash
# Preview only — verifies SHA + rescrub, counts rows, exits without writing
mdemg beta-import ~/Downloads/mdemg-beta-share-20260810.tar.gz --dry-run

# Real import, interactive opt-in
mdemg beta-import ~/Downloads/mdemg-beta-share-20260810.tar.gz

# Script-friendly (skip prompt)
mdemg beta-import ~/Downloads/mdemg-beta-share-20260810.tar.gz --yes
```

### `mdemg beta-janitor delete --submission-id <sid>`

Delete rows for a specific submission. Targets `space_id = 'beta-tester-<sid>'` exactly. Idempotent — unknown SID → 0 rows deleted, exit 0.

### `mdemg beta-janitor sweep --older-than-days N`

Bulk-delete rows across ALL beta-tester spaces older than N days (default 30). Preview counts grouped by space_id BEFORE deletion so the operator knows exactly which submissions get pruned. Safe: never touches non-beta-tester spaces (space_id LIKE prefix guard).

## Sprint reference

Plan: `docs/development/beta-import-001/sprint_plan.md`
Post: `docs/development/beta-import-001/post.md`
