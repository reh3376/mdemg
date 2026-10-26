# U2 · The Stack + Status + Health (7 slides)

**Intent**: Show the operational foundation, 5 containers + mdemg server, the 4-command happy path, /ui/ Status, `healthz`, watchdog, breakers. Everything else in U3-U17 lives inside this picture.
**Walk-time**: ~6 min
**Target slides**: 7
**Mermaid**: 1 (U2.2, stack sketch)
**Natural Q&A anchor**:, (operator pauses after U3 Memory next)

**Depth template coverage**:
- Landing, U2.1
- Mental model, U2.2 (Mermaid: 5-container stack)
- Where to see, U2.3 (`docker compose ps`, /ui/ Status, `healthz`)
- How to touch, U2.4 (4-command happy path)
- How to touch, U2.5 (healthz CLI deep dive)
- Mental model, U2.6 (watchdog + circuit breakers)
- Current state + gotchas, U2.7

---

## U2.1 · Section Landing

**Title**: `U2 · The Stack`

**Body**:
- Intent: "Everything in U3-U17 lives inside the 5-container stack. This section is the foundation, the stack picture, how to confirm it's up, how to probe its health, and what the watchdog + breakers protect against."
- Preview: stack diagram → `docker compose ps` → 4-command path → `healthz` → watchdog → breakers → gotchas
- Points back to U1.4 "How Does It Solve Them?", this is the implementation of surface #1 (observability + the substrate it runs on)

**Provenance**:
- CLAUDE.md "Project Context" + Docker Compose section

---

## U2.2 · Mental Model: 5 Containers + 1 Server

**Title**: `What's Running`

**Body**:
- 1-sentence context: "A healthy MDEMG stack is 5 Docker containers coordinated by one mdemg server process."
- **Mermaid #2** (horizontal flow):
  - operator → `mdemg server :9999`
  - `mdemg server` → `{neo4j :7687, timescaledb :5433, neural-sidecar :8100, grafana :3030}`
  - All five labeled as "Docker Compose services"
- One-line role each:
  - **mdemg**: HTTP API + CLI backend (ghcr.io/reh3376/mdemg)
  - **neo4j**: graph substrate (CMS; the "cognitive" layer, U3 Memory)
  - **timescaledb**: telemetry hypertables (TSDB; the "measurement" layer, U13 Grafana)
  - **neural-sidecar**: rerank + NLI model inference (U15 APIs)
  - **grafana**: read-only observability panels (U13)

**Provenance**:
- `docker-compose.yml` top-level `services:` section
- CLAUDE.md "Multi-Instance" note: "Each instance: 5 containers, 6 ports, ~2.3 GiB RAM"

**note**:
- Operator cue: "This is the whole stack. Nothing else is required to run MDEMG."
- PM-fallback: "mdemg = brain; neo4j = long-term memory; timescaledb = EEG; neural-sidecar = math co-processor; grafana = X-ray screen."

---

## U2.3 · Where To See It: `docker compose ps`

**Title**: `Confirm The Stack`

**Body**:
- CLI:
  ```bash
  $ docker compose ps
  NAME                        STATUS                 PORTS
  mdemg-mdemg-1               Up (healthy)           0.0.0.0:9999->9999/tcp
  mdemg-neo4j-1               Up (healthy)           0.0.0.0:7687->7687/tcp
  mdemg-timescaledb-1         Up (healthy)           0.0.0.0:5433->5432/tcp
  mdemg-neural-sidecar-1      Up (healthy)           0.0.0.0:8100->8100/tcp
  mdemg-grafana-1             Up (healthy)           0.0.0.0:3030->3000/tcp
  ```
- Expected: 5 × `Up (healthy)`
- Anything else (`Restarting`, `Exit (0)`) is a triage trigger, `docker compose logs <service>` is the next command

**Provenance**:
- `docker-compose.yml` port mappings

**note**:
- Operator cue: "This is the ground truth of 'is the stack up?'. Everything else assumes YES to this question."

---

## U2.4 · How To Touch It: The 4-Command Happy Path

**Title**: `The 4-Command Happy Path`

