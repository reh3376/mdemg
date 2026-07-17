# JIMINY-CORPUS-001 — Post

**Status: SHIPPED — the substantial guidance-quality work.** · 2026-07-03 · branch `reh3376_dev01`

DASHBOARD-TRUTH-001 made the Jiminy metrics honest (follow rate ~0.17). This sprint attacks the genuinely-low capability behind that number: the guidance surface was ~half junk and a tiny node set repeated relentlessly. Fixed via data + repetition + phrasing.

## Shipped (7 epics)

1. **E1 — stop junk becoming constraints (forward-fix first).** `CreateConstraintNodes` (`internal/hidden`) promoted ANY `conversation_observation` carrying a `constraint:<type>` regex tag into a `role_type='constraint'` node — no content/provenance gate, and the tagger is regex-only (the F6a LLM classifier defaults off). So fabricated `Build/test succeeded:` (progress) / `Bash error` (error) observations got promoted, then Lever C surfaced them. Added `ConstraintPromotionGate`: provenance deny-set on obs_type (`CONSTRAINT_PROMOTION_DENY_OBS_TYPES` = progress,error,task,context) + a config-driven content-pattern backstop; default-on; genuine constraints unchanged.
2. **E2 — purge the junk corpus (operator-authorized, tombstone-only).** 140 → **61** live constraint nodes: 49 junk (`jiminy_corpus_junk_purge`) + 30 dedup (`jiminy_corpus_dedup`, one canonical kept per duplicated rule — the 12-node `CONSTRAINT 1–5` schema set → kept `never-direct-alter-schema`). Backup + `LIMIT 5` small-batch + operator sign-off; reversible via `is_archived`. Gate widened (own code) so the `t8j3`/`xinav` phase-completion class rejects going forward. Precision: `bj4w17ne` (a genuine rule the prior list mislabeled) was spared.
3. **E3 — repetition control.** 37 nodes were surfacing ~49× each in 7d. Lever A session cooldown (a node ignored ≥`JIMINY_SURFACE_COOLDOWN_IGNORED_COUNT`=3 consecutive times in a session is dropped; followed resets; contradicted keeps surfacing) + Lever B effectiveness prior (soft re-rank by the stable [0,1] followed/surfaced ratio, RRF-SCALE-001-safe). Never-fully-dark guarantees (all-cooled → least-recently-ignored first; min-actionable quota kept satisfiable).
4. **E4 — precise 4-band relevance gate.** JIMINY-OUTCOME-002 had over-corrected the entire sub-LOW tail to `not_applicable`, dropping real near-LOW ignores. Restored the 4-band structure: `[NA(0.10), LOW(0.20))` → `ignored` (relevant-domain), `< NA` → `not_applicable` (unrelated). `JIMINY_OUTCOME_NOT_APPLICABLE_SIMILARITY`=0.10; tier-2 LLM verdicts take precedence.
5. **E5 — enable Lever B (directive synthesis).** Exposed `JIMINY_DIRECTIVE_SYNTHESIS_ENABLED` in compose (×3) + the UI config tab + enabled via `.env`. Live-verified: warm synthesis renders imperative directives ("you MUST implement…", "you MUST NOT ignore…") with node citations, vs the prior advisory prose. No new hot-path LLM call.

## Tier-3 (live)

| Check | Observation |
|---|---|
| E1 promotion gate | junk rejected / genuine promoted (unit); dry-run 47/140 would reject |
| E2 purge | live constraint nodes **140 → 61**; top junk surfacers (t8j3 169/7d, qoaje05 143/7d) tombstoned + excluded from the partition |
| E3 repetition | cooldown + effectiveness prior live (default-on); unit + fallback-quota tests |
| E4 relevance gate | 4-band classifier live |
| E5 Lever B | warm synthesis → imperative directives, live-verified |

**Follow-rate measurement is forward-looking.** Baseline at ship: **0.165** (7d window). The 7d `constraint_outcomes` window still holds pre-purge outcomes; the lift materialises over the coming days as new outcomes (cleaned corpus + repetition control + relevance gate) age into the window. The purge lift is the most cleanly attributable (49 junk nodes = ~58% of the last 7d's constraint surfacings, mechanically removed). **Recommended follow-up check: re-measure the 7d windowed follow rate ~1 week out** (once the window is fully post-purge).

## New config knobs (all default-sane, no-hardcoding rule)

- E1: `CONSTRAINT_PROMOTION_GATE_ENABLED` (true), `CONSTRAINT_PROMOTION_DENY_OBS_TYPES`, `CONSTRAINT_PROMOTION_REJECT_PATTERNS`
- E3: `JIMINY_SURFACE_COOLDOWN_IGNORED_COUNT` (3), `_COOLDOWN_CAPACITY` (5000), `JIMINY_SURFACE_EFFECTIVENESS_PRIOR_WEIGHT` (0.3), `_TTL_SEC` (300), `_MIN_SAMPLES` (5)
- E4: `JIMINY_OUTCOME_NOT_APPLICABLE_SIMILARITY` (0.10)
- E5: `JIMINY_DIRECTIVE_SYNTHESIS_ENABLED` (.env-enabled; code default false)

## Reversibility / follow-ups

- Purge is reversible: `MATCH (n) WHERE n.archive_reason IN ['jiminy_corpus_junk_purge','jiminy_corpus_dedup'] SET n.is_archived=false REMOVE n.archive_reason, n.archived_at`. Backups: `.mdemg-backup-jiminy-corpus-20260703_154420/` + `tombstoned_{junk,dedup}.csv`.
- Deferred: `RetrieveForJiminy` role_type adapter gap (why `correction`-type guidance has zero rows); HITL curation (the JIMINY-RELEVANCE-001 arc toward >0.9); the two paraphrased "mdemg db start" rule groups (optional merge).

## Documents Accessed

- `internal/hidden/{constraint_nodes.go,constraint_gate.go,service.go}`, `internal/jiminy/{service.go,outcome_classifier.go,surface_cooldown.go,guidance_prompt.go,synthesizer.go}`, `internal/config/{config.go,yaml_config.go}`, compose templates, `internal/api/ui/tabs/config.js`
- `docs/development/dashboard-truth-001/investigation_findings.md`
- Neo4j `role_type='constraint'` nodes; TSDB `constraint_outcomes`, `guidance_training_rows`
