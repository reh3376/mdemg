# Sprint NEURAL-RERANK-QUALITY-AB-001 — UVTS A/B: openai vs neural rerank

## 1. Header & Metadata
- **Sprint ID:** NEURAL-RERANK-QUALITY-AB-001
- **Sprint line:** `docs/development/neural-rerank-quality-ab-001/`
- **Date opened:** 2026-07-20
- **Target version:** v0.11.7 (patch — default flip iff A/B verdict permits; no schema changes)
- **Estimated effort:** ~1 dev-day (2× 120q UVTS runs at ~30-60 min each on quiet system + compare + docs)
- **OpenAI spend:** $0 (retrieval hits local llama-server for openai path; local sidecar for neural)
- **Risk level:** Low — data-decided sprint; either flip on evidence OR no code change

## 2. Problem Statement
NEURAL-RERANK-PRECHECK-001 E3 measured `RERANK_PROVIDER=neural` completing rerank in **122ms vs 2588ms for openai (21× faster)** on the same live retrieve. That latency win is compelling — IF the neural cross-encoder produces at least equivalent retrieval quality on the UVTS corpus. Data-decide via A/B before flipping the default.

## 3. Scope & Constraints

### In scope
- Full 120q UVTS A/B against `docs/tests/uvts/specs/lnl_demo_validation.uvts.json` — baseline `RERANK_PROVIDER=openai`, candidate `RERANK_PROVIDER=neural`.
- Apply the Note 02 merge gate (from RETRIEVAL-TYPED-EDGES-002 pattern):
  - **Pass:** candidate mean ≥ baseline mean AND no per-question regression > `ab_mode.regression_threshold_per_question` (default 0.10).
  - **Fail:** either mean or regression gate trips.
