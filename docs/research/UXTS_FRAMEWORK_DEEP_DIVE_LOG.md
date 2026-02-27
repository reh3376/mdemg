# UxTS Framework Deep Dive Log

Last updated: 2026-02-27 05:39:56 EST
Owner: Codex review session
Purpose: Persistent, compaction-safe working log for UxTS framework review, threat/gap analysis, and portable agent specification design.

## Cadence Trigger
- Review cadence: every 120 seconds (target)
- At each cadence: summarize new findings, update relationship map, record newly identified risks/gaps, and track open questions.

## Scope
- Primary targets: UPTS (Universal Parser Test Specification), UATS (Universal API Test Specification)
- Extended UxTS family: UOTS, UBTS, USTS, UDTS, UETS, UVTS, UAMS, UNTS governance integration

## Checkpoints

### 2026-02-26 17:12:35 EST — Session Start
- User requested deep documentation review and planning for comprehensive UxTS vulnerability/gap analysis.
- Initial inventory captured via repository-wide search for UxTS artifacts.
- Next step: read canonical governance/matrix and framework READMEs/schemas before scoring gaps.

### 2026-02-26 17:13:17 EST — Checkpoint 1 (Inventory + Governance Baseline)
- Completed framework inventory and governance baseline review.
- Confirmed broad UxTS family coverage: UPTS, UATS, UDTS, UOTS, UOBS, UBTS, USTS, UAMS, UVTS, UETS, plus UNTS hash governance.
- Key immediate signal: governance and matrix docs already acknowledge overlap and runner/CI gaps (notably UOBS/UOTS convergence, UVTS spec-only state, and uneven CI gating).
- Next step: deep review of each framework README + schema + runner references to identify concrete specification vulnerabilities and implementation gaps.

### 2026-02-26 17:14:12 EST — Checkpoint 2 (Schema + Runner Surface)
- Reviewed schemas for UATS, UPTS, UDTS, UOTS, UOBS, UBTS, USTS, UAMS, UETS, UVTS.
- High-signal mismatch confirmed: documentation maturity states do not consistently match implementation state (example: UNTS marked not started in spec doc while code artifacts and changelog indicate implementation exists).
- Noted architectural overlap risk: both UOBS and UOTS define observability validation with different schema models and runner expectations.
- Runner-level review in progress to confirm which schema features are actually enforced vs documented only.

### 2026-02-26 17:18:27 EST — Checkpoint 3 (Hard Mismatch Findings)
- UPTS schema/spec divergence is significant: 9 parser languages in active specs are not allowed by schema enum; multiple active symbol types/patterns are also schema-invalid.
- UDTS contains mixed incompatible spec dialects: 4/11 specs do not satisfy current UDTS schema required fields (`service`, `method`, `expected`).
- UVTS has split dialects: one spec (`lnl_demo_validation`) matches schema; one (`dynamic_emergence_quality.phase103`) uses a different structure and crashes the UVTS runner.
- UOBS logging test path is effectively a no-op pass (`0/0` checks) and can pass even with unreachable base URL.
- UATS and UPTS runners do not enforce many schema-defined fields (e.g., setup/teardown in UATS; relationships/strict symbol governance fields in UPTS).

### 2026-02-26 17:20:55 EST — Checkpoint 4 (Planning Artifacts Completed)
- Added consolidated findings and risk-ranked gap analysis document: `docs/research/UXTS_FRAMEWORK_GAP_ASSESSMENT_20260226.md`.
- Added portable coding-agent implementation specification for cross-repo UxTS governance: `docs/specs/UXTS_PORTABLE_AGENT_SPEC.md`.
- Embedded two explicit planning phases in the assessment: (1) deep-dive vulnerability/gap program and (2) portable implementation program.
- Next action: execute the deep-dive workstreams in priority order and convert recommendations into tracked remediation tasks.

