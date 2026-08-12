# SEC-TRANCHE-3 Sprint Plan (v1.0)

## 1. Header & Metadata
- **Sprint line**: security
- **Sprint name**: SEC-TRANCHE-3
- **Date**: 2026-08-12
- **Branch**: reh3376_dev01
- **Owner**: security-tranche subagent
- **Predecessors**: SEC-TRANCHE-2 (path injection)

## 2. Problem Statement
33 open GitHub code-scanning alerts on the mdemg repo (after CRITICAL #22
dismissed in a prior session). Categories: clear-text-logging (10),
regex/missing-anchor (8), uncontrolled-allocation-size (8),
incorrect-integer-conversion (2), py incomplete-URL-sanitization (2), go
incomplete-URL-scheme-check (1), weak-sensitive-data-hashing (1), js xss
through DOM (1). Target: 33 → single digits by structural fix or dismiss.

## 3. Scope & Constraints
- Session no-touch list (JIMINY-CEILING-BREAK-2 measurement window active
  until 2026-08-19): `internal/jiminy/**`, `internal/hidden/{correction,
  constraint}_nodes.go`, Neo4j substrate mutation paths,
  `constraint_outcomes` writer, `internal/retrieval/scope_gate.go` /
  Lever C. Alerts touching these DEFER (report in summary).
- Build + lint clean at end. No new test failures.
- Single commit; push to `reh3376_dev01`; auto-PR fires.

## 4. Dependencies
- `gh` CLI for CodeQL alert dismissal.
- CodeQL re-scan on next PR merge to close structurally-fixed alerts.

## 5. Implementation Plan
- **Epic 1** (regex anchors): add `\b` prefix to 13 URL host patterns in
  `internal/gaps/detector.go::extractDataSourceReferences`. Gate: alerts
  #31–#38 close, tests green.
- **Epic 2** (URL scheme): expand href-scheme reject-list in
  `plugins/docs-scraper/extractor.go` to include `data:` and `vbscript:`,
  lowercase-normalized. Gate: alert #39 closes.
- **Epic 3** (allocation caps): defensive cap `if n > 10000 { n = 10000 }`
  in 6 sites (`retrieval/{consensus,distribution,rerank,service}.go`,
  `ape/calibration.go`, `api/handlers_enforcement.go`). Gate: alerts
  #15–#21 close (except DEFERRED #71).
- **Epic 4** (int32 bounds): bounds-check before `int32(n)` in
  `cli/serve.go` (TSDBMaxConns, cap 4096) and
  `llmclient/client.go::SetDefaultFailureThreshold` (cap 1_000_000).
  Gate: alerts #13, #14 close.
- **Epic 5** (Py URL check): add `_is_openai_endpoint(base_url)` helper
  using `urlparse().hostname` comparison in `neural/benchmarks/llm_judge.py`
  + `neural/training/distill_driver.py`. Gate: alerts #10, #11 close.
- **Epic 6** (XSS DOM): add `EXPORT_ID_RE = /^[a-z0-9]{20,32}$/` +
  `isValidExportId()` in `internal/api/ui/tabs/training_data.js`; gate
  `startStatusPolling` and the download click handler. Gate: alert #8
  closes.
- **Epic 7** (secret-mask widening): expand
  `internal/config/yaml_config.go::isSensitive` from 2 → 8 env-var names
  (defense in depth; not tied to a specific alert).
- **Epic 8** (dismissals): dismiss 12 false-positive/won't-fix alerts
  via `gh api PATCH …code-scanning/alerts/<N>` with per-alert rationale
  ≤ 280 chars.
- **Epic 9** (docs + commit): sprint dir + CHANGELOG + CLAUDE.md pins;
  single commit; push.

## 6. Testing Plan
- **Tier 1 (unit)**: existing suites in `internal/{retrieval,ape,api,
  config,gaps,llmclient,cli}` continue to pass.
- **Tier 2 (integration)**: `go build ./...` clean.
- **Tier 3 (e2e/live)**: DEFERRED — alerts do not have observable
  runtime surface beyond what unit tests already exercise (regex
  matches, allocation, URL parsing, DOM validation). Live re-scan
  verification happens after CI re-runs CodeQL on PR merge.

## 7. Commit Strategy
Single commit `security: tranche 3 — <N> alerts closed (SEC-TRANCHE-3)`.

## 8. Verification Checklist
- [x] `go build ./...` clean
- [x] `golangci-lint run` on touched packages clean
- [x] Unit tests green on touched packages
- [x] All 12 dismissals landed via gh api
- [x] Sprint dir populated
- [x] CHANGELOG entry under [Unreleased] Security

## 9. Documentation Update
- CHANGELOG.md entry
- CLAUDE.md pin naming the arch rules discovered
- This sprint plan + post.md

## 10. Risks & Mitigations
- **Risk**: Allocation cap of 10000 might truncate a legitimate large
  request. **Mitigation**: 10000 is >100× typical top-K; upstream config
  caps values well below this.
- **Risk**: Regex `\b` prefix might miss patterns like `://github.com`
  after a colon. **Mitigation**: `\b` matches between `:` and word chars,
  so `://github.com/…` still matches (`:` is non-word, `g` is word).
- **Risk**: `exportId` client-side validation breaks a legitimate
  server-returned ID. **Mitigation**: regex `^[a-z0-9]{20,32}$` is the
  full CUIDv2 spec (typical length 24).
- **Risk**: `_is_openai_endpoint` breaks proxy setups where hostname
  isn't `api.openai.com`. **Mitigation**: only affects token-key
  selection (`max_completion_tokens` vs `max_tokens`); non-OpenAI proxies
  intentionally take the else branch already.

## 11. Rollback Procedures
`git revert <commit>` — no destructive changes.

## 12. Documents Accessed
- `internal/gaps/detector.go`
- `plugins/docs-scraper/extractor.go`
- `internal/retrieval/{consensus,distribution,rerank,service}.go`
- `internal/ape/calibration.go`
- `internal/api/handlers_enforcement.go`
- `internal/cli/{serve,init,db,config_cmd,embeddings}.go`
- `internal/llmclient/client.go`
- `internal/config/yaml_config.go`
- `internal/auth/apikey.go`
- `internal/api/server.go`
- `internal/api/ui/tabs/training_data.js`
- `internal/api/ui/api.js`
- `plugins/linear-module/main.go`
- `neural/benchmarks/llm_judge.py`
- `neural/training/distill_driver.py`
