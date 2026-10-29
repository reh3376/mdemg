# U16 · Alerts + Enforcement Escape Hatches (7 slides)

**Intent**: Terminal banner + severity map + alerts channel mechanics + `mdemg jiminy mode` + `mdemg jiminy override` + the auto-execute policy + audit trail.
**Target slides**: 7
**Mermaid**: 1 (U16.5, override flow)
**Natural Q&A anchor**: after U16

---

## U16.1 · Section Landing

**Title**: `U16 · Alerts + Enforcement, Operator Triage`

**Body**:
- Intent: "This is where you RESPOND. Alerts tell you what's wrong (historical, hours-scale). Enforcement is Jiminy preventing a durable constraint violation (right-now). Both ship escape hatches that preserve operator agency."
- Preview: alerts channel → terminal banner → severity map → clearing → enforcement in-session → override flow → audit trail
- Points back to U1.4 "How does it solve them?", the ENFORCEMENT surface

**Provenance**:
- CLAUDE.md "Service Alert System (SR-001 + SNA-001)" + "JIMINY-ENFORCE arc"

---

## U16.2 · The Alerts Channel

**Title**: `The Alerts Channel`

**Body**:
- Storage: `~/.mdemg/alerts/current.json` (configurable via `ALERT_FILE_PATH`); JSON array of alert objects
- Shape: `{severity, service, title, body, timestamp, cleared_at: null}`
- Severities (3 shipped):
  - **CRITICAL**: immediate operator attention; typically triggers macOS notification
  - **HIGH**: significant degradation; operator-triage within hours
  - **MEDIUM**: regression trajectory, often arcs-in-progress; operator-aware
  - **LOW**: informational trajectory (occasional)
- Sources (3): RSIC alert actions (U5), circuit breaker state changes (U2), health prober transitions, server-native alert evaluator (24+ TSDB-query rules), Grafana webhook (backward compat)
- The 44 pending alerts on your session start banner are a mix of MEDIUM (Jiminy follow-rate drops, arcs-in-progress per JIMINY-CEILING-BREAK-2) + HIGH (FT benchmark staleness, scheduled-job-staleness) + LOW (cache hit ratio)

**Provenance**:
- CLAUDE.md "Service Alert System" + "Scheduled-job health (NOSILENT-001)"
- `internal/alert/` package

**note**:
- Operator cue: "The banner at session start summarizes active alerts. Each line is one alert, severity-coded."

---

## U16.3 · Terminal Banner Pattern

**Title**: `What The Banner Tells You`

**Body**:
- Session-start hook (`.claude/hooks/session-start.sh`) reads `current.json` and surfaces CRITICAL + HIGH alerts
- Prompt-context hook (`.claude/hooks/prompt-context.sh`) shows ALL pending alerts every prompt
- Format: `[SEVERITY] [timestamp] service: title, body`
- Reading the banner:
  - Group by severity (CRITICAL + HIGH first)
  - Group by service (same service multiple fires = root cause probably one issue)
  - Age matters: a 30-min-old alert is current; a 2-week-old alert is stale (JIMINY-FOLLOW-RATE-REMEASURE-001 trajectory re-check)
- Clearing: hooks auto-clear surfaced alerts via `POST /v1/alerts/clear` (HOOKSYNC-001, cleared = delivered)

**Provenance**:
- CLAUDE.md "Hook stdin contract (HOOKWIRE-001)" + "Hook channel health (HOOKSYNC-001)"
- `.claude/hooks/session-start.sh`

**note**:
- Operator cue: "If the banner feels overwhelming, triage by severity first, service second. 44 pending breaks down into maybe 5 distinct ROOT issues."

---

## U16.4 · Enforcement In-Session

**Title**: `When Jiminy Blocks You`

**Body**:
- When active:
  - Mode = `strict` AND
  - A durable constraint has WARNED+ escalation on current session AND
  - No active override applies
- Example block (hook output on Write/Edit/Bash):
  ```
  [/strict] Durable constraint violation
    code: never-direct-main-commits
    severity: must_not
    reason: Attempted git commit directly to main branch
    override: mdemg jiminy override apply --constraint never-direct-main-commits --reason "cherry-pick post-merge" --duration 15m
  ```
- Blocked tool call RETURNS error (not silent) with copy-paste override command inline
- Classify endpoint `/v1/jiminy/classify` is the server-side decision site; `pre-write-check.py` + `pre-bash-check.py` hooks fire it before every mutation

**Provenance**:
- CLAUDE.md "JIMINY-ENFORCE-001" (strict-by-default; alert-on-block; fail-open-with-warning)
- CLAUDE.md "JIMINY-ENFORCE-002" (Bash coverage)
- CLAUDE.md "JIMINY-CLASSIFY-ESCALATION-INSPECT-001" (code surfaced in deny)