- If PASS: flip `RerankProvider` default from `openai` → `neural` in `internal/config/config.go`, and any compose template / `.env.example` that mirrors it.
- If FAIL: no code change; sprint proceeds to docs recording the finding and the reason for keeping openai as default.
- Restore `RERANK_PROVIDER=openai` in `.env` post-A/B regardless of verdict (leave the live substrate unchanged from operator's config).
- Canonical docs record the verdict + ab_verdict.json artifact.

### Out of scope
- Retuning either provider (openai vs neural). This sprint measures as-is.
- Changing rerank prompts / neural model.
- UVTS spec authoring.
- Grafana dashboards.
- Skip-rate gauge (still-open follow-up from LLM-HEALTH-INVESTIGATION-001).

### Constraints
- Sequential epics.
- **Live Tier-3 required** — every measured number comes from a real UVTS run on real services.
- No hardcoded literals beyond the ontology (`"openai"`, `"neural"` provider strings, matching existing switch).
- RRF-SCALE-001-safe (this measures score-scale independent UVTS grades, not raw RRF scores).
- The A/B runs on `mdemg-dev` — the live substrate stays untouched between runs (only `.env`'s `RERANK_PROVIDER` flips + server restart).
- Quiet-system requirement: don't kick off consolidation / heavy ingest during the A/B runs (per RETRIEVAL-TYPED-EDGES-002's "load-fragility" lesson).

## 4. Dependencies
- **NEURAL-RERANK-PRECHECK-001** (merged as PR #511) — verified both providers work end-to-end with correct budget preflight.
- Neural sidecar UP at `:8100` (verified pre-sprint: `cross-encoder/ms-marco-MiniLM-L-6-v2` + `cross-encoder/nli-MiniLM2-L6-H768` loaded).
- Existing UVTS harness (`make test-uvts-full`, `uvts_ab_compare.py`).

## 5. Implementation Plan

### Epic 0 — Sprint plan committed
This document.

### Epic 1 — UVTS baseline (openai)
1. Confirm `.env`: `RERANK_PROVIDER=openai` (current default).
2. Restart mdemg via `launchctl kickstart -k`.
3. Run 120q: `python3 docs/tests/uvts/runners/uvts_runner.py --spec docs/tests/uvts/specs/lnl_demo_validation.uvts.json --base-url http://localhost:9999 --profile full --output-dir /tmp/uvts-ab/baseline-openai --report /tmp/uvts-ab/baseline-openai-report.json`.
4. Capture: `/tmp/uvts-ab/baseline-openai/grades.json` + report.

### Epic 2 — UVTS candidate (neural)
1. `.env`: swap to `RERANK_PROVIDER=neural`.
2. Restart mdemg.
3. Same runner + same spec + `--output-dir /tmp/uvts-ab/candidate-neural`.
4. Capture grades.json + report.

### Epic 3 — Compare + decision
1. `python3 docs/tests/uvts/runners/uvts_ab_compare.py --baseline /tmp/uvts-ab/baseline-openai/grades.json --candidate /tmp/uvts-ab/candidate-neural/grades.json --spec docs/tests/uvts/specs/lnl_demo_validation.uvts.json --out /tmp/uvts-ab/verdict.json`.
2. Read the verdict:
   - exit 0 (PASS: mean ≥ + no regression > threshold) → **flip default**.
   - exit 1 (FAIL) → **keep openai** default.
   - exit 2 (DRIFT: mean ok but per-q regressions) → **keep openai** default; record disposition.
3. Copy `verdict.json` to `docs/development/neural-rerank-quality-ab-001/ab_verdict.json`.
4. **Revert `.env` to `RERANK_PROVIDER=openai` regardless of verdict** (E4 handles the code-default change; the live `.env` override belongs to the operator).

### Epic 4 — Implement default (conditional)
- **If E3 verdict = flip:**
  - `internal/config/config.go`: change `RerankProvider` default in the `FromEnv` `get()` call from `"openai"` to `"neural"`.
  - `internal/cli/compose_templates/docker-compose.yml`: update the `RERANK_PROVIDER=` env line if present.
  - `deploy/docker/docker-compose.prod.yml`: same if present.
  - `.env.example` / init template: same.
- **If E3 verdict = keep:** no code change; record the finding in docs.

### Epic 5 — Live re-verify (conditional) + docs
- **If flipped:**
  - Restart with `.env` still holding `RERANK_PROVIDER=openai` (operator explicit override); verify openai path still works.
  - Remove the `.env` override line; restart; verify the new default (`neural`) is live via a retrieve returning `rerank_ms < 500ms` (neural's typical latency).
- **In all cases:** CLAUDE.md architecture note; CHANGELOG entry; `docs/features/*rerank*` update; `post.md` with the A/B evidence and verdict rationale.

## 6. Testing (3 tiers)
- **Tier 1** — no new unit tests (measured behavior, not new logic).
- **Tier 2** — existing rerank_budget_test.go continues to pass (delegating helper).
- **Tier 3** — the A/B run IS the Tier 3 evidence; 120q × 2 = 240 live retrieves.

## 7. Commit Strategy
Sequential commits on `reh3376_dev01`:
1. `docs(neural-rerank-quality-ab-001): E0 — sprint plan`
2. `docs(neural-rerank-quality-ab-001): E1 — UVTS baseline (openai) grades captured`
3. `docs(neural-rerank-quality-ab-001): E2 — UVTS candidate (neural) grades captured`
4. `docs(neural-rerank-quality-ab-001): E3 — A/B compare + verdict`
5. `feat(neural-rerank-quality-ab-001): E4 — default RerankProvider → neural` (only if verdict=flip)
6. `docs(neural-rerank-quality-ab-001): E5 — CLAUDE.md/CHANGELOG/feature/post`

## 8. Verification Checklist
- [ ] E0 committed
- [ ] Baseline (openai) 120q grades captured
- [ ] Candidate (neural) 120q grades captured
- [ ] `ab_verdict.json` committed as sprint artifact
- [ ] Verdict decision (flip vs keep) reasoned in the post-mortem
- [ ] `.env` reverted to `RERANK_PROVIDER=openai` post-run
- [ ] If flip: config.go + compose defaults updated
- [ ] If flip: live re-verify with new default active
- [ ] `go build ./...` clean
- [ ] `go test ./...` full-suite green
- [ ] `golangci-lint run` clean
- [ ] CLAUDE.md note
- [ ] CHANGELOG entry
- [ ] `post.md` with A/B evidence table

## 9. Documentation Update — Epic 5.

## 10. Risks & Mitigations
| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Neural quality noticeably lower than openai (mean regression) | Medium | Low (data-decided) | Sprint's purpose IS to decide; a FAIL result means "keep openai default", NOT a bug |
| Per-question regressions on specific query classes even with mean parity | Medium | Low | Note 02 gate captures this; the ab_verdict.json breaks down per-question |
| Neural sidecar goes down during the A/B run | Very Low | Low | Pre-check verified sidecar `:8100/health` = ok pre-sprint; if it drops mid-run, restart + re-run E2 |
| Load contention with consolidation degrades neural results unfairly | Medium | Medium | Run during quiet window; check `Neo4j CPU` before starting; the retrieval-quality gauge should be stable |
| Cache-hit inflates neural or openai results (different provider = different cache key?) | Low | Medium | Rerank result IS the response; scorer version cache key already accounts for RRF params but NOT for RerankProvider — check if cache_hit interferes; if it does, add a cache-key knob (out of scope) or run with cache disabled |
| Operator has strong RerankModel-config preference that changes verdict | Low | Low | The A/B measures the AS-IS defaults for both providers; deviations from default should be re-tested |

## 11. Documents Accessed
- `internal/retrieval/rerank.go::Rerank` (dispatch switch + E2/E1 pre-check)
- `internal/retrieval/rerank_neural.go` (neural sidecar client)
- `internal/config/config.go::RerankProvider` default definition
- `docs/tests/uvts/runners/uvts_runner.py` (harness)
- `docs/tests/uvts/runners/uvts_ab_compare.py` (verdict tool)
- `docs/tests/uvts/specs/lnl_demo_validation.uvts.json` (spec)
- Neural sidecar live health: `curl :8100/health` → `models_loaded: [cross-encoder/ms-marco-MiniLM-L-6-v2, cross-encoder/nli-MiniLM2-L6-H768]`
- NEURAL-RERANK-PRECHECK-001 live evidence (rerank_ms=122 neural vs 2588 openai on same query)

## 12. Rollback Procedures
- **If verdict=flip lands on main and proves problematic in production:** revert the config.go default change; operators keep working via explicit `.env` override.
- The A/B artifact (`ab_verdict.json`) is permanent evidence; a future re-A/B can either confirm or contradict.
- If the A/B run itself fails midway: no rollback needed — nothing was committed, `.env` still holds `RERANK_PROVIDER=openai` on any resume.

## Acceptance Criteria
1. Baseline (openai) + candidate (neural) 120q UVTS grades captured.
2. `ab_verdict.json` committed under the sprint dir.
3. Decision (flip vs keep) is data-driven per Note 02 gate; rationale documented in `post.md`.
4. If flip: default lands in config.go + compose templates + live-verified.
5. `.env` restored to explicit `RERANK_PROVIDER=openai` post-A/B (operator's explicit override).
6. Full test suite green; lint clean.
7. Canonical docs updated.
