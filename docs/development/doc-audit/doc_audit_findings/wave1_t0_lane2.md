# Wave 1 / Lane 2 — remaining T0 (audited @ 831eb0a)

8 files. 7 CURRENT, 1 DRIFT_MINOR.

## README.md — DRIFT_MINOR
- claim "8 dashboard tabs" | reality: 10 (`internal/api/ui/tabs/` —
  status, memory, learning, config, logs, rsic, plugins, features,
  backup, training_data)
- hardcoded port literals in examples (9999/7687/3000/5433) sit in
  tension with the documented FindFreePort dynamic allocation
  (`internal/cli/init.go`); the "6 free TCP ports" statement is correct.
- Proposed fix (operator batch): update tab count; caption examples as
  "default-port illustration".

## CURRENT (no discrepancies)
CONTRIBUTING.md · SECURITY.md (dated placeholder noted) ·
docs/user/{quickstart,multi-instance,FAQ}.md · CHANGELOG.md
(structure/recency) · ROADMAP_2026Q3.md (sprint table matches shipped
reality incl. EVENTGRAPH-001..004, JIMINY-OUTCOME-002, Phase 13.5).
