# Sprint INSTANCE-ID-PIN-001 — Sprint Post

**Sprint line**: `docs/development/instance-id-pin-001/`
**Ship date**: 2026-09-27
**Sprint kind**: Bug fix (durable) — Q5 §5 self-disclosed follow-up
**Parent bug**: INSTANCE-ID-HOSTNAME-DRIFT-001 (documented `docs/development/dashboard-metrics-review-2026-09-21/hostname_drift_bug.md`)
**Prior mitigation**: A — `MDEMG_INSTANCE_ID=Mac.lan-mdemg-dev` in `.env` (2026-09-22)

---

## 1. Problem

`resolveInstanceID` (in `internal/cli/root.go`) derived instance_id at every invocation as `<hostname>-<space_id>` whenever `MDEMG_INSTANCE_ID` was not set. On 2026-09-22, a hostname change (`Mac.lan` → `reh3376s-MacBook-Pro.local`) silently orphaned all TSDB rows tagged `Mac.lan-mdemg-dev` from `mdemg data export-auto`'s WHERE clause — export queried under the new derived instance_id and matched zero rows. Manifested as HIGH `scheduled-job-failure` + `scheduled-job` alerts firing every ~15 min.

Mitigation A (env pin in `.env`) was applied same-day and cleared the alarm. This sprint delivers the **durable fix** so future hostname changes on any operator's machine do not require detective work.

## 2. Fix

Extended `resolveInstanceID` with a **pin-file first-run write** step, mirroring the well-established MDEMG-DOCS-INGEST-002 config-with-`=-`-escape-hatch pattern.

**New priority order** (existing behavior preserved for explicit + env):
1. Explicit CLI arg (unchanged)
2. `MDEMG_INSTANCE_ID` env var (unchanged; operator-authored trumps everything)
3. **NEW: pin-file at `~/.mdemg/instance_id`** — if present + non-empty, use its contents
4. Fallback: derive `<hostname>-<space_id>` **AND write it to the pin-file** so subsequent runs pin to that value regardless of hostname changes

**Pin-file mechanics**:
- Path: `~/.mdemg/instance_id` (single-line text file; trailing newline preserved by writer)
- Override path via `MDEMG_INSTANCE_ID_PIN_PATH=<path>`
- **Escape hatch**: `MDEMG_INSTANCE_ID_PIN_PATH=-` disables pinning entirely (mirrors `MDEMG_DOCS_INGEST_EXCLUDE_DIRS=-` from MDEMG-DOCS-INGEST-002)
- Atomic write: tmp file + rename (avoids torn-file class if process killed mid-write)
- **Best-effort failure**: WARN log on write failure but do NOT abort — the shipped hostname-derived value still returns; a machine with a read-only `~/.mdemg` degrades gracefully to the pre-sprint behavior
- **Malformed pin recovery**: empty / whitespace-only pin content falls through to hostname derivation with a WARN

## 3. Files changed

- `internal/cli/root.go` — extended `resolveInstanceID`; added `instanceIDPinPath()`, `readInstanceIDPin()`, `writeInstanceIDPin()` helpers (+~50 LOC, +3 imports)
- `internal/cli/instance_id_test.go` — new file, 7 pin tests via `t.TempDir()` (150 LOC)

Zero substrate mutation. Zero schema change. Zero config-field additions (env vars are read directly — no `Config` struct plumbing needed since `resolveInstanceID` is called from CLI paths, not the API server hot path).

## 4. Testing

### Tier 1 (unit) — 7 pin tests

All via `t.TempDir()` + `t.Setenv()` for isolation and auto-restore:

- `TestResolveInstanceID_ExplicitArgWins` — CLI arg trumps env + pin
- `TestResolveInstanceID_EnvWinsOverPinFile` — regression pin for the shipped `MDEMG_INSTANCE_ID` contract (Mitigation A path stays authoritative)
- `TestResolveInstanceID_PinFileWinsOverHostname` — core new behavior
- `TestResolveInstanceID_FirstRunWritesPinFile` — first-run derivation + persistence in one shot
- `TestResolveInstanceID_MalformedPinFallsThrough` — 3 subcases (empty / whitespace / tab-only)
- `TestResolveInstanceID_DashDashDisablesPin` — escape-hatch pin
- `TestResolveInstanceID_TrimsWhitespace` — write-then-read round-trip (trailing newline preserved by writer, trimmed on read)

Verification: `go test ./internal/cli/... -run TestResolveInstanceID -v` → all pass.

### Tier 3 (live smoke) — mdemg-dev

Executed against real filesystem + running mdemg + real TSDB:

- **E2.4**: Wrote correct historical pin `Mac.lan-mdemg-dev` to `~/.mdemg/instance_id` (operator-authored rescue path)
- **E2.5**: With pin-file present, no `.env` pin → `mdemg data export-auto` **succeeded** with 11810 rows (proved pin-file path works end-to-end)
- **E2.6**: Restored `.env` pin (belt-and-suspenders)
- **E2.7**: With BOTH env + pin-file present, env wins (contract preserved live, not just in unit test)
- **E2.8**: With pin-file absent, first-run derives `reh3376s-MacBook-Pro.local-mdemg-dev` (current hostname) AND writes it to the pin-file — confirms first-run write path fires; restored correct value after smoke

## 5. Deployment / rollout

**Fresh machines** — pin-file is created on first `resolveInstanceID` call and future hostname changes no-op. Zero operator action required.

**Existing machines** — the shipped `.env` pin (Mitigation A) continues to win via env priority. Rescue path: manually write the historical value to `~/.mdemg/instance_id` before removing the `.env` pin.

**Rollback** — set `MDEMG_INSTANCE_ID_PIN_PATH=-` in `.env` or remove the pin-file. Behavior reverts to pre-sprint hostname-derivation.

## 6. Arch rules pinned

**Two rules pinned to CLAUDE.md**:

1. **Hostname as identity is fragile** — any code that derives an identity from `os.Hostname()` MUST pin the derived value at first-use to a persistent surface, else a hostname change silently orphans downstream state. Follow the shipped INSTANCE-ID-PIN-001 pattern: explicit > env > pin-file > derive-and-persist. Applies retroactively to any future call site deriving identity from `os.Hostname()` — audit at each new site.

2. **Best-effort writes to operator-owned paths (`~/.mdemg/`) MUST use atomic tmp+rename + fail-open WARN** — never abort the caller on write failure. Locked / read-only / permission-denied paths are a normal degrading class; the shipped `resolveInstanceID` returns the derived value and logs a WARN, matching the shipped MDEMG-DOCS-INGEST-002 + WARM-STORE-PERSIST-001 shape.

## 7. Follow-ups disclosed

None. Sprint closes INSTANCE-ID-HOSTNAME-DRIFT-001 bug class completely.

## 8. Documents Accessed

- `docs/development/dashboard-metrics-review-2026-09-21/hostname_drift_bug.md`
- `docs/development/instance-id-pin-001/sprint_plan.md`
- `internal/cli/root.go` (before + after)
- `internal/cli/instance_id_test.go` (new)
- `.env` (Mitigation A verification)
- `~/.mdemg/instance_id` (live pin file)
- CLAUDE.md § MDEMG-DOCS-INGEST-002 (pattern reference)
- CHANGELOG.md
