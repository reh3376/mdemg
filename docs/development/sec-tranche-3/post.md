# SEC-TRANCHE-3 Post — 33 open alerts → 1 deferred

## Outcome
33 open alerts (post CRITICAL #22 dismissal) triaged and closed:
- **11 dismissed** (false positive / won't fix), each with a per-alert
  rationale in the alert history.
- **21 structural fixes** shipped in code (will auto-close on next
  CodeQL re-scan after PR merge).
- **1 deferred** (#71 `internal/jiminy/service.go:3838`) — inside the
  JIMINY-CEILING-BREAK-2 measurement window's no-touch list.

## Structural fixes (21 alerts)
| Rule | # of alerts | Files touched |
|---|---:|---|
| go/regex/missing-regexp-anchor | 8 | `internal/gaps/detector.go` (added `\b` prefix) |
| go/uncontrolled-allocation-size | 7 | `internal/retrieval/{consensus,distribution,rerank,service}.go`, `internal/ape/calibration.go`, `internal/api/handlers_enforcement.go` (10000-elem cap) |
| go/incorrect-integer-conversion | 2 | `internal/cli/serve.go` (TSDBMaxConns cap 4096), `internal/llmclient/client.go` (failure-threshold cap 1M) |
| py/incomplete-url-substring-sanitization | 2 | `neural/benchmarks/llm_judge.py`, `neural/training/distill_driver.py` (urlparse hostname compare) |
| go/incomplete-url-scheme-check | 1 | `plugins/docs-scraper/extractor.go` (added `data:`, `vbscript:` reject) |
| js/xss-through-dom | 1 | `internal/api/ui/tabs/training_data.js` (CUIDv2 regex validation on exportId) |

## Dismissed alerts (11)
| # | Rule | File | Rationale |
|---|---|---|---|
| 12 | weak-sensitive-data-hashing | `internal/auth/apikey.go:50` | API keys are 256-bit random, SHA-256 appropriate (bcrypt/argon2 for user passwords, not high-entropy tokens) |
| 23,24 | clear-text-logging | `internal/cli/config_cmd.go` | Values masked upstream by `EffectiveConfig` via `isSensitive()` |
| 25 | clear-text-logging | `internal/cli/db.go:507` | Operator terminal — needs password to log in to Neo4j browser |
| 26 | clear-text-logging | `internal/cli/embeddings.go:199` | Masked (`first4****last4`) for credential-verification UX |
| 27,28 | clear-text-logging | `internal/cli/init.go:690,721` | `runDBStart` errors never contain password (static messages only; password appears in docker argv, not stderr) |
| 29,30 | clear-text-logging | `internal/cli/init.go:1008,1009` | Docker deployment success — operator needs Grafana/Neo4j passwords to log in |
| 72 | clear-text-logging | `internal/api/server.go:405` | Logs `%T` type name (not the key); default branch of type-switch |
| 73 | clear-text-logging | `plugins/linear-module/main.go:123` | Logs env-var NAME (not the value); CodeQL confused `apiKeyEnv` with `apiKey` |

## Deferred (1)
- **#71** `internal/jiminy/service.go:3838` (uncontrolled-allocation-size) — inside JIMINY-CEILING-BREAK-2 measurement no-touch list. Fix in a follow-up sprint after the measurement window closes (2026-08-19).

## Bonus hardening (not tied to a specific alert)
- **Widened `isSensitive`** in `internal/config/yaml_config.go` from 2 to 8 env-var names (`AUTH_API_KEYS`, `AUTH_JWT_SECRET`, `LINEAR_WEBHOOK_SECRET`, `TSDB_PASSWORD`, `RERANK_JINA_API_KEY`, `GRAFANA_PASSWORD` added alongside the existing `NEO4J_PASS`, `OPENAI_API_KEY`). Closes a class of "add a new secret env var + forget to mask it" incidents in `mdemg config show` output.

## Architectural rules pinned
1. **Regex patterns for URL/host detection MUST use `\b` word-boundary anchors** on the host side to prevent `evilgithub.com/…` matching the `github` integration. `internal/gaps/detector.go::extractDataSourceReferences` is the enforcement site.
2. **URL scheme deny-lists MUST include `data:` and `vbscript:` alongside `javascript:`**, matched after lowercase normalization (so `JavaScript:` cannot slip past).
3. **Substring-in-URL checks are unsafe** — use `urlparse(url).hostname` and compare hostnames. `"api.openai.com" in url` matches attacker-controlled URLs like `evil.com?spoof=api.openai.com`.
4. **When adding a secret-shaped env var, add it to `internal/config/yaml_config.go::isSensitive` in the same commit** — `mdemg config show` displays the effective config in cleartext otherwise. Widened list is the enforcement seam.
5. **Every `make([]T, n)` where `n` traces to an operator-tunable value MUST have a defensive cap** — even when `n` is bounded by an upstream `len(...)`, a future refactor widening that upstream slice can blow up memory. Class rule; site-specific caps.
6. **Every `int32(n)` conversion of a `strconv.Atoi`-derived value MUST bounds-check first** — an oversized env-var value silently truncates to a nonsensical int32.
7. **DOM sinks that receive server-response IDs MUST client-side validate the ID shape before use** — even trusted server IDs, since a compromised server response could poison the closure. CUIDv2 regex `^[a-z0-9]{20,32}$` is the shape gate for MDEMG-minted IDs.

## Verification
- `go build ./...` clean
- `golangci-lint run` on touched packages: 0 issues
- Unit tests green on all touched packages (`retrieval`, `ape`, `api`, `config`, `gaps`, `llmclient`, `cli`)

## Follow-ups disclosed
- **SEC-TRANCHE-4** (after 2026-08-19): revisit alert #71 (`internal/jiminy/service.go:3838` uncontrolled-allocation) once the JIMINY-CEILING-BREAK-2 window closes.
