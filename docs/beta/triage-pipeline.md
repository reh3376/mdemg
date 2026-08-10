# Beta Triage Pipeline

Maintainer-facing runbook for the beta-tester feedback funnel.

## Pipeline shape

```
Tester runs mdemg diagnostics collect / mdemg beta-share
         │
         ▼
Tester opens GH issue via a beta-* template          ← .github/ISSUE_TEMPLATE/*.yml
         │  (frontmatter → carries `beta` label)
         ▼
.github/workflows/beta-triage.yml fires              ← Sprint D
   - creates label palette (idempotent)
   - parses issue body for OS / install-method / severity signals
   - applies severity:* / os:* / install:* labels
   - posts one-time welcome comment
         │
         ▼
Maintainer triages using labels + digest
         │
         ▼
.github/workflows/beta-weekly-digest.yml (Monday 14:00 UTC)  ← Sprint D
   - reads all open `beta`-labelled issues
   - updates a single rolling tracker issue (label `beta-digest`)
   - snapshot table + top blockers + last-7d activity

If tester attached an `mdemg beta-share` bundle:
         │
         ▼
.github/workflows/beta-submission-indexer.yml            ← Sprint B3 (2026-08-10)
   - fires on issues:[opened, edited] + nightly 06:00 UTC
   - scans issue body for "Submission ID: <cuid2>" pattern
   - records to a rolling tracker issue (label `beta-submissions`)
     with 30-day expiry per submission (retention promise)
   - posts one-time receipt comment on the tester's issue
```

## Labels used by auto-triage

| Label | Meaning | Applied by |
|---|---|---|
| `beta` | Any beta-cycle issue | Template frontmatter |
| `install-report` | Filed via beta-install-report.yml | Template frontmatter |
| `bug-report` | Filed via beta-bug-report.yml | Template frontmatter |
| `feature-friction` | Filed via beta-feature-friction.yml | Template frontmatter |
| `triage` | Awaiting maintainer review | Template frontmatter |
| `severity:blocker` | Tester cannot proceed (install-blocked or T1.1-T1.3 ❌) | Auto-triage body scan |
| `severity:degraded` | Works but degraded / partial failure | Auto-triage body scan |
| `severity:cosmetic` | Typo / small polish | Auto-triage body scan |
| `os:macos` | Reported on macOS | Auto-triage body scan |
| `os:linux` | Reported on Linux | Auto-triage body scan |
| `os:wsl2` | Reported on WSL2 | Auto-triage body scan |
| `install:brew` | Installed via Homebrew tap | Auto-triage body scan |
| `install:script` | Installed via curl `install.sh` | Auto-triage body scan |
| `install:source` | Built from source | Auto-triage body scan |
| `beta-digest` | Pinned digest tracker | Digest workflow (auto-created on first run) |
| `beta-submissions` | Pinned submission-registry tracker (B3) | Submission-indexer workflow (auto-created on first submission) |

## Manual triage overrides

If auto-triage mis-labels an issue, just remove the wrong label and add the right one. The workflow re-runs on `edited` events but is idempotent — it will NOT remove labels the maintainer applied manually, only add missing ones.

## Weekly digest cadence

- **Auto-run:** Mondays 14:00 UTC (~10am EDT / 9am CDT)
- **Manual run:** Actions → Beta Weekly Digest → Run workflow
- **Where it lands:** the open issue with label `beta-digest`. If none exists, one is created on first run.

Only ONE `beta-digest` issue should exist at a time. If the workflow ever creates a duplicate (e.g., a maintainer closes the tracker and the next run auto-creates a new one), close the extras and keep the one with the freshest `updated_at`.

## Ending the beta cycle

At the end of the beta cycle:
1. Close the `beta-digest` tracker issue.
2. Disable both workflows via GitHub Actions UI (Actions → Beta Issue Auto-Triage → ⋯ → Disable workflow; same for Beta Weekly Digest). This prevents late-arriving beta reports from triggering triage after the cycle is closed.
3. Keep the label palette — labels are useful for post-mortem analysis.

## Related files

- `.github/ISSUE_TEMPLATE/beta-install-report.yml` — Tier-1 checklist as a form
- `.github/ISSUE_TEMPLATE/beta-bug-report.yml` — specific failure report
- `.github/ISSUE_TEMPLATE/beta-feature-friction.yml` — "worked but confusing" report
- `.github/ISSUE_TEMPLATE/config.yml` — chooser page contact links
- `docs/beta/install-checklist.md` — printable Tier-1 walkthrough for testers
- `internal/cli/diagnostics.go` — the `mdemg diagnostics collect` command
- `internal/cli/beta_share.go` — the `mdemg beta-share` command
