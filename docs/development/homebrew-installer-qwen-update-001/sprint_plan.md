# HOMEBREW-INSTALLER-QWEN-UPDATE-001 — Sprint Plan (Phase B)

**Task**: #134 (BLOCKING for E4 promote)
**Sprint format**: v1.0 (12-section)
**Phase**: B of A/B/C multi-phase arc (see §12 for Phase A + C)

## 1. Header & Metadata

- **Sprint ID**: `HOMEBREW-INSTALLER-QWEN-UPDATE-001` (Phase B — code safety fix + publish guide)
- **Author**: reh3376 / claude
- **Date**: 2026-08-19
- **Branch**: `reh3376_dev01`
- **Estimated wall-clock**: ~2 hours
- **Follow-up sprint**: `HOMEBREW-INSTALLER-QWEN-UPDATE-002` (Phase C — wire real manifest after operator publishes)

## 2. Problem Statement

**Operator directive** (2026-08-19): "we need to update the homebrew version to accommodate the new Qwen model, the installer was originally set up to retrieve the base model and adapters from Ollama."

Recon revealed a latent code bug: today's `mdemg model pull --name mdemg-llm-v2` would 404 on Ollama (v2 unpublished) — but the moment it IS published, the shipped SHA-verify block (`internal/cli/model.go:321-326`) would fire a **false SHA-mismatch error** because the loaded manifest is `mdemg-llm-v1`'s manifest with v1's SHAs, and the map lookup by quant name (`Q5_K_M`) succeeds against v1's SHA — which won't match v2's blob. Operator would see: `SHA mismatch for Q5_K_M: pulled blob has <v2 hash>, quant manifest expects <v1 hash> — do not trust this artifact`.