**Body**:
- 1-sentence context: "From zero to open dashboard in four commands. The smoke-test for every fresh install, every machine, every day."
- CLI block:
  ```bash
  mdemg init --defaults        # creates .env + .mdemg config (idempotent)
  docker compose up -d         # starts the 5-service stack
  mdemg healthz                # verify server + subsystems
  open http://localhost:9999/ui/   # dashboard (macOS)
  ```
- Variants: `--quick` alias (CLAUDE.md); `xdg-open` on Linux; `mdemg restart` for one-shot refresh
- The `mdemg tutorial` command (SERVE-001) is the one-shot that opens THIS deck

**Provenance**:
- CLAUDE.md Docker (primary) section
- `docs/features/tutorial-serve.md`
- `INIT-DEFAULTS-LOCAL-FIRST-001` sprint

**note**:
- Operator cue: "If any of these 4 fails, triage THAT command before anything else. Logs → ports → env file."

---

## U2.5 · `mdemg healthz` Deep Dive

**Title**: `healthz, The Right-Now Probe`

**Body**:
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
- Fix order on `degraded`:
  1. `docker compose ps`, is the subsystem actually up?
  2. `docker compose logs <subsystem>`, is it erroring?
  3. `curl -s http://localhost:9999/healthz | jq`, fresh HTTP reading
- Exit codes: 0 if `status=ok`, 1 otherwise, scriptable in `until` loops

**Provenance**:
- CLAUDE.md "Enhanced /healthz" section (DH-004)
- `internal/api/handlers_health.go`
- CONFIG-VALIDATE-TRANSIENT-DISTINGUISH-001 sprint, "not started" vs "broken"

**note**:
- Operator cue: "healthz is cheap. Run it. Don't guess."

---

## U2.6 · Watchdog + Circuit Breakers

**Title**: `What The Watchdog Watches`

**Body**:
- The LLM endpoint watchdog (`MLX_WATCHDOG_ENABLED=true`): probes `127.0.0.1:8102/v1/models` every 5s
- States: `up → degraded → down` with hysteresis; fast-fail short-circuits retries at `down`
- CLI: `mdemg watchdog status` (or `--json` for scripts)
- Circuit breakers: 24+ named breakers across LLM calls, retrieval, consulting, Jiminy; `GET /v1/admin/breakers` surfaces state (gated by `AUTH_API_KEYS`)
- DH-004 escape hatch: `POST /v1/admin/breakers/reset {"name":"<name>"}` manually closes a tripped breaker
- /ui/ Status tab visualizes watchdog + breaker state (U11 Config also surfaces this)

**Provenance**:
- CLAUDE.md "Local LLM Runtime" + "DH-004 Dashboard Remediation"
- `internal/mlxprobe/`
- `/v1/admin/breakers` HTTP surface

**note**:
- Operator cue: "When the LLM is slow, these breakers prevent the whole system from cascade-failing on retry storms."

---

## U2.7 · Current State + Gotchas

**Title**: `Honest State + Gotchas`

**Body**:
- Current typical state on a fresh-installed, well-provisioned machine:
  - `healthz: ok` most of the time
  - Occasional `degraded` on `sidecar: timeout` during cold-start model load (resolves ~15s)
  - Breakers almost always `open: 0`
- `/ui/ Status` tab is the GUI version of `healthz` + more (uptime, version, config knobs)
- Gotchas:
  - `health: ok` with pending HIGH alerts (U16) is NOT contradictory, healthz samples NOW, alerts integrate over hours
  - `mdemg healthz` from a shell without `.env` loaded can show stale readings (CLI reads config at invoke time)
  - `docker compose ps` showing `Up` but `healthz` saying `degraded` means the subsystem's TCP port is open but the handler is misbehaving
- Where to go deeper: `docs/features/mlx-watchdog.md`, CLAUDE.md "Service Alert System"

**Provenance**:
- CLAUDE.md DH-004 + CONFIG-VALIDATE-TRANSIENT-DISTINGUISH-001

**note**:
- Operator cue: "healthz is the stethoscope check; alerts (U16) are the medical history. Both matter; different timescales."
- Transition: "Stack confirmed. Next up: U3, what MDEMG REMEMBERS, the central idea."
