# JIMINY-ENFORCE-003 — Sprint Post

**Date:** 2026-08-03 | **Branch:** `reh3376_dev01`
**Arc:** JIMINY-ENFORCE sprint 3 of 5
**Trigger:** Operator escape-hatch. JIMINY-ENFORCE-001/-002 shipped Write/Edit and Bash enforcement with `[/strict]` deny paths — but without an override CLI, a stuck deny (classifier flags a false positive on a WARNED+ escalation) would deadlock the operator with no path to unblock.

## Verdict

**Shipped.** Time-boxed constraint overrides via `mdemg jiminy override [apply|list|revoke]` CLI, `POST/GET/DELETE /v1/jiminy/override` HTTP surface, in-memory session-scoped OverrideManager with lazy expiry, JSONL audit at `~/.mdemg/jiminy-overrides.jsonl`, and StrictClassifier consultation that downgrades deny→pass on fully-overridden verdicts (with the overridden reasons annotated in DenialReason so the audit trail still records WHY the block would have fired).

Live-verified end-to-end: apply via CLI → list via CLI → GET via curl → audit JSONL line written → revoke via CLI → list-empty.

## What shipped

### E1 — `OverrideManager` (`internal/jiminy/override.go`)
Session-scoped in-memory store with per-`(session, constraint_code)` entry:
- **Apply** validates all fields (session, code, reason, positive duration) — `duration<=0` is an explicit error (forgotten overrides silently disable enforcement forever if unbounded)
- **Get** returns active entry or nil; lazy-purges expired entries under double-checked lock
- **List** returns active entries, optionally filtered to a session; lazy-purges as it walks
- **Revoke** removes before scheduled expiry, returns the removed entry (nil on not-found)
- **Audit** — every apply/revoke/expire writes one JSONL line to `JIMINY_OVERRIDE_AUDIT_PATH` (default `~/.mdemg/jiminy-overrides.jsonl`); best-effort — WARN log on write failure but caller succeeds (audit is durable forensic record, not load-bearing for the override itself)
- File permissions `0o600` (operator-only readable)

### E2 — Classifier suppression (`internal/jiminy/strict_classifier.go`)
`StrictClassifier.SetOverrides()` wires the manager. In `Classify()`, after computing the deny verdict, iterate `ViolatedCodes`:
- If the classifier couldn't extract a code (empty string), keep the reason (can't check override without a key)
- If Get() returns a matching active override, annotate it and skip
- If not, keep the code in the deny set

Three outcomes:
- All codes overridden → verdict=pass, `DenialReason="override-suppressed: [override:CODE reason=…]…"`
- Some codes overridden → verdict=deny, reason includes `(partial-override-suppressed: [override:… reason=…])` annotation
- No codes overridden → shipped deny behavior (no-op for the override path)

### E3 — HTTP endpoints (`internal/api/handlers_jiminy.go`)
`POST /v1/jiminy/override` — apply. Body `{session_id, constraint_code, reason, duration_sec}`. Returns 400 on missing fields or `duration_sec<=0`.

`GET /v1/jiminy/override[?session_id=X]` — list active. Returns `{data: {overrides: [...], count: N}}`. Empty array when none active (never null).

`DELETE /v1/jiminy/override` — revoke. Body `{session_id, constraint_code}`. Returns 404 when no active entry exists.

Multiplexed via `handleJiminyOverride` on method. Route registered at `internal/api/server.go`.

### E4 — CLI (`internal/cli/jiminy.go`)
```
mdemg jiminy override apply  --constraint <code> --reason <text> --duration <window>
mdemg jiminy override list   [--session-id X] [--json]
mdemg jiminy override revoke --constraint <code>
```
Duration parses via `time.ParseDuration` (15m, 1h, 2h30m). Reason is REQUIRED (audit trail depends on it). Default session key resolves to `$JIMINY_STRICT_DEFAULT_SESSION_ID` or `claude-core`.

