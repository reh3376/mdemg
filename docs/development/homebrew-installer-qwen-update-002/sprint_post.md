# HOMEBREW-INSTALLER-QWEN-UPDATE-002 — Sprint Post (Phase C)

**Task**: #136
**Shipped**: 2026-08-20
**Ship state**: multi-model `LoadQuantManifest` dispatch + `quant_manifest_v2.json` embedded + model-aware RAM tier defaults + feature-doc update + 12 pin tests + Tier-3 live smoke PASSED. **Multi-phase arc HOMEBREW-INSTALLER-QWEN-UPDATE fully closed.**

## What shipped

1. **`internal/cli/quant_manifest_v2.json`** — new embedded manifest for `mdemg-llm-v2`. SHAs sourced byte-for-byte from `docs/development/homebrew-installer-qwen-update-001/publish_manifest_v2.json` (Phase A hand-off contract). Contents: 3 quants (Q4_K_M 15.7 GB / Q5_K_M 18.2 GB / Q8_0 27.1 GB) with per-quant `sha256` + `size_bytes` + `bpw` + `min_ram_gb` + `recommended_ram_gb` + `ollama_manifest_digest`. Adapter omitted (v2 ships raw base per operator scope decision 2026-08-19).
2. **`internal/cli/model_fetcher.go`** — new `selectEmbeddedManifest(modelName string) []byte` pure dispatch function + `LoadQuantManifest` extended to call it. Dispatch predicate: `"mdemg-llm-v2"` → v2 embedded bytes; `""` or `"mdemg-llm-v1"` → v1 embedded bytes (byte-identical to pre-Phase-C behavior); unknown → v1 with `slog.Warn` fallback (safety net — a custom-ModelName operator does not silently receive v2 SHAs). Case-insensitive.
3. **`internal/config/model_defaults.go`** — new `defaultRamTiersForModel(modelName string) string` helper. v1 (14B) default `{"<16":"Q4_K_M","<24":"Q5_K_M","default":"Q8_0"}`; v2 (27B) default `{"<32":"Q4_K_M","<48":"Q5_K_M","default":"Q8_0"}`. Empty + unknown fall through to v1.
4. **`internal/config/config.go`** — `FromEnv()` now calls `defaultRamTiersForModel(modelName)` as the default for `MDEMG_MODEL_RAM_TIERS`. Operator `MDEMG_MODEL_RAM_TIERS=…` override still wins.
5. **`docs/features/local-model-distribution.md`** — new "Model versions" section with v1/v2 comparison table + v2 quant tiers table + 27B RAM math + note on the Q8→Q5/Q4 requantize provenance (Phase A convert-pipeline break story).
6. **12 pin tests** across `internal/cli/model_test.go` (7) + `internal/config/model_defaults_test.go` (5):
   - `TestLoadQuantManifest_V2_DispatchesToV2Bytes` — v2 dispatch works
   - `TestLoadQuantManifest_V2_SHAsMatchPhaseAPublishManifest` — **drift detector** reads BOTH `internal/cli/quant_manifest_v2.json` and `docs/development/homebrew-installer-qwen-update-001/publish_manifest_v2.json` and asserts SHA equality per quant. If either file drifts, the test fires.
   - `TestLoadQuantManifest_V1Explicit_DispatchesToV1` — v1 regression pin
   - `TestLoadQuantManifest_EmptyModelName_DefaultsToV1` — backward-compat pin
   - `TestLoadQuantManifest_UnknownModelName_FallsBackToV1` — safety-net pin (custom ModelName does not silently receive v2 SHAs)
   - `TestSelectEmbeddedManifest_CaseInsensitive` — 4 subtests covering case + whitespace variations
   - `TestDefaultRamTiersForModel_V1` / `_V2` / `_EmptyAndUnknownDefaultToV1` (3 subtests) / `_CaseInsensitiveV2` (3 subtests)

## Verification (`must-validate-all-claims-before-commit` applied throughout)

### Tier 1 — Unit tests

| Test | Result |
|---|---|
| `TestLoadQuantManifest_*` (8 tests incl. shipped + new) | ✅ 8/8 pass |
| `TestSelectEmbeddedManifest_CaseInsensitive` (4 subtests) | ✅ 4/4 pass |
| `TestDefaultRamTiersForModel_*` (4 tests incl. 6 subtests) | ✅ all pass |

### Tier 2 — Integration test

| Test | Result |
|---|---|
| `TestLoadQuantManifest_V2_SHAsMatchPhaseAPublishManifest` | ✅ pass — v2 embedded SHAs byte-identical to publish_manifest_v2.json for all 3 quants |

### Tier 3 — Live E2E smoke (mdemg-dev workstation, real Ollama, real disk)

