# JIMINY-BUDGET-001 + OUTCOME-ATTRIB-001 — Sprint Post

**Status: COMPLETE** · 2026-06-12 · branch `reh3376_dev01` · ~0.5d actual vs 3d est

All seven roadmap items landed (budget derivation, reformulate config,
Validate() warning, token floors, sessionID threading, surface/outcome
split, hook-chain documentation). Live Tier 3: guide→feedback round-trip
produced the **first session-attributed GUIDANCE_OUTCOME edge in system
history** (`session_id='jiminy-budget-smoke'`, outcome `followed`);
surfaced/dropped counters verified landing in metric_samples.

Notes:
- "14 hook_cancelled drops" from the roadmap: the term doesn't exist in
  code; the real artifact is the tracker-expiry drop warn (now counted
  via `mdemg_jiminy_feedback_dropped_total`). TTL already 86400s
  (DD-P1P2); the stale 1800 comment was the only defect there.
- max_tokens=100 on the outcome classifier was arguably intentional
  (short labels) but unsafe: the `reasoning` field can exceed it and a
  truncated JSON degrades to the heuristic — the exact artifact class
  JIMINY-OUTCOME-002 made distinguishable. Floors cost nothing.
- Historical GUIDANCE_OUTCOME edges keep null session_id (forward-only;
  the EVENTGRAPH precedent).

Tier evidence: derivation unit tests green; full suite green; scanner
674/674; lint 0; live edge + counters verified.