### E5 — Config (`internal/config/config.go`)
`JIMINY_OVERRIDE_AUDIT_PATH` — audit JSONL path. Default `~/.mdemg/jiminy-overrides.jsonl`. Empty string disables audit (in-memory-only overrides for tests + spaces that can't allocate a home dir).

### Tests (`internal/jiminy/override_test.go`)
9 pins:
- `TestOverrideManager_ApplyValidation` — 5 subcases covering empty session/code/reason + zero/negative duration
- `TestOverrideManager_ApplyGetRevoke` — happy path
- `TestOverrideManager_ExpiryLazyPurge` — expired entry purged on Get, re-apply works
- `TestOverrideManager_SessionIsolation` — different sessions with same constraint don't collide
- `TestOverrideManager_ListFiltersBySession` — filter semantics
- `TestOverrideManager_AuditLogWritesOnApplyAndRevoke` — audit JSONL contains both ops
- `TestOverrideManager_AuditFailsOpen` — unwritable audit path doesn't crash apply
- `TestStrictClassifier_OverrideSuppressesDeny` — Get() semantics the classifier relies on

All pass. Full test sweep + lint clean.

## Live Tier-3 (mdemg-dev, 2026-08-03)

```bash
$ mdemg jiminy override apply --constraint TEST-RULE --reason "false-positive during live smoke" --duration 5m
override applied: constraint=TEST-RULE session=claude-core duration=5m0s reason="false-positive during live smoke"

$ mdemg jiminy override list
1 active override(s)
  session=claude-core constraint=TEST-RULE expires_at=2026-08-03T00:45:22.035095-04:00 reason="false-positive during live smoke"

$ curl -s "http://localhost:9999/v1/jiminy/override?session_id=claude-core" | jq .data
{"count": 1, "overrides": [{"session_id":"claude-core","constraint_code":"TEST-RULE","reason":"false-positive during live smoke","applied_at":"2026-08-03T00:40:22.035095-04:00","expires_at":"2026-08-03T00:45:22.035095-04:00"}]}

$ tail -1 ~/.mdemg/jiminy-overrides.jsonl
{"applied_at":"2026-08-03T04:40:22Z","constraint_code":"TEST-RULE","expires_at":"2026-08-03T04:45:22Z","logged_at":"2026-08-03T04:40:22Z","op":"apply","reason":"false-positive during live smoke","session_id":"claude-core"}

$ mdemg jiminy override revoke --constraint TEST-RULE
override revoked: constraint=TEST-RULE session=claude-core

$ mdemg jiminy override list
0 active override(s)
```

## Rules pinned

⚠️ **An enforcement gate MUST have an operator escape-hatch — enforcement without override is a foot-gun.** A stuck deny (classifier false positive on a WARNED+ escalation) with no override path deadlocks the operator. The override is intentionally time-boxed (no `duration=0` = permanent) so a forgotten override can't silently disable enforcement forever.

⚠️ **Every override apply MUST require a reason and write it to a durable audit trail.** The `reason` field is not for the classifier — it's for the operator (and RSIC-004's future learning consumer) to understand WHY enforcement was overridden in a given moment. JSONL audit at `~/.mdemg/jiminy-overrides.jsonl` is the durable record (TSDB migration deferred to JIMINY-ENFORCE-004 which will consume the events).

⚠️ **Session-scoped overrides — never global.** An override for session S must not affect session T. Session isolation was pinned live: same constraint code + two different sessions produces two independent entries. Global overrides would create a stealth enforcement bypass across all agents.

## Not shipped (arc scope, disclosed)

- **JIMINY-ENFORCE-004** — RSIC enforcement-learning outcome types (will consume the override JSONL as `blocked_false_positive` signal and migrate the audit trail to TSDB for query/retention/dashboarding). Also: propose an auto-consolidation flow — if the same constraint is overridden N times in a rolling window with the same-shape reason, RSIC should propose the constraint be tombstoned or reworded.
- **JIMINY-ENFORCE-005** — Post-hoc missed-violation detector.
- **UI display of active overrides** — the Jiminy tab (JIMINY-MODE-001) could gain an "Active Overrides" table with revoke buttons. Ship as a small UI-only follow-up if operator asks; not blocking the arc.

## Rollback

Single-commit revert. The audit file at `~/.mdemg/jiminy-overrides.jsonl` persists (harmless — a JSONL log). Active in-memory overrides are lost on restart even without the revert (they were designed to be short-lived + operator-initiated).

## Documents Accessed

- JIMINY-ENFORCE-001/-002 posts (arc context + shipped patterns)
- ESCALATION-ACCUMULATE-001 post (escalation is the prerequisite for deny to fire)
- `internal/jiminy/strict_classifier.go` (deny path — extension site)
- `internal/jiminy/override.go` (new file)
- `internal/jiminy/override_test.go` (new file, 9 tests)
- `internal/jiminy/service.go` (wiring)
- `internal/api/handlers_jiminy.go` (3 new handlers)
- `internal/api/server.go` (route registration)
- `internal/cli/jiminy.go` (3 new CLI subcommands)
- `internal/config/config.go` (JIMINY_OVERRIDE_AUDIT_PATH)
- Live server (CLI apply/list/revoke, curl GET, JSONL audit tail)
