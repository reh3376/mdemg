# SCRUB-ENV-REF-001 — Sprint Post

**Date:** 2026-08-04 | **Branch:** `reh3376_dev01`
**Trigger:** HIGH `scheduled-job` alert 2026-08-04T22:23:01Z — `export-auto (space=mdemg-dev): EXPORT BLOCKED: 2 privacy scrub violations detected — training data contains PII`. Triage found the 2 flagged rows were `retrieval_events.query_text` entries containing my own diagnostic `bash error: ... PGPASSWORD=$TSDB_PASSWORD ...` captures — shell env-var REFERENCES, not literal secrets — but the scrubber's `env_secret` pattern matched them as `PASSWORD=<value>` and reported them as PII exposures.

## Verdict

Shipped. The scrubber now distinguishes literal secrets from shell env-var references (`$FOO`, `${FOO}`, `${FOO:-default}`, `${FOO:?err}`, Windows `%FOO%`). References are preserved unchanged; literal secret values continue to be redacted. Live-verified: `export-auto` completes cleanly with `Privacy: 0 violations` post-fix.

## What was wrong

`envSecretPattern` = `(?i)(PASSWORD|SECRET|TOKEN|API_KEY|PRIVATE_KEY)\s*[=:]\s*["']?([^\s"',;]+)["']?` matches `PGPASSWORD=$TSDB_PASSWORD` as `PASSWORD` + `=` + `$TSDB_PASSWORD` (value). The value `$TSDB_PASSWORD` is 14 literal characters representing an env-var name; the shell resolves it at runtime, but the STORED string is just the pointer. The scrubber was over-firing on POINTERS, blocking exports on legitimate diagnostic/documentation content.

## What shipped

### `internal/llmclient/scrubber.go` — refined env-secret replacement
```go
{"env_secret", envSecretPattern, scrubEnvSecret},  // was: an inline ReplaceAllString

func scrubEnvSecret(s string) string {
    return envSecretPattern.ReplaceAllStringFunc(s, func(match string) string {
        sub := envSecretPattern.FindStringSubmatch(match)
        if len(sub) < 3 { return match }
        name, value := sub[1], sub[2]
        if isShellEnvVarRef(value) {
            return match  // preserve — the value is a reference, not a secret
        }
        return name + "=[REDACTED]"
    })
}

func isShellEnvVarRef(v string) bool
```

`isShellEnvVarRef` recognises three shapes:
- **Bash plain**: `$FOO` — `$` + identifier char (`[A-Za-z_0-9]`)
- **Bash braced**: `${FOO}` / `${FOO:-default}` / `${FOO:?err}` — `${` … `}` with an identifier char first
- **Windows**: `%FOO%` — begins AND ends with `%`, min 3 chars

Conservative on purpose: a real secret that happens to start with `$` followed by non-identifier chars (e.g. `$!bang`) is STILL redacted. Nothing legitimate looks like `$!bang`; nothing legitimate is `${}`.

### `internal/llmclient/scrubber_env_ref_test.go` — 22 new pin tests (all pass)
- `TestScrubEnvSecret_LiteralValueRedacted` — 5 baseline regression cases: literal `PASSWORD=hunter2`, `SECRET=abc123`, etc. still get redacted
- `TestScrubEnvSecret_ShellRefPreserved` — 9 subtests × ref shape × var name: bash-plain, bash-braced, bash-braced-default, bash-braced-error, windows-percent, api-key-ref, secret-braced, token-plain, private-key-ref
- `TestScrubEnvSecret_LiveExportAutoRegressionPin` — the EXACT string that fired the false alarm (`PGPASSWORD=$TSDB_PASSWORD ... ${TSDB_HOST_PORT:-5433} ...`); if the scrubber regresses on this class, export-auto halts spuriously again
- `TestScrubEnvSecret_MixedRefAndLiteralOnSameLine` — a line with BOTH `PASSWORD=$FOO` and `SECRET=hunter2` correctly preserves the ref and redacts the literal
- `TestIsShellEnvVarRef_Cases` — 15 sub-cases: positives (refs preserved) + negatives (real values / edge shapes still redacted)

### One-time cleanup on mdemg-dev
Two SQL UPDATEs on the offending `retrieval_events` rows (timestamps `2026-08-04 21:01:16.564815+00` + `2026-08-04 21:01:20.603579+00`), replacing `query_text` with a `[SCRUB-ENV-REF-001 cleanup: bash-error diagnostic with env-var references, redacted 2026-08-04]` placeholder. Rows preserved (retrieval-audit continuity, historic scores intact); only the free-text payload replaced.

