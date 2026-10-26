# U3 — Memory (CMS) (8 slides)

**Intent**: The central idea. CMS mental model (L0 obs → L1 constraint/correction → L2+ emergent concept), `/ui/ Memory` tab, `mdemg data observe|recall|correct`.
**Walk-time**: ~7 min
**Target slides**: 8
**Mermaid**: 1 (U3.2 — hierarchy flow)
**Natural Q&A anchor**: after U3

**Depth template**:
- Landing — U3.1
- Mental model — U3.2 (Mermaid: layer hierarchy)
- Mental model continued — U3.3 (Hebbian learning + consolidation)
- Where to see — U3.4 (/ui/ Memory tab)
- How to touch — U3.5 (observe), U3.6 (recall), U3.7 (correct)
- Current state + gotchas — U3.8

---

## U3.1 — Section Landing

**Title**: `U3 · Memory — The Cognitive Substrate`

**Body**:
- Intent: "MDEMG remembers. Not in RAM, not in logs — in a Neo4j graph that grows, consolidates, and forgets honestly."
- Preview: hierarchy mental model → UI → observe/recall/correct → honest state
- Natural question this answers: "What does MDEMG actually store, and how do I inspect or change it?"

**Provenance**:
- CLAUDE.md "MDEMG IS Your Internal Dialogue" + "Memory Principles"

---

## U3.2 — Mental Model: The L0→L1→L2+ Hierarchy

**Title**: `How Memory Is Shaped`

**Body**:
- 1-sentence context: "MDEMG stores raw observations and gradually distills them into reusable rules and concepts."
- **Mermaid #2** (vertical flow):
  - L0 (`conversation_observation`, `obs_type=...`) — raw operator/agent utterances
  - L1 (`role_type=constraint|correction`) — promoted rules extracted from L0 patterns
  - L2 (`role_type=hidden_pattern`) — clusters of L0 that co-activate
  - L3+ (`role_type=emergent_concept`) — abstractions across layers
- Each layer: fewer nodes, more durable, higher-value
- Role-filtered retrieval scopes by layer (e.g., Jiminy only surfaces L1+)

**Provenance**:
- CLAUDE.md "Hidden-pattern identity + consolidation completion (CHURN-002)"
- CLAUDE.md "JIMINY-CORRECTION-PRODUCER-001" (L1 correction producer)
- `internal/hidden/` package

**note**:
- Operator cue: "L0 is a river; L1 is where sediment hardens into rules; L2+ is topology."

---

## U3.3 — Hebbian Learning + Consolidation

**Title**: `How Memory Strengthens`

**Body**:
- Hebbian principle: nodes that co-activate on the same retrieval → `CO_ACTIVATED_WITH` edge weight increases
- Consolidation cycles (background, every ~5 min): fire → identify clusters → promote stable patterns to L1/L2
- EVENTGRAPH-001 federates each reinforcement into TSDB `reinforcement_events`
- Scheduled maintenance: weekly decay + prune (LaunchAgent) removes archived + low-weight nodes
- CHURN-002: consolidation runs under `context.WithoutCancel` so it completes even if the triggering request walked away

**Provenance**:
- CLAUDE.md "Event Graph Federation (EVENTGRAPH-001)"
- CLAUDE.md "Stable concept identity + coverage (HIDDEN-CHURN-001)"
- CLAUDE.md "Scheduled maintenance is LIVE (MAINT-LIVE-001)"

**note**:
- Operator cue: "You don't trigger consolidation. It runs. You see the OUTCOME in the Memory tab."

---

## U3.4 — /ui/ Memory Tab

**Title**: `The Memory Tab`

**Body**:
- Where: `/ui/` → **Memory**
- Shows: node counts by role_type/layer, spaces, recent activity, archived vs live
- Interaction: filter by space, drill into a space's node histogram
- When you'd open it: "did my `observe` land?" / "how big is mdemg-dev today?" / "any orphan nodes?"
- Related: U7 Review tab shows the HITL queue over memory writes

**Provenance**:
- `internal/api/ui/tabs/memory.js`

**note**:
- Operator cue: "If this tab is empty, your `mdemg data observe` isn't landing. Fix THAT before anything else."

---

## U3.5 — `mdemg data observe`

**Title**: `Writing To Memory`

**Body**:
- CLI:
  ```bash
  $ mdemg data observe --space-id mdemg-dev \
      --content "The thing you want remembered" \
      --obs-type decision
  obs_id: ckh3uf9fv0003qr5x2a8dnmwl
  node_id: ckh3uf9fv0004qr5x5kz3bgpe
  ```
