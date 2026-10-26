# U11 — Config + Features (7 slides)

**Intent**: `/ui/ Config` + `/ui/ Features` tabs — what's safe to toggle live, what requires restart, how the ~400 env vars are organized, sensitive-value redaction.
**Target slides**: 7
**Mermaid**: 0 (tables carry the content)

---

## U11.1 — Section Landing

**Title**: `U11 · Config + Features — The Toggle Surface`

**Body**:
- Intent: "~400 config knobs ship with MDEMG. Most defaults are correct. This section covers the ones operators flip + how to tell which can be flipped live vs needing a restart."
- Preview: /ui/ Config → /ui/ Features → ENV-FROM-ENV priority chain → sensitive-value redaction → gotchas

**Provenance**:
- CLAUDE.md "Config: struct fields → `FromEnv()` → struct literal (~3500 lines)"
- `internal/config/config.go`

---

## U11.2 — /ui/ Config Tab

**Title**: `The Config Tab`

**Body**:
- Where: `/ui/` → **Config**
- Shows: current effective config values (post-FromEnv overlay); grouped by domain (LLM / embeddings / retrieval / Jiminy / RSIC / backup / etc.)
- Interaction:
  - View current effective values (sensitive values auto-masked — `AUTH_API_KEYS`, `TSDB_PASSWORD`, `RERANK_JINA_API_KEY`, `GRAFANA_PASSWORD`, `LINEAR_WEBHOOK_SECRET`, `AUTH_JWT_SECRET`, 2 more per SEC-TRANCHE-3)
  - See which values are env-override vs code-default
  - For admin-flagged knobs, flip live (gated by `AUTH_API_KEYS`)
- Related CLI: `mdemg config show` + `mdemg config validate`

**Provenance**:
- CLAUDE.md "Code-scanning tranche 3 — isSensitive widened to 8 env-var names (SEC-TRANCHE-3)"
- `internal/api/ui/tabs/config.js`

**note**:
- Operator cue: "If an env var LOOKS sensitive but shows unmasked, that's a BUG — file it. isSensitive is the gatekeeper."

---

## U11.3 — /ui/ Features Tab

**Title**: `The Features Tab — Feature Flags`

**Body**:
- Where: `/ui/` → **Features**
- Shows: shipped feature flags with their current enabled state; `*_ENABLED` boolean knobs grouped by feature family
- Common operator-touched flags:
  - `TUTORIAL_ENABLED` (U-this-sprint; default true)
  - `JIMINY_STRICT_DEFAULT_ENABLED` + `JIMINY_MODE` (U4)
  - `FT_LOOP_ENABLED` (U8)
  - `RETRIEVAL_COLUMN_VOTING_ENABLED` + `RETRIEVAL_CONTEXT_COLUMN_ENABLED` (U3 retrieval surface)
  - `GUARDRAIL_PRODUCER_ENABLED` (prod-time guardrail producer)
- HEBB-ETA-001 arch rule: behavior-changing feature flags default OFF in BOTH code AND `.env`
- Interaction: toggle flags; most take effect at next restart, some live (per-flag)

**Provenance**:
- CLAUDE.md "HEBB-ETA-001" default-off invariant + specific feature docs
- `internal/api/ui/tabs/features.js`

**note**:
- Operator cue: "Features tab is less-visited than Config but EQUALLY important. If something feels missing, it may be `*_ENABLED=false` here."

---

## U11.4 — Config Priority Chain

**Title**: `Where Does A Value Come From?`

**Body**:
- Priority (highest to lowest wins):
  1. CLI flag override (per-command, e.g. `mdemg start --port 9999`)
  2. Env var set in process environment (shell `export`)
  3. `.env` file (loaded by `mdemg init` and compose; dotenv-style)
  4. Code default in `internal/config/config.go` struct literal
- `mdemg config validate` (CONFIG-VALIDATE-TRANSIENT-DISTINGUISH-001):
  - Validates present config
  - Distinguishes "not started" (transient, exit 0) from "broken" (persistent, exit 1)
  - Needed because fresh install without services up got FAILED exit 1 pre-sprint

