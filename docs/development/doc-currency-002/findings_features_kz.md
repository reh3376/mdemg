# Findings — docs/features k-z (fixer agent). Apply each; verify vs code first; minimal diffs. Skip files not listed. prometheus-observability-monitoring.md and observability-dashboards.md RESERVED for orchestrator except where listed.
| file:line | stale claim | current reality | fix |
|---|---|---|---|
| llm-retry.md:18-19,29 | attempts 3 / delay 10000; 502 not retryable | 5 / 60000; 502 IS retried | fix all |
| llm-powered-intelligence.md:204,151,228,219 | RETRIEVAL_LLM_CLASSIFY_ENABLED; score>=0.5; OLLAMA_URL; EMERGENCE_MODEL gpt-4o-mini | QUERY_CLASSIFY_ENABLED; CONSULTING_CONSTRAINT_SCORE_FLOOR=0.45; OLLAMA_ENDPOINT; inherits mdemg-llm-v1 | fix all |
| local-llm-runtime.md:76,78,59 | LLM_MAX_TOKENS row; ALLOW_NO_MLX primary; ~5800 tokens | var absent; ALLOW_NO_LLM primary; 7489→3500 budget | fix all |
| llm-response-sanitization.md:19 | "vllm-mlx" | llama-server | fix |
| mlx-watchdog.md:46,48,131 | recovery 1; MLX_MAX_CONSECUTIVE_FAILURES env; GET /metrics | 2; hardcoded 3; TSDB metric_samples | fix all |
| mcp-memory-channel.md:30 | 23 tools | 24 (+memory_reject) | fix |
| meta-cognition-enforcement.md:62,65 | SignalLearner no persistence | Neo4j persist 30s flush | remove limitation |
| observability-dashboards.md:65,140 ONLY | 4 metrics "removed"; COMPRESSION 3.0 | all live; 2.0 | fix these 2 lines only |
| rsic-feedback-loop.md:222,140 | /metrics; 15% stability | /v1/metrics/snapshot; 13% | fix |
| neural-training-pipeline.md:53,169,243-245 | gpt-5.4-mini; NEURAL_MODEL_DIR; openai example | mdemg-llm-v1; CLI --model-dir only; localize | fix all |
| strict-mode.md:157,263,152 | ESCALATION_BLOCK default true; missing tier-gate knob | false; add J17_TIER_GATE_MODE row | fix |
| service-resilience.md:237-272,240,185 | 6 phantom/renamed vars; internal/rsic/coordinator.go; 29 patterns | RSIC_LLM_CONCURRENCY_LIMIT=2 real; delete phantoms; ape/cycle.go; ~36 | fix all |
| rsic-sk1-guidance-calibration.md:91,96 | "persist not implemented" | shipped | update |
| split-pipeline-execution.md:35,63,29 | RunPhaseRange(10,20); step list | (10,22); +correction step | fix |
| sparse-retrieval.md:97,117,124,40 | default-off MIN=3; Prometheus histograms | default-ON 15; TSDB-only | rewrite defaults + relabel |
| tsdb-data-governance.md:75,119,273,158 | schema 8; migrations 001-009; ft "not built" | 31; 31 files; FT-RECURSIVE shipped | fix all |
| tsdb-data-management.md | inventory ends V0025 | V0027-V0031 exist | add later-additions note |
| synergy-optimization.md:70 | 10% | 0.05 | fix |
| training-data-capture-verification.md:168,186,242,162 | 30s flush; 16 consumers | 60s; 17 | fix all |
| uaits-framework.md:15,45,48 | 10th; 15 frameworks; 16 contracts | 15th of 16; 16; 17 | fix |
| ults-framework.md:15,26,129,181 | 16 tasks/specs | 17 | fix |
| uvts-validation.md:83 + ubench-framework.md | no --apply-tsdb | flag exists (run_benchmark.py:579) | add row both |
| ui-gap-analysis.md:12 | 125 routes 38% | 187 registrations; list new absent routes | update numbers + note |
| unified-cli.md:15,747,779-791,+ | 30+/6 groups; missing flags/groups | 37 top-level; add index table of missing groups (data/model/graph/concepts/corrections/eventgraph/ft-loop/tsdb/watchdog/maintenance/synergy/embeddings), upgrade flags, hooks doctor | update + compact index table |
