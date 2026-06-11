# JIMINY-OUTCOME-002 — Sprint Post

**Status: COMPLETE** · 2026-06-11 · branch `reh3376_dev01` · ~1d actual

## What shipped

1. **Tier-2 `not_applicable`** — both classify prompts + the Ollama grammar
   schema now offer it, with the ignored-vs-not_applicable distinction
   spelled out ("use ignored only when applying the guidance was genuinely
   possible here"). The parser (`mapOutcomeString`) and all four persistence
   sinks (escalation, protocol metrics, Neo4j `GUIDANCE_OUTCOME`, TSDB
   `constraint_outcomes`) already handled the value — service.go skips it
   everywhere — so denominators self-correct for new data with **zero**
   stats-query changes. ULTS `jiminy_evaluate_llm.ults.json` prompt hashes
   re-pinned in the same PR (verify-hashes 11/11 PASS).
2. **Verdict provenance** — `ClassificationResult.Source`
   (`tier1|llm|heuristic|explicit`) stamped at every decision path
   (including the nil-embedder and embed-failure fallbacks) and persisted
   via V0026 `constraint_outcomes.classifier_source` (TSDB schema 25→26;
   applied live twice, idempotent).
3. **Re-baseline annotations** — Grafana effectiveness panels (2) +
   `docs/features/jiminy-effectiveness-tracking.md`: pre-2026-06-11T19:00Z
   history is heuristic-dominated, not comparable.

## Tier 3 live verification

- Guide → feedback round-trip on the live stack: an action with topical
  overlap but no applicability ("read documentation; no code changed" vs
  must-use-CUIDv2 guidance) produced **3/8 `not_applicable` verdicts with
  coherent reasoning** from the Tier-2 LLM; genuinely-applicable-but-
  unaddressed guidance still classified `ignored` (the prompt distinction
  works — no over-rotation in this sample).
- Sink check: **zero `not_applicable` rows** in `constraint_outcomes`;
  the 5 `ignored` rows all carry `classifier_source='llm'`.
- V0026 visible live; `mdemg` restarted via LaunchAgent, healthz ok;
  full `go test ./internal/...` green; lint 0 issues.

## Design note

The fix surface was far smaller than the investigation implied: the
not_applicable plumbing existed end-to-end (types, parser, sink-skips) —
only the LLM's option set was missing. No denominator changes; no
backfill (forward-only, EVENTGRAPH precedent).

## Recorded follow-ups (not actioned)

- `jiminy.synthesize` ~20% error rate under llama-server load
  (GUIDANCE-SYNTH-001 territory).
- Low-evidence NLI calibration-bias warnings (window_size 1–2).
- Watch item: if effectiveness now inflates (LLM over-using
  not_applicable), `classifier_source` + per-verdict reasoning make it
  analyzable; the prompt's "genuinely possible here" clause is the tunable.

## Documents Accessed

Investigation report (2026-06-11);
internal/jiminy/{outcome_classifier,service,types,stats}.go;
internal/tsdb/constraint_outcomes_writer.go; migrations 011/026;
docs/tests/ults/specs/jiminy_evaluate_llm.ults.json + runner;
deploy/docker/grafana/dashboards/mdemg-jiminy.json; live stack.
