# U1 — Welcome + What's Running (5 slides)

**Intent**: Set the mental model — "you have 5 containers + 1 server; here's the 4-command path from install to open-browser."
**Walk-time**: ~4 min
**Target slides**: 5
**Mermaid**: 1 (U1.2 — stack sketch)
**Natural Q&A anchor**: after U1

**Depth template coverage**:
- Landing — U1.1
- Mental model — U1.2
- Where to see — U1.3
- How to touch — U1.4
- Current state + verify — U1.5

---

## U1.1 — Section Landing (replaces `#end-user-path` stub)

**Title**: `U1 · You're Operating MDEMG`
**Slide ID**: `end-user-path` (**ANCHOR PRESERVED** — SERVE-001 landing fork links here)

**Body**:
- 1-sentence intent: "The next ~85 minutes cover the operator's view of MDEMG: what's running, what to watch, what to type, how to handle alerts."
- U1-U17 at-a-glance mini-TOC (section number · topic · ~slides each)
- Keyboard nav reminder: ← → nav, ESC overview, F fullscreen, S speaker view, ? help
- Return-link row to `#landing-fork` + `#table-of-contents`

**Provenance**:
- `docs/onboarding/mdemg-onboarding-deck.md` current `#end-user-path` stub (replaces it)
- `presentation_spec.md` §1 Audience + §5.4 anchor-preservation invariant

**note**:
- Operator cue: "You just came from the fork. If you're building MDEMG, hit ESC and pick the Software Dev path. Otherwise, let's go."

---

## U1.2 — Mental Model: 5 Containers + 1 Server

**Title**: `What's Running`

**Body**:
- 1-sentence context: "A healthy MDEMG stack is 5 Docker containers coordinated by one mdemg server process."
- **Mermaid #1** (horizontal flow):
  - operator → `mdemg server :9999`
  - `mdemg server` → `{neo4j :7687, timescaledb :5433, neural-sidecar :8100, grafana :3030}`
  - All five labeled as "Docker Compose services"
- One-line role each:
  - **mdemg** — HTTP API + CLI backend (ghcr.io/reh3376/mdemg)
  - **neo4j** — graph substrate (CMS; the "cognitive" layer)
  - **timescaledb** — telemetry hypertables (TSDB; the "measurement" layer)
  - **neural-sidecar** — rerank + NLI model inference
  - **grafana** — read-only observability panels

**Provenance**:
- `docker-compose.yml` top-level `services:` section (5 images)
- CLAUDE.md "Multi-Instance" note: "Each instance: 5 containers, 6 ports, ~2.3 GiB RAM"

**note**:
- Operator cue: "This is the whole stack. Nothing else is required to run MDEMG. Everything in the next 100 slides lives inside this picture."
- PM-fallback: "Think of mdemg as the brain, neo4j as long-term memory, timescaledb as the EEG, neural-sidecar as a math co-processor, grafana as the X-ray screen."

---

## U1.3 — Where To See It

**Title**: `Confirm The Stack`

**Body**:
- 1-sentence context: "Three commands tell you if the stack is up and reachable."
- CLI block:
  ```bash
  $ docker compose ps
  NAME                        STATUS                 PORTS
  mdemg-mdemg-1               Up (healthy)           0.0.0.0:9999->9999/tcp
  mdemg-neo4j-1               Up (healthy)           0.0.0.0:7687->7687/tcp
  mdemg-timescaledb-1         Up (healthy)           0.0.0.0:5433->5432/tcp
  mdemg-neural-sidecar-1      Up (healthy)           0.0.0.0:8100->8100/tcp
  mdemg-grafana-1             Up (healthy)           0.0.0.0:3030->3000/tcp

  $ open http://localhost:9999/ui/      # macOS; linux: xdg-open ...
  $ open http://localhost:3030          # grafana (admin/password printed by init)
  ```
- Expected: 5 `Up (healthy)`; anything else is a triage trigger

**Provenance**:
- `docker-compose.yml` port mappings
- `mdemg init` CLI output (prints grafana credentials)

**note**:
- Operator cue: "If any show `Exit (0)` or `Restarting`, your first move is `docker compose logs <service>` on that one, not more mdemg commands."

---

## U1.4 — How To Touch It: The 4-Command Happy Path

**Title**: `The 4-Command Happy Path`

**Body**:
- 1-sentence context: "From zero to open dashboard in four commands. This is the smoke-test for every fresh install, every machine, every day."
- CLI block:
  ```bash
  mdemg init --defaults        # creates .env + .mdemg config (idempotent)
  docker compose up -d         # starts the 5-service stack
  mdemg healthz                # verify server + subsystems
  open http://localhost:9999/ui/   # dashboard (macOS)
  ```
- Variants:
  - `mdemg init --quick` is the alias used in CLAUDE.md examples; `--defaults` is the current primary flag (INIT-DEFAULTS-LOCAL-FIRST-001)
  - Linux: `xdg-open http://localhost:9999/ui/` or paste into browser
  - Already running? `mdemg restart` is a one-shot
- The `mdemg tutorial` command (shipped by ONBOARDING-TUTORIAL-SERVE-001 E5) is the one-shot that opens THIS deck

**Provenance**:
- CLAUDE.md: `Docker (primary)`: `mdemg init --quick` → `docker compose up -d` (all 5 services)
- `docs/features/tutorial-serve.md` for `mdemg tutorial`
- `INIT-DEFAULTS-LOCAL-FIRST-001` sprint for `--defaults` fresh-install behavior

**note**:
- Operator cue: "If any of these 4 fails, triage THAT command before anything else. Logs, ports, env file."

---

## U1.5 — Verify You're Alive + What Degraded Means

**Title**: `Verify You're Alive`

**Body**:
- 1-sentence context: "`healthz` is the single source of truth for 'is MDEMG working right now?'"
- CLI example (healthy):
  ```bash
  $ mdemg healthz
  status: ok
  checks: {neo4j: ok, tsdb: ok, sidecar: ok}
  circuit_breakers: {open: 0}
  ```
- CLI example (degraded):
  ```
  status: degraded
  checks: {neo4j: ok, tsdb: ok, sidecar: timeout}
  circuit_breakers: {open: 1, names: [neural-sidecar]}
  ```
- What `degraded` means + fix order:
  1. `docker compose ps` — is the subsystem actually up?
  2. `docker compose logs <subsystem>` — is it erroring?
  3. `curl -s http://localhost:9999/healthz | jq` — fresh HTTP reading
- Honest note: `healthz` is the RIGHT-NOW view; the alerts channel (U16) is the LONG-RUNNING view. They don't always agree. That's fine — one samples now, the other integrates over hours.

**Provenance**:
- CLAUDE.md: Enhanced `/healthz`: returns `status: "degraded"` with `checks` map when subsystems unhealthy
- `internal/api/handlers_health.go` — the handler
- `CONFIG-VALIDATE-TRANSIENT-DISTINGUISH-001` sprint — distinguishes "not started" from "broken"

**note**:
- Operator cue: "healthz is the stethoscope check; alerts (U16) are the medical history. Both matter; different timescales."
- Next up (U2): the /ui/ Status tab, which is `healthz` visualized plus more.
