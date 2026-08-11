# Beta Share — Tester Export Guide

**Feature doc for beta testers using `mdemg beta-share`** (shipped v0.11.0-beta.2). This is the tester-facing companion to the maintainer-side [beta-import.md](beta-import.md).

If you are a beta tester who wants to understand how `mdemg beta-share` works, where your data goes, and what happens to it after you send it, this is the doc for you. Point questions at this file first — if it doesn't answer, that's a doc bug worth filing.

## Why this feature exists

Beta testers are running MDEMG against their own workflows on their own machines. When you file a bug or a feature suggestion, the maintainer often needs to SEE what your MDEMG saw — the actual LLM interactions, retrieval events, and guidance corpus rows from your recent session. Copy-pasting log excerpts is lossy and error-prone. `mdemg beta-share` packages the relevant TSDB rows into a single tar.gz you can attach to a GitHub issue.

There's a second purpose: **corpus contribution**. If you opt in, your `guidance_training_rows` (which is what MDEMG's Jiminy guidance system was actually surfacing + how the agent responded) flow into the maintainer's local retrain corpus. Over time, real tester behavior improves the shipped model's follow rate. Contribution is opt-in per submission — no traffic leaves your machine without you explicitly running the command and confirming the prompt.

## What gets captured

The bundle contains rows from these TSDB tables, filtered to your `--space-id` + the last `--since-days N` days:

| Table | Purpose | Bytes/row (typical) |
|---|---|---|
| `llm_interactions` | Every LLM call MDEMG made — system prompt, user prompt, response, latency, tokens, error | ~2–8 KB |
| `retrieval_events` | Every `/v1/memory/retrieve` call — query text, top-K results, RRF scores, latency | ~1–4 KB |
| `embedding_events` | Every embedding computation — text, model, dimensions, latency (default OFF via `--no-embedding` — much smaller bundles) | ~500 bytes |
| `guidance_training_rows` | Every guidance item Jiminy surfaced + whether the agent followed/ignored it (schema v9+, ships in v0.11.0-beta.4+) | ~1–3 KB |

**NOT included**: your Neo4j graph, your `~/.mdemg/*` config, your code files, your git history, your environment variables. The bundle is TSDB-only.

## Privacy — how PII is handled

The exporter runs every text field through a **strict privacy scrubber** BEFORE writing the bundle:

- API keys (`sk-…`, `ghp_…`, `AKIA…`, `Bearer …`) → `[REDACTED_KEY]`
- Absolute paths (`/Users/…`, `/home/…`, `C:\Users\…`) → `/[PATH]/last/two/segments`
- Env-secrets (`PASSWORD=`, `SECRET=`, `TOKEN=`, `API_KEY=`, `PRIVATE_KEY=` with literal values) → `<VAR>=[REDACTED]`
- Email addresses → `[EMAIL]`
- Neo4j connection strings with embedded creds → `neo4j://[REDACTED]@`

Shell env-variable REFERENCES (`$FOO`, `${FOO}`, `${FOO:-default}`, `%FOO%`) are **PRESERVED** — they're pointers, not literal secrets. Fixed in SCRUB-ENV-REF-001 (2026-08-04).

**Hard gate on scrub violations**: if the scrubber detects PII that WASN'T successfully redacted, the export **BLOCKS entirely** — no bundle is produced, exit code non-zero, the failure is logged. This is a strong-contract guarantee: you can't accidentally ship a bundle that contains raw PII. If the block fires, either your workspace has an unusual pattern (report it as an issue) OR the scrubber has a genuine PII match — either way, don't override the block.

## How it works — end-to-end lifecycle

