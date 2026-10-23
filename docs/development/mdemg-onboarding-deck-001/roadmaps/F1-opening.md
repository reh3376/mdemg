# F1 — Opening (Section Roadmap)

**Section family**: F1
**Status**: Epic 2 draft (section-level roadmap)
**Target slide count**: 12 (envelope 10–14 per global_roadmap.md)
**Governed by**: `../presentation_spec.md`, `../global_roadmap.md`

---

## 1 Why This Section Exists

The deck's audience (senior engineers / tech-PMs from the engineer track) must leave F1 with four durable mental hooks before any component-level detail lands:

1. What MDEMG *is* (one sentence they could quote back)
2. The core analogy (so the vocabulary later lands without needing the primer)
3. The honest history (so compromises later aren't surprises)
4. A system-overview diagram they can mentally refer back to for the next 90 minutes

F1 is **not** a 5-beat component block. Per presentation_spec §3 ("Deviation requires an operator-authorized exception documented in the section roadmap"), F1 is explicitly logged as the operator-authorized exception. The 5-beat template applies starting at F2.

## 2 Placement in Deck

- First section after title slide
- Precedes F2 (Substrate Core)
- Target duration: ~10 min (per presentation_spec §5 cadence — "Opening (first 10 slides): aim for 10 min")

## 3 Subsection Structure

| ID | Subsection | Slides | Content shape |
|---|---|---|---|
| F1.1 | Welcome + framing | 1 | Title, date, audience hint, "this is engineering-dense" posture |
| F1.2 | Overall Goal | 2 | Vision statement + the internal-dialogue analogy |
| F1.3 | History — key milestones | 3 | Timeline Mermaid + pivot moments + current state numbers |
| F1.4 | How it works at 10k ft | 5 | System overview + 5-layer hierarchy + hot path + learning loop + observability |
| F1.5 | Section wrap | 1 | 4-bullet recap + pointer to F2 |
| **Total** | | **12** | within envelope |

Optional stretch to 14 slides: split F1.4 system overview into two (static diagram + narrative walkthrough). Decide in Epic 5 (slide authoring) based on how the diagram fits at 1280×720.

## 4 Per-Subsection Specs

### F1.1 Welcome + framing (1 slide)

**Title**: `MDEMG — Cognitive Substrate for AI-Assisted Development`

**Content**:
- Byline, date (quarterly-refreshable `Last verified: YYYY-MM-DD` footer per presentation_spec §9)
- One-line audience hint: "Senior-engineer-shaped reader; engineering-dense; no fluff"
- One-line posture hint: "Trajectory-framed; honest about current state"

**Speaker notes**:
- "If anything here feels like a vague marketing claim, flag it — every substantive claim traces to a CLAUDE.md pin, a feature doc, a sprint record, or shipped code."
- Pause for questions on audience fit before continuing

**No diagram.**

**Provenance**: presentation_spec §1 (Audience + Posture)

---

### F1.2 Overall Goal (2 slides)

#### F1.2.1 Vision (1 slide)

**Title**: `What MDEMG Is Trying To Do`

**Content**: single-sentence vision quoted verbatim from CLAUDE.md's `project_mdemg_purpose` memory:

> **Help developers create software that connects in a manner that increases the likelihood of making better decisions.**

Three supporting bullets (concise, no fluff):
- Persistent memory + cognitive substrate — not a tool
- Decisions are the atomic unit of improvement — not features, not metrics
- Connections (memory → retrieval → classification → synthesis) are what the system actually optimizes

**Speaker notes**: "Everything that follows — Jiminy, J17, LoRA, UxTS — exists because it moves one of those connection-layer qualities. Rank future design options against that purpose, not against aggregate scores."

**No diagram** — the sentence is the deliverable.

**Provenance**: CLAUDE.md `MDEMG Core Purpose` section, `memory/project_mdemg_purpose.md`

#### F1.2.2 The core analogy (1 slide)

**Title**: `MDEMG : artificial neural network :: internal dialogue : biological neural network`

**Content**: the quoted analogy from CLAUDE.md:

> MDEMG is your (the LLM's) persistent memory and cognitive substrate. Markdown memory files are **REMINDERS AND KEYS only** — real memory lives in CMS (Neo4j graph, `mdemg-dev` space). When CMS is disconnected, your memory is disconnected.

Supporting:
- CMS disconnected ⇒ cognitive outage, not graceful degradation
- Hooks + MCP are the dialogue's sensory + motor channels
- The agent (Claude Code, in practice) is the "voice" that reads/writes to this substrate

**Speaker notes**: "The 'tool' framing — MDEMG as a plugin or library — is the wrong mental model. If you catch yourself calling MDEMG a tool during Q&A, pause and reframe."

**No diagram** — analogy stands alone.

**Provenance**: CLAUDE.md § MDEMG IS Your Internal Dialogue — Not a Tool

---

### F1.3 History — key milestones (3 slides)

#### F1.3.1 Timeline (1 slide)

**Title**: `History — Build Trajectory`

**Content**: left-to-right Mermaid timeline (`M1` in Epic 2 diagram list).

**Mermaid M1 content**:
```
Origin (concept)
  → Phases 1-105 core (substrate + retrieval + Hebbian)
  → Sidecar phases 101-105 (cognitive gaps closed)
  → RSIC self-improvement loop
  → Jiminy inner-voice + /strict enforcement
  → J17 protocol learning
  → LoRA on Qwen → mdemg-llm-v1 (Phase 5 SFT + Phase 13.5 llama.cpp cutover)
  → Recursive retraining loop (FT-RECURSIVE-001..004 shipped 2026-06..07)
  → UxTS forcing-function framework family (16 frameworks)
  → Guardrail producer + enforcement outcome loop (JIMINY-ENFORCE-001..005)
  → Current frontier: JIMINY-CEILING-BREAK-2 arc (follow rate >80% target)
```

Each node → year/quarter label. Keep ≤12 nodes to stay legible at 1280×720.

**Speaker notes**: "This is the shape of ~2 years of work. We'll zoom into each layer. The pivots in the next slide are what you'd read if you only had 5 minutes."

#### F1.3.2 Pivot moments (1 slide)

**Title**: `Honest Pivots`

**Content**: 4-6 bullet pivots, each a 1-sentence summary + the forcing-function moment:

- **2026-04-22 — MoE → dense pivot.** Metal 499K MTLResource ceiling blocked every non-trivial MoE LoRA backward pass. Qwen3-14B-4bit dense replaced Qwen3.6-35B-A3B.
- **2026-05-03 — mlx_lm.server → llama.cpp cutover.** mlx_lm.server crashed every ~14 min via Metal OOM; llama-server bounds KV architecturally.
- **2026-06-26 — Intent translation disabled.** UVTS A/B: intent OFF 0.4170 vs ON 0.4070 (net-negative) + was 70% of all LLM errors.
- **2026-04-22 — FT-OAI-003 dropped.** OpenAI fine-tuning no longer pursued; all FT targets local MLX LoRA.
- **2026-08-01 — Jiminy is an ENFORCER (not advisor).** Prior framing of /strict as opt-in enrichment superseded.

**Speaker notes**: "Each of these was a 'the data decided' call, not an opinion. The playbook is: measure before flipping; honest no-ops are legitimate verdicts."

#### F1.3.3 Current state — concrete numbers (1 slide)

**Title**: `Where We Are Today`

**Content** (table or bullet grid with real numbers from CLAUDE.md as of deck date):
- ~105 CLAUDE.md architecture pins
- ~16 LLM call sites (all single-shot, no-tool-calling policy)
- ~16 UxTS frameworks (UATS/UVTS/ULTS/UBTS/UOBS/UITS/UDTS/UETS/UOTS/UATS01/UBENCH/UAITS/UDTS/UTDS/USTS/UAMS)
- 28 ingested languages (verified via UPTS specs)
- Production model: `mdemg-llm-v1.Q5_K_M` (Qwen3-14B-4bit base + Phase 5 SFT LoRA) served via llama-server :8102
- Current honest aggregate benchmark: 0.9188 on 16-task augmented eval
- Open-arc: JIMINY-CEILING-BREAK-2 (actionable follow rate ~13% today, targeting ≥80%)

**Speaker notes**: "This slide gets quarterly refresh. The follow-rate number is the one that'll move most — it's the live measure of substrate quality."

**Provenance**: CLAUDE.md § Architecture Notes (all), CLAUDE.md MDEMG Fine-Tuning section, `docs/development/UXTS_FRAMEWORK_MATRIX.md`

---

### F1.4 How it works at 10k ft (5 slides)

#### F1.4.1 System overview diagram (1 slide)

**Title**: `MDEMG — System Overview`

**Content**: full-slide Mermaid diagram (`M2`) with ~12 nodes.

**Mermaid M2 content** (nodes + key edges):
- Nodes: Hook Channel, MCP, HTTP API (:9999), Retrieval Pipeline (RRF 4-column), Neo4j CMS (substrate), TSDB (telemetry + corpora), Hebbian Learning, RSIC Self-Assess, Jiminy Classifier, J17 Protocol, LLM Runtime (llama-server :8102), HITL Review, Grafana + Alerts
- Key edges: Hook → API (per-prompt events); API → Retrieval → CMS; CMS ↔ Hebbian (co-activation); API → Jiminy → LLM (classify); Jiminy → TSDB (outcomes); TSDB → RSIC (assess) → Insights → Actions; HITL → Substrate (reinforcement); All → Grafana (metrics)

**Design discipline**: edge labels name the ACTION (per presentation_spec §6: "promotes, rerank-feeds, scope-gates" — not the DATA). Light-theme-neutral styling.

**Speaker notes**: "Spend one minute per cluster — don't go deep. We'll return to each cluster in sections F2-F9."

#### F1.4.2 5-layer memory hierarchy (1 slide)

**Title**: `The 5-Layer Memory Hierarchy`

**Content**: stacked-boxes Mermaid (`M3`) showing L0 → L5 with 1-2 example nodes per layer.

**Mermaid M3 layers**:
- **L0** — raw observations (`obs_type ∈ constraint|correction|decision|error|progress|...`)
- **L1** — promoted durable rules (`role_type ∈ constraint|correction`) via consolidation gates
- **L2** — themes (clusters of observations by centroid cosine)
- **L3** — emergent concepts (abstracted patterns)
- **L4** — higher-order concepts
- **L5** — meta / global patterns

Each layer labeled with the Cypher predicate that promotes to it + the shipped consolidation step name.

**Speaker notes**: "The 5-layer hierarchy is what retrieval traverses. The abstraction edges (GENERALIZES, ABSTRACTS_TO) are what links L0→L5."

**Provenance**: CLAUDE.md § Project Context, `docs/features/hidden-pattern-identity.md`, `internal/hidden/*`

#### F1.4.3 Hot path — Claude Code → MDEMG (1 slide)

**Title**: `Hot Path — What Happens On a Tool Use`

**Content**: Mermaid sequence diagram (`M4`) tracing one Claude Code tool-use roundtrip.

**Mermaid M4 sequence**:
1. Claude agent emits tool_use → PostToolUse hook fires
2. Hook POSTs event to `/v1/conversation/observe` (if obs-worthy) OR `/v1/jiminy/classify` (if Write/Edit/Bash)
3. API resolves space, embeds, writes L0 observation
4. If Write/Edit/Bash under /strict: classifier runs → deny/pass verdict
5. If deny: pre-write-check hook blocks + emits HIGH alert
6. Else: /v1/jiminy/warm fires background guide computation
7. Next prompt: prompt-context hook fetches `/v1/jiminy/latest` + CMS recall
8. Guidance appended to prompt as system-reminder
9. Agent proceeds with guidance; feedback outcome recorded post-response

**Speaker notes**: "This loop happens on every single tool use. Observability is Grafana `mdemg-jiminy` dashboard + alert evaluator."

**Provenance**: `.claude/hooks/*.{sh,py}`, `internal/api/server.go`, `internal/jiminy/service.go`

#### F1.4.4 Learning loop (1 slide)

**Title**: `The Recursive-Improvement Arc`

**Content**: Mermaid flowchart (`M5`) with 4 inner loops:

**Mermaid M5 loops** (named):
1. **Hebbian**: observation → embed → retrieve → activation spreading → CO_ACTIVATED_WITH weight updates (per-retrieve); ApplyCoactivation + CoactivateSession + ApplySymbolCoactivation + ApplyNegativeFeedback
2. **Jiminy outcomes**: guidance surfaced → agent acts → feedback classified → `constraint_outcomes` row + `GUIDANCE_OUTCOME` edge → trust EMA update
3. **RSIC**: 7-dim health assessment → LLM reflector → insights → recommended actions (gate-guarded; dispatcher dispatches)
4. **FT-RECURSIVE** (Phase 6+): production drift → export → curate → train → convert → gate → promote (default-off; opt-in via `FT_LOOP_ENABLED`)

HITL review loop sits beside, feeding operator-graded corpora back into substrate reinforcement.

**Speaker notes**: "Four loops at different time constants: Hebbian is sub-second; Jiminy is per-tool-use; RSIC is per-30-min; FT is per-week. All share the same substrate."

**Provenance**: `docs/features/rsic-orchestration.md`, `docs/features/ft-recursive-loop.md`, `docs/features/hitl-review.md`, `docs/features/event-graph-federation.md`

#### F1.4.5 Observability (1 slide)

**Title**: `How We Know It's Working`

**Content**: three-column view:
- **Metrics in-flight**: `/v1/metrics/snapshot` (JSON); `metric_samples` TSDB table (historical); honor the TSDB-only access model
- **Dashboards**: 8 Grafana dashboards (`mdemg-rsic`, `mdemg-jiminy`, `mdemg-j17`, `mdemg-llm-routing`, `mdemg-ft-training`, `mdemg-hitl`, `mdemg-graph-topology`, `mdemg-overview`)
- **Alerts**: ~35 TSDB-query alert rules + dispatcher; `~/.mdemg/alerts/current.json` file backend + optional macOS notifications

**Speaker notes**: "The alert noise reduction sprint (MDEMG-ALERT-NOISE-REDUCTION-001, 2026-10-22) is the latest example of the alert-truth family — most 'scary' alerts are measurement artifacts, not real regressions. The dashboard-truth family (001/002/003) is the paper trail."

**Provenance**: `docs/features/service-alerting.md`, `deploy/docker/grafana/dashboards/*.json`, `internal/alert/rules.go`, CLAUDE.md § Alert correctness family

---

### F1.5 Section wrap (1 slide)

**Title**: `F1 Recap → F2`

**Content**: 4-bullet mental model to carry into F2:
1. MDEMG = cognitive substrate, not a tool
2. 5-layer hierarchy (L0 observations → L5 meta concepts) is what retrieval traverses
3. Every Claude Code tool use completes a hook → classify → observe → guide → feedback loop
4. Four learning loops run at different time constants against the same substrate

Pointer: "Next: F2 — Substrate Core (Neo4j CMS + Hebbian + consolidation)"

**Speaker notes**: "Pause for 2 minutes of questions before F2. This is the heaviest Q&A gate — unresolved confusion here contaminates everything downstream."

## 5 Mermaid Diagram Inventory (F1)

| ID | Where | Diagram | Rough node count |
|---|---|---|---|
| M1 | F1.3.1 | History timeline (left-to-right phases) | 12 |
| M2 | F1.4.1 | System overview | ~12 nodes + ~18 edges |
| M3 | F1.4.2 | 5-layer memory hierarchy stack | 6 layers × 2 example nodes |
| M4 | F1.4.3 | Hot-path sequence diagram | 5 participants, 9 steps |
| M5 | F1.4.4 | Learning-loop flowchart | 4 inner loops |

All light-theme-neutral per presentation_spec §6. If M2 exceeds 20 nodes, split per presentation_spec §6 into 2 slides.

## 6 Factual-Provenance Index for F1

Required per presentation_spec §8 — every substantive claim traces to one of:

| Subsection | Primary sources |
|---|---|
| F1.1 | presentation_spec.md §1 |
| F1.2.1 | CLAUDE.md § MDEMG Core Purpose; `memory/project_mdemg_purpose.md` |
| F1.2.2 | CLAUDE.md § MDEMG IS Your Internal Dialogue — Not a Tool |
| F1.3.1 | CLAUDE.md § Architecture Notes (chronological pin index) |
| F1.3.2 | CLAUDE.md § Project Phase 5 MoE pivot; CLAUDE.md § Local LLM Runtime; CLAUDE.md § INTENT-DISABLE-001; CLAUDE.md § JIMINY-ENFORCE-001 |
| F1.3.3 | CLAUDE.md § MDEMG Fine-Tuning Shipped State + Policies; `docs/development/UXTS_FRAMEWORK_MATRIX.md`; CLAUDE.md § JIMINY-CEILING-BREAK-2 |
| F1.4.1 | `internal/api/server.go` route list; `docs/features/*` |
| F1.4.2 | `docs/features/hidden-pattern-identity.md`; `internal/hidden/*` |
| F1.4.3 | `.claude/hooks/*`; `internal/api/handlers_jiminy.go`; `internal/jiminy/service.go` |
| F1.4.4 | `docs/features/rsic-orchestration.md`; `docs/features/ft-recursive-loop.md`; `docs/features/hitl-review.md`; `docs/features/event-graph-federation.md` |
| F1.4.5 | `docs/features/service-alerting.md`; `deploy/docker/grafana/dashboards/*.json`; `internal/alert/rules.go` |
| F1.5 | synthesis of above |

Each content doc produced in Epic 3 for F1 MUST include a provenance footer listing exactly these sources (plus any new ones its draft adds).

## 7 Open Design Decisions

- **F1.3.3's quarterly refresh**: current numbers rot fast. Add a shell script `scripts/deck_f1_refresh.sh` in Epic 7 that regenerates the concrete-numbers slide from `grep -c '^- \*\*' CLAUDE.md`, UxTS matrix count, benchmark TSDB query. Decision: ship with the current snapshot in Epic 4; add the script in Epic 7 (docs epic).
- **M2 node count**: 12 is the target. If audience hand-tests in Epic 6 show cognitive overload, split into "Core 6" (Hook/API/Retrieval/CMS/TSDB/LLM) + "Learning 6" (Hebbian/RSIC/Jiminy/J17/HITL/FT) on sequential slides.
- **Pause-for-questions anchors**: F1.1 (audience fit), F1.3.2 (pivot moments — operator-opinionated), F1.5 (recap). Marked explicitly in speaker notes.
- **Live-metric slot in F1.4.5**: tempting to pull a live Grafana screenshot. Decision: use a styled annotated screenshot asset in `docs/onboarding/assets/`, NOT a live embed. Static assets rotate quarterly with the deck.

## 8 Dependencies on Other Section Roadmaps

- F1.4.1's system overview depends on each later F2-F9 subsection owning a known cluster. If F2 reclassifies a subsystem (e.g., moves "Context Fingerprinting" from F2 to F3), re-check M2's node list in Epic 5.
- F1.3.3's "current state" numbers overlap with F10 (Conclusion). F10 extends with "what's left" + "where we're going"; F1.3.3 is the snapshot.
- F1.4.4's learning-loop boxes mirror F7 (Ops) + F4 (Jiminy/J17) + F5 (LoRA/Qwen) coverage. F7 and F4 go deeper; F1.4.4 is deliberately shallow.

## 9 Non-Goals (F1)

- No deep-dive into any component (that's F2-F9)
- No benchmarks, no scoring details beyond the current aggregate
- No code snippets (deferred to F2+ where mechanisms get examined)
- No API contract tables (feature docs are reference material — this is onboarding)
- No live screenshots that depend on a running stack

## 10 Documents Accessed

- `../sprint_plan.md`
- `../presentation_spec.md`
- `../global_roadmap.md`
- CLAUDE.md § MDEMG IS Your Internal Dialogue — Not a Tool
- CLAUDE.md § MDEMG Core Purpose
- CLAUDE.md § Project Context
- CLAUDE.md § MDEMG Fine-Tuning — Shipped State & Policies
- CLAUDE.md § Architecture Notes (chronological pin index — F1.3.1 timeline source)
- `memory/project_mdemg_purpose.md`
- `docs/development/UXTS_FRAMEWORK_MATRIX.md`
- `docs/features/hidden-pattern-identity.md` (for M3)
- `docs/features/rsic-orchestration.md` (for M5 loop 3)
- `docs/features/ft-recursive-loop.md` (for M5 loop 4)
- `docs/features/service-alerting.md` (for F1.4.5)
