# HOMEBREW-INSTALLER-QWEN-UPDATE-002 — Sprint Plan (Phase C)

**Task**: #136
**Arc**: HOMEBREW-INSTALLER-QWEN-UPDATE (Phase A shipped 2026-08-20 PR #643 = `reh3376/mdemg-llm-v2` LIVE on Ollama Library; Phase B shipped 2026-08-19 PR #641 = ModelName-guarded SHA verify)
**Owner**: `reh3376`
**Scope**: SMALL — one new embedded JSON + narrow dispatch in `LoadQuantManifest` + model-aware RAM tier defaults + feature doc update + pin tests + live E2E smoke.

---

## 1. Header & Metadata

- **Sprint name**: HOMEBREW-INSTALLER-QWEN-UPDATE-002 (Phase C of the multi-phase arc)
- **Version**: 1.0
- **Author**: Claude (opus 4.7) + operator `reh3376`
- **Date**: 2026-08-20
- **Est. duration**: 1 focused session (~2-4 hours)
- **Category**: dev / code + docs

## 2. Problem Statement

Phase A published `reh3376/mdemg-llm-v2:{Q4_K_M,Q5_K_M,Q8_0}` on Ollama Library with real SHAs captured in `docs/development/homebrew-installer-qwen-update-001/publish_manifest_v2.json`. Phase B shipped the SHA-verify safety guard so a v2 pull today skips SHA verify with an explicit "loaded manifest is for mdemg-llm-v1" log — the pull WORKS but the SHA contract is bypassed.

**Gap**: no `quant_manifest_v2.json` shipped as embed → no SHA contract enforced for v2 pulls. `LoadQuantManifest` only knows about v1. Also: `MDEMG_MODEL_RAM_TIERS` defaults are calibrated for 14B (v1) — a v2 operator on 24 GB RAM would get Q5_K_M (18 GB) which fits but is tight, when Q4_K_M (16 GB) is safer for that tier.

**Impact of not fixing**: (1) SHA verify silently skipped for every v2 pull, so a tampered/corrupted v2 blob would land undetected; (2) `--quant auto` on v2 dispatches to the wrong tier for 24-32 GB machines; (3) E4 promote (task PHASE-E4-GATE-PROMOTE-001) is blocked because it can't confidently distribute a promoted v2 artifact without SHA enforcement.

## 3. Scope & Constraints

**In scope**:
- New embedded `internal/cli/quant_manifest_v2.json` with real SHAs + sizes + Ollama digests + 27B RAM math
- `LoadQuantManifest(cfg)` extended to dispatch by `cfg.ModelName` — v1 → v1 manifest (unchanged), v2 → v2 manifest, unknown → v1 fallback with WARN log
- Model-aware `MDEMG_MODEL_RAM_TIERS` DEFAULT (v1: `{"<16":"Q4_K_M","<24":"Q5_K_M","default":"Q8_0"}`; v2: `{"<32":"Q4_K_M","<48":"Q5_K_M","default":"Q8_0"}`) — operator override still wins
- Update `docs/features/local-model-distribution.md` with v2 quant tiers + RAM math
- Pin tests covering the dispatch + defaults
- Live E2E smoke: `MDEMG_MODEL_NAME=mdemg-llm-v2 mdemg model pull --quant Q5_K_M` → SHA verify PASSES → symlink lands

**Out of scope** (each disclosed in §11):
- `mdemg model use <name>` shorthand — deferred to its own sprint; scope-creep guard
- v2 adapter distribution — v2 ships as raw base only per operator scope decision
- Windows/Linux support for v2 — Apple Silicon-only continues per MODEL-DIST-001 line
- v3 or beyond — one bump ahead of the current v2 is unnecessary
- Live full-tier pull (~18 GB Q5) — dry-run smoke + spot-check with `--dry-run=false --quant Q4_K_M` (16 GB) if operator has time

**Constraints**:
- `quant_manifest.json` (v1) MUST stay byte-identical — v1's shipped SHAs are the deployed contract; touching them is a customer-break risk
- Additive-only: existing `LoadQuantManifest` behavior for `cfg.ModelName == "mdemg-llm-v1"` must be byte-identical
- Zero substrate mutation (docs + code + embed only)
- No Go module bumps — pure stdlib change
- `//go:embed` cannot cross package boundaries — both JSON files must live in `internal/cli/`
- SHA values in the new manifest MUST match `publish_manifest_v2.json` exactly (Phase A hand-off contract)

## 4. Dependencies

- **PR #643 (Phase A) MERGED** — `publish_manifest_v2.json` in repo ✅
- **PR #641 (Phase B) MERGED** — `manifestAppliesToRequest` helper + `LoadQuantManifest` signature stable ✅
- **`reh3376/mdemg-llm-v2:*` LIVE on Ollama Library** — verified 2026-08-20 all 3 tags → 200 ✅
- Ollama binary installed locally (Phase A prerequisite; operator has it)
- `MDEMG_MODEL_DIR` writable + ≥ 20 GB free (for live smoke; dry-run bypasses this)

## 5. Implementation Plan (sequential epics + gates)

### Epic 1 — Embed v2 manifest

Create `internal/cli/quant_manifest_v2.json` mirroring v1's shape with real Phase A values.

**Structure**:
- `version`: "1.0.0" (schema stable)
- `model_name`: "mdemg-llm-v2"
- `namespace_default`: "reh3376"
- `build_date`: "2026-08-20" (Phase A ship date)
- `quants`: 3 entries (Q4_K_M / Q5_K_M / Q8_0) — SHAs + sizes from `publish_manifest_v2.json`; add `bpw`, `min_ram_gb`, `recommended_ram_gb` for 27B model (§5.1 below)
- `adapter`: OMITTED (`nil` in Go) — v2 ships raw base per scope. Field remains optional per shipped `QuantManifest.Adapter *QuantRecord `json:"adapter,omitempty"``.
- No `ollama_local_id` (that's a per-operator local Docker digest; not deterministic across machines — v1 has it as legacy, don't replicate in v2)

**RAM math for 27B (§5.1)**:
- Q4_K_M: 15.7 GB on disk → ~18 GB RSS with context + KV. `min_ram_gb=20`, `recommended_ram_gb=24`.
- Q5_K_M: 18.2 GB on disk → ~22 GB RSS. `min_ram_gb=24`, `recommended_ram_gb=32`.
- Q8_0: 27.1 GB on disk → ~32 GB RSS. `min_ram_gb=36`, `recommended_ram_gb=48`.

**Deliverable**: `internal/cli/quant_manifest_v2.json` (~50 lines).

**Gate**: JSON is valid, SHAs match `publish_manifest_v2.json` byte-for-byte, `go build ./internal/cli/...` still passes (embed pickup).

### Epic 2 — Extend `LoadQuantManifest` for multi-model dispatch

Modify `internal/cli/model_fetcher.go`:

```go
//go:embed quant_manifest.json
var embeddedQuantManifestV1 []byte  // RENAMED for clarity; `embeddedQuantManifest` alias preserved to avoid churn

//go:embed quant_manifest_v2.json
var embeddedQuantManifestV2 []byte

// selectEmbeddedManifest returns the embedded manifest bytes for the given
// model name. Unknown names fall back to v1 with a WARN log — preserves
// the pre-Phase-C behavior for any operator running a custom ModelName
// without dispatching v2 SHAs at them.
func selectEmbeddedManifest(modelName string) []byte {
    switch strings.ToLower(strings.TrimSpace(modelName)) {
    case "mdemg-llm-v2":
        return embeddedQuantManifestV2
    case "", "mdemg-llm-v1":
        return embeddedQuantManifestV1
    default:
        slog.Warn("LoadQuantManifest: unknown ModelName, using v1 manifest as fallback", "model_name", modelName)
        return embeddedQuantManifestV1
    }
}
```

Update `LoadQuantManifest`:
```go
func LoadQuantManifest(cfg config.Config) (*QuantManifest, error) {
    var data []byte
    if p := strings.TrimSpace(cfg.ModelManifestPath); p != "" {
        b, err := os.ReadFile(p)
        // ...
        data = b
    } else {
        data = selectEmbeddedManifest(cfg.ModelName)
    }
    // ... parse unchanged
}
```

**Deliverable**: `internal/cli/model_fetcher.go` diff (~20 lines).

**Gate**: `TestLoadQuantManifest_EmbeddedFallback` still passes (default `cfg.ModelName=""` → v1 manifest → byte-identical parse). New test asserts v2 dispatch.

### Epic 3 — Model-aware RAM tier defaults

Extend `internal/config/config.go` so the DEFAULT `MDEMG_MODEL_RAM_TIERS` depends on `MDEMG_MODEL_NAME`:

```go
func defaultRamTiersForModel(modelName string) string {
    switch strings.ToLower(strings.TrimSpace(modelName)) {
    case "mdemg-llm-v2":
        return `{"<32":"Q4_K_M","<48":"Q5_K_M","default":"Q8_0"}`
    default:
        return `{"<16":"Q4_K_M","<24":"Q5_K_M","default":"Q8_0"}`
    }
}
```

Wire in `FromEnv()`:
```go
modelName := get("MDEMG_MODEL_NAME", "mdemg-llm-v1")
modelRamTiers := get("MDEMG_MODEL_RAM_TIERS", defaultRamTiersForModel(modelName))
```

Operator override (`MDEMG_MODEL_RAM_TIERS` explicitly set) still wins.

**Deliverable**: `internal/config/config.go` diff (~10 lines).

**Gate**: existing v1-default test (v1 default `{"<16":...}`) passes; new v2 test asserts `{"<32":...}` default; explicit override test unchanged.

### Epic 4 — Pin tests

Extend `internal/cli/model_test.go`:

- `TestLoadQuantManifest_V2_DispatchesToV2Bytes` — `cfg.ModelName="mdemg-llm-v2"` → parsed manifest has `ModelName="mdemg-llm-v2"` + all 3 quants + Q5_K_M.SHA256 matches `publish_manifest_v2.json`
- `TestLoadQuantManifest_V1Explicit_DispatchesToV1` — `cfg.ModelName="mdemg-llm-v1"` → v1 manifest (regression pin)
- `TestLoadQuantManifest_EmptyModelName_DefaultsToV1` — `cfg.ModelName=""` → v1 (backward-compat)
- `TestLoadQuantManifest_UnknownModelName_FallsBackToV1` — `cfg.ModelName="mdemg-llm-v99"` → v1 with WARN log

Add to `internal/config/config_test.go` (or wherever RAM tier defaults are tested):
- `TestFromEnv_V2ModelHas27BRamTierDefault` — with `MDEMG_MODEL_NAME=mdemg-llm-v2` unset elsewhere, `cfg.ModelRamTiers` == `{"<32":"Q4_K_M","<48":"Q5_K_M","default":"Q8_0"}`
- `TestFromEnv_ExplicitRamTiersOverridesModelDefault` — operator `MDEMG_MODEL_RAM_TIERS={"default":"Q4_K_M"}` wins regardless of ModelName

**Deliverable**: 4 model_test.go + 2 config_test.go pin tests.

**Gate**: `go test ./internal/cli/... ./internal/config/... -v -run 'TestLoadQuantManifest_V|TestFromEnv_V2\|TestFromEnv_Explicit'` all green.

### Epic 5 — Feature doc update

Extend `docs/features/local-model-distribution.md`:

- New "Model versions" section at top: v1 (production canonical since 2026-05-11, Qwen3-14B) + v2 (published 2026-08-20, Qwen3.8-27B, no adapter, base-only)
- Per-version quant tier tables (v1 already documented; add v2)
- Per-version RAM math notes
- Cross-reference `docs/development/homebrew-installer-qwen-update-001/publish_manifest_v2.json` for the definitive SHAs
- Note the multi-model `LoadQuantManifest` dispatch (operator only needs to set `MDEMG_MODEL_NAME=mdemg-llm-v2`; everything else auto-picks)

**Deliverable**: `docs/features/local-model-distribution.md` diff.

**Gate**: `python3 scripts/verify_doc_env_vars.py` clean on the new content (no invented env vars).

### Epic 6 — Live Tier 3 E2E smoke

Order:
1. Build: `go build -o bin/mdemg ./cmd/mdemg`
2. Dry-run v2 pull: `MDEMG_MODEL_NAME=mdemg-llm-v2 ./bin/mdemg model pull --quant Q5_K_M --dry-run` → prints resolved URL + expected SHA from v2 manifest
3. Verify SHA matches publish_manifest_v2.json without running the real pull
4. `./bin/mdemg model list` shows the v2 tag if already pulled (Phase A had it locally); confirm output shape
5. RAM auto-picker smoke: `MDEMG_MODEL_NAME=mdemg-llm-v2 MDEMG_MODEL_QUANT=auto ./bin/mdemg model pull --dry-run` → picks per host RAM against v2 tiers
6. If operator has bandwidth: full pull of Q4_K_M (smallest v2 tier ~16 GB), verify SHA verify PASSES (not skipped), symlink lands at `~/.mdemg/models/mdemg-llm-v2.Q4_K_M.gguf`

**Deliverable**: `sprint_post.md` §Verification with each command + output.

**Gate**: SHA verify prints `SHA verify: ok (b8456d5d96b5…)` for v2 Q5_K_M (not "skipped").

### Epic 7 — CHANGELOG + sprint post + arc rules pinned to CLAUDE.md

- `CHANGELOG.md` [Unreleased] entry
- `docs/development/homebrew-installer-qwen-update-002/sprint_post.md`
- CLAUDE.md pin the 4 arch rules disclosed in Phase A `post_a.md` (they cover the multi-phase pattern + convert-pipeline lessons; land them here since Phase C is the arc-closer)

**Deliverable**: 3 docs.

**Gate**: `verify_doc_env_vars.py` clean; sprint post has "Documents Accessed" per `end-with-docs-accessed`.

## 6. Testing Plan (3 tiers)

### Tier 1 — Unit tests

- `TestLoadQuantManifest_V2_DispatchesToV2Bytes`
- `TestLoadQuantManifest_V1Explicit_DispatchesToV1`
- `TestLoadQuantManifest_EmptyModelName_DefaultsToV1`
- `TestLoadQuantManifest_UnknownModelName_FallsBackToV1`
- `TestSelectEmbeddedManifest_CaseInsensitive` — `"MDEMG-LLM-V2"` also dispatches to v2
- `TestFromEnv_V2ModelHas27BRamTierDefault`
- `TestFromEnv_ExplicitRamTiersOverridesModelDefault`

### Tier 2 — Integration tests

- `TestLoadQuantManifest_V2_SHAsMatchPhaseAPublishManifest` — reads BOTH `internal/cli/quant_manifest_v2.json` and `docs/development/homebrew-installer-qwen-update-001/publish_manifest_v2.json` and asserts SHA equality per quant. This is the drift-detector: if either file changes without the other, the test fires.
- `TestModelPull_DryRunV2Q5_UsesV2Manifest` — invokes the model-pull dry-run code path with a mock Fetcher that captures the resolved SHA; asserts SHA matches v2 manifest.

### Tier 3 — Live e2e

- `MDEMG_MODEL_NAME=mdemg-llm-v2 ./bin/mdemg model pull --quant Q5_K_M --dry-run` → prints v2's Q5 SHA verbatim
- `MDEMG_MODEL_NAME=mdemg-llm-v2 MDEMG_MODEL_QUANT=auto ./bin/mdemg model pull --dry-run` → picks tier per host RAM against v2 defaults
- (Optional bandwidth-permitting) full `MDEMG_MODEL_NAME=mdemg-llm-v2 ./bin/mdemg model pull --quant Q4_K_M` → SHA verify PASSES (not "skipped"), symlink appears at `~/.mdemg/models/mdemg-llm-v2.Q4_K_M.gguf`
- `MDEMG_MODEL_NAME=mdemg-llm-v1 ./bin/mdemg model pull --quant Q5_K_M --dry-run` → still points at v1 manifest (regression pin — v1 unchanged)

## 7. Commit Strategy

Single feature commit (docs-adjacent code change; no reason to split). Body:
```
feat(cli): HOMEBREW-INSTALLER-QWEN-UPDATE-002 Phase C — wire v2 quant manifest

- new internal/cli/quant_manifest_v2.json with real Phase A SHAs
- LoadQuantManifest dispatches on cfg.ModelName (v1/v2/fallback)
- model-aware RAM tier defaults for 27B (v2)
- feature doc updated with v2 tiers + 27B RAM math
- 7 pin tests (dispatch + defaults + SHA-parity drift-detector)
- Tier 3 live smoke: v2 pull SHA-verifies OK against embedded manifest

Task #136; closes multi-phase arc HOMEBREW-INSTALLER-QWEN-UPDATE.
Unblocks PHASE-E4-GATE-PROMOTE-001.
```

## 8. Verification Checklist

- [ ] `internal/cli/quant_manifest_v2.json` exists, parses, SHAs match `publish_manifest_v2.json` byte-for-byte
- [ ] `go build ./...` green
- [ ] `golangci-lint run ./internal/cli/... ./internal/config/...` green
- [ ] `go test ./internal/cli/... ./internal/config/... -v` all pass, including 7 new tests
- [ ] Tier-3 dry-run smoke: v2 pull resolves v2 SHA (not v1); v1 pull resolves v1 SHA (regression pin)
- [ ] Tier-3 auto-quant smoke: v2 defaults route by 27B RAM math
- [ ] Tier-3 optional real pull: SHA verify PASSES for v2 Q4 (16 GB download acceptable)
- [ ] `docs/features/local-model-distribution.md` describes both v1 + v2 with cross-ref to `publish_manifest_v2.json`
- [ ] `scripts/verify_doc_env_vars.py` clean on the diff
- [ ] `sprint_post.md` includes "Documents Accessed"
- [ ] CHANGELOG updated
- [ ] PR comment (sprint summary) posted per `must-comment-sprint-summary-on-pr`
- [ ] Task #136 marked completed
- [ ] Arc HOMEBREW-INSTALLER-QWEN-UPDATE fully closed

## 9. Documentation Update (final epic — never cut)

Epic 5 above IS the doc update. Additionally:
- CLAUDE.md pin: the 4 arch rules disclosed in Phase A `post_a.md` (recorded there but not yet in CLAUDE.md — this sprint's PR body drops them into CLAUDE.md under the Architecture Notes section, batched with the sprint's own arc closure note)
- Sprint post updates CLAUDE.md pins per the shipping style used for HOMEBREW-INSTALLER-QWEN-UPDATE-001 Phase B

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| SHA in `quant_manifest_v2.json` diverges from `publish_manifest_v2.json` (typo, copy error) | Low | HIGH — v2 pulls would ERROR with false mismatch | Tier-2 integration test `TestLoadQuantManifest_V2_SHAsMatchPhaseAPublishManifest` reads BOTH files + asserts equality — hard drift detector |
| v1 pull path silently regresses (bytes different from Phase B state) | Low | HIGH — customer break | `TestLoadQuantManifest_EmbeddedFallback` + `TestLoadQuantManifest_V1Explicit_DispatchesToV1` pin v1's byte-identical parse |
| Operator's existing `MDEMG_MODEL_RAM_TIERS` override stops working after model-aware defaults | Low | MEDIUM | `TestFromEnv_ExplicitRamTiersOverridesModelDefault` pins override precedence; operator override is checked BEFORE the model-aware default |
| 27B RAM math is wrong (e.g. `min_ram_gb=20` is too optimistic for Q4) | Low | MEDIUM | Cross-reference `llama.cpp`'s docs + Phase A live experience running Q4/Q5 on the operator's M5 Max; conservative rounding up |
| CI Lint fails again on unrelated file (like PR #643's staticcheck surprise) | Low | LOW | Not this sprint's concern; task #137 CI-GOLANGCI-PIN-001 addresses the root cause |
| `//go:embed` doesn't pick up the new file | Very low | HIGH | `go build` will fail with clear error; caught immediately in Epic 1 gate |

## 11. Non-Goals (explicit — deferred to future sprints)

- **`mdemg model use <name>` shorthand CLI** — deferred. Value: collapses env-var edit + `launchctl kickstart` into one command. Scope: modest (env-file writer + kickstart runner) but deserves its own sprint per `sequential-epics`. Filed as future task.
- **v2 adapter distribution** — v2 ships raw base per operator scope decision (task #134 clarifying answer 2026-08-19). Adapter training + publish is a separate arc (PHASE-E3-RETRAIN-BENCHMARK-001 → E4).
- **Windows/Linux v2 support** — Apple Silicon-only continues per MODEL-DIST-001 line (operator-flagged as low priority in `project_scoop_low_priority.md`).
- **Re-publish v2 from a fixed llama.cpp converter** — deferred to optional Phase D once llama.cpp lands the Qwen3.5 arch fix (documented in `post_a.md` §Follow-ups).
- **UBENCH re-run of v2 vs v1** — task #91 bake-off already showed v2 = 0.9105 vs v1 = 0.8047; no need to re-verify at this sprint. If operator wants a fresh benchmark, ship as its own sprint.

## 12. Documents Accessed

- `internal/cli/quant_manifest.json` (v1 shape reference)
- `internal/cli/model_fetcher.go` (LoadQuantManifest + QuantManifest struct)
- `internal/cli/model.go:307-349` (SHA verify block from Phase B)
- `internal/cli/model_test.go` (existing test patterns)
- `internal/config/config.go:68-75, 2235-2237, 5993-5996` (model config fields + defaults)
- `docs/development/homebrew-installer-qwen-update-001/publish_manifest_v2.json` (Phase A hand-off SHAs)
- `docs/development/homebrew-installer-qwen-update-001/post_a.md` (arc rules + follow-ups)
- `docs/development/homebrew-installer-qwen-update-001/sprint_post.md` (Phase B ship state)
- `docs/features/local-model-distribution.md` (feature doc to extend)
- `CLAUDE.md` (MODEL-DIST-001/002 + HOMEBREW-INSTALLER-QWEN-UPDATE-001 pins)
- Live `curl https://ollama.com/reh3376/mdemg-llm-v2:{Q4_K_M,Q5_K_M,Q8_0}` verifications (Phase A already confirmed all 200; not re-verified)
- Sprint plan template + `must-follow-12-section-format` skill