```
YOUR MACHINE                                          MAINTAINER'S MACHINE
────────────                                          ────────────────────
1. mdemg beta-share --space-id X --since-days 7
   │
   ├─ dry-run first → shows row counts, no bundle
   │
   └─ Real run → interactive opt-in prompt
      ├─ scrubber runs on every text field
      ├─ if any PII survives → HARD BLOCK (no bundle)
      └─ else → writes bundle:
         ~/.mdemg/beta-share/mdemg-beta-share-<ts>.tar.gz
         (~50 KB – 5 MB depending on --since-days)

2. Bundle contents (outer tar.gz):
   ├─ README-BETA.md          (human-readable overview)
   ├─ submission_receipt.json (submission_id, produced_at, window, row counts)
   └─ utds-export.tar.gz      (nested UTDS-compliant archive)
        └─ manifest.json + per-table JSONL files with SHA-256 checksums

3. You attach the tar.gz to a GitHub issue on
   https://github.com/reh3376/mdemg/issues
   (or email it to the maintainer if the issue is sensitive).

4. Maintainer receives the notification.               ┌───────────────────────
                                                       │ 5. Maintainer runs:
                                                       │    mdemg beta-import <bundle>
                                                       │
                                                       │    Verifies:
                                                       │    - SHA-256 of every JSONL
                                                       │      matches manifest
                                                       │    - Re-runs the privacy
                                                       │      scrubber (defense-in-depth)
                                                       │
                                                       │    Imports guidance rows into
                                                       │    space_id=beta-tester-<sid>
                                                       │    (NEVER the maintainer's
                                                       │    own space).
                                                       │
                                                       │ 6. After 30 days, the maintainer
                                                       │    runs:
                                                       │    mdemg beta-janitor sweep
                                                       │      --older-than-days 30 --yes
                                                       │
                                                       │    (or on-demand for a specific
                                                       │    submission:
                                                       │    mdemg beta-janitor delete
                                                       │      --submission-id <sid>)
                                                       │
                                                       │    Rows deleted from TSDB.
                                                       └───────────────────────
```

## Where the bundle saves

Default output path:

```
~/.mdemg/beta-share/mdemg-beta-share-<UTC-timestamp>.tar.gz
```

Override with `--out /path/to/wherever.tar.gz`. The directory is created if missing.

The parent dir (`~/.mdemg/beta-share/`) is safe to `ls` and inspect before submitting — you can open the outer tar with any archive tool (macOS Finder, `tar tvfz`, WinRAR) to see the three top-level entries.

## How the tester submits

Two paths, tester's choice:

### Path A — GitHub issue attachment (recommended)