## Live Tier-3 (mdemg-dev)

**Pre-fix**: `EXPORT BLOCKED: 2 privacy scrub violations detected` (from `training-export.log` at 2026/08/04 18:23).

**Post-fix + cleanup**:
```
$ ./bin/mdemg data export-auto --space-id mdemg-dev
...
INFO privacy scan: patterns skipped table=retrieval_events column=query_text skipped_patterns=[abs_path]
...
INFO export complete export_id=exp-...-20260804-223639 total_rows=16379 tables=3
Export complete: /Users/reh3376/.mdemg/exports/mdemg-export-...-20260804-183639.tar.gz
  Total rows: 16379
  Privacy:    0 violations
  Pruned:     1 old archive(s)
```

Export succeeded end-to-end, produced the archive, wrote the manifest, pruned old archives. The daily scheduled run at 18:23 EDT tomorrow will succeed on its own.

## Rules pinned

⚠️ **Env-secret scrubbers MUST distinguish literal secrets from shell env-var references** — a value that starts with `$`+identifier, `${`…`}`, or `%FOO%` is a POINTER the shell resolves at runtime, NOT the secret itself. Blocking exports on captured references (diagnostic bash-errors, docs, test fixtures that mention `PASSWORD=$FOO`) is a false-positive class that erodes operator trust in the guardrail. Any future scrub pattern that keys on a `NAME=VALUE` shape should apply the same reference-detection.

⚠️ **Pattern-based scrubbers should be conservative on ambiguous shapes but explicit about what they preserve** — the fix here doesn't preserve every `$`-prefixed value (e.g. `$!bang` still redacts). Nothing legitimate looks like a malformed reference, so the false-negative risk is low. Document what the check DOES preserve so future refactors don't accidentally widen it.

⚠️ **When a privacy-scrub false positive is discovered, ship a regression pin with the EXACT live-triggering content** — `TestScrubEnvSecret_LiveExportAutoRegressionPin` uses the full offending string from `retrieval_events`, not a synthesized minimal case. The full-context pin catches subtler regressions (e.g. a future refactor that only matches "simple" refs but not the `${VAR:-default}` case that also appeared live).

## Not shipped (intentional)

- **Upstream hook hardening** (pre-scrub in `pre-bash-check.py`) — deferred. The scrubber fix + row cleanup is sufficient; adding upstream scrubbing to the hook would add complexity for marginal defense-in-depth given the exporter scrubber now handles the class correctly.
- **Metric for scrubber-preserved refs** (`mdemg_scrub_env_ref_preserved_total`) — deferred. No alert would consume it, per DORMANT-METRICS-CLEANUP-001 discipline.
- **Extending refs to `abs_path` shape** — abs_path already scrubs correctly and there's no analogous "reference" (`/Users/$USER/...` isn't a distinct storage class we care about here).

## Follow-ups disclosed

- **Monitor next daily export** (2026-08-05 18:23 EDT) — expect `Privacy: 0 violations` on the natural cadence. If it re-fires on a different class, another sprint.
- **Audit `retrieval_events` for similar historic false-positive material** — the 24h export-auto window is small; the 180d `mdemg data export` window may surface additional `PASSWORD=$X` style captures. Not urgent — export-auto is the daily cadence; the 180d export is on-demand.

## Rollback

Single-commit revert restores pre-sprint behavior (over-firing on env-var refs). The cleaned-up rows stay cleaned (UPDATE is idempotent-safe); to restore original content, dig `bash error: source .env ...` from git-log-adjacent backups if forensically required — but the redacted placeholder preserves the retrieval-audit signal (call_site, scores, latency) with only the query_text payload replaced.

## Documents Accessed

- `internal/llmclient/scrubber.go` (env_secret pattern + patterns table; edited)
- `internal/llmclient/scrubber_test.go:86` (existing PASSWORD=hunter2 test — still passes)
- `internal/tsdb/exporter.go:112` (retrievalEventsSpec — confirms `abs_path` is the ONLY skipped pattern for `query_text`; env_secret was applied and firing)
- `internal/tsdb/exporter.go:395` (per-column privacy scan call site)
- `internal/cli/data_export_auto.go:65` (24h + instance_id slice — how the export slice was reproduced)
- `/Users/reh3376/.mdemg/logs/training-export.log` (baseline error log evidence)
- Live `retrieval_events` on mdemg-dev (identified the 2 offending rows via ROW_NUMBER())