**Provenance**:
- CLAUDE.md "`config validate` distinguishes 'not started' from 'broken' (CONFIG-VALIDATE-TRANSIENT-DISTINGUISH-001)"
- `internal/cli/config_cmd.go`

**note**:
- Operator cue: "A `FAILED` from `config validate` on a fresh install usually means services aren't started yet. Run `docker compose up -d` first."

---

## U11.5 — Sensitive-Value Redaction

**Title**: `What's Hidden From You`

**Body**:
- `internal/config/yaml_config.go::isSensitive` masks these in `mdemg config show`:
  - `AUTH_API_KEYS` — admin API-key list
  - `AUTH_JWT_SECRET` — JWT signing key
  - `TSDB_PASSWORD` — TSDB connect password
  - `LINEAR_WEBHOOK_SECRET` — Linear webhook signing
  - `RERANK_JINA_API_KEY` — if using Jina rerank
  - `GRAFANA_PASSWORD` — Grafana admin
  - `NEO4J_PASSWORD` — Neo4j connect
  - `OPENAI_API_KEY` — OpenAI client key (if set)
- Any env var with `PASSWORD` / `PASS` / `API_KEY` / `TOKEN` / `SECRET` in name should be masked; SEC-TRANCHE-3 widened the list from 2 to 8
- Rule: when adding a new secret-shaped env var, extend `isSensitive` in the SAME COMMIT

**Provenance**:
- CLAUDE.md "SEC-TRANCHE-3 isSensitive widening" + arch rule pin

**note**:
- Operator cue: "If an operator's screen-share reveals a credential, that's a `isSensitive` bug. File it; don't patch locally."

---

## U11.6 — No-Hardcoded-Values Rule

**Title**: `Why Every Operator-Visible Value Is Config-Driven`

**Body**:
- CLAUDE.md invariant `feedback_no_hardcoded_values`: "Never hardcode values — use CLI flags, env vars, config with sensible defaults."
- Why: makes the system operable without code changes; makes operator-authored overrides discoverable
- Rule applies to:
  - Thresholds (alert floors, orphan ratios, latency SLOs)
  - Timeouts (every LLM call site; every HTTP handler)
  - Rate limits (RSIC per-action, auto-execute per-hour)
  - Paths (model dir, backup dir, assets dir)
  - Model names (`LLM_MODEL`, `OPENAI_MODEL`)
- The MODEL-DIST-001 model distribution sprint pinned this: "every operator-visible value is dynamic" — 11+ env vars for one feature

**Provenance**:
- CLAUDE.md MODEL-DIST-001 + `feedback_no_hardcoded_values` memory pin
- Numerous sprint records enforcing the rule

**note**:
- Operator cue: "If you ever see a hardcoded value in a code review, flag it. The system's operability depends on this."

---

## U11.7 — Gotchas + Where To Go Deeper

**Title**: `Config + Features — Gotchas + More`

**Body**:
- Gotchas:
  - DOC-CURRENCY-002 forcing function: `scripts/verify_doc_env_vars.py` catches env vars named in docs but absent from code (prevents the "fabricated env var that no-ops when set" class); run after any doc change
  - `.env` loaded by `mdemg init` BUT NOT by shell commands unless you `source .env` — a bare `mdemg healthz` reads shell env, not `.env`
  - Compose templates in `internal/cli/compose_templates/` must stay in sync with repo-root `docker-compose.yml` (CI gate)
  - Behavior-changing flags default OFF in code AND `.env` (HEBB-ETA-001 — mentioned 20+ sprint pins)
- Feature docs:
  - `docs/features/*.md` (41 shipped feature docs; most carry a Config section)
  - CLAUDE.md "Campaign Configuration" section lists ~15 common operator-touched flags
- Where this feeds into: U15 APIs (`/v1/admin/*` surface flips flags via API), U16 Alerts (threshold values live here)

**note**:
- Operator cue: "If you're about to flip a flag, read the feature doc FIRST — most behavior-changing flags have invariant protections you need to understand."
- Transition: "U12 next — Plugins, the extension surface for external code."
