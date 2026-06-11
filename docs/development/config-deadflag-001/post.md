# CONFIG-DEADFLAG-001 — Sprint Post

Closed: 2026-06-11 · branch `reh3376_dev01` · Roadmap Q3 Phase 2.

## Numbers

| | |
|---|---|
| Config fields (before → after) | 689 → 660 |
| Dead fields found / remaining | 57 / **0** |
| Wired (defaults preserved) | 28 |
| Deleted (all born-dead per git -S) | 29 (3 Go-field-only; env vars stay live for hooks) |
| Allowlisted | **0** |
| LoadYAMLConfig swallow sites fixed | 9 |
| getBool | strict (live-verified: `JIMINY_ENABLED=ture` refuses startup naming the var) |

## Execution shape

Parallel three-lane assembly: two sub-agents wired the heavy clusters in
disjoint file sets (REL_* through internal/symbols; LLM_SUMMARY_* +
RSIC thresholds through cli/ape) while the main lane scripted the 29
deletions, the 11 tiny wires, strict getBool, and the un-swallow. Each
agent reported exact server.go one-liners for the files it couldn't
touch. Scanner converged 57 → 16 → 5 → 0.

## Live Tier 3

- `JIMINY_ENABLED=ture bin/mdemg serve` → `Error: config error: invalid
  boolean env value(s): JIMINY_ENABLED="ture" (want true/false, 1/0,
  yes/no, on/off)`.
- LaunchAgent restart on the triaged config: healthz all-ok.

## Operator-facing truth fixes

- compose template's `LLM_SUMMARY_ENABLED` forward was a no-op — now real.
- `.env.example` documented the dead `RETRIEVAL_LLM_CLASSIFY_*` namespace
  as the classify toggle (live analog: `QUERY_CLASSIFY_*`) — cleaned,
  along with 11 other dead env-doc lines.

## Follow-ups recorded (not started)

- GENERALIZES-vs-THEME_OF edge drift (triage discovery: runtime creates
  GENERALIZES at hidden/service.go:4493 despite the V0016 conversion).
- `RSICStore.CleanupExpired` has no caller (the newly-wired retention
  config is ready when the cleanup gets scheduled).
- EVENTGRAPH pair cap covers the ApplyCoactivation path per the field's
  documented scope; the EVENTGRAPH-003/004 emission sites forward
  unbounded — deliberate-decision candidate.

## Documents Accessed

Sprint plan §11; triage evidence table (sub-agent, per-field git -S);
recon report (yaml_config/getBool/EVENTGRAPH/FILE_WATCHER); both wire
agents' reports; live server log + healthz.
