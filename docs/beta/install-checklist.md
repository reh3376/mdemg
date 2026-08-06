# MDEMG Beta Install Checklist (v0.11.0-beta.1)

**Print this. Fill it in as you go. Submit as a GitHub issue when done.**

Fastest path from `brew install` to a running dashboard + a first observation you can inspect. If you hit a snag at ANY step, that's data — note it in the "Notes" column and keep going. Blockers should be filed as issues under [Beta Install Report](https://github.com/reh3376/mdemg/issues/new?template=beta-install-report.yml).

**Time budget**: ~30 minutes end-to-end for a clean install (add ~10-15 min for Docker Desktop install if not present).

---

## Prerequisites

Check these before you start. Fill in what you have; note what's missing.

| # | Requirement | ✓ | Value / version | Notes |
|---|---|---|---|---|
| P1 | macOS 12+ (Apple Silicon) **OR** Linux (Ubuntu 20.04+/Debian 11+/Fedora 36+) **OR** WSL2 (Windows 10 build 19041+) | ☐ | | |
| P2 | Docker Desktop (macOS/Windows) or Docker Engine (Linux) with Compose v2, **running** | ☐ | `docker --version`: | Must succeed: `docker info` (no error) |
| P3 | 4+ GB free RAM | ☐ | free memory: | Neo4j + TSDB + Grafana + MDEMG + neural-sidecar ≈ 2.3 GB |
| P4 | 5+ GB free disk | ☐ | | Docker images + Neo4j data volumes |
| P5 | Internet access | ☐ | | For `brew install`, Docker image pulls |
| P6 (optional) | `OPENAI_API_KEY` in shell env | ☐ | `echo $OPENAI_API_KEY \| head -c 10`: | Optional — MDEMG runs in disabled mode without one |

---

## Tier 1: Install & First Boot (~15 min)

Each row is a checkpoint. Circle ✅ / ❌ / ⏭. Note the exact error text or unexpected output.

### T1.1 — Install

**macOS** (this order is REQUIRED):
```bash
brew trust reh3376/mdemg    # ← without this, brew install fails with a Sorbet stack trace
brew tap reh3376/mdemg
brew install mdemg
```

**Linux / WSL2:**
```bash
curl -fsSL https://raw.githubusercontent.com/reh3376/mdemg/main/scripts/install.sh | bash
```

- [ ] ✅ PASS  [ ] ❌ FAIL  [ ] ⏭ SKIP
- Notes: _______________

### T1.2 — Verify binary

```bash
mdemg version
```

Expected: `mdemg version 0.11.0-beta.1`

- [ ] Version reads exactly `0.11.0-beta.1` — [ ] ✅ [ ] ❌
- If different: what does it say? _______________

### T1.3 — Initialize + auto-start

```bash
cd ~/mdemg-beta-test   # or wherever you want to test
mdemg init --defaults
```

Expected on init:
- Files created: `.env`, `.mdemg/config.yaml`, `.mdemgignore`, `docker-compose.yml`
- Docker containers start automatically (5 services: neo4j, timescaledb, mdemg, neural-sidecar, grafana)
- If no `OPENAI_API_KEY` set: a "⚠ LLM & Jiminy DISABLED" summary prints with 2 next-step paths
- Final "MDEMG Docker deployment ready!" block prints dashboard/Grafana/Neo4j URLs

- [ ] All 4 files created — [ ] ✅ [ ] ❌
- [ ] All 5 containers `Up (healthy)` after ~90s (`docker compose ps`) — [ ] ✅ [ ] ❌
- [ ] "Deployment ready" summary printed — [ ] ✅ [ ] ❌
- Notes: _______________

### T1.4 — Set up your MDEMG_BASE_URL

```bash
export MDEMG_PORT=$(grep '^MDEMG_PORT' .env | cut -d= -f2)
export MDEMG_BASE_URL="http://localhost:${MDEMG_PORT}"
echo "Using $MDEMG_BASE_URL"
```

Use `$MDEMG_BASE_URL` in every subsequent test — init assigns a **dynamic port** (usually 9999, but bumps if 9999 is taken).

- [ ] `MDEMG_BASE_URL` set — [ ] ✅ [ ] ❌
- Port assigned: _______________

### T1.5 — Health check

```bash
curl -sS "$MDEMG_BASE_URL/healthz"
```

Expected: JSON with `"status":"ok"` and `"checks"` map showing `neo4j`, `tsdb`, `circuit_breakers` all `ok`.

- [ ] Health returns ok — [ ] ✅ [ ] ❌
- Full response: _______________

### T1.6 — Config validate

```bash
mdemg config validate
echo "exit=$?"
```

Expected one of (all exit 0 = healthy):
- `Validation: PASSED`
- `Validation: PASSED (services not started — run: docker compose up -d)` — if services aren't up yet
- `Validation: FAILED (errors found)` — exit 1 = real problem, file an issue

