# U10 · Logs (5 slides)

**Intent**: `/ui/ Logs` tab + log-scope filter + when to look at logs vs alerts. Debugging surface for when healthz is OK but something still feels off.
**Target slides**: 5
**Mermaid**: 0

---

## U10.1 · Section Landing

**Title**: `U10 · Logs`

**Body**:
- Intent: "`healthz` is now, alerts are history, logs are DETAIL. When a request latency spike happens but nothing's fired, logs are where you look."
- Preview: UI → filter → CLI tail → interpretation → gotchas

---

## U10.2 · /ui/ Logs Tab

**Title**: `The Logs Tab`

**Body**:
- Where: `/ui/` → **Logs**
- Shows: recent server slog output; filterable by level (INFO/WARN/ERROR) and by scope (`jiminy:*`, `retrieval:*`, `rsic:*`, `tsdb:*`, `backup:*`)
- Interaction: live tail (auto-refresh) + text search; pause/resume to inspect a specific moment
- When you'd open it: "healthz says ok but a specific request errored" / "post-restart I want to confirm boot sequence" / "backup job finished, did it error silently?"

**Provenance**:
- `internal/api/ui/tabs/logs.js`
- The server's structured slog output

**note**:
- Operator cue: "The Logs tab is a last-resort debugging surface. Normal operator work rarely needs it."

---

## U10.3 · CLI Tail Alternative

**Title**: `Tailing From Docker`

**Body**:
- CLI:
  ```bash
  $ docker compose logs -f mdemg | grep -E 'ERROR|WARN'
  $ docker compose logs --tail=100 mdemg
  $ docker compose logs -f mdemg --since=10m | jq -c 'select(.level=="ERROR")'
  ```
- The server emits structured JSON slog when `LOG_FORMAT=json`; otherwise key=value pairs
- Scoping: filter by level, time window, or search text; `jq` works great against JSON mode

**Provenance**:
- CLAUDE.md server startup slog contract

**note**:
- Operator cue: "The CLI version is better than the /ui/ Logs tab when you need to time-correlate across multiple services (grep across neo4j + timescaledb + mdemg logs)."

---

## U10.4 · Reading The Log Scopes

**Title**: `What Each Scope Means`

**Body**:
- Common scopes you'll see:
  - `jiminy:`, guidance surfacing, classification, mode toggles, override apply/revoke
  - `retrieval:`, the 4-column RRF + rerank + cache hits/misses
  - `rsic:`, self-improvement cycle events (admit/trigger/complete)
  - `tsdb:`, TSDB flushes, schema migrations, writer backpressure
  - `backup:`, scheduled backup lifecycle events
  - `embeddings:`, embed calls + call_site attribution (EMBED-CALLSITE-002 pinned call_site)
  - `health:`, subsystem probe transitions (up/degraded/down)
- Boot-sequence slogs: watch for `tutorial handler enabled=true url=/ui/tutorial/ files=71 bytes=...` (SERVE-001) + `jiminy: strict mode auto-enabled` (JIMINY-MODE-001) + `scheduled autograde started` (AUTOGRADE-SCHEDULE-001)

**Provenance**:
- Various package slog call sites
- CLAUDE.md "Embedding attribution (EMBED-CALLSITE-002)"

**note**:
- Operator cue: "If you see an unfamiliar scope, grep the codebase for the slog site. It's structured on purpose."

---

## U10.5 · Gotchas + Where To Go Deeper

**Title**: `Logs, Gotchas + More`

**Body**:
- Gotchas:
  - `docker compose logs -f` without a service name tails ALL containers, overwhelming on a busy stack; always name the service
  - Logs are LOCAL to the container's lifetime; container restart wipes stdout history (persisted logs would need a logging driver change)
  - PII scrubber (`scripts/mdemg_usage_curate.py` + `llmclient.ScrubStringExcluding`) runs at TSDB-write time, NOT at log-emission time, raw logs may contain env vars; be careful sharing
  - Trace-level logging requires setting `LOG_LEVEL=DEBUG` + restart
- Where this feeds into: U13 Grafana (log-derived metrics panels), U16 Alerts (log-triggered alert rules), U14 CLI (`mdemg data clean` scrubs TSDB rows that originally came from loggy flows)

**note**:
- Operator cue: "Logs tell you WHAT happened. The TSDB tells you how often. Grafana tells you the trend. All three answer different questions."
- Transition: "U11 next, Config + Features, the two tabs for TOGGLING what the system does."