### 2026-02-26 17:27:48 EST — Checkpoint 5 (Priority Remediation Applied)
- Completed Priority 1 remediation: UPTS schema now covers active spec enum usage; added `scripts/verify_upts_schema_parity.py` and wired it to Makefile + parser CI workflow (27 specs, 30 languages, 33 symbol types, 25 patterns verified).
- Began Priority 2 remediation via dialect separation: non-canonical UDTS and UVTS draft specs moved out of canonical `specs/` paths into `drafts/`.
- Hardened UVTS runner to fail cleanly on legacy draft shape with explicit guidance (no traceback crash path).
- Eliminated UOBS false-pass for logging tests: runner now returns explicit failure for unimplemented logging validation.

### 2026-02-26 17:29:49 EST — Checkpoint 6 (Remediation Continuation)
- Validated canonical/draft split status on UDTS and UVTS directories after migration.
- Began wiring automated guardrails so canonical `specs/` directories only contain schema-conforming dialects while drafts remain versioned under `drafts/`.
- Began observability overlap remediation planning by comparing UOBS runner capabilities vs UOTS schema/runtime expectations.

### 2026-02-26 17:34:49 EST — Checkpoint 7 (Priority 2 + Priority 3 Progress)
- Added `scripts/verify_uxts_canonical_specs.py` to enforce canonical UDTS/UVTS dialect shape in `specs/` and block legacy dialect keys there.
- Added focused CI workflow `.github/workflows/uxts-canonical-specs.yml` to enforce canonical UDTS/UVTS split on PR/push changes.
- Implemented UOTS runtime harness at `docs/api/api-spec/uots/runners/uots_runner.py` with active checks for `prometheus_metrics`, `grafana_dashboard`, and `alert_rules`.
- Added explicit UOBS/UOTS framework boundary in READMEs to reduce overlap ambiguity while broader convergence governance remains pending.

### 2026-02-26 17:35:38 EST — Checkpoint 8 (Validation Pass)
- `python3 scripts/verify_uxts_canonical_specs.py` passed: UDTS canonical/draft and UVTS canonical/draft splits are consistent.
- `make verify-uxts-canonical` passed.
- `make test-uots` passed across all current UOTS specs (5/5 pass) and generated `/tmp/uots-report.json`.

### 2026-02-26 17:38:54 EST — Checkpoint 9 (Priority 4 UATS Hardening)
- UATS runner now supports active schema fields used in current specs: `config.response_time_max_ms`, `config.follow_redirects`, and `variants[].variables`.
- UATS loader now hard-fails when specs declare unimplemented schema features to avoid silent false confidence (`setup`, `teardown`, `chain`, `request.body_file`, `expected.body_file`, `expected.body_schema`, OAuth2, and others listed in runner errors).
- Validation confirmed on live suite: `make test-api` passed with 116 specs / 198 variants and 0 failures.

### 2026-02-26 17:40:01 EST — Checkpoint 10 (UOTS Spec Stability Fix)
- During UOTS runner validation, `prometheus_neo4j_container.uots.json` intermittently failed because CPU percent can exceed 100 on multi-core hosts.
- Updated spec assertion for `mdemg_neo4j_container_cpu_percent` to remove brittle `max: 100` cap and keep `min: 0`.
- Re-ran `make test-uots`: 5/5 specs passing.

### 2026-02-26 17:42:38 EST — Checkpoint 11 (Portable Spec Fixture Governance)
- Added fixture governance requirements to `docs/specs/UXTS_PORTABLE_AGENT_SPEC.md` (version bumped to `1.0.1-draft`).
- Fixture coverage now explicit across core principles, canonical framework metadata, repository layout, schema-runner compatibility, UNTS hash-scanner expectations, threat model, and UATS/UPTS template controls.

