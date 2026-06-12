-- Migration 014: Jiminy task_name swap backfill (Phase 11.6.x)
-- Purpose: Repair the historical jiminy.evaluate ↔ jiminy.evaluate_llm task_name
--   mislabel introduced by swapped WithContext("...") arguments at the production
--   call sites prior to commit (this sprint). The actual content of each row was
--   never wrong — only the task_name label that the analytics + RL pipelines
--   read. Phase 11.5e Epic 1 first surfaced the swap and routed around it via
--   `scripts/x11_jiminy_evaluate_rescue.py`. This migration ports that script's
--   content-routing table directly into SQL so the TSDB itself is consistent
--   without depending on the rescue layer.
--
-- Routing table (derived from ULTS specs, frozen here for the historical row set):
--   * caf70a3d… (eval_prompt.go evalSystemPrompt)
--       → spec: docs/tests/ults/specs/jiminy_evaluate.ults.json
--       → currently labeled jiminy.evaluate_llm in TSDB (≈106 rows)
--       → MUST become jiminy.evaluate
--   * 1f02ee46… (outcome_classifier.go classifySystemPromptCompact)
--       → spec: docs/tests/ults/specs/jiminy_evaluate_llm.ults.json (compact form)
--       → currently labeled jiminy.evaluate in TSDB (≈248 rows)
--       → MUST become jiminy.evaluate_llm
--   * f897ae32… (outcome_classifier.go classifySystemPrompt — historical variant
--                seen 2026-04-06 only, before the prompt was refactored to the
--                compact form. Same prompt family, same task_name target.)
--       → spec: not present (predates Phase 11.5e ULTS spec freeze)
--       → currently labeled jiminy.evaluate in TSDB (≈90 rows)
--       → MUST become jiminy.evaluate_llm
--
-- Sprint: FT-LORA-PHASE11.6.x (2026-05-01)
--
-- TIME-SCOPED (JIMINY-OUTCOME-002 hotfix, 2026-06-11): migrations re-run on
-- every auto-migrate startup, and the original Step-4 check asserted ALL-TIME
-- hash membership against this frozen hash set — so the first legitimate
-- prompt evolution (the not_applicable classifier prompt, 8 rows) made V0014
-- raise forever, aborting every migration run at 013 and pinning
-- schema_version=13 (live RSIC schema-drift alerts, correctly firing). The
-- backfill is a HISTORICAL repair: all statements now scope to rows from
-- before this migration shipped (time < 2026-05-02). New rows are labeled
-- correctly at the call site since Phase 11.6.x; future prompt hashes are
-- none of this migration's business.
--
-- Rollback (manual — matches 012/013 convention):
--   UPDATE llm_interactions SET task_name = 'jiminy.evaluate'
--     WHERE task_name = 'jiminy.evaluate_llm'
--       AND system_prompt_hash IN (
--         '1f02ee469728a422bd8e83629764be1a31115a02f7fbb5a372ef64243fb76bea',
--         'f897ae322c7f15b4e374eec7c40b810bb160f5284c8b015950e3e8f9aadd7da1'
--       );
--   UPDATE llm_interactions SET task_name = 'jiminy.evaluate_llm'
--     WHERE task_name = 'jiminy.evaluate'
--       AND system_prompt_hash = 'caf70a3d392f37d4d5b2d70d10b03d07b59a2f1cf18ed8cfd75d597cafa0a773';
--   UPDATE tsdb_schema_meta SET value = '13' WHERE key = 'schema_version';

