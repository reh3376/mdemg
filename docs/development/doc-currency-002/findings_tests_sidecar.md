# Findings — docs/tests + docs/sidecar (fixer agent). Apply each; verify vs code first; minimal diffs. UXTS_FRAMEWORK_MATRIX.md §2/§3/§5 RESERVED for orchestrator (skip matrix entirely).
| file:line | stale claim | current reality | fix |
|---|---|---|---|
| docs/tests/uits/README.md:5 | "soft-fail CI gate" | no UITS step in workflows | "no CI gate (manual/Makefile only)" |
| docs/tests/usts/README.md:21-24,44-48 | 4-spec inventory incl auth_required; quick-start uses drafts-moved file | 5 canonical (control_char_injection, input_injection, metrics_snapshot_security, rate_limit_enforcement, sensitive_data_exposure); auth_required in drafts/ | refresh inventory; example → specs/input_injection.usts.json |
| docs/tests/uobs/README.md:21-23 | 3 specs; log_format canonical | 11 canonical; log_format in drafts (type unimplemented) | list 11; move log_format under drafts |
| docs/tests/uets/README.md:68 | --skip-hash flag | removed from runner | delete flag |
| docs/tests/homebrew-install-test-plan.md:3,40 | pinned v0.2.1 artifacts | project ≥v0.11 | parameterize "<CURRENT_VERSION>" |
| docs/sidecar/implementation-journal.md:305 | probes "mdemg-api (HTTP /healthz)" conflation | API=/healthz; neural sidecar=GET /health | clarify one line |
