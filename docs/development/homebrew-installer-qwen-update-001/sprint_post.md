# HOMEBREW-INSTALLER-QWEN-UPDATE-001 — Sprint Post (Phase B)

**Task**: #134
**Shipped (Phase B)**: 2026-08-19
**Ship state**: PUBLISH_GUIDE + ModelName-guarded SHA verify shipped. Phase A (operator publish) + Phase C (real manifest wire-up) tracked as sequel work.

## What shipped

1. **`docs/development/homebrew-installer-qwen-update-001/PUBLISH_GUIDE.md`** (~200 lines) — operator-actionable runbook for publishing Qwen3.8-27B GGUF quants (Q4_K_M / Q5_K_M / Q8_0) to `reh3376/mdemg-llm-v2:*` on Ollama Library. Mirrors MODEL-DIST-001's shipped pipeline shape (dequant → f16 GGUF → llama-quantize → Ollama Modelfile → `ollama create` → `ollama push`). Includes SHA capture step, Modelfile template, 6-step wall-clock estimate (2-4h), common pitfalls (chat template drift, size limits, storage headroom).
2. **`internal/cli/model.go`** — SHA verify block (lines ~307-346) gains a ModelName-scoped guard. When the loaded manifest's `ModelName` doesn't match the request's `cfg.ModelName`, the SHA verify is skipped with an explicit log: `"SHA verify: skipped — loaded manifest is for %q, request is for %q (expected when pulling a new model version before its manifest ships)"`. Prevents the latent false-mismatch bug where a v2 pull would find v1's SHA at the same quant key and fire `SHA mismatch: pulled blob has <v2-hash>, quant manifest expects <v1-hash> — do not trust this artifact`.
3. **`internal/cli/model_fetcher.go`** — new pure function `manifestAppliesToRequest(manifestModelName, requestModelName string) bool` that encapsulates the guard predicate: matches exact + case-insensitive; treats empty `manifestModelName` as unversioned/applies-to-any (backward-compat).
4. **`internal/cli/model_test.go`** — 2 new pin tests (10 assertions total):
   - `TestManifestAppliesToRequest` — 8 subtests covering match, mismatch, case-insensitive, empty-manifest-unversioned backward-compat
   - `TestManifestAppliesToRequest_ShippedManifestVsV2Request` — regression pin: with the SHIPPED embedded `quant_manifest.json` (`ModelName="mdemg-llm-v1"`) and a v2 request, the guard MUST skip; and with a v1 request, MUST verify (v1's shipped contract preserved).

## Verification (`must-validate-all-claims-before-commit` applied throughout)

| Claim | Verification | Verdict |
|---|---|---|
| Task #91 bake-off winner | Read `training_data/eval/qwen27b-bakeoff/*.json`: Qwen3.8-27B @ 180s = 0.9105 vs baseline 0.8047 (+0.11); Qwen3.6-27B @ 180s = 0.9010 | ✅ 3.8 wins (operator-locked) |
| `mdemg-llm-v2` not published on Ollama Library | `curl https://ollama.com/reh3376/mdemg-llm-v2 → 404`; v1 → 200 | ✅ confirmed; Phase A prerequisite for `mdemg model pull --name mdemg-llm-v2` to succeed live |
| SHA verify block would false-mismatch on v2 pull today | Read `internal/cli/model.go:321-326`: `rec, ok = mf.Quants[quant]` keyed on quant name only → v1 Q5_K_M SHA returned for a v2 Q5_K_M blob → hard error | ✅ latent bug confirmed; guard fixes it |
| `mdemg model use` does NOT exist as shipped | grep `internal/cli/model.go`: subcommands are `pull|list|verify|remove|where|run|swap|rollback`; no `use` | ✅ consistent with BETA-DOCS-MODEL-VERSIONING-001 finding |
| v1 pull path unchanged post-sprint | `TestManifestAppliesToRequest_ShippedManifestVsV2Request` v1-branch asserts `manifestAppliesToRequest("mdemg-llm-v1", "mdemg-llm-v1") == true` → SHA verify runs as today | ✅ backward-compat pinned |
| `mdemg model pull --help` still lists all flags post-sprint | Live invocation prints unchanged help + flag list | ✅ |
| `go test ./internal/cli/...` green | `ok mdemg/internal/cli 0.482s` including new tests | ✅ |
| `golangci-lint run ./internal/cli/` clean | `0 issues.` | ✅ |

## Decisions

| Decision | Rationale |
|---|---|
| Publish-first sequencing (Phase A operator, Phase B this sprint, Phase C follow-up) | Operator-locked (2026-08-19): "no phantom SHAs". Ships useful code (the safety fix) + the operator runbook NOW; real manifest wire-up waits for real SHAs. |
| Extract guard to pure function `manifestAppliesToRequest` | Testable in isolation without live Ollama or mock manifest scaffolding; regression-pinnable against the shipped embedded manifest. |
| Empty `manifestModelName` treated as "applies to any" | Backward-compat: preserves legacy quant_manifest.json shapes that predate the ModelName field. Predicate returns `true` on empty → SHA verify runs → behavior identical to pre-sprint for legacy manifests. |
| Case-insensitive match (`strings.EqualFold`) | Matches existing SHA-verify code pattern (line 325: `strings.EqualFold(result.SHA256, rec.SHA256)`); resilient to `MDEMG-LLM-V1` vs `mdemg-llm-v1` config drift. |
| Log message names BOTH manifest + request models + explains context | Operator visibility on why verify skipped; "(expected when pulling a new model version before its manifest ships)" prevents interpretation as an error. |
| No live end-to-end pull-of-v2 in this sprint | v2 doesn't exist on Ollama yet (Phase A prerequisite). Bounded live smoke: v1 pull still works (contract-preserved), `mdemg model pull --help` unaffected. Full end-to-end is Phase C. |
| Don't touch `quant_manifest.json` | v1's SHAs are shipped/deployed contract; even accidental whitespace change would risk verify contract. Zero-touch = maximum safety. |
| Don't ship `quant_manifest_v2.json` scaffold with placeholder SHAs | Placeholder SHAs would either (a) fire false-mismatch when v2 is finally pulled, defeating the point, or (b) be immediately reverted by Phase C. Skip entirely per "no phantom SHAs". |

## Follow-ups

### 🔴 Phase A — operator publish (external, blocks Phase C)
Follow `PUBLISH_GUIDE.md`. Publishes 3 quants of Qwen3.8-27B to `reh3376/mdemg-llm-v2:{Q4_K_M,Q5_K_M,Q8_0}`. Captures per-quant SHA256 + Ollama manifest digest + file sizes. Verifies via `curl https://ollama.com/reh3376/mdemg-llm-v2 → 200`. Estimated 2-4h wall-clock; mostly `llama-quantize` + `ollama push` (both unattended).

### 🔴 Phase C — HOMEBREW-INSTALLER-QWEN-UPDATE-002 (follow-up sprint, depends on Phase A)
- Create `internal/cli/quant_manifest_v2.json` with real SHAs + Ollama digests from Phase A
- Extend `LoadQuantManifest(cfg)` to pick `quant_manifest.json` vs `quant_manifest_v2.json` based on `cfg.ModelName` (embed both side-by-side via `//go:embed` — verified feasible per LoadQuantManifest's current shape)
- Update `docs/features/local-model-distribution.md` with v2 quant tiers + RAM math (27B needs different RAM auto-pick thresholds than 14B)
- Full end-to-end live smoke: `mdemg model pull --name mdemg-llm-v2 --quant Q5_K_M` → SHA verify PASSES → symlink lands → operator edits `.env` `MDEMG_MODEL_PATH` → `launchctl kickstart -k gui/$UID/com.mdemg.llama-server` → llama-server serves the 27B model
- Optional: `mdemg model use <name>` shorthand command (collapses env-var edit + kickstart into one CLI action; operator flagged as "may ship as part of #134" in BETA-DOCS-MODEL-VERSIONING-001; can be part of Phase C or its own tiny sprint)

### Downstream unblocked
- **PHASE-E4-GATE-PROMOTE-001** — once Phase A+C ship, E4 (LoRA promote) has an installer that can distribute v2 to end users. Not blocked by E3 retrain outcome; E4 promotes whatever E3 produces (or a raw base, depending on operator choice).

## Arch rules pinned

- **When a code change would break the "pulling a new model version" flow on customer machines (SHA-mismatch false-alarm), ship the safety guard BEFORE the operator publishes the new version.** The guard is a defensive no-op for the current v1 world; it's the safety net for the v2 crossover. Never assume "we'll ship the manifest and the guard at the same time" — publish ordering may drift, and a broken customer pull is worse than a redundant guard.
- **When a code change's live end-to-end verification requires external infrastructure (operator publishes a model, third-party CDN, etc.) that isn't ready yet, ship the code with bounded live testing (does the flag still work? does help still print?) + an operator runbook (PUBLISH_GUIDE) for the external prerequisite + a follow-up sprint scoped for the wire-up after the external work lands.** Same shape as the FT-RECURSIVE / MODEL-DIST arc's multi-phase sprints. Don't try to squeeze external-dependency work into the same sprint as code.

## Documents Accessed

- `training_data/eval/qwen27b-bakeoff/{baseline_20260817.json,qwen3.6_27b_20260817_180s.json,qwen3.8_27b_20260817_180s.json,…}` (task #91 verdict data)
- `docs/development/model-swap-muse-glimmer-eval-001/sprint_plan.md` (unrelated sprint; sanity-checked to confirm task #91 is elsewhere)
- `internal/cli/model.go` (SHA verify block lines 307-346 — MODIFIED to add guard)
- `internal/cli/model_fetcher.go` (QuantManifest shape lines 69-118; NEW helper `manifestAppliesToRequest`)
- `internal/cli/model_test.go` (test file shape; NEW 2 test blocks appended)
- `internal/cli/quant_manifest.json` (v1's shipped SHAs — READ ONLY; sacred; not modified)
- `docs/development/model-dist-001/quant_manifest.json` (source-of-truth mirror; not modified)
- Live `curl https://ollama.com/reh3376/mdemg-llm-v{1,2}` (v1: 200, v2: 404)
- Live `mdemg model pull --help` (spot-check; flags unchanged)
- CLAUDE.md pins (MODEL-DIST-001, MODEL-DIST-002, task #91 MODEL-SWAP-QWEN27B-EVAL, BETA-DOCS-MODEL-VERSIONING-001, PHASE-E1/E2, `must-validate-all-claims-before-commit`)
- Operator scope answers 2026-08-19: 27B (Qwen3.8), Ollama Library, version-tag both, publish-first
- `docs/development/homebrew-installer-qwen-update-001/sprint_plan.md`, `PUBLISH_GUIDE.md`