1. Open a new issue at [https://github.com/reh3376/mdemg/issues/new/choose](https://github.com/reh3376/mdemg/issues/new/choose).
2. Pick the appropriate template (bug report / feature request / feedback).
3. Drag the `.tar.gz` from your Finder / file browser into the issue body — GitHub attaches it.
4. Submit.

The auto-triage workflow (`.github/workflows/beta-triage.yml`) labels the issue as `beta-submission` and pings the maintainer. The submission indexer (`.github/workflows/beta-submission-indexer.yml`) records receipt via the `submission_id` in the receipt file so the maintainer's dashboard knows the bundle arrived.

### Path B — email (for sensitive bugs)

If the bug involves something you don't want in a public issue tracker:

1. Email `rogerhenley345@gmail.com` with subject line `Beta bug: <one-line summary>`.
2. Attach the `.tar.gz`.
3. Include the `submission_id` from the receipt in the body.

## What the receipt tells you

Inside every bundle, `submission_receipt.json` looks like:

```json
{
  "submission_id": "d3n2ekjmwlbwjxa2cnjnkxrq",
  "produced_at": "2026-08-11T17:42:33Z",
  "window": { "since_days": 7 },
  "row_counts": {
    "llm_interactions": 812,
    "retrieval_events": 1043,
    "embedding_events": 0,
    "guidance_training_rows": 156
  },
  "retention_days": 30,
  "deletion_contact": "rogerhenley345@gmail.com",
  "deletion_subject_line": "Beta delete: d3n2ekjmwlbwjxa2cnjnkxrq"
}
```

**Keep this ID.** If at any point over the next 30 days you want your submission deleted early — no reason required — email the deletion contact with the subject line shown in the receipt. The maintainer runs `mdemg beta-janitor delete --submission-id <sid>` and the rows are removed from their TSDB.

## Retention model

- The maintainer commits to **30-day retention** from the date of receipt. Rows are stored in a synthetic per-tester space (`beta-tester-<submission_id>`) that never mixes with the maintainer's own workspace.
- After 30 days, the maintainer's scheduled `mdemg beta-janitor sweep --older-than-days 30 --yes` sweeps all beta-tester rows past that age.
- Early deletion on request: any time within the window, email with the deletion subject line + your submission_id. Turnaround: within 3 business days.
- The maintainer's substrate-mutation policy (protected `mdemg-dev` space) means beta-tester deletions can NEVER accidentally touch other spaces — the janitor's SQL is `space_id LIKE 'beta-tester-%'`, prefix-guarded by construction.

## Command reference (short form)

```bash
# See how many rows would be exported without producing a bundle
mdemg beta-share --space-id <your-space> --since-days 7 --dry-run

# Real run — interactive opt-in
mdemg beta-share --space-id <your-space> --since-days 30

# Include embedding_events (default off — much smaller bundle without them)
mdemg beta-share --space-id <your-space> --since-days 7 --no-embedding=false

# Script-friendly — skip the confirmation prompt
mdemg beta-share --space-id <your-space> --since-days 7 --yes

# Custom output path
mdemg beta-share --space-id <your-space> --out ~/Desktop/bug-42-share.tar.gz
```

Full flag documentation: [`docs/user/cli-reference.md#mdemg-beta-share`](../user/cli-reference.md#mdemg-beta-share).

## What NOT to expect

Explicit non-goals so testers know what they're NOT signing up for:

- **NOT continuous telemetry.** Nothing runs in the background. No traffic leaves your machine unless you explicitly run `mdemg beta-share` and confirm.
- **NOT phone-home.** The bundle is a local file. It doesn't get uploaded anywhere by the command — you decide when / whether to send it.
- **NOT crash reporting.** For crash artifacts you'd use `mdemg diagnostics collect` (a separate command with a separate opt-in flow).
- **NOT a monitoring beacon.** The bundle is a one-shot snapshot of a chosen window; there's no persistent "beacon" tracking your usage.
- **NOT credential harvesting.** The strict scrubber + hard-block-on-detected-PII gate is the mechanism that ensures this by construction. If you spot a case where PII survives, that's a P0 bug and worth reporting.

## Troubleshooting

**"EXPORT BLOCKED: N privacy scrub violations detected — training data contains PII"**
→ The strict scrub gate found something. Options: (a) reduce `--since-days` and try again (maybe a specific short window was clean); (b) inspect your recent `llm_interactions` / `retrieval_events` for the exact content and decide whether to file it as a scrub-bug (if the flagged content is a false-positive like SCRUB-ENV-REF-001 was) or NOT export until the maintainer ships a fix. Never work around the block.

**"Error: connect to TSDB failed"**
→ Your MDEMG stack isn't running. Start it with `docker compose up -d`. Wait for `curl -s http://localhost:9999/healthz` to return `status:ok`.

**"Row counts are 0 across all tables"**
→ Your `--space-id` may be wrong. Check with `mdemg data status`. Default is `mdemg-dev`; yours may be `mdemg-<your-project-name>`.

**"Bundle is huge (tens of MB)"**
→ `embedding_events` is included. Add `--no-embedding` (the default). If bundle is still huge, reduce `--since-days`.

**"I want to see what's inside the bundle before sending"**
→ Any archive tool works. Command line: `tar tvfz ~/.mdemg/beta-share/mdemg-beta-share-*.tar.gz` shows the 3 top-level entries; `tar xvfz` extracts them; the inner `utds-export.tar.gz` is another tar you can open. All JSONL files are plain text — open in any editor.

## For maintainers (or the curious)

The receiver side lives in [`docs/features/beta-import.md`](beta-import.md). Two-layer defense before any DB write: SHA-256 verification against the manifest, then a re-run of the same privacy scrubber the exporter used (defense-in-depth against a hand-crafted bundle that also matches its manifest SHA).

Sprint history:
- [BETA-IMPORT-001](../development/beta-import-001/post.md) — the receiver + janitor sprint (B5 arc)
- [EXPORT-SCRUB-INTAKE-001](../development/export-scrub-intake-001/post.md) — intake-side scrub parity so exports don't chronically block
- [SCRUB-ENV-REF-001](../development/scrub-env-ref-001/) — env-var reference preservation

## Related docs

- [`docs/user/cli-reference.md#mdemg-beta-share`](../user/cli-reference.md#mdemg-beta-share) — full flag reference
- [`docs/features/beta-import.md`](beta-import.md) — maintainer-side receiver
- [`packaging/homebrew-mdemg/README_BETA.md`](https://github.com/reh3376/homebrew-mdemg/blob/main/README_BETA.md) — beta-tester onboarding (installation, testing, feedback)
- [`docs/features/utds-framework.md`](utds-framework.md) — Universal Training Data Specification (the bundle's inner format)
