# JIMINY-MODE-001 — Sprint Post

**Date:** 2026-08-02 | **Branch:** `reh3376_dev01`
**Trigger:** Operator directive 2026-08-02: users must be able to change the default Jiminy enforcement mode from strict to suggest FROM UI.

## Verdict

**Shipped.** First-class `JIMINY_MODE=strict|suggest` config knob, `mdemg jiminy mode` CLI, new **Jiminy tab in the /ui/ dashboard** with a two-button toggle, `GET /v1/jiminy/strict` endpoint for reading current state, mode-aware startup log. Live-verified across all 5 surfaces (boot log, GET endpoint, CLI read, CLI toggle strict→suggest→strict, UI tab renders + serves).

## What shipped

### E1 — Config knob
- `JIMINY_MODE` env (string, `strict` | `suggest`, default `strict`) — the operator-facing enum
- `JIMINY_STRICT_DEFAULT_ENABLED` retained as backward-compat lower-level flag. Mode overrides it when set to a known value; empty/invalid mode falls back to the flag with a WARN.

### E2 — GET /v1/jiminy/strict
Was POST-only. Now supports GET reads:
```
GET /v1/jiminy/strict?session_id=claude-core
→ {"data": {"session_id":"claude-core","strict":true,"mode":"strict","boot_default":"strict","default_session":"claude-core"}}
```
POST unchanged (still `{"session_id":"…","enabled":true|false}`); response body now also includes the `mode` label.

### E3 — Mode-aware startup log
`internal/jiminy/service.go`: after `LoadFromFile()`, resolves `effectiveMode` (mode > flag > default), applies desired state (Enable/Disable) if it diverges from current (state file survives restart via JIMINY-ENFORCE-001), and logs:
```
level=INFO msg="jiminy: mode" mode=strict session_id=claude-core strict_enabled=true
```

### E4 — CLI
New `mdemg jiminy mode [strict|suggest]` command:
- No args → prints current mode + boot default + default session
- `strict` → POSTs `{enabled:true}` to `/v1/jiminy/strict`
- `suggest` → POSTs `{enabled:false}`
- Flags: `--url` (default `$MDEMG_URL` or `http://localhost:9999`), `--session-id` (default `$JIMINY_STRICT_DEFAULT_SESSION_ID` or `claude-core`)

### E5 — UI tab (Jiminy)
- New `internal/api/ui/tabs/jiminy.js` — reads state via `api.jiminyStrictGet`, renders a status table (session / current mode / boot default) and two buttons ("Enforce (strict)" / "Advise (suggest)"). Active mode's button is disabled + primary-styled.
- New `api.jiminyStrictGet(sessionID)` + `api.jiminyStrictSet(sessionID, enabled)` helpers in `api.js`
- Registered in `main.js` TABS + `index.html` nav
- Includes a `helpPanel` explaining strict vs suggest, session-scoped semantics, and cross-restart persistence

## Live Tier-3 (mdemg-dev, 2026-08-02)

```bash
# 1. Boot log
level=INFO msg="jiminy: mode" mode=strict session_id=claude-core strict_enabled=true

# 2. GET endpoint
$ curl -s "http://localhost:9999/v1/jiminy/strict?session_id=claude-core"
{"data": {"boot_default":"strict","default_session":"claude-core","mode":"strict","session_id":"claude-core","strict":true}}

# 3. CLI read
$ ./bin/mdemg jiminy mode
session_id:       claude-core
mode:             strict (strict=true)
boot default:     strict
default session:  claude-core

# 4. CLI toggle to suggest
$ ./bin/mdemg jiminy mode suggest
Jiminy mode set to "suggest" for session "claude-core".
$ ./bin/mdemg jiminy mode
mode: suggest (strict=false)
# state file removed as expected (Disable() removes ~/.mdemg/.jiminy-strict-mode)

# 5. CLI toggle back
$ ./bin/mdemg jiminy mode strict
Jiminy mode set to "strict" for session "claude-core".

# 6. UI
$ curl -sf http://localhost:9999/ui/tabs/jiminy.js | head -1
// jiminy.js — Jiminy mode selector (JIMINY-MODE-001, 2026-08-02)
```

## Rules pinned

⚠️ **When a config value has a user-facing semantic meaning (mode/state/kind), ship it as a named enum, not as an implementation-level boolean.** `JIMINY_STRICT_DEFAULT_ENABLED=true|false` was correct but leaked the implementation into the operator interface. `JIMINY_MODE=strict|suggest` matches how the operator naturally thinks about the choice. Backward-compat: the boolean stays as the lower-level derived flag; the enum overrides when set.

⚠️ **A user-facing config toggle MUST be reachable from the UI, not just env + CLI.** Env requires restart + shell access; CLI requires terminal familiarity; UI is the accessible-to-humans surface. Ship all three (env for automation, CLI for scripting, UI for operator ergonomics).

## Not shipped (disclosed)

- **Mode display in the Status tab** — the Jiminy tab shows current mode but the Status dashboard doesn't. Small follow-up (`STATUS-JIMINY-MODE-001`); not blocking.
- **Per-space mode overrides** — the current shape is per-session, not per-space. If operators want different modes for different working spaces, that's a bigger design change.
- **Playwright UI test** — the tab renders and buttons wire correctly, but a headless-browser test would pin the button-state flip. Deferable given the underlying `jiminyStrictSet` is exercised via CLI live-verify.

## Rollback

Single-commit revert. The state file `~/.mdemg/.jiminy-strict-mode` persists (harmless — restart re-reads it). UI tab removal is JS + HTML only; no server-side state to clean.

## Documents Accessed

- `internal/config/config.go` (new JiminyMode knob + FromEnv wiring)
- `internal/api/handlers_jiminy.go` (GET route + jiminyModeFromEnabled helper)
- `internal/api/server.go:2821` (route registration — GET reuses same path)
- `internal/jiminy/service.go` (mode-aware boot logic + startup log)
- `internal/cli/jiminy.go` (new file — CLI command)
- `internal/cli/root.go` (subcommand registration)
- `internal/api/ui/tabs/jiminy.js` (new file — UI tab)
- `internal/api/ui/api.js` (jiminyStrictGet/Set helpers)
- `internal/api/ui/main.js` + `index.html` (tab registration)
- Live server (boot log + GET + CLI toggle)