- obs_type cheat sheet: `decision`, `correction`, `error`, `preference`, `task`, `technical_note`, `insight`, `context`, `progress`, `constraint`, `learning`, `note`
- Observations always land at L0; promotion to L1 (if `obs_type=correction` or tagged `constraint:...`) happens at next consolidation
- Each observation is embedded (OpenAI or disabled); embedding failures DON'T block the observation itself

**Provenance**:
- CLAUDE.md "Observe Continuously" section with the 12 obs_type table
- `POST /v1/conversation/observe` HTTP surface

**note**:
- Operator cue: "The hooks observe for you automatically. Explicit CLI observes are for edge cases: back-filling, correcting, or scripting."

---

## U3.6 — `mdemg data recall`

**Title**: `Reading From Memory`

**Body**:
- CLI:
  ```bash
  $ mdemg data recall --query "how do I toggle enforcement mode" --space-id mdemg-dev
  [0] role=correction  score=0.82  "mdemg jiminy mode strict|suggest ..."
  [1] role=learning    score=0.74  "..."
  [2] role=constraint  score=0.71  "..."
  ```
- Under the hood: 4-column RRF (embedding + BM25 + graph + structural); CONTEXT-FINGERPRINTING-001 adds a 5th column
- URL overrides: `?sparse=true|false`, `?intent=true|false`, `?reverse_ref=true|false` for A/B
- Related: MCP tool `memory_recall` is the agent-facing version (CLAUDE.md "MCP memory channel")

**Provenance**:
- CLAUDE.md "Column-Voting Retrieval (Phase 13 + 13.1)"
- CLAUDE.md "Context Fingerprinting (Phase 14.2.3)"

**note**:
- Operator cue: "If recall feels empty, the first check is `/ui/ Memory` — maybe the content isn't there yet."

---

## U3.7 — `mdemg data correct`

**Title**: `Correcting Memory`

**Body**:
- CLI:
  ```bash
  $ mdemg data correct --space-id mdemg-dev \
      --incorrect "mdemg uses mlx-server at :8101" \
      --correct "mdemg uses llama-server at :8102 (Phase 13.5 cutover)" \
      --context "Model serving"
  obs_id: ...
  (will promote to L1 on next consolidation)
  ```
- Structured fields land on the L0 obs; JIMINY-STRUCTURED-CORRECTION-001 propagates them to the eventual L1 node
- `incorrect` + `correct` + `context` triple is first-class — guidance synthesis uses both sides
- Related: U7 Review HITL — contradicted drafts auto-feed into this surface via JIMINY-CONTRADICTED-BRIDGE-001

**Provenance**:
- CLAUDE.md "Structured correction propagation (JIMINY-STRUCTURED-CORRECTION-001)"
- CLAUDE.md "Contradicted-outcome → correction draft bridge (JIMINY-CONTRADICTED-BRIDGE-001)"

**note**:
- Operator cue: "Correct beats re-observe. The structured triple is the training signal."

---

## U3.8 — Current State + Gotchas

**Title**: `Memory Honest State + Gotchas`

**Body**:
- Live substrate on `mdemg-dev` typically runs:
  - ~90K+ MemoryNodes across all layers; non-archived ~70-80%
  - 128K+ CO_ACTIVATED_WITH edges with weight entropy ~0.27
  - Consolidation cycles ~every 5 min, completing in ~1-10s (post-CONSOLIDATE-PERF-001)
- Gotchas:
  - `mdemg-dev` is PROTECTED SPACE — hardcoded deletion protection; never bypassed
  - L0 obs with no `obs_type` tag defaults to `note` (lowest promotion priority)
  - Scheduled maintenance LaunchAgent default is `--dry-run=false` only for the SCHEDULED run; manual `mdemg maintenance` defaults to dry-run (safety)
  - Archived nodes stay forever by design (reversible); `graph repair` doesn't delete them
- Where to go deeper: `docs/features/consolidation-performance.md`, CLAUDE.md "RSIC admission + archival attribution"

**Provenance**:
- CLAUDE.md "Scheduled maintenance is LIVE (MAINT-LIVE-001)"
- CLAUDE.md "Protected space `mdemg-dev`"
- CLAUDE.md "Consolidation Performance (CONSOLIDATE-PERF-001)"

**note**:
- Operator cue: "Protected-space rule applies even in dev. If you're bulk-deleting, you're doing it wrong — tombstone via `is_archived=true` instead."
- Next up (U4): Jiminy — what USES this memory to tell the agent what to do.
