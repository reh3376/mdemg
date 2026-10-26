# U12 — Plugins (5 slides)

**Intent**: `/ui/ Plugins` tab + plugin RPC surface + how operators inspect, install, validate plugins.
**Target slides**: 5
**Mermaid**: 0

---

## U12.1 — Section Landing

**Title**: `U12 · Plugins — The Extension Surface`

**Body**:
- Intent: "Plugins are gRPC services the mdemg server talks to over the `mdemg-module` protocol. Four shipped: docs-scraper, uxts-module, linear-module, obsidian-module. Operators inspect state; developers build + ship."
- Preview: UI → validator → RPC contract → security guards → feature doc

**Provenance**:
- `api/proto/mdemg-module.proto` — the plugin interface
- `internal/plugins/` package
- CLAUDE.md "Plugin orphans" + "SEC-TRANCHE-2 path-injection sanitizer" (plugin surfaces are HTTP-reachable)

---

## U12.2 — /ui/ Plugins Tab

**Title**: `The Plugins Tab`

**Body**:
- Where: `/ui/` → **Plugins**
- Shows: installed plugins (name, version, status, port, PID), manifest metadata (declared permissions, binary SHA)
- Interaction:
  - View manifest + validation report
  - See per-plugin recent-call latencies
  - Admin flip: enable/disable a plugin (gated by `AUTH_API_KEYS`)
- Related CLI: `mdemg plugin list / validate / install / remove`

**Provenance**:
- `internal/api/ui/tabs/plugins.js`

**note**:
- Operator cue: "If a plugin is listed but showing errors, start with `mdemg plugin validate <name>` from the CLI."

---

## U12.3 — Plugin Validator

**Title**: `mdemg plugin validate`

**Body**:
- CLI:
  ```bash
  $ mdemg plugin validate --manifest /path/to/manifest.json
  ✓ Schema valid
  ✓ Binary SHA matches manifest
  ✓ Declared permissions ⊂ allowed
  ✓ Entry point reachable (gRPC Health check passed)
  ```
- Validates:
  - Manifest JSON against `plugin.schema.json`
  - Binary file SHA matches manifest-declared SHA (ships-what-you-shipped invariant)
  - Declared permissions subset of allowed set
  - gRPC Health check reaches the plugin
- SEC-TRANCHE-2 made validator path-safe (routes through `internal/pathsafe::SafeJoinUnderDir` to CodeQL-recognizable sanitizer)

**Provenance**:
- `internal/plugins/validator.go`
- CLAUDE.md "Path-injection tranche (SEC-TRANCHE-2)"

**note**:
- Operator cue: "Before trusting a shared plugin, validate the manifest. SHA mismatch = DO NOT INSTALL."

---

## U12.4 — The 4 Shipped Plugins

**Title**: `What's Shipped`

**Body**:
- **docs-scraper** — mdemg-module interface; scrapes a URL set + ingests docs into a space
- **uxts-module** — UxTS runner-as-plugin shape (experimental; separate from docs/-based runners)
- **linear-module** — Linear issue CRUD via plugin RPC (mirrors MCP Linear tools; useful for scripted bulk ops)
- **obsidian-module** — Obsidian vault adapter; reads vault state as observations
- Plugin manager at `internal/plugins/manager.go` supervises each plugin as a subprocess over a Unix socket; kickstart -k reaps prior-generation processes via pgrep by full socket path (MCP-REVIVE-001)

**Provenance**:
- `plugins/*/` subdirectories
- CLAUDE.md "MCP memory channel unified (MCP-REVIVE-001)"

**note**:
- Operator cue: "If an MCP client stays connected after a mdemg restart, it keeps its OLD plugin process until reconnect. Normal."

---

## U12.5 — Gotchas + Where To Go Deeper

**Title**: `Plugins — Gotchas + More`

**Body**:
- Gotchas:
  - Plugin manager launches subprocess over Unix socket; `kickstart -k` reaps stale socket owners BEFORE spawning new (prevents orphan class)
  - Plugin paths route through `pathsafe.SafeJoinUnderDir` at HTTP boundaries — the regex allowlists at distant callers DO NOT close the CodeQL flow (SEC-TRANCHE-2 arch rule)
  - A failing plugin doesn't bring down mdemg — the plugin manager isolates failures per subprocess
  - Plugin RPC calls record to `embedding_events` with `call_site=plugin.<name>` (EMBED-CALLSITE-001 attribution contract)
- Feature docs:
  - `docs/features/plugins.md` (if shipped — else `api/proto/mdemg-module.proto`)
  - CLAUDE.md "SEC-TRANCHE-2" + "MCP-REVIVE-001"
- Where this feeds into: U15 APIs + gRPC (plugin RPC is a gRPC surface), U16 Alerts (plugin-crash alerts)

**note**:
- Operator cue: "Plugins are OPTIONAL extensions. Vanilla MDEMG runs fine with zero plugins installed."
- Transition: "U13 next — Grafana, the historical companion to /ui/."
