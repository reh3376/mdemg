# INIT-DEFAULTS-LOCAL-FIRST-001 — Sprint Post

**Date:** 2026-08-05 | **Branch:** `reh3376_dev01`
**Trigger:** Beta-readiness Phase 1 dry-run of `mdemg init --defaults` against a fresh scratch directory (`/tmp/mdemg-beta-scratch`) surfaced 2 fresh-install blockers B1 + B2. Combined into one sprint because both are `init.go`-adjacent + address the same fresh-install UX class.

## Verdict

Shipped. Fresh `mdemg init --defaults` without an `OPENAI_API_KEY` no longer produces a config that fails validation — falls back to `disabled` mode with a clear next-steps summary. Fresh init also seeds `RSIC_PROTECTED_SPACES` with the new space so the scary "destructive actions have no space protection" warning is gone. Both changes preserve the sunny-path (with-key) flow byte-for-byte.

## What was wrong

### B1 — `--defaults` writes OpenAI-dependent config even when there's no key
`init.go:292-297` `--defaults` branch chose `openai` for the embedding provider regardless of whether `OPENAI_API_KEY` was in the environment. The comment on line 296 explicitly said "user will be prompted for key" — but `--defaults` NEVER prompts (that's the whole point). Result: fresh beta tester with no key gets `.env` with `JIMINY_SYNTHESIS_MODEL=gpt-4.1`, `JIMINY_SYNTHESIS_PROVIDER=openai`, and `config validate → FAILED`.

### B2 — `RSIC_PROTECTED_SPACES` empty on fresh init
`init.go` never wrote `RSIC_PROTECTED_SPACES` to the generated `.env`. `config validate` on the fresh install warns: `"RSICProtectedSpaces is empty with RSIC cycles enabled — destructive actions have no space protection"`. Reads as scary "beta tester might lose data."

## What shipped

### `internal/cli/init.go` — `--defaults` respects OPENAI_API_KEY presence
```go
} else if flags.defaults {
    if hasOpenAIKey {
        opts.EmbeddingProvider = "openai"
    } else {
        opts.EmbeddingProvider = "disabled" // no key → don't lock the user out
    }
}
```
New `case "disabled":` block in the provider switch — writes empty `EmbeddingModel`/`LLMModel`, sets `LLMProvider = "disabled"` (empty-string check-friendly). Jiminy defaults-branch respects `LLMProvider == "disabled"` and picks `JiminyEnabled = false` (nothing to synthesize against).

### `internal/cli/init.go` — RSIC_PROTECTED_SPACES seed
```go
if !envContains(envLines, "RSIC_PROTECTED_SPACES") {
    envLines = append(envLines, fmt.Sprintf("RSIC_PROTECTED_SPACES=%s", opts.SpaceID))
}
```
Placed alongside the MDEMG_INSTANCE_ID seed (same lifecycle, same env-guard shape).

### `internal/cli/init.go` — next-steps summary when disabled
```
⚠  LLM & Jiminy DISABLED because no OPENAI_API_KEY was detected.
   MDEMG will still run: ingest, BM25 retrieval, dashboard, metrics.
   To enable the full feature set, pick ONE of:

   (A) OpenAI (simplest — 1 key):
       export OPENAI_API_KEY=sk-...  # then re-run: mdemg init --defaults

   (B) Ollama (local, free — ~5GB model download):
       brew install ollama && ollama serve &
       ollama pull qwen3-embedding:8b && ollama pull qwen3:8b
       mdemg init --embedding-provider ollama --defaults

   Re-run `mdemg config validate` after either change.
```
Prints AFTER "Initialization complete" ONLY when `opts.LLMProvider == "disabled"` — the with-key user sees zero extra output.

## Live Tier-3 (both paths, real subprocess, real .env inspection)

**Without key** (the beta-tester default):
```
$ env -u OPENAI_API_KEY mdemg init --defaults
...
Space ID:      mdemg-beta-scratch
⚠  LLM & Jiminy DISABLED because no OPENAI_API_KEY was detected.
   MDEMG will still run: ingest, BM25 retrieval, dashboard, metrics.
   To enable the full feature set, pick ONE of:
   (A) OpenAI (simplest — 1 key):
       export OPENAI_API_KEY=sk-...  # then re-run: mdemg init --defaults
   (B) Ollama (local, free — ~5GB model download): ...
...

$ grep -E '^(JIMINY|EMBEDDING|LLM|OPENAI|RSIC)' .env
RSIC_PROTECTED_SPACES=mdemg-beta-scratch
```
No unwanted JIMINY_*/OpenAI defaults written; the operator sees an explicit two-option next step.

**With key** (sunny path — must stay unchanged):
```
$ OPENAI_API_KEY="sk-fake-for-init-test" mdemg init --defaults
...
Initialization complete!
Space ID:      mdemg-beta-key-test
(NO disabled-mode summary — clean flow)

$ grep -E '^(JIMINY|EMBEDDING|LLM|OPENAI|RSIC)' .env
RSIC_PROTECTED_SPACES=mdemg-beta-key-test
JIMINY_ENABLED=true
JIMINY_SYNTHESIS_MODEL=gpt-4.1
JIMINY_SYNTHESIS_PROVIDER=openai
```
Sunny path preserved: full JIMINY defaults + RSIC seed. No behavior change for existing operators upgrading.

Both paths cleaned up (docker containers torn down, scratch dirs removed). `go test ./internal/cli/` clean. Lint 0 issues.

## Rules pinned

⚠️ **`--defaults` (non-interactive) init MUST detect whether the required credentials are present in the environment**, not blindly pick a provider that will fail validation. The pre-sprint code silently produced an unvalidatable config for every fresh install without OPENAI_API_KEY — a documented CONFIG-LOCAL-DEFAULTS-001-adjacent class of "code says one thing, template writes another." When adding a new provider to init.go, the `--defaults` branch MUST have a working no-config-required fallback, or explicitly refuse with a next-steps message.

⚠️ **Every knob whose empty value fires a validation WARNING must have a sensible default seeded during `mdemg init`** — the operator seeing a scary warning on their first `config validate` call reads as "MDEMG is broken." `RSIC_PROTECTED_SPACES=<space_id>` is trivially correct for a fresh install (the just-created space IS the space the operator wants protected); leaving it empty was a shipped-since-forever gap. Audit `config validate --defaults` output on every future `.env` template change.

⚠️ **Fresh-install "next steps" messages should offer EXACTLY 2 clear paths, not a menu** — the with-key path is 1 command (`export OPENAI_API_KEY=...`); the without-key path is 3 commands. More than 2 options paralyzes a beta tester who just wants to know what to type next.

## Not shipped (intentional)

- **B3 (`config validate → FAILED` on pre-server-start install)** — separate subsystem (validator, not init). Distinct sprint recommended: `CONFIG-VALIDATE-DISTINGUISH-TRANSIENT-001` or similar. That fix should re-classify "Neo4j UNREACHABLE" as "SERVICE NOT STARTED" when docker containers aren't running yet.
- **Ollama installer bootstrapping** — the ollama option instructs the user to `brew install ollama`. Could be automated in a follow-up but adds ~5min of install time and requires a large model download; keeping it operator-visible is honest.
- **Model download in `disabled` mode** — for local-first-with-mdemg-llm-v1 path (`mdemg model pull`), we'd need to distribute a 5-10 GB GGUF as part of `brew install mdemg`. Deferred — that's a MODEL-DIST/packaging conversation, not a fresh-install-UX conversation.

## Follow-ups disclosed

- **B3 sprint** — `config validate` on pre-server-start install should distinguish transient (containers not up) from configural errors. ~1-2h.
- **README quickstart audit** — the "Quick Start" section still recommends `--defaults`; verify it now points the user at the disabled-mode next-steps or updates the recommendation. Small doc edit.
- **`mdemg model pull` promotion** — should the beta guide recommend the local-first path (with `mdemg model pull` after init)? Depends on whether we want to onboard beta testers to a 10 GB download.

## Rollback

Single-commit revert. Pre-sprint behavior restored: `--defaults` picks openai unconditionally, `.env` has no RSIC_PROTECTED_SPACES, no next-steps summary.

## Documents Accessed

- `internal/cli/init.go:289-406` (embedding-provider + Jiminy defaults branches; edited)
- `internal/cli/init.go:623-660` (.env writer + post-init summary; edited)
- `packaging/homebrew-mdemg/mdemg_beta_testing.md` (T1.3 + T1.8 — the beta plan sections that would have caught this)
- `docs/development/roadmap/ROADMAP_2026Q4.md` (PLUGIN-HYGIENE decision — sibling operator-disposition item)
- Live `/tmp/mdemg-beta-scratch` + `/tmp/mdemg-beta-key-test` fresh-install runs (Tier-3 evidence)
- CONFIG-LOCAL-DEFAULTS-001 CLAUDE.md pin (the "runtime is local, template writes openai" tension this sprint resolves for `--defaults`)
