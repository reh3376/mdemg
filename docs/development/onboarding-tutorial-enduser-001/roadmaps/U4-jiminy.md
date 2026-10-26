# U4 · Jiminy, Guidance + Enforcement (11 slides)

**Intent**: Jiminy is MDEMG's "inner voice", it surfaces durable rules before every agent action, classifies the outcome, and (in strict mode) enforces durable constraints. This section covers mental model, classifier mechanics, rules corpus, /ui/ Jiminy + Rules tabs, mode toggle, override escape, current trajectory, and gotchas.
**Target slides**: 11 (no-caps policy, may expand at E3 authoring if specific sub-topics need more)
**Mermaid**: 2 (U4.2, guidance loop; U4.4, override flow is in U16, cross-ref)
**Natural Q&A anchor**: after U4

**Depth template coverage** (expanded for Jiminy, the headline topic):
- Landing, U4.1
- Mental model (guidance loop), U4.2
- Mental model (classifier mechanics), U4.3
- Rules corpus mental model, U4.4
- Where to see, U4.5 (/ui/ Jiminy), U4.6 (/ui/ Rules)
- How to touch, U4.7 (`mode`), U4.8 (`rules` CRUD)
- Current state (trajectory), U4.9
- Gotchas, U4.10
- Where to go deeper, U4.11

---

## U4.1 · Section Landing

**Title**: `U4 · Jiminy, The Inner Voice`

**Body**:
- Intent: "Jiminy is the subsystem that READS memory before every agent action, SURFACES applicable durable rules, CLASSIFIES whether the agent followed them, and (in strict mode) ENFORCES them."
- Preview: 11-slide walkthrough, loop → classifier → corpus → UI → CLI → current state → gotchas
- Points back to U1.3 "What problems does it solve?", this is the primary answer to "behavioral drift"
- Points back to U1.4 "How does it solve them?", surface #2 (retrieval + guidance)

**Provenance**:
- CLAUDE.md "Jiminy" + "/strict Mode" sections
- `docs/features/jiminy-*.md` (12 shipped feature docs)

**note**:
- Operator cue: "This is the headline section of the end-user path. 11 slides. Buckle in."

---

## U4.2 · Mental Model: The Guidance Loop

**Title**: `How Jiminy Thinks`

**Body**:
- **Mermaid #4** (loop):
  1. Agent is about to act (or has just acted)
  2. **Hook fires** (pre-write, pre-bash, or post-tool-observe)
  3. **Hook POSTs** to `/v1/jiminy/latest` with the action context
  4. **Jiminy retrieves** applicable `role_type=constraint|correction` nodes via embedding + metadata
  5. **Jiminy synthesizes** a terse directive from the top-K candidates
  6. **Agent sees** the directive in-prompt before deciding
  7. **After the action**, hook POSTs `/v1/jiminy/feedback` with the action summary
  8. **Jiminy classifier** verdict: `followed` / `ignored` / `partial_compliance` / `contradicted` / `not_applicable`
  9. **Outcome lands** in `constraint_outcomes` TSDB hypertable → feeds trust score + RSIC + Grafana
- One-sentence summary: "Jiminy is a loop: retrieve, surface, classify, measure, strengthen."

**Provenance**:
- CLAUDE.md "Jiminy effectiveness-tracker (JIMINY-EFFECTIVENESS-001)" + "JIMINY-OUTCOME-001" + "JIMINY-CLASSIFIER-CONTEXT-001"
- `internal/jiminy/service.go` main entry point

**note**:
- PM-fallback: "Jiminy is Jiminy Cricket from Pinocchio, the external conscience reminding the agent of durable rules before it acts, then recording whether the agent listened."

---

## U4.3 · Mental Model: Classifier Mechanics

**Title**: `How Jiminy Grades`

