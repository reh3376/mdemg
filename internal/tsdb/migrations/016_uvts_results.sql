-- Migration 016: UVTS results (Phase 12 — UVTS Activation)
-- Purpose: Persist one row per UVTS validation run + one row per scored
-- question. UVTS = Unified Validation Test Suite for semantic retrieval
-- quality (distinct from Phase 10's UBENCH-precursor reward registry, which
-- lives in V0012 `benchmark_results`). UVTS is the gating framework for
-- all post-FT-LORA architectural extensions; Phase 12 activates it.
--
-- Distinction from V0012:
--   * benchmark_results (V0012) tracks LLM task rewards per (run, task,
--     prompt_hash) — closed-form scoring against the reward registry.
--   * uvts_results (V0016) tracks per-question retrieval-quality scores
--     across (evidence_match × 0.70 + semantic_sim × 0.15 +
--     concept_overlap × 0.15) per the grader_v4.py three-axis scorer.
--
-- Population: written by `docs/tests/uvts/runners/uvts_runner.py` via psycopg
-- when `--persist-tsdb` is set. The runner's existing JSON-on-disk path stays
-- — TSDB persistence is additive, so dev/CI runs that don't need TSDB can
-- keep emitting just the canonical UxTS report.
--
-- Sprint: POST-FT-LORA-PHASE12 (2026-05-01)
--
-- Rollback (manual — matches 012/013/014/015 convention):
--   DROP TABLE IF EXISTS uvts_results CASCADE;
--   DROP TABLE IF EXISTS uvts_runs CASCADE;
--   UPDATE tsdb_schema_meta SET value = '15' WHERE key = 'schema_version';

-- ── uvts_runs: one row per UVTS validation run ──────────────────────────────
CREATE TABLE IF NOT EXISTS uvts_runs (
    run_id            TEXT             PRIMARY KEY,  -- CUIDv2 (MEMORY)
    spec_path         TEXT             NOT NULL,
    spec_sha          TEXT,                          -- SHA256 of the spec file at run time
    -- Free-text branch label so A/B harness can group results
    -- (e.g. 'baseline' / 'candidate' / 'A_vs_B_<short>'); not enforced.
    branch_label      TEXT,
    -- Validation target codebase (the spec's validation.codebase field)
    -- + an optional SHA pin so historical runs can be rerun on a frozen
    -- codebase commit if needed.
    codebase_root     TEXT,
    codebase_sha      TEXT,
    -- Profile name from the spec (quick / standard / full).
    profile           TEXT,
    -- Aggregate of grader_v4 mean across all per-question rows.
    aggregate_score   NUMERIC,
    -- Threshold gate verdict. 'pending' until uvts_runner finishes scoring.
    gate_verdict      TEXT             NOT NULL DEFAULT 'pending'
        CHECK (gate_verdict IN ('pending', 'pass', 'fail')),
    -- Frozen copy of the spec's threshold block at the time of the run, so
    -- changing thresholds in the spec doesn't invalidate historical verdicts.
    threshold_json    JSONB,
    -- For A/B verdict rows: which baseline/candidate run_ids this row links.
    -- NULL on a normal "single-run" row; populated only on harness-emitted
    -- ab-comparison rows.
    ab_baseline_run_id  TEXT REFERENCES uvts_runs(run_id) ON DELETE SET NULL,
    ab_candidate_run_id TEXT REFERENCES uvts_runs(run_id) ON DELETE SET NULL,
    started_at        TIMESTAMPTZ      NOT NULL,
    completed_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_uvts_runs_started
    ON uvts_runs (started_at DESC);
CREATE INDEX IF NOT EXISTS idx_uvts_runs_verdict
    ON uvts_runs (gate_verdict, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_uvts_runs_branch
    ON uvts_runs (branch_label, started_at DESC);

-- ── uvts_results: one row per (run, question) ───────────────────────────────
CREATE TABLE IF NOT EXISTS uvts_results (
    -- TSDB hypertable partition key
    recorded_at       TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    result_id         TEXT             NOT NULL,    -- CUIDv2
    -- Soft FK (no constraint to allow uvts_runs garbage-collection without
    -- cascading; analytic queries join in app layer).
    run_id            TEXT             NOT NULL,
    question_id       TEXT             NOT NULL,
    category          TEXT,
    -- Final blended score (grader_v4: evidence × 0.70 + semantic × 0.15
    -- + concept × 0.15). Range [0,1].
    final_score       NUMERIC,
    -- Per-axis components (kept verbatim from grader_v4 output for forensic
    -- regrading without rerunning retrieve).
    evidence_score    NUMERIC,
    semantic_score    NUMERIC,
    concept_score     NUMERIC,
    -- Evidence tier: 'strong' / 'moderate' / 'weak' / 'minimal' / 'none'.
    evidence_tier     TEXT,
    -- True when the answer's file:line citations matched the expected ones
    -- within the spec's line_tolerance (default 10).
    file_loc_ok       BOOLEAN          NOT NULL DEFAULT FALSE,
    -- Full grader_v4 Grade.to_dict() blob, preserved for re-grading + audit.
    raw_grade         JSONB,
    PRIMARY KEY (recorded_at, result_id)
);

-- TimescaleDB hypertable on recorded_at. 7-day chunk fits a 3-month observation
-- window in ~13 chunks while keeping insert overhead low (matches V0015 sizing).
SELECT create_hypertable(
    'uvts_results',
    'recorded_at',
    if_not_exists => TRUE,
    chunk_time_interval => INTERVAL '7 days'
);

CREATE INDEX IF NOT EXISTS idx_uvts_results_run
    ON uvts_results (run_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_uvts_results_category
    ON uvts_results (category, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_uvts_results_question
    ON uvts_results (question_id);

-- ── pre-migration audit ─────────────────────────────────────────────────────
DO $$
DECLARE
    v_existing_runs    INTEGER;
    v_existing_results INTEGER;
BEGIN
    -- Safety: V0016 is additive-only. If somehow these tables already had
    -- rows from a previous partial apply, raise NOTICE so the operator sees
    -- it before anything irreversible happens.
    SELECT COUNT(*) INTO v_existing_runs FROM uvts_runs;
    SELECT COUNT(*) INTO v_existing_results FROM uvts_results;
    IF v_existing_runs > 0 OR v_existing_results > 0 THEN
        RAISE NOTICE 'V0016 pre-migration: found % uvts_runs rows, % uvts_results rows (preserved across re-apply)',
            v_existing_runs, v_existing_results;
    ELSE
        RAISE NOTICE 'V0016 pre-migration: empty tables, fresh install';
    END IF;
END $$;

UPDATE tsdb_schema_meta SET value = '16' WHERE key = 'schema_version';