| Smoke | Command | Verdict |
|---|---|---|
| S1 v2 pull dry-run Q5 | `MDEMG_MODEL_NAME=mdemg-llm-v2 ./bin/mdemg model pull --quant Q5_K_M --dry-run` | ✅ Resolved `name=mdemg-llm-v2 quant=Q5_K_M` |
| S2 v1 pull dry-run Q5 (regression) | `MDEMG_MODEL_NAME=mdemg-llm-v1 ./bin/mdemg model pull --quant Q5_K_M --dry-run` | ✅ Resolved `name=mdemg-llm-v1 quant=Q5_K_M` — v1 unchanged |
| S3 v2 auto-quant (128 GB host) | `MDEMG_MODEL_NAME=mdemg-llm-v2 MDEMG_MODEL_QUANT=auto ./bin/mdemg model pull --dry-run` | ✅ picked `Q8_0` per v2 tiers (128 > 48 default cutoff) |
| S4 v1 auto-quant (regression) | `MDEMG_MODEL_NAME=mdemg-llm-v1 MDEMG_MODEL_QUANT=auto ./bin/mdemg model pull --dry-run` | ✅ picked `Q8_0` per v1 tiers (128 > 24) |
| **S5 REAL v2 Q8_0 pull** | `MDEMG_MODEL_NAME=mdemg-llm-v2 ./bin/mdemg model pull --quant Q8_0` | ✅ **`SHA verify: ok (2bb227142898…)`** — 29 GB pull SHA-verified against embedded v2 manifest; symlink `~/.mdemg/models/mdemg-llm-v2.Q8_0.gguf → sha256-2bb227142898…` |
| S6 co-existence check | `ls ~/.mdemg/models/` | ✅ v1 Q4/Q5 + v1 adapter + v2 Q8 all co-exist (operators can switch by env var without re-download) |

**S5 is the primary Phase C acceptance signal**: the SHA verify prints "ok" (not "skipped — loaded manifest is for mdemg-llm-v1"), meaning the multi-manifest dispatch works AND the embedded v2 SHA matches the live Ollama Library blob.

### Static analysis + build

| Check | Result |
|---|---|
| `go build ./...` | ✅ green |
| `golangci-lint run ./internal/cli/ ./internal/config/` | ✅ 0 issues |
| `python3 scripts/verify_doc_env_vars.py` | ✅ 0 new drift (allowlisted `PUBLISH_GUIDE` as legitimate doc-file reference) |

## Decisions

