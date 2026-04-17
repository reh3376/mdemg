-- TSDB Training Data Cleanup
-- Removes records that are harmful to fine-tuning dataset quality:
--   1. Error records (failed LLM calls — transport-level failures with no response)
--   2. Silent failures (no error flag but empty/very short response < 10 chars)
--
-- Data snapshot (2026-04-16):
--   Pre-cleanup:     39,629 records (space_id = 'mdemg-dev')
--   Explicit errors:  3,103 (rate limit 429, DNS failure, timeout, TLS, JSON parse)
--   Silent failures:    347 (empty/short response, no error field)
--   Post-cleanup:    36,633 clean records (cleanup executed 2026-04-15)
--
-- Error root causes (all transient infrastructure, not code bugs):
--   Rate limit 429: 1,273 | DNS failure: 910 | Timeout: 869 | Other: 51
--
-- Usage:
--   1. Review the dry-run preview (SELECT with LIMIT 5)
--   2. Verify counts are within expected range
--   3. Uncomment the DELETE statements and run in a transaction
--   4. Verify post-cleanup counts
--
-- Primary execution path: `mdemg data clean` CLI command (preferred over raw SQL)

-- ============================================================
-- STEP 1: Pre-deletion counts (verify before proceeding)
-- ============================================================

SELECT 'pre_cleanup' AS stage,
       COUNT(*) AS total,
       COUNT(*) FILTER (WHERE error IS NOT NULL AND error != '') AS errors,
       COUNT(*) FILTER (WHERE (error IS NULL OR error = '') AND (response IS NULL OR length(response) < 10)) AS silent_fails,
       COUNT(*) FILTER (WHERE (error IS NULL OR error = '') AND length(COALESCE(response, '')) >= 10) AS clean
FROM llm_interactions
WHERE space_id = 'mdemg-dev';

-- ============================================================
-- STEP 2: Dry-run preview — show 5 records that would be deleted
-- ============================================================

-- Error records preview
SELECT 'error_record' AS category, time, task_name, model_name,
       length(COALESCE(response, '')) AS resp_len,
       LEFT(error, 80) AS error_snippet
FROM llm_interactions
WHERE space_id = 'mdemg-dev'
  AND error IS NOT NULL AND error != ''
ORDER BY time DESC LIMIT 5;

-- Silent failure preview
SELECT 'silent_fail' AS category, time, task_name, model_name,
       length(COALESCE(response, '')) AS resp_len,
       response AS full_response
FROM llm_interactions
WHERE space_id = 'mdemg-dev'
  AND (error IS NULL OR error = '')
  AND (response IS NULL OR length(response) < 10)
ORDER BY time DESC LIMIT 5;

-- ============================================================
-- STEP 3: DELETE (uncomment to execute — run in transaction)
-- ============================================================

-- BEGIN;
--
-- -- Delete error records (failed LLM calls)
-- DELETE FROM llm_interactions
-- WHERE space_id = 'mdemg-dev'
--   AND error IS NOT NULL AND error != '';
--
-- -- Delete silent failures (no error but empty/short response)
-- DELETE FROM llm_interactions
-- WHERE space_id = 'mdemg-dev'
--   AND (error IS NULL OR error = '')
--   AND (response IS NULL OR length(response) < 10);
--
-- COMMIT;

-- ============================================================
-- STEP 4: Post-deletion verification
-- ============================================================

-- SELECT 'post_cleanup' AS stage,
--        COUNT(*) AS total,
--        COUNT(*) FILTER (WHERE error IS NOT NULL AND error != '') AS errors,
--        COUNT(*) FILTER (WHERE (error IS NULL OR error = '') AND (response IS NULL OR length(response) < 10)) AS silent_fails
-- FROM llm_interactions
-- WHERE space_id = 'mdemg-dev';
--
-- -- Per-task distribution
-- SELECT task_name, COUNT(*) AS records,
--        ROUND(AVG(length(response))) AS avg_resp_len,
--        ROUND(AVG(latency_ms)) AS avg_latency_ms
-- FROM llm_interactions
-- WHERE space_id = 'mdemg-dev'
-- GROUP BY task_name ORDER BY records DESC;