**Body**:
- Three verdict tiers:
  - **Tier 1** (fast-path): embedding-similarity gate on `not_applicable` (sub-threshold = unrelated)
  - **Tier 2** (LLM): routes `[naThreshold, lowThreshold)` AND `≥highThreshold` through LLM classifier for honest follow/ignore decision
  - **Tier 3** (process observer): hooks and git-commits grade DIFFERENT verifiability classes (process/human), see UxTS §U6
- The 5 verdicts:
  - `followed`, action aligned with constraint
  - `ignored`, action within relevant context but didn't apply the rule
  - `partial_compliance`, action partially aligned (0.5 credit historically; recalibrated post-JIMINY-HEURISTIC-DEFAULT-001)
  - `contradicted`, action directly opposed the constraint (highest-signal; feeds auto-correction via JIMINY-CONTRADICTED-BRIDGE-001)
  - `not_applicable`, rule doesn't govern the action's context (routed via `nonViolationCreditClause` + `mechanismScopeCreditClause`)
- Verifiability classes (JIMINY-METRIC-PARTITION-001): classifier / process / hybrid / human, different signals, different writers, different gauges

**Provenance**:
- CLAUDE.md "JIMINY-TIER1-BYPASS-001" + "JIMINY-CLASSIFIER-CONTEXT-002" + "JIMINY-METRIC-PARTITION-001"
- `internal/jiminy/outcome_classifier.go`

**note**:
- Operator cue: "You don't need to grade things yourself. The classifier does. You SEE outcomes in Grafana + /ui/ Jiminy. If a classifier verdict feels wrong, that's the HITL Review tab (U7)."

---

## U4.4 · Rules Corpus Mental Model

**Title**: `What Jiminy Reads`