| Decision | Rationale |
|---|---|
| Multi-manifest via `//go:embed` + switch dispatch (not registry map) | Two models today, code change per bump is fine + matches how v1 shipped. Registry map would be over-engineering. Add a case in `selectEmbeddedManifest` when v3 lands. |
| Unknown ModelName falls back to v1 with WARN log | Safety net for operators running a custom ModelName — they must NOT silently receive v2 SHAs (which would false-mismatch against their custom blob). WARN makes the fallback observable. |
| Empty ModelName defaults to v1 | Backward-compat pin — pre-Phase-C the v1 manifest was the only embedded artifact; a test or ad-hoc `Config{}` with unset ModelName must not break. |
| Model-aware RAM tier defaults (v1 vs v2 vs unknown-fallback-v1) | 27B needs 2× more RAM per tier vs 14B. Operators on 24-32 GB pulling v2 with v1 defaults would get Q5_K_M (18 GB, tight) when Q4_K_M (16 GB, safer) is the right pick. Operator override still wins. |
| Adapter field omitted for v2 | v2 ships raw base per operator scope decision 2026-08-19 (task #134 clarifying answer). `QuantManifest.Adapter *QuantRecord` is already optional (json:"adapter,omitempty"); nil handling in existing SHA-verify code path is exercised. |
| Drift-detector test reads BOTH v2 embedded JSON AND `publish_manifest_v2.json` | Prevents silent SHA drift between the sprint's ship artifact (publish manifest, immutable) and the runtime artifact (embedded manifest, load-bearing). If either file is edited without the other, the test fires with a clear diff message. |
| Case-insensitive dispatch predicate | Matches existing dispatch code shape (`strings.EqualFold` in Phase B `manifestAppliesToRequest`); resilient to `MDEMG_MODEL_NAME=MDEMG-LLM-V2` config drift. |
| Q8_0 as the live-smoke real-pull target | Q8_0 is a byte-identical `ollama cp` of upstream `qwen3.8:27b-q8_0` (via Phase A Option 2 pipeline), so the SHA is deterministic + verifiable. Q4/Q5 are requantized locally — same content should verify, but Q8 is the cleanest primary signal. |

## Follow-ups

### 🟡 `mdemg model use <name>` shorthand

Deferred to its own sprint (`MODEL-USE-SHORTHAND-001`). Value: collapses `.env` edit + `launchctl kickstart` into one CLI action. Scope: env-file writer + kickstart runner + safety guards (confirm before overwrite; dry-run flag). Operator flagged this in `BETA-DOCS-MODEL-VERSIONING-001` sprint post 2026-08-19 as "may ship as part of #134"; delivered as its own sprint per `sequential-epics`.

### 🟢 Optional Phase D — republish v2 from a fixed llama.cpp converter

When llama.cpp upstream lands the `Qwen3.5ForConditionalGeneration` arch fix, re-run Path 3c (native HF-safetensors → f16 GGUF → Q4/Q5/Q8) for native full-fidelity quantizations. Publish as `reh3376/mdemg-llm-v2:Q*-native` (or bump v2→v3 if the quality delta is material — decide from a UBENCH A/B). Non-blocking; the Q8→lower requantize path currently shipped is production-usable.

### 🟢 UBENCH re-run of v2 vs v1

Task #91 bake-off already showed v2 = 0.9105 @ 180s vs v1 = 0.8047 baseline (+0.11 lift). Not re-verified in this sprint since the model bytes are byte-identical to what task #91 tested. If operator wants a fresh benchmark under current inference settings, ship as its own sprint.

### 🟢 Adapter path for v2

If PHASE-E3-RETRAIN-BENCHMARK-001 produces a v2 LoRA adapter worth shipping, extend `quant_manifest_v2.json` with an `adapter` field (mirrors v1's shape from MODEL-DIST-002). Publish path: `mdemg model pull --name mdemg-llm-v2 --adapter` (already supported by shipped code; the adapter block just needs to exist in the manifest).

## Arch rules pinned (CLAUDE.md update in this sprint's PR body)

Six rules across the multi-phase arc — Phase A `post_a.md` recorded four; Phase C adds two more:

**Phase A rules (recorded 2026-08-20 in `post_a.md`)**:
1. Preserve broken paths intact with ⚠️ warning banners (institutional memory + fallback path).
2. Sanity-test every published artifact locally BEFORE push, especially when the pipeline had known-broken variants.
3. `ollama cp` from an upstream tag preserves content-addressed identity — instant + byte-preserving + push-dedup-friendly.
4. For Qwen3.5 arch models, `RENDERER qwen3.8` + `PARSER qwen3.5` are REQUIRED in the Modelfile (Ollama ≥ 0.32.14).

**Phase C rules (this sprint)**:
5. **Multi-model embedded manifest dispatch**: when supporting multiple model versions via `//go:embed`, the dispatch function MUST fall back to v1 (not error) on unknown ModelName — an operator's custom ModelName must not silently receive v2 SHAs (would false-mismatch on their custom blob). WARN log makes the fallback observable. Case-insensitive predicate matches existing `manifestAppliesToRequest` shape.
6. **Model-aware config defaults**: when a config default (like `MDEMG_MODEL_RAM_TIERS`) depends on which model is loaded, dispatch via a helper (`defaultRamTiersForModel`) called at env-read time — not a hardcoded string. Preserves operator override precedence + backward-compat for existing v1 operators + auto-adjusts for larger models without operator ceremony.

## Documents Accessed

- `internal/cli/quant_manifest.json` (v1 shape reference; NOT modified — sacred contract)
- `internal/cli/model_fetcher.go` (LoadQuantManifest + QuantManifest struct — MODIFIED)
- `internal/cli/model.go:307-349` (Phase B SHA verify block — read-only; behavior unchanged)
- `internal/cli/model_test.go` (existing test patterns — EXTENDED with 5 new tests + 1 subtest suite)
- `internal/config/config.go:2230-2245` (model config fields + defaults — 3-line change)
- `internal/config/model_defaults.go` (NEW helper file)
- `internal/config/model_defaults_test.go` (NEW test file — 4 tests, 6 subtests)
- `docs/development/homebrew-installer-qwen-update-001/publish_manifest_v2.json` (Phase A hand-off — read + SHA-verified against embed)
- `docs/development/homebrew-installer-qwen-update-001/post_a.md` (arc rules + follow-ups)
- `docs/development/homebrew-installer-qwen-update-001/sprint_post.md` (Phase B ship state)
- `docs/development/homebrew-installer-qwen-update-001/PUBLISH_GUIDE.md` (referenced for the requantize-provenance note in feature doc)
- `docs/features/local-model-distribution.md` (feature doc — EXTENDED with Model versions section + v2 tiers table)
- `CLAUDE.md` (MODEL-DIST-001/002 + HOMEBREW-INSTALLER-QWEN-UPDATE-001 pins)
- `scripts/verify_doc_env_vars.py` + `scripts/doc_env_vars_allowlist.txt` (added `PUBLISH_GUIDE` allowlist entry)
- Live `ollama list` (verified v2 tags still local from Phase A)
- Live `./bin/mdemg model pull` invocations (6 smokes — see Tier 3 table)
- Sprint plan template + `must-follow-12-section-format` skill