**note**:
- Operator cue: "The block message is COPY-PASTE READY. Don't fight it; use the override command inline if you need to proceed."

---

## U16.5 · Mermaid #7, Override Flow

**Title**: `The Override Escape Hatch`

**Body**:
- **Mermaid #7** (horizontal flow):
  1. Agent attempts action (Write / Edit / Bash)
  2. Hook POSTs to `/v1/jiminy/classify`
  3. Classifier returns `verdict=deny` with code
  4. Hook EITHER blocks (strict mode) OR surfaces warning (suggest mode)
  5. Operator OPTIONALLY runs `mdemg jiminy override apply --constraint CODE --reason "..." --duration 15m`
  6. Override lands in `~/.mdemg/jiminy-overrides.jsonl` + `constraint_overrides` TSDB hypertable (ENFORCE-OVERRIDES-TSDB)
  7. On next hook invocation, classifier respects override + returns `verdict=pass`
  8. Agent proceeds; audit row stays FOREVER for compliance review
- Invariants:
  - Reason REQUIRED (not optional)
  - Duration > 0 REQUIRED (forgotten overrides can't silently disable enforcement)
  - Session-scoped (not global)
  - Audit via BOTH JSONL + TSDB (portability + queryability)

**Provenance**:
- CLAUDE.md "JIMINY-ENFORCE-003" (operator escape-hatch CLI) + "JIMINY-ENFORCE-004" (RSIC enforcement-learning outcomes)
- `internal/jiminy/override.go`

**note**:
- Operator cue: "Overrides EXPIRE. 15m is my default. Longer durations require more justification in the reason."

---

## U16.6 · RSIC Auto-Execute Policy

**Title**: `When The System Mutates Itself`

**Body**:
- 3 potential auto-executable actions, all DEFAULT OFF:
  - `archive_constraint_by_code` (ENFORCE-AUTO-EXECUTE): auto-tombstones a constraint when `enforcement_false_positive_high` fires N times
    - 5 guards: enabled flag · empty-check · protected-space check · rate limit + per-code cooldown · dry-run (defaults true)
  - `trigger_training_pipeline` (FT-RECURSIVE-002): opens a retrain cycle
    - Gated by `FT_LOOP_ENABLED=false` default
  - `adjust_guidance_confidence` (deferred): not auto-executable yet; non-trivially reversible, HITL-only
- Rule: `archive_*` boolean mutations auto-executable; `adjust_*` scalar mutations require HITL
- Audit: every auto-execute emits a MEDIUM alert (`rsic-auto-archive` service) with REVERSAL CYPHER in the message body, operator sees + can reverse

**Provenance**:
- CLAUDE.md "RSIC auto-execute for enforcement patterns (ENFORCE-AUTO-EXECUTE)" (3/3 of JIMINY-ENFORCE arc)

**note**:
- Operator cue: "By default, the system doesn't mutate itself. If you flip auto-execute on, read the 5-guard sequence in CLAUDE.md FIRST."

---

## U16.7 · Gotchas + Where To Go Deeper

**Title**: `Alerts + Enforcement, Gotchas + More`

**Body**:
- Gotchas:
  - `/v1/alerts/clear` is RECEIPT-OF-DELIVERY, not RESOLUTION, the underlying condition may still be true next evaluation window
  - 44 pending alerts on your session is NOT a crisis, most are MEDIUM Jiminy follow-rate (arc-in-progress) + HIGH FT benchmark staleness (one issue, re-evaluates hourly)
  - Enforcement is session-scoped (per `session_id`); multi-agent setups need per-agent session IDs
  - Overrides expire, forgotten overrides can't silently disable enforcement forever (invariant: duration > 0)
  - Auto-execute is OPT-IN; `ENFORCEMENT_AUTO_EXECUTE_ENABLED=false` default
  - Cooldown-key contract (NOSILENT-001): distinct Services per rule, two rules sharing `(service, severity)` key would suppress each other
- Feature docs:
  - `docs/features/service-alerts.md` (or CLAUDE.md "Service Alert System" section)
  - `docs/features/jiminy-strict.md`
  - `docs/features/jiminy-overrides.md`
- Where this feeds into: U5 RSIC (feeds back the enforcement-learning signal), U7 HITL (contradicted drafts from missed-violation detector), U13 Grafana (`mdemg-jiminy` + `mdemg-overview` alert panels)

**note**:
- Operator cue: "Alerts are HONEST SIGNALS. Enforcement is PREEMPTIVE PROTECTION. Overrides are CONSCIOUS DISAGREEMENT. The three together are the operator's full agency surface."
- Transition: "U17 next, where to go from here."
