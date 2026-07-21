# Findings — docs/features a-j (fixer agent). Apply each; verify vs code first; minimal diffs. Skip files not listed.
| file:line | stale claim | current reality | fix |
|---|---|---|---|
| consolidation-performance.md:80 | DYNAMIC_EDGES_MAX_NODES (2000) | removed; DYNAMIC_EDGE_{MIN_LAYER,TOPK,SIM_THRESHOLD,OVERSAMPLE} | delete var; document 4 replacements |
| consolidation-performance.md:40 | circuit-breaker section | breaker removed; vector-index top-K shipped | rewrite short per RETRIEVAL-TYPED-EDGES-002 |
| consolidation-performance.md:88 | "What's next (Sprint B)" | shipped 2026-07-03 | mark shipped |
| bridges-edge-type.md:57 | Cartesian creation, min layer 3 | vector-index top-K; DYNAMIC_EDGE_MIN_LAYER=1 | update |
| bridges-edge-type.md:98 | missing new knobs | DYNAMIC_EDGE_* exist | add 4 vars |
| column-voting-retrieval.md:85 | RETRIEVAL_AUDIT_ENABLED false | true | flip |
| column-voting-retrieval.md:41 | cache ns v1-rrf4 example | v2-rrf5|…|tge= | update example |
| column-voting-retrieval.md:15 | "four columns" | 5th Context column default-on (14.2.3) | note 5th |
| alert-dispatcher.md:120 | neo4j_high_cpu >80% | NEO4J_CPU_ALERT_THRESHOLD_PERCENT=500 windowed AVG | update row |
| alert-dispatcher.md:121 | neo4j_pool_exhausted row | rule deleted | remove row |
| backup-restore.md:34,68 | SNAPSHOT_WAIT 300 | 3600 | fix twice |
| browser-ui.md:15,191,38-50 | 10/9 tabs; no Review row | 11 tabs; Review exists | update + add row |
| constraint-nodes.md:51,82,97 | "no config, always active"; no correction sibling | CONSTRAINT_PROMOTION_* gate default-on; correction_nodes.go | add gate section + sibling note |
| docker-deployment.md:144 | schema version 8 | 31 | fix |
| dev-cycle-summary-synergy.md:31,62 | synergy 10% weight | 0.05 config-driven | fix |
| dynamic-emergence.md:92 | EMERGENCE_MODEL gpt-4o-mini | inherits LLM_MODEL=mdemg-llm-v1 | fix |
| database-embedding-migrations.md:50-55 | `mdemg db start` normal | footgun (empty volume) | warn; use docker compose up -d neo4j |
| fine-tuning-pipeline.md:51,180,206,193 | FT-OAI-003 refs; .env gpt-5.4-mini | FT-OAI-003 dropped 2026-04-22; local-first defaults | tombstone refs; note local-first |
| event-graph-federation.md:293 | "chunks forever fine" | V0025 retention 180d/compression 14d | state applied |
| embedding-retrieval-data-collection.md:53 | "9 call sites" | 20 post EMBED-CALLSITE-001 | defer to embedding-attribution.md |
| goroutine-supervisor.md:41-47 | "15 loops" | +ft-loop-controller conditional 16th | add |
| intent-translation.md:97,65,+ | TIMEOUT 2000; ~200-500ms; no ?intent= | 15000; 3.8s avg; ?intent= override + A/B −0.010 bar | fix all |
| guidance-training-corpus.md:187,151 | floor 0.5 / red <0.5 | 0.05 | fix both |
| hitl-review.md:41 | 17 datasets | 18 (contradicted_drafts) | fix |
| ide-repo-integration.md:231 | "5 hooks"; .claude/hooks source of truth | 6; templates canonical (HOOKSYNC-001) | fix + invert |
| j17-ai2ai-protocol.md:195,740,748-753,1091 | TICKET_TTL 168; trust table ×6 wrong; missing vars | 4h; 0.65/0.05/0.02/0.04/0.75/0.35; +CALIBRATION_MIN_SAMPLES=50, COMPRESSION_TARGET=2.0 | fix all |
| j17-feedback-loop-closure.md:50,173-191 | initial 0.5, T1 0.80, 6 feedbacks | 0.65 / 0.75 / 2 | rederive table |
| j17-tier-gate.md:26 | "toward 3-5×" | target 2.0 | fix |
| jiminy-inner-voice.md:124,143,192,198,+ | TIMEOUT 6000; SYNTHESIS 10000; 2-band NA | 0→derive 90000; 30000; 4-band (<0.10 NA, [0.10,0.20) ignored) | fix + addendum note for ~12 newer JIMINY_* vars |
| jiminy-effectiveness-tracking.md:60 | NA < 0.20 | < 0.10; [0.10,0.20)=ignored | fix |
| jiminy-actionability.md:29,49 | "all three levers default-off" | Lever B on in .env; cooldown default-on | qualify code vs operational |
