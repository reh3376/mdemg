# Sprint Plan — CONFIG-DEADFLAG-001: Make the Silent-No-Op Flag Class Structurally Impossible

## 1. Header & Metadata
Sprint: CONFIG-DEADFLAG-001 · 2026-06-11 · branch `reh3376_dev01` ·
Roadmap Q3 Phase 2 · effort 4d · risk medium (touches FromEnv semantics +
deletes config surface).

## 2. Problem Statement
The parsed-but-never-read flag class caused the 24-day Hebbian no-op and
the 9-week guidance dormancy: an operator sets an env var, FromEnv parses
it, and nothing ever reads the field. Census (scanner, 2026-06-11): **689
Config fields, 57 dead**. Two adjacent silent-failure mechanics: 9 call
sites blank-assign `LoadYAMLConfig` errors (malformed config.yaml degrades
to defaults invisibly), and `getBool` parses any unrecognized value —
including typos — as `false`.

## 3. Scope & Constraints
**In**: consumer scanner + merge-blocking CI gate; triage of all 57 dead
fields (wire-trivial / delete / allowlist-with-citation, delete-biased);
wire `EVENTGRAPH_MAX_PAIRS_PER_EVENT_BATCH`; strict getBool
(true/1/yes/on, false/0/no/off; errors accumulate and fail FromEnv);
un-swallow the 9 LoadYAMLConfig sites (warn-loudly; the file is known to
exist, so errors are real — but a corrupt yaml must not brick the CLI).
**Out**: FILE_WATCHER work (roadmap claim stale — recon shows it fully
wired and operable, default-off); YAML-schema validation of config.yaml
contents (separate concern); compose-template/env-docs sweeps beyond the
vars actually deleted.

## 4. Dependencies
`internal/config/{config,yaml_config}.go`; the 9 CLI call sites; the
EVENTGRAPH-001 writer path (`internal/learning/service.go` →
`internal/tsdb/reinforcement_writer.go`); per-field triage evidence
(sub-agent, git -S history per field); ci.yml.

## 5. Implementation Plan
Epic 0 plan · **Epic 1** scanner (`scripts/verify_config_consumers.py`,
dot-reference consumption; 57-field census) · **Epic 3** strict getBool +
un-swallow (done with Epic 1; shared commit) · **Epic 2** triage execution
per the evidence table (deletes remove struct field + FromEnv parse +
literal + env-doc mentions; wires name exact file:line) · **Epic 4** CI
gate (merge-blocking step), live Tier 3 (server boots on the live stack;
strict-bool rejection observed against the real binary; EVENTGRAPH cap
observable), CHANGELOG/CLAUDE.md/post, push.

## 6. Testing Plan
Tier 1: strict-bool tests (typo fails, accumulation, canonical forms);
scanner self-test via census; per-wire unit coverage. Tier 2: full
`go test ./internal/...` (deletions compile everywhere); scanner green
after triage. Tier 3 (live): real binary boots against the live stack
post-deletions; `JIMINY_ENABLED=ture mdemg serve` refuses to start naming
the var; EVENTGRAPH pair cap visible in live behavior/log.

## 7. Commit Strategy
Epics 1+3 one commit (shared files) · Epic 2 one commit · Epic 4 gate +
docs · push once (auto-PR), iterate PR CI.

## 8. Verification Checklist
- [ ] Scanner green: 0 unallowlisted dead fields; allowlist entries cite plans
- [ ] EVENTGRAPH_MAX_PAIRS enforced at the recorded-pairs path
- [ ] Deleted fields removed from struct + FromEnv + literal + env docs
- [ ] Strict-bool live rejection (`=ture` refuses startup, names the var)
- [ ] 9 swallow sites warn loudly; corrupt-yaml live check
- [ ] CI gate merge-blocking; PR CI green
- [ ] CHANGELOG + CLAUDE.md + post.md

## 9. Documentation Update — Epic 4.

## 10. Risks & Mitigations
Strict getBool breaks deployments using junk values → only canonical
forms were ever *documented*; failure message names every var. Deletions
break an unseen consumer → scanner counts dot-references across
internal/cmd/tests; full build+test after each batch. Scanner
false-positives → method-receiver consumption included (verified on
JiminyWarmComputeTimeoutMs); allowlist escape hatch with citations.

## 11. Documents Accessed
Recon (agent): yaml_config.go, getBool, EVENTGRAPH writer path,
FILE_WATCHER truth, config test conventions. Triage evidence table
(sub-agent, per-field git -S). Roadmap entry line 54.

## 12. Rollback Procedures
Code-only; revert commits. Deleted fields restorable from git; the CI
gate is one step, independently removable.