### 2026-02-26 18:11:38 EST — Checkpoint 12 (Remediation Plan Review in `.claude/plans`)
- Located user-authored remediation plan at `/Users/reh3376/.claude/plans/prancy-cuddling-scone.md`; verified explicit mapping coverage for all 12 original findings plus 2 additional findings (#13 UBTS assertions, #14 UVTS duplicate draft copy).
- Plan quality is strong on scope coverage and sequencing, but review flagged several execution-level risks: ambiguous UBTS p99 baseline strategy, possible false-confidence via SKIP/WARN handling for unsupported controls, fixture governance treated as mostly UAMS-local instead of cross-framework policy, and drift checker scope focused on count mismatches only.
- Next step: convert review findings into concrete remediation edits (tightened acceptance criteria, hard-fail policy where needed, explicit fixture governance checks, and stronger automation coverage beyond doc counts).

### 2026-02-27 04:57:43 EST — Checkpoint 13 (Portable Agent Spec Review: Why/How/Where Completeness)
- Reviewed `docs/specs/UXTS_PORTABLE_AGENT_SPEC.md` as a bootstrap playbook for coding agents starting in new repositories.
- High-priority clarity gaps identified: lifecycle/status model inconsistency (`block` used as both status and gate mode), transition criteria conflict (`>=95%` parity vs "full parity"), and missing canonical hash procedure (no rule for excluding in-file hash field from digest input / canonicalization strategy).
- Additional gaps: missing standardized runner report schema, no deterministic discovery/grep command procedure, and no explicit non-GitHub CI mapping guidance despite "arbitrary codebases" target.
- File-path usability note: user-requested path `docs/UXTS_PORTABLE_AGENT_SPEC.md` does not exist; canonical document is under `docs/specs/`.

### 2026-02-27 05:07:33 EST — Checkpoint 14 (Post-Update Spec Re-Review)
- Re-reviewed updated `docs/specs/UXTS_PORTABLE_AGENT_SPEC.md` (v2.1.0-draft). Prior high-risk gaps were largely addressed: status/gate separation is now explicit, parity threshold conflict removed, hash procedure is specified, CI portability expanded, deterministic discovery improved, and canonical runner report schema added.
- Residual issues remain: (1) contradiction between hash bypass behavior ("continue execution") and report semantics that treat bypass as `skip`, and (2) bootstrap guidance still says embed hash in `config` section, which conflicts with per-framework hash-path convention defined elsewhere.
- Recommendation: resolve those two semantics-level mismatches to avoid divergent agent implementations.

### 2026-02-27 05:20:20 EST — Checkpoint 15 (Spec v2.2.0 Hash-Intent Re-Review)
- Reviewed `docs/specs/UXTS_PORTABLE_AGENT_SPEC.md` v2.2.0-draft after user clarified hash purpose as change-detection/review alert.
- Major improvements confirmed: hash intent now explicit; runner behavior states "always verify, always execute"; report schema separates assertion outcomes from integrity outcomes; bootstrap hash-path guidance now aligns with per-framework `hash_field_convention`.
- Remaining ambiguity detected: Section 5.1 Step 1 says `hash_verified=false` for "mismatch or no hash present", while later text and report schema require `hash_verified=null` when no hash field exists. This needs one canonical rule.
- Additional minor ambiguity: block-mode integrity policy uses SHOULD (recommendation) rather than MUST (deterministic requirement), leaving CI behavior open to interpretation.

### 2026-02-27 05:33:53 EST — Checkpoint 16 (Behavior-Shaping Review for Brownfield Adoption)
- Reviewed the portable spec for its ability to induce reviewing agents to proactively scan existing codebases and propose/implement UxTS-aligned recurring-construct remediation.
- Key gap: document strongly defines framework architecture and lifecycle, but does not yet mandate a brownfield execution loop with required remediation deliverables (opportunity backlog, prioritized implementation set, and closure criteria).
- Recommended additions: explicit Greenfield vs Brownfield operating modes, recurring-construct detection thresholds, mandatory opportunity scoring rubric, required code-change outcomes (or documented waiver), and acceptance criteria that require both discovery artifacts and implemented remediations.

### 2026-02-27 05:39:56 EST — Checkpoint 17 (Behavior-Shaping Changes Implemented)
- Updated `docs/specs/UXTS_PORTABLE_AGENT_SPEC.md` to operationalize brownfield adoption: added mandatory operating mode declaration (greenfield vs brownfield), recurring-construct thresholds, deterministic discovery command protocol, extension-first/new-framework gate, and mandatory brownfield remediation loop.
- Added enforceable brownfield deliverables in governance artifacts (`UXTS_DISCOVERY`, `UXTS_OPPORTUNITY_BACKLOG`, `UXTS_REMEDIATION_PLAN`, waiver artifact).
- Strengthened acceptance criteria so brownfield engagements require scored backlog evidence plus either implemented high-priority remediation or documented waiver.
