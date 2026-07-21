# Findings — docs/user + docs/api + docs/guides + docs/operations (fixer agent). Apply each; verify vs code first; minimal diffs. INGEST_CODEBASE_API.md RESERVED for orchestrator (skip).
| file:line | stale claim | current reality | fix |
|---|---|---|---|
| user/api-reference.md:4339vs4250 | /v1/prometheus live section + tombstone | 410 Gone only | delete live section |
| user/api-reference.md:2668,2997,4659 | ingest-codebase, alerts/grafana, /v1/feedback schemas | pruned (DORMANT-CENSUS-001) | tombstone; point to /v1/memory/ingest/trigger + /v1/alerts/clear |
| user/api-reference.md:2087,2146,2095-96 | JIMINY_ENABLED false; OUTCOME_LLM false; synth 2000/10000 | true/true (compose overrides JIMINY_ENABLED); 3000/30000 | fix + binary-vs-compose note |
| user/api-reference.md:137-153 | /readyz minimal shape | version + checks map | show full shape |
| user/api-reference.md global | missing /v1/alerts/clear, /v1/hooks/event, /v1/review/* | registered | add stub sections |
| user/cli-reference.md:2799-2800,2955-3011 | gpt-5.4-mini/OpenAI; six gpt-4o-mini defaults | mdemg-llm-v1@:8102; inherit LLM_MODEL | fix all |
| user/cli-reference.md:576,3635-3765,104,3297+ | migrations 001-010; missing cmd groups + --native; missing Phase13/14 env section | 001-031; add groups list; add retrieval env section (COLUMN_VOTING/SPARSE_*/CONTEXT_FINGERPRINT_*/RETRIEVAL_AUDIT default-ON) | fix/add compactly |
| user/cms-rsic-guide.md:774,1214 | 4-dim fixed formula; JIMINY_ENABLED false | DH-005 7-dim normalized; true | replace formula + fix |
| user/ingestion-guide.md:1238 | BATCH_INGEST_MAX_ITEMS 2000 | default 500 max 2000 | fix |
| user/upgrade-guide.md:75,43-54 | RETRY 3; unset→gpt-4.1-nano | 5; →mdemg-llm-v1 | fix + v0.8+ note |
| user/module-developer-tutorial.md:53 | Go 1.21+ | go 1.26 | fix |
| user/mdemg_beta_testing.md:146,151-153,291 | mdemg-windows install.ps1; apt-mdemg; v0.3.x | archived repos; WSL2 + scripts/install.sh; v0.11+ | fix |
| guides/grafana-dashboard-development.md:370,577,684-689,60,689,39,49,47 | prometheus datasource examples; wrong compose path; 22 rules/3 groups; 8080; missing llm-routing | postgres/timescaledb uid; deploy/docker/ + main compose; 27/4; 9999 only; add llm-routing | fix all |
| operations/campaign-task-activation.md:16,32,19 | recommends INTENT_ENABLED=true; JIMINY_EVALUATE_LLM_ENABLED | proven net-negative — keep false + warning; var is JIMINY_OUTCOME_LLM_ENABLED (already true) | fix + warn |
| operations/pre-campaign-checklist.md:21 | schema 26/V0026 | 31/V0031 | fix |
| operations/vllm-mlx-setup.md:36 | LLM_BASE_URL :8100 | key LLM_ENDPOINT; runtime :8102 (keep SUPERSEDED banner) | note inside banner |
| guides/UXTS_DEVELOPER_GUIDE.md:267,681,686,692 | UVTS spec-only; UATS 129; UOTS 5 | active/live-gated; 214; 11 | sync to matrix |
