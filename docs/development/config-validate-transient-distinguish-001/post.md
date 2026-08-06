# CONFIG-VALIDATE-TRANSIENT-DISTINGUISH-001 — Sprint Post

**Date:** 2026-08-05 | **Branch:** `reh3376_dev01`
**Trigger:** B3 blocker from beta-readiness Phase 1 (INIT-DEFAULTS-LOCAL-FIRST-001 sibling). Fresh `mdemg init --defaults` immediately followed by `mdemg config validate` reported `Validation: FAILED` + exit 1 — beta tester reads that as "MDEMG is broken" when the actual state is "user hasn't run `docker compose up -d` yet."

## Verdict

Shipped. Validator now distinguishes "containers not started yet" (transient, config-fine, PASSED with next-step hint) from "container up but service broken" (real error, FAILED). Exit code preserved for real errors; transient state returns exit 0 with a helpful "run: docker compose up -d" note.

## What was wrong

`internal/cli/config_cmd.go:157-197` `runConfigValidate` did:
```go
if testNeo4jReachable(neo4jURI) {
    fmt.Println("OK")
} else {
    fmt.Println("UNREACHABLE")
    hasErrors = true    // ← blindly treats "not started" the same as "broken"
}
```

Result: every fresh install that hadn't yet run `docker compose up -d` reported `FAILED` with a scary UNREACHABLE label. Beta testers would abandon at this step.

## What shipped

### `internal/cli/config_cmd.go` — three-way state instead of binary
```go
composeUp := composeStackRunning()
var hasTransient bool

// Neo4j:
if testNeo4jReachable(neo4jURI) {
    // OK
} else if !composeUp {
    fmt.Println("NOT STARTED")
    hasTransient = true    // ← config's fine; user needs to start services
} else {
    fmt.Println("UNREACHABLE (containers up but Bolt port not responding)")
    hasErrors = true       // ← real error; containers running but broken
}
```

Same pattern for embedding provider (`ollama` unreachable → transient; `openai` unknown → error unchanged).

### Final classification:
```go
if hasErrors { FAILED, exit 1 }
else if hasTransient { PASSED (services not started — run: docker compose up -d), exit 0 }
else { PASSED, exit 0 }
```

### New helper: `composeStackRunning()`
```go
cmd := exec.CommandContext(ctx, "docker", "compose", "ps", "-q")
out, err := cmd.Output()
if err != nil { return false }  // docker missing / permission denied → safe default
return len(strings.TrimSpace(string(out))) > 0
```

Bounded 2s timeout. Any failure returns false (which flips the caller into the "not started" branch — the safe default when we can't tell).

### Also fixed: `""` and `"disabled"` embedding provider now both render as `(disabled)`
Pre-sprint only empty string mapped to "(disabled)"; the new `disabled` value from INIT-DEFAULTS-LOCAL-FIRST-001 would have shown as `unknown provider "disabled"` + hasErrors. Fixed here so the two work together end-to-end.

## Live Tier-3 (three paths verified)

**Path 1 — fresh init, no key, no services started (the beta blocker)**:
```
$ env -u OPENAI_API_KEY mdemg init --defaults > /dev/null
$ docker compose down -v > /dev/null  # simulate "just after init, before start"
$ mdemg config validate
...
Neo4j:    bolt://localhost:7688 ... NOT STARTED
Embedding: (disabled)
Validation: PASSED (services not started — run: docker compose up -d)
exit=0
```
Was `UNREACHABLE + FAILED + exit 1` pre-sprint.

**Path 2 — with-key init, key not exported, no services**:
```
$ OPENAI_API_KEY="sk-fake" mdemg init --defaults > /dev/null && docker compose down -v > /dev/null
$ env -u OPENAI_API_KEY mdemg config validate
Neo4j:    bolt://localhost:7688 ... NOT STARTED
Embedding: openai (WARNING: OPENAI_API_KEY not set)
Validation: PASSED (services not started — run: docker compose up -d)
exit=0
```
Warning preserved. PASSED because config is valid (the WARNING is a WARNING, not an ERROR).

**Path 3 — with-key init, key exported, no services**:
```
$ OPENAI_API_KEY="sk-fake-test" mdemg config validate
Neo4j:    bolt://localhost:7688 ... NOT STARTED
Embedding: openai (API key set)
Validation: PASSED (services not started — run: docker compose up -d)
```

All 3 scratch runs cleaned up. `go test ./internal/cli/` clean.

## Rules pinned

⚠️ **Config validators MUST distinguish transient service-not-running state from persistent config errors** — the two produce identical low-level failure modes (TCP connect refused) but require completely different operator responses. "Not started" → run `docker compose up -d`. "Broken" → fix your config. Conflating them treats every fresh install as broken and erodes operator trust in the validation output. When adding a new dependency check (Redis, Ollama sidecar, etc.), audit whether "not reachable" can also mean "not started yet" and branch on the stack-running signal.

⚠️ **When adding a new provider enum value, audit every switch that handles the old enum values** — INIT-DEFAULTS-LOCAL-FIRST-001 introduced `EMBEDDING_PROVIDER=disabled`, and this validator would have crashed to `unknown provider "disabled"` if we hadn't caught it here. Enum-value additions are code changes; treat them as such (grep for the switch statements, update all).

⚠️ **`docker compose ps -q` is the honest "is my stack running" signal** — cheaper than Docker API calls, works across compose v1/v2, respects the current directory's compose project. Reserve `docker compose ps --format json` for when you need per-service state (health, ports); use `-q` for boolean "any container running."

## Not shipped (intentional)

- **Per-service state reporting** — `NOT STARTED` doesn't distinguish "no containers at all" from "some containers but not Neo4j specifically." For beta, the simpler binary check is fine; the fix path is `docker compose up -d` regardless. If beta testers report confusion, add per-service `docker compose ps neo4j` inspection.
- **Auto-start prompt** — validator could ASK "start services now? (y/n)" and run `docker compose up -d`. Deferred — that's `mdemg init --defaults`'s job (which DOES auto-start); post-`init` validate should be a passive read-only check, not an actionable prompt.
- **Retry with exponential backoff after `docker compose up -d`** — if the user runs `validate` while services are STARTING (containers created but not healthy yet), they'll still see NOT STARTED because Bolt won't respond. Fine — the operator re-runs validate after healthchecks pass.

## Follow-ups disclosed

- **README quickstart audit** — the shipped Quick Start section may still tell testers to run `mdemg init` then `mdemg config validate` immediately; the new PASSED-with-next-step output is much friendlier. Small doc verify.
- **Beta plan T1.8 update** — `packaging/homebrew-mdemg/mdemg_beta_testing.md` T1.8 "Configuration Display & Validation" should note the new pre-start-PASSED behavior so beta testers know what's normal.

## Rollback

Single-commit revert. Pre-sprint behavior restored: any UNREACHABLE = FAILED + exit 1.

## Documents Accessed

- `internal/cli/config_cmd.go:1-208` (validator; edited)
- INIT-DEFAULTS-LOCAL-FIRST-001 post (parent + shipped `disabled` enum value)
- `packaging/homebrew-mdemg/mdemg_beta_testing.md` T1.8 (the beta test that would have caught this)
- Live `/tmp/mdemg-beta-b3` + `/tmp/mdemg-beta-b3-key` scratch runs (3-path Tier-3 evidence)