This sprint (Phase B) ships:
1. A **PUBLISH GUIDE** operator-runbook that walks through publishing Qwen3.8-27B (the task #91 bake-off winner, 0.9105 @ 180s vs baseline 0.8047) to `reh3376/mdemg-llm-v2:*` on Ollama Library.
2. A **ModelName-guarded SHA verify** safety fix so `mdemg model pull --name mdemg-llm-v2` (once v2 is published) doesn't hit the false-mismatch bug — it detects manifest-vs-request mismatch, prints a friendly "SHA verify: skipped — manifest is for mdemg-llm-v1, request is for mdemg-llm-v2" line, and proceeds.

Phase C (follow-up sprint, after operator executes the publish guide) will wire the real v2 manifest with SHAs captured at publish time.

## 3. Scope & Constraints

### In scope
1. **`docs/development/homebrew-installer-qwen-update-001/PUBLISH_GUIDE.md`** — operator-runbook covering: source model URL (Qwen3.8-27B), quantization pipeline (`llama-quantize` for Q4_K_M/Q5_K_M/Q8_0 — reuses MODEL-DIST-001's shipped pattern), Ollama Modelfile shape, `ollama create` + `ollama push` commands, per-quant SHA capture, and Phase-C follow-up sprint pointer.
2. **`internal/cli/model.go`** — SHA verify block extended with `manifest.ModelName == cfg.ModelName` guard. Mismatch → skip with clear log; match → verify as today.
3. **`internal/cli/model_test.go`** — pin test for the guard behavior: (a) matching model → verify runs; (b) mismatching model → verify skipped, no error.
4. Sprint plan + post; CLAUDE.md arch note; CHANGELOG entry.

### Out of scope (deferred to Phase C — `HOMEBREW-INSTALLER-QWEN-UPDATE-002`)
- Publishing the actual v2 GGUF quants to Ollama Library (operator-driven per §12 Phase A).
- Populating `quant_manifest_v2.json` with real SHAs (blocked on Phase A).
- Multi-model manifest schema restructure (defer until v3+ arrives; current single-manifest-per-model shape with name-guard suffices for v1+v2 coexistence).
- `mdemg model use <name>` shorthand command (mentioned in the operator's scope answer but out-of-scope for a compact sprint; env-var + kickstart works today).
- Updating `packaging/homebrew-mdemg/README.md` — already updated in BETA-DOCS-MODEL-VERSIONING-001 to reference the incoming v2.
- 27B RAM tier tuning for auto-pick — will land in Phase C when the real v2 manifest ships (RAM tiers are per-quant, need real quant sizes).

### Hard invariants
- **Byte-identical `quant_manifest.json`**: this sprint does NOT touch the v1 manifest. Any change would risk v1's SHA-verify contract.
- **Backwards-compat**: existing `mdemg model pull` (bare, default `--name mdemg-llm-v1`) MUST work identically post-change (v1's manifest ModelName matches v1 default → verify path unchanged).
- **Fail-open on nil**: the guard skips verify (does NOT hard-fail) on ModelName mismatch — the operator's rollback path (pull v1) must still work if they later swap manifests.
- **`must-validate-all-claims-before-commit`**: recon verified — bake-off numbers checked live (`training_data/eval/qwen27b-bakeoff/`), Ollama Library confirmed 404 for v2 (`curl https://ollama.com/reh3376/mdemg-llm-v2 → 404`), 200 for v1.

## 4. Dependencies

**Upstream (must be shipped)**:
- ✅ MODEL-DIST-001 (2026-05-11): `mdemg model pull` + Ollama fetcher
- ✅ MODEL-DIST-002 (2026-05-25): adapter fetch path
- ✅ Task #91 bake-off completed (2026-08-17) — Qwen3.8-27B @ 180s scores 0.9105 vs baseline 0.8047
- ✅ Operator-locked scope answers (2026-08-19): 27B / Ollama / version-tag both

**Downstream (this sprint unblocks)**:
- Phase A (operator-driven publish per PUBLISH_GUIDE.md)
- Phase C follow-up sprint (wire real v2 manifest after publish)

## 5. Implementation Plan (sequential — 4 epics)

### Epic 1: PUBLISH_GUIDE.md (~45min)
- Operator-facing runbook, mirrors MODEL-DIST-001's shipped pipeline shape:
  - Prerequisite: Qwen3.8-27B GGUF conversion (llama.cpp `convert_hf_to_gguf.py` from an HF-safetensors source, OR direct MLX→bf16 dequant if starting from MLX)
  - Quantize to 3 tiers: Q4_K_M (~16 GB), Q5_K_M (~19 GB estimated for 27B), Q8_0 (~28 GB estimated)
  - Modelfile template (FROM ./quant.gguf; TEMPLATE ...; PARAMETER ...)
  - `ollama create reh3376/mdemg-llm-v2:Q5_K_M -f Modelfile`
  - `ollama push reh3376/mdemg-llm-v2:Q5_K_M`
  - SHA capture: `shasum -a 256 <blob>` on the pushed GGUF; also record `ollama_manifest_digest` from the push output
  - Repeat for Q4_K_M + Q8_0 quants
  - Verify published: `curl https://ollama.com/reh3376/mdemg-llm-v2 → 200`
  - Hand-off to Phase C: fill in the captured SHAs in `quant_manifest_v2.json` (Phase C sprint will create the file)

### Epic 2: ModelName-guarded SHA verify (~30min)
- Modify `internal/cli/model.go` around line 321-334 (SHA verify block inside `runModelPull`)
- Add: if `mf.ModelName != "" && !strings.EqualFold(mf.ModelName, cfg.ModelName)`, log "SHA verify: skipped — loaded manifest is for %s, request is for %s (this is expected when pulling a new model version before its manifest ships)" and skip the SHA check.
- Preserves all other behavior (empty ModelName in manifest → continue verify as today; match → verify as today).

### Epic 3: pin test (~30min)
- Add test in `internal/cli/model_test.go`:
  - `TestModelPull_SHAVerifySkipsOnModelNameMismatch` — mock/fake manifest with `ModelName: "mdemg-llm-v1"`, invoke SHA-verify path via a test-scoped helper OR by refactoring the verify block into a testable function; assert skip log + no error for mismatched requests.
- If refactor scope is too large, ship a smaller unit test that exercises just the guard predicate.

### Epic 4: docs + verification (~15min)
- Sprint plan (this file) + sprint_post.md
- CLAUDE.md arch note
- CHANGELOG Unreleased entry
- Verification: `go build ./... && go test ./internal/cli/... && golangci-lint run ./internal/cli/`

## 6. Testing Plan (3 tiers)

### Tier 1 — Unit
- Pin test for ModelName-guarded verify behavior (matching → runs, mismatching → skips)
- Existing `internal/cli/model_test.go` + `model_run_test.go` stay green (backward-compat)

### Tier 2 — Integration
- `go build ./...` clean
- `go test ./internal/cli/... ./internal/config/... ./internal/api/...` green

### Tier 3 — Live end-to-end
- **Live is BOUNDED** this sprint: v2 isn't published yet, so pull-of-v2 will 404 at Ollama — that's the expected pre-publish state. What we CAN verify:
  - `mdemg model pull --name mdemg-llm-v2 --dry-run` prints the target tag correctly (no crash)
  - `mdemg model pull --name mdemg-llm-v1` (default path) still works identically to pre-sprint
  - Boot log (`launchctl kickstart -k gui/$UID/com.mdemg.server`) shows no regressions
- Full end-to-end pull-then-verify of v2 is Phase C.

## 7. Commit Strategy

- 1 primary commit: PUBLISH_GUIDE.md + model.go verify guard + test + docs
- Follow-up commit if live smoke uncovers a surprise defect
- Phase C is a separate sprint + separate commits

## 8. Verification Checklist

- [ ] `docs/development/homebrew-installer-qwen-update-001/PUBLISH_GUIDE.md` present + operator-actionable
- [ ] `internal/cli/model.go` SHA verify block gains ModelName guard; log message names both models
- [ ] Pin test for the guard behavior green
- [ ] `go build ./...` clean
- [ ] `golangci-lint run ./internal/cli/` — 0 issues
- [ ] `go test ./internal/cli/... ./internal/config/... ./internal/api/...` — all green
- [ ] `mdemg model pull --name mdemg-llm-v1` (default) works identically to pre-sprint (spot-check)
- [ ] Sprint plan + post in `docs/development/homebrew-installer-qwen-update-001/`
- [ ] CLAUDE.md arch note added
- [ ] CHANGELOG entry
- [ ] PR sprint-summary comment posted

## 9. Documentation Update

### Files created
- `docs/development/homebrew-installer-qwen-update-001/{sprint_plan,sprint_post,PUBLISH_GUIDE}.md`

### Files modified
- `internal/cli/model.go` — SHA verify block gains ModelName guard
- `internal/cli/model_test.go` — new pin test
- `CLAUDE.md`, `CHANGELOG.md`

### Files NOT modified (out-of-scope)
- `internal/cli/quant_manifest.json` (v1's SHAs; sacred)
- `packaging/homebrew-mdemg/*` (BETA-DOCS-MODEL-VERSIONING-001 already covered)
- Any `.env` / runtime config default

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Guard skips SHA verify when it should have caught a real corruption | Low | Medium | Guard only skips when `manifest.ModelName != cfg.ModelName`; matching-name case (v1 pull with v1 manifest) is byte-identical to today's behavior |
| PUBLISH_GUIDE.md gets steps wrong → operator's publish fails | Medium | Medium | Guide mirrors shipped MODEL-DIST-001 pipeline; operator can cross-ref MODEL-DIST-001's post doc; if steps drift from reality, sprint post gets a "corrections applied" section |
| Refactor of SHA-verify block breaks something not covered by tests | Low | Medium | Minimal-touch: only ADDS a conditional skip; doesn't restructure existing verify logic |
| Operator interprets "skipped" log as an error | Low | Low | Log wording explicitly says "this is expected when pulling a new model version before its manifest ships" |

## 11. Rollback Procedures

- Zero substrate mutation (no Neo4j / TSDB touch); zero schema change.
- Code rollback: `git revert` the sprint commit.
- Runtime: no `.env` change, no service restart needed for the rollback to take effect (unless already deployed the new binary).

## 12. Documents Accessed + multi-phase arc plan

### Documents accessed (`must-validate-all-claims-before-commit`)
- `training_data/eval/qwen27b-bakeoff/*.json` (bake-off scores: Qwen3.8-27B @ 180s = 0.9105 wins)
- `docs/development/model-swap-muse-glimmer-eval-001/sprint_plan.md` (unrelated sprint; not task #91's home)
- `internal/cli/model.go` (verified subcommands: `pull|list|verify|remove|where|run|swap|rollback`; `use` absent; SHA verify block at 321-334)
- `internal/cli/model_fetcher.go` (QuantManifest shape lines 69-118; ModelName field exists on top-level manifest)
- `internal/cli/quant_manifest.json` (v1's shipped SHAs — sacred, not modified)
- Live `curl https://ollama.com/reh3376/mdemg-llm-v{1,2}` (v1: 200, v2: 404 — confirms Phase A prerequisite)
- CLAUDE.md pins (MODEL-DIST-001/002, task #91 MODEL-SWAP-QWEN27B-EVAL, PHASE-E1/E2, BETA-DOCS-MODEL-VERSIONING-001, `must-validate-all-claims-before-commit`)
- Operator scope answers 2026-08-19: 27B (task #91 follow-up), Ollama Library, version-tag both, publish-first sequencing

### Multi-phase arc structure

**Phase A — Operator-driven publish (external, unblocks Phase C)**:
Follow `PUBLISH_GUIDE.md` shipped in this sprint. Publishes Qwen3.8-27B Q4_K_M / Q5_K_M / Q8_0 quants to `reh3376/mdemg-llm-v2:*` on Ollama Library. Captures per-quant SHAs + Ollama manifest digests. Verifies `curl https://ollama.com/reh3376/mdemg-llm-v2` → 200. Confirms operator has valid Ollama account credentials + storage headroom for the pushes.

**Phase B — Code safety + publish guide (this sprint)**:
Ships the PUBLISH_GUIDE.md + ModelName-guarded SHA verify + tests + docs.

**Phase C — Wire real v2 manifest (follow-up sprint `HOMEBREW-INSTALLER-QWEN-UPDATE-002`)**:
Depends on Phase A completing. Creates `internal/cli/quant_manifest_v2.json` with real SHAs from Phase A. Extends `LoadQuantManifest` to pick correct manifest based on `cfg.ModelName` (embed v1 + v2 side-by-side). Updates `docs/features/local-model-distribution.md` with v2 quant tiers + RAM math. Full end-to-end live smoke: `mdemg model pull --name mdemg-llm-v2 --quant Q5_K_M` → SHA verify PASSES → symlink lands → operator edits `.env` `MDEMG_MODEL_PATH` → `launchctl kickstart` → llama-server serves the 27B model.

---