-- ── Step 1: pre-migration audit row counts (as a NOTICE for the operator) ───
DO $$
DECLARE
    v_caf_to_evaluate    INTEGER;
    v_1f_to_evaluate_llm INTEGER;
    v_f8_to_evaluate_llm INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_caf_to_evaluate
        FROM llm_interactions
        WHERE task_name = 'jiminy.evaluate_llm'
          AND system_prompt_hash = 'caf70a3d392f37d4d5b2d70d10b03d07b59a2f1cf18ed8cfd75d597cafa0a773';
    SELECT COUNT(*) INTO v_1f_to_evaluate_llm
        FROM llm_interactions
        WHERE task_name = 'jiminy.evaluate'
          AND system_prompt_hash = '1f02ee469728a422bd8e83629764be1a31115a02f7fbb5a372ef64243fb76bea';
    SELECT COUNT(*) INTO v_f8_to_evaluate_llm
        FROM llm_interactions
        WHERE task_name = 'jiminy.evaluate'
          AND system_prompt_hash = 'f897ae322c7f15b4e374eec7c40b810bb160f5284c8b015950e3e8f9aadd7da1';
    RAISE NOTICE 'V0014 pre-migration: caf70a3d→evaluate=%, 1f02ee46→evaluate_llm=%, f897ae32→evaluate_llm=%',
        v_caf_to_evaluate, v_1f_to_evaluate_llm, v_f8_to_evaluate_llm;
END $$;

-- ── Step 2: caf70a3d… (eval_prompt content) → jiminy.evaluate ───────────────
UPDATE llm_interactions
   SET task_name = 'jiminy.evaluate'
 WHERE task_name = 'jiminy.evaluate_llm'
   AND time < '2026-05-02'
   AND system_prompt_hash = 'caf70a3d392f37d4d5b2d70d10b03d07b59a2f1cf18ed8cfd75d597cafa0a773';

-- ── Step 3: outcome_classifier hashes → jiminy.evaluate_llm ─────────────────
UPDATE llm_interactions
   SET task_name = 'jiminy.evaluate_llm'
 WHERE task_name = 'jiminy.evaluate'
   AND time < '2026-05-02'
   AND system_prompt_hash IN (
       '1f02ee469728a422bd8e83629764be1a31115a02f7fbb5a372ef64243fb76bea',
       'f897ae322c7f15b4e374eec7c40b810bb160f5284c8b015950e3e8f9aadd7da1'
   );

-- ── Step 4: post-migration consistency check — every jiminy.evaluate row must
-- now have hash = caf70a3d…, and every jiminy.evaluate_llm row must have one of
-- the outcome-classifier hashes. Aborts the migration on any mismatch so the
-- operator sees the inconsistency before the schema_version bump commits.
DO $$
DECLARE
    v_eval_mismatch     INTEGER;
    v_eval_llm_mismatch INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_eval_mismatch
        FROM llm_interactions
        WHERE task_name = 'jiminy.evaluate'
          AND time < '2026-05-02'
          AND system_prompt_hash IS NOT NULL
          AND system_prompt_hash <> 'caf70a3d392f37d4d5b2d70d10b03d07b59a2f1cf18ed8cfd75d597cafa0a773';
    SELECT COUNT(*) INTO v_eval_llm_mismatch
        FROM llm_interactions
        WHERE task_name = 'jiminy.evaluate_llm'
          AND time < '2026-05-02'
          AND system_prompt_hash IS NOT NULL
          AND system_prompt_hash NOT IN (
              '1f02ee469728a422bd8e83629764be1a31115a02f7fbb5a372ef64243fb76bea',
              'f897ae322c7f15b4e374eec7c40b810bb160f5284c8b015950e3e8f9aadd7da1'
          );
    IF v_eval_mismatch > 0 OR v_eval_llm_mismatch > 0 THEN
        RAISE EXCEPTION 'V0014 post-migration mismatch: jiminy.evaluate=% rows with non-eval_prompt hash, jiminy.evaluate_llm=% rows with non-classifier hash',
            v_eval_mismatch, v_eval_llm_mismatch;
    END IF;
    RAISE NOTICE 'V0014 post-migration: all jiminy.evaluate / jiminy.evaluate_llm rows now content-consistent.';
END $$;

-- ── Step 5: bump schema version ─────────────────────────────────────────────
UPDATE tsdb_schema_meta SET value = '14' WHERE key = 'schema_version';
