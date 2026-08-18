# JIMINY-SUBSTRATE-NATIVE-001 — Master Arc

**Opened**: 2026-08-18 · branch `reh3376_dev01` · authorized by operator directive 2026-08-18

## The correction

**MDEMG is the infrastructure. Jiminy is the internal dialogue. LoRA adapters (`mdemg-llm-v1`) assist the dialogue.**

Explicit hierarchy:
- **MDEMG framework** — graph DB topology, retrieval columns (RRF+rerank+diversity), 22+-phase consolidation, Hebbian learning + precision-η, RSIC self-improvement, event-graph federation, TSDB event streams, hooks, alerts, jobhealth — the infrastructure that stores + weights + reinforces + self-improves knowledge.
- **Jiminy** — the internal dialogue that emerges from MDEMG's infrastructure serving rules/instructions/observations. Speaks before every action. Reads from MDEMG's shipped primitives.
- **`mdemg-llm-v1` + future LoRA adapters** — reasoning + phrasing + synthesis assistance for the dialogue. NOT the memory. NOT the decider. NOT the fact store. The stylist.

## The drift being corrected

Recent Jiminy sprint activity (JIMINY-CORPUS-001..003, CLASSIFIER-CONTEXT-001/002, HEURISTIC-DEFAULT, LEVER-C-TIGHTEN-001/002, META-SCOPE, ACTIONABILITY-INVERSION, CEILING-BREAK-2 arc) has been LLM-prompt-clause engineering + retrieval-config tuning. Each fixes a real failure mode. But each moves discriminating intelligence **out of the topology** (where it COMPOUNDS across sessions + users via Hebbian, edge weights, consolidation, RSIC learning) and **into per-call LLM prompts** (where it doesn't compound — stateless per call).

Concrete symptoms:
- `outcome_classifier.go` prompt grew ~4× (2200→8000 chars) across 4 stacked clauses
- `JIMINY_TIER1_BYPASS_ENABLED=true` routes ~90% of feedback verdicts to LLM (was ~35%)
- Constraint discovery is a per-source loose union of 4-5 parallel Neo4j vector queries — **bypasses RRF, activation-spreading, edge attention, Hebbian weights**
- Jiminy uses 19 node-property primitives but IGNORES: `CO_ACTIVATED_WITH` weight reads, activation spreading, `GENERALIZES`/`ABSTRACTS_TO`/`GROUNDED_BY` edges, `SpreadingActivationWithAttention`, `activation_confidence` (precision-η), layer-scoped filtering, RSIC health signals as inputs, symbol-graph edges, `n.tags` filtering, consolidation output (L2+ emergent concepts)

## The correction direction

**Make MDEMG's shipped infrastructure the load-bearing intelligence in Jiminy's dialogue. Retire the accumulated LLM-prompt clauses as their topology equivalents come online. Reframe LoRA adapters from fact-storage to dialogue-quality.**

## Phase plan (arc, not one sprint)

Each phase is 1-3 sprints. Phases execute IN ORDER — later phases depend on earlier infrastructure. Everything ships default-off or opt-in until validated live.

### Phase A — MDEMG infrastructure repair (prerequisite)

**Goal**: fix the gaps preventing MDEMG's shipped topology from doing what it was designed for.

- **A1: INGEST-TOPOLOGY-REPAIR-001** (in progress, task #126) — write `n.content` on MemoryNode via `IngestObservation` (was writing to separate `Observation` node only); extend 5 retrieval columns to project content via `coalesce(n.content, latest HAS_OBSERVATION content)`; add `IncludeContent` opt-in flag on `RetrieveRequest`; synthesis prompt renders full content when present; backfill legacy nodes; deterministic-observation-picker via `ORDER BY o.created_at DESC LIMIT 1`.
- **A2: GROUNDED-BY-TRAVERSAL-001** (new) — traverse `GROUNDED_BY` skip-connections when retrieval surfaces L≥1 emergent-concept nodes; attach top-N grounded L0 verbatim content as `Evidence` (field already exists on `RetrieveResult`). Makes VISION.md line 434's design promise operational.
- **A3: `GET /v1/memory/nodes/{id}/content`** (new, small) — low-level primitive for debug + ad-hoc queries. Cheap add-on.

### Phase B — Substrate-native constraint discovery in Jiminy

**Goal**: replace Jiminy's parallel raw vector queries with topology-driven discovery.

- **B1: ACTIVATION-DRIVEN-DISCOVERY-001** — replace Lever C's raw `vector.similarity.cosine` scan + Sources A/B/C's independent vector queries with a SINGLE call to `SpreadingActivationWithAttention` over the query's node neighborhood. Typed-edge attention weights favor `IMPLEMENTS_CONSTRAINT`/`IMPLEMENTS_CORRECTION`/`CO_ACTIVATED_WITH` for constraint discovery. Result: one graph walk decides candidates + relevance ordering in one pass. Deprecates 4 parallel vector queries.
- **B2: HEBBIAN-EFFECTIVENESS-PRIOR-001** — replace Lever B's internal counter (`effectivenessPriorRates`) with the actual substrate signal: `CO_ACTIVATED_WITH.weight` (Hebbian weighted-successful surfacing) + `GUIDANCE_OUTCOME` reinforcement counters (already in graph). Removes the parallel Jiminy-owned counter; effectiveness is now the substrate's Hebbian signal.
- **B3: PRECISION-CONFIDENCE-WEIGHTING-001** — down-weight surfaces from nodes with low `activation_confidence` (HEBB-ETA-001 precision-η). Jiminy currently ignores this; the graph already computes it.

### Phase C — Layer + edge-aware surfacing

**Goal**: surface emergent concepts as guidance (currently filtered out); use edge types for applicability.

- **C1: EMERGENT-CONCEPT-SURFACING-001** — remove the `role_type IN ['constraint','correction']` filter that hides L2+ emergent concepts from Jiminy. Categorize emergent concepts as pattern-scale / principle-scale guidance (distinct from imperative rules). Consolidation already produces these — Jiminy just needs to consume them.
- **C2: GROUNDED-EVIDENCE-CONSUMPTION-001** — when Jiminy surfaces an L2+ emergent concept, follow `GROUNDED_BY` (from Phase A2) to attach L0 verbatim evidence. The concept says the pattern; the grounded L0s prove it.
- **C3: RSIC-HEALTH-SIGNAL-CONSUMPTION-001** — Jiminy reads RSIC health at Guide-time. Low-confidence dimensions → down-weight related surfaces. High-effectiveness constraints → up-weight. Currently Jiminy ignores all RSIC signals for its own decisions.

### Phase D — Prompt clause retirement

**Goal**: as topology handles cases previously handled by classifier prompt clauses, retire the clauses one by one. Each retirement is guarded by data (comparison A/B).

- **D1: RETIRE-MECHANISM-SCOPE-CLAUSE-001** — replaced by activation-spread neighborhood scoping (Phase B1) + scope-gate filter (already ships). Retire the ~1600 chars clause + gate.
- **D2: RETIRE-CONTEXT-MISMATCH-CLAUSE-001** — replaced by activation-spread's neighborhood-relevance filtering. Retire the ~1200 chars clause + gate.
- **D3: RETIRE-MENTION-VS-PERFORM-CLAUSE-001** — replaced by edge-type semantics (`IMPLEMENTS_*` for perform vs `REFERS_TO` for mention). Retire the ~2200 chars clause + gate.
- **D4: RETIRE-NONVIOLATION-CREDIT-CLAUSE-001** — replaced by `is_informational` flag + effectiveness scoring. Retire the ~800 chars clause + gate.
- **D5: TIER-1-SHORT-CIRCUIT-RESTORE-001** — with topology handling most classification decisions, `JIMINY_TIER1_BYPASS_ENABLED` can flip back to false; LLM tier-2 only for genuinely ambiguous cases.
- **D6: RETIRE-CONSULTING-CLASSIFY-PER-CANDIDATE-001** — replace `consulting.classify` LLM-per-candidate with topology-derived constraint typing (Hebbian evidence + edge-type membership). Reserves the LLM for genuinely-new candidates without topology signal.

### Phase E — LoRA adapter reframe

**Goal**: reset the LoRA adapter roadmap. Adapters carry dialogue quality, not facts.

- **E1: LORA-ADAPTER-ROLE-DOC-001** — new doc + CLAUDE.md pin: LoRA adapters (`mdemg-llm-v1` and successors) carry DIALOGUE STYLE / SYNTHESIS QUALITY / phrasing patterns, NOT facts. Facts live in MDEMG substrate. Close CLAUDE-DOCS-TRAINING-* arc explicitly with this framing (Rule F extended).
- **E2 (optional, future): DIALOGUE-QUALITY-LORA-001** — if we still want LoRA training, target it at *how to phrase directives from retrieved constraints* / *how to weight competing guidance in synthesis* / *how to structure a substrate-native dialogue turn* — measured by dialogue coherence + user follow-through, not fact recall.

## Rules pinned by this arc (will be added to CLAUDE.md as each phase ships)

- **Rule K — MDEMG is infrastructure, Jiminy is the dialogue, LoRA is assistance.** MDEMG's graph + retrieval + consolidation + Hebbian + RSIC + event-federation is the infrastructure that facilitates internal dialogue. Jiminy is the specific product that speaks that dialogue. `mdemg-llm-v1` (and future LoRA adapters) assist the dialogue with reasoning + phrasing + synthesis. Never conflate the three layers.
- **Rule L — Jiminy READS from MDEMG's shipped primitives.** Every Jiminy decision that CAN be made from substrate topology (Hebbian weights, activation spreading, edge attention, layer scoping, effectiveness scores, RSIC signals, is_informational, GROUNDED_BY skip-connections, symbol-graph edges) MUST be made from those primitives. LLM prompt clauses are the last resort, not the first.
- **Rule M — Prompt clauses have a retirement path.** Every LLM-classifier prompt clause added to Jiminy MUST document which MDEMG primitive it substitutes for AND under what condition the primitive becomes available AND the plan to retire the clause. No permanent additive prompt clauses.
- **Rule N — LoRA adapters do NOT carry facts.** Facts live in MDEMG substrate. LoRA training corpora carry dialogue style + synthesis quality + reasoning patterns. Any future proposal to bake facts into `mdemg-llm-v1` weights requires citing Rule N + explicit exception rationale.

## Ordering + dependency

```
Phase A (infrastructure repair) ──┬── Phase B (substrate-native discovery) ──┬── Phase C (layer + edge-aware surfacing) ──┬── Phase D (prompt clause retirement) ── Phase E (LoRA reframe doc)
                                  │                                          │                                            │
                                  A1 → A2 → A3                              B1 → B2 → B3                                  C1 → C2 → C3
```

Phase D sprints run in parallel with Phase C once B1+B2 land; D1 (mechanism-scope) is the first candidate for retirement once activation-spreading discovery is validated.

Phase E is a documentation + policy sprint; can run anytime after Phase D starts.

## Immediate next action

**Resume INGEST-TOPOLOGY-REPAIR-001 (Phase A1)** — the sprint is scoped + partially executed (E1 ingest fix + models.go struct fields shipped locally). Complete Epics 2-7 (retrieval Cypher projection, backfill CLI, synthesis prompt, tests, live smoke, sprint post + docs).

## Success criteria for the arc

- LLM call volume per Guide() reduced ≥50%
- Classifier prompt shrunk back to ≤3000 chars
- Constraint discovery is ONE activation-spreading call instead of 4 parallel vector queries
- L2+ emergent concepts surface as guidance in ≥20% of Guide() calls
- Follow rate lift is measured (target: substrate-native discovery matches or beats the current LLM-heavy path in the 168h passive re-check window)
- Every retired prompt clause is regression-tested vs the topology substitute
- CLAUDE.md Rule K–N pinned; LoRA adapter roadmap redirected