- [ ] Exit code 0 — [ ] ✅ [ ] ❌
- Which outcome: _______________

### T1.7 — Open the dashboard

```bash
open "$MDEMG_BASE_URL/ui/"      # macOS
xdg-open "$MDEMG_BASE_URL/ui/"  # Linux
```

Expected: browser opens to the MDEMG dashboard (dark theme, sidebar with Memory/Constraints/Jiminy/Config tabs). All tabs load without error.

- [ ] Dashboard opens — [ ] ✅ [ ] ❌
- [ ] All top-nav tabs load — [ ] ✅ [ ] ❌
- Any 404s or blank tabs? _______________

---

## Tier 2: First-Hour Value (~15 min)

### T2.1 — Write your first observation

```bash
curl -sS -X POST "$MDEMG_BASE_URL/v1/conversation/observe" \
  -H "Content-Type: application/json" \
  -d '{"space_id":"beta-first","session_id":"onboarding","content":"MDEMG is up and I just wrote my first observation","obs_type":"note"}'
```

Expected response: JSON with `obs_id`, `node_id`, `surprise_score`, `summary`.

- [ ] Got obs_id + node_id — [ ] ✅ [ ] ❌
- Response: _______________

### T2.2 — Verify it landed in the graph

Open the dashboard → **Memory tab** → filter by `space_id=beta-first`. Your observation should appear.

- [ ] Observation visible in dashboard — [ ] ✅ [ ] ❌

### T2.3 — Retrieve it back

```bash
curl -sS -X POST "$MDEMG_BASE_URL/v1/memory/retrieve" \
  -H "Content-Type: application/json" \
  -d '{"space_id":"beta-first","query_text":"first observation","top_k":3}'
```

Expected: JSON with `results` array containing your observation (BM25 match on "first observation").

- [ ] Retrieval returns the observation — [ ] ✅ [ ] ❌

### T2.4 — Ingest a small codebase

```bash
mkdir -p ~/mdemg-test-project && cd ~/mdemg-test-project
git init && git config user.email "beta@example.com" && git config user.name "Beta"
cat > main.go <<'EOF'
package main
import "fmt"
func main() { fmt.Println("hello from beta") }
EOF
git add . && git commit -m "initial"
cd - > /dev/null

mdemg ingest ~/mdemg-test-project --space-id beta-first
```

Expected: ingest completes, prints file count + observations created.

- [ ] Ingest succeeded — [ ] ✅ [ ] ❌
- Files processed: _______________, observations created: _______________

### T2.5 — Inspect your space

```bash
mdemg space list
```

Expected: table showing `beta-first` space with a non-zero node count.

- [ ] `beta-first` space present with observations — [ ] ✅ [ ] ❌

---

## When you're done

**If all green** — you're in a good spot. Continue with **Tiers 3-7** in the full [beta plan](../../packaging/homebrew-mdemg/mdemg_beta_testing.md) at your own pace.

**Submit this checklist** — either:
1. Fill in the **[Beta Install Report](https://github.com/reh3376/mdemg/issues/new?template=beta-install-report.yml)** GitHub issue form (mirrors this checklist), OR
2. Attach this filled-in file to a new issue with the label `beta`.

**If ANY row is ❌** — file a **[Beta Bug Report](https://github.com/reh3376/mdemg/issues/new?template=beta-bug-report.yml)** with the exact test number, expected vs actual, and (if available) attach a diagnostic bundle from `mdemg diagnostics collect` (ships in Sprint A — coming soon).

**If something worked but felt harder than it should have** — file a **[Beta Feature Friction](https://github.com/reh3376/mdemg/issues/new?template=beta-feature-friction.yml)** report. Docs gaps, unclear error messages, ergonomic missteps are all valuable feedback.

---

## Quick reference — the "shape" of a healthy fresh install

```
~/mdemg-beta-test/
├── .env                        # dynamic ports, RSIC_PROTECTED_SPACES seeded
├── .mdemg/
│   ├── config.yaml
│   └── backups/
├── .mdemgignore
├── docker-compose.yml
└── bin/mdemg                   # copy for hooks
```

**5 Docker services** (`docker compose ps`):
- `mdemg` (the server, port MDEMG_PORT)
- `neo4j` (graph DB, ports 7687 bolt + 7474 http, dynamic)
- `timescaledb` (metrics + telemetry, port 5433 dynamic)
- `neural-sidecar` (Python re-ranker, port 8100 dynamic)
- `grafana` (dashboards, port 3000 dynamic — login: admin/admin)

**Total RAM footprint**: ~2.3 GB fresh; grows with ingested data.

---

Thanks for testing MDEMG! Your feedback shapes what beta.2 looks like.
— Roger Henley (`rogerhenley345@gmail.com`)