**Body**:
- Rules live as `MemoryNode` with `role_type IN ['constraint', 'correction']`
- Each rule carries:
  - `constraint_code` (mnemonic ID like `never-direct-main-commits`)
  - `constraint_type` severity (`must` / `must_not` / `should` / `should_not` / `info` / `deadline`)
  - `is_informational` (boolean, meta/epistemic rules that don't grade per-action)
  - `verifiability_class` (classifier / process / hybrid / human, JIMINY-METRIC-PARTITION-001)
- Rules come from THREE sources:
  1. **Hook-promoted L0 observations**: operator `observe --obs-type=correction` → consolidation → L1 correction node (JIMINY-CORRECTION-PRODUCER-001)
  2. **UI-authored**: `/ui/ Rules` tab lets operators add/edit directly (JIMINY-RULES-UI-001)
  3. **Auto-mined**: tagged `conversation_observation` content triggering `constraint:` regex (JIMINY-CORPUS-CONSTRAINT-DETECTOR-DEDUP-001)
- Current corpus state (`mdemg-dev`): ~30-40 active rules post-JIMINY-CORPUS-003 purge + JIMINY-CORRECTION-CORPUS-001 cleanup

**Provenance**:
- CLAUDE.md "JIMINY-CORPUS-003" + "JIMINY-CORRECTION-CORPUS-001" + "JIMINY-RULES-UI-001"

**note**:
- Operator cue: "The corpus IS the behavioral surface area. If a rule shouldn't exist, tombstone it in /ui/ Rules. If a rule is missing, author it in /ui/ Rules or `observe --obs-type=correction`."

---

## U4.5 · /ui/ Jiminy Tab

**Title**: `The Jiminy Tab`

**Body**:
- Where: `/ui/` → **Jiminy**
- Shows: current mode (strict/suggest), active overrides table (CODE | reason | expires), recent override timeline from `constraint_overrides` TSDB (ENFORCE-OVERRIDES-TSDB sprint), help panel
- Interaction: 2-button Mode toggle (strict ↔ suggest); per-row Revoke on active overrides
- Related: U16 Alerts + Enforcement is the OPERATIONAL twin of this view (what alerts fire, how to respond)

**Provenance**:
- CLAUDE.md "JIMINY-MODE-001" + "ENFORCE-UI-OVERRIDES"
- `internal/api/ui/tabs/jiminy.js`

**note**:
- Operator cue: "If you only remember two /ui/ tabs, make them Jiminy and Rules."

---

## U4.6 · /ui/ Rules Tab (CRUD)

**Title**: `The Rules Tab, Author Durable Rules`

**Body**:
- Where: `/ui/` → **Rules**
- Shows: filterable table of all `role_type=constraint|correction` nodes with `constraint_code`, content preview, severity, is_informational
- Actions: Inline accordion Expand → Edit (tombstone-and-recreate semantic preserving round-1 immutable-tombstone lock) → Save; Tombstone (confirms via `prompt()`); Create (gated by `JIMINY_RULES_UI_WRITE_ENABLED`, default off; operator flips before authoring)
- Dedup warn: on Create, if cosine sim ≥ `JIMINY_RULES_DEDUP_SIM_THRESHOLD` (0.75) against any existing rule, UI prompts "nearly duplicate, override?"
- Related: `mdemg jiminy constraint mark` CLI mirror for `is_informational` flipping (JIMINY-INFORMATIONAL-CATEGORY-001)

**Provenance**:
- CLAUDE.md "JIMINY-RULES-UI-001" + "JIMINY-INFORMATIONAL-CATEGORY-001"
- `internal/api/ui/tabs/rules.js`

**note**:
- Operator cue: "Write-flag defaults OFF for safety. Flip it in Config (U11) when you're actually editing the corpus."

---

## U4.7 · `mdemg jiminy mode <strict|suggest>`

**Title**: `Mode Toggle From The CLI`

**Body**:
- Two modes:
  - **suggest** (default on fresh install): Jiminy SURFACES rules in-prompt but does NOT block tool calls
  - **strict**: Jiminy BLOCKS Write/Edit/Bash tool calls when a WARNED+ escalation on an un-overridden constraint fires; blocked tool call returns actionable error with override command
- CLI:
  ```bash
  $ mdemg jiminy mode
  mode: suggest  session: claude-core  strict_enabled: false

  $ mdemg jiminy mode strict
  mode: strict  session: claude-core  strict_enabled: true
  → state file written: ~/.mdemg/.jiminy-strict-mode

  $ mdemg jiminy mode suggest
  mode: suggest ...
  → state file removed
  ```
- State is PER-SESSION (session_id); default session is `claude-core`

**Provenance**:
- CLAUDE.md "JIMINY-MODE-001" + "JIMINY-ENFORCE-001"
- `internal/cli/jiminy_mode.go`

**note**:
- Operator cue: "Flip to strict when you want REAL enforcement. Suggest mode is observability-only."

---

## U4.8 · `mdemg jiminy override` + CLI Rules CRUD

**Title**: `Overrides + Rules From The CLI`

**Body**:
- Override escape hatch:
  ```bash
  $ mdemg jiminy override apply --constraint never-direct-main-commits \
      --reason "post-merge cherry-pick, verified" \
      --duration 15m
  ```
- Reason is REQUIRED (CLAUDE.md invariant); duration must be > 0 (forgotten overrides can't silently disable enforcement forever)
- Audit trail: dual-write to `~/.mdemg/jiminy-overrides.jsonl` + `constraint_overrides` TSDB hypertable (ENFORCE-OVERRIDES-TSDB)
- Related CLI: `mdemg jiminy override list [--session-id]`, `mdemg jiminy override revoke --constraint CODE`
- Rules CRUD CLI (minimal): `mdemg jiminy constraint mark --code X --informational=true|false` (JIMINY-INFORMATIONAL-CATEGORY-001)
- Rules add/edit at scale lives in /ui/ Rules (U4.6), CLI is for scripted session management

**Provenance**:
- CLAUDE.md "JIMINY-ENFORCE-003" (override CLI + audit) + "ENFORCE-OVERRIDES-TSDB"
- `internal/cli/jiminy_override.go`

**note**:
- Operator cue: "Overrides are time-boxed by design. Expired overrides AUTO-DISABLE; the audit row stays."

---

## U4.9 · Current State (Honest Trajectory)

**Title**: `Jiminy Today, Honest`

**Body**:
- Current state (`mdemg-dev`, 2026-10):
  - Mode: strict (operator opt-in per JIMINY-MODE-001 default-off)
  - Follow rate (classifier-verifiable): **~13%** (current baseline)
  - Target: **≥80%** via the JIMINY-CEILING-BREAK-2 arc
  - Phase 1 corpus purge SHIPPED (JIMINY-CORPUS-003); Phase 2 retrieval tightening SHIPPED (LEVER-C-TIGHTEN-002); Phase 3 classifier semantics SHIPPED (JIMINY-CLASSIFIER-CONTEXT-002 + JIMINY-TIER1-BYPASS-001); Phase 4a HITL velocity SHIPPED; Phase 4b retrain + Phase 5 model swap IN PROGRESS
- Honest framing (trust-signal rule):
  - WRONG: "follow rate 13% is low BY DESIGN because the corpus is honest"
  - RIGHT: "follow rate ~13% today; arc is JIMINY-CEILING-BREAK-2; trajectory expected to approach 80% through 2026-Q4 as phases 4b/5 ship"
- Per-class gauges (`mdemg_jiminy_follow_rate_<class>_verifiable`): classifier/process/hybrid; human-class auto-grade-dismissed per HITL-AUTO-DISMISS-001

**Provenance**:
- CLAUDE.md "JIMINY-CEILING-BREAK-2 arc" (sprint record)
- CLAUDE.md JIMINY-CEILING-INVESTIGATION-002 (follow-rate ceiling arithmetic)

**note**:
- Operator cue: "The low number is honest. The arc moving it is named. That's the posture across the whole system."

---

## U4.10 · Gotchas

**Title**: `Jiminy Gotchas`

**Body**:
- **Mode is session-scoped**: toggling `strict` on session A doesn't affect session B; agents use `MDEMG_SESSION_ID` env
- **Overrides don't persist beyond expiry**: if you override `never-direct-main-commits` for 15m and go to lunch, your post-lunch commit is blocked again
- **`is_informational` rules DON'T grade** per-action, they surface via Lever D (concept bias) but don't contribute to follow-rate
- **The classifier can mislabel**: HITL Review tab (U7) is where you correct false `ignored` / false `followed` verdicts
- **Fresh install starts in `suggest`**: strict mode is operator opt-in; see JIMINY-MODE-001 default-off reasoning
- **Rule tombstoning is REVERSIBLE** (`is_archived=true` with reason); never bypass the archive mechanism to hard-delete

**Provenance**:
- CLAUDE.md "JIMINY-MODE-001" + "JIMINY-INFORMATIONAL-CATEGORY-001" + "JIMINY-CORPUS-003 tombstone-only pattern"

**note**:
- Operator cue: "If you're ever tempted to `cypher DELETE` a constraint node, STOP. Tombstone it."

---

## U4.11 · Where To Go Deeper

**Title**: `Jiminy, More`

**Body**:
- Feature docs:
  - `docs/features/jiminy-strict.md`, enforcement + override + fail-open semantics
  - `docs/features/jiminy-actionability.md`, Lever A/B/C/D levers
  - `docs/features/jiminy-rules-ui.md`, UI tab mechanics
  - `docs/features/jiminy-metric-partition.md`, per-class gauges
  - `docs/features/jiminy-missed-violation.md`, post-hoc correction detector
- CLAUDE.md arch pins (12+ Jiminy sprints documented)
- The master arc record: ` docs/development/jiminy-ceiling-break-2/`
- Where this feeds into: U5 RSIC (follow-rate improvement is a self-improvement loop), U7 HITL (manual verdict correction), U8 Learning/Training (retrain corpus)

**note**:
- Operator cue: "Jiminy touches every other topic. From here, U5 RSIC shows what the system does with the signal Jiminy produces."
