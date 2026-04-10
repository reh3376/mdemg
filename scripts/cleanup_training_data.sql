-- TSDB Training Data Cleanup
-- Removes records that are harmful to fine-tuning dataset quality:
--   1. Error records (failed LLM calls with no response content)
--   2. Silent failures (no error flag but empty/very short response)
--
-- Total expected: ~1,851 records (1,702 errors + 149 silent failures)
-- Remaining after cleanup: ~4,245 usable training records

BEGIN;

-- Count before
SELECT COUNT(*) AS total_before FROM llm_interactions;

-- Test batch: show 5 records that will be deleted
SELECT time, task_name, model_name, length(response) as resp_len,
       CASE WHEN error != '' THEN 'error' ELSE 'silent_fail' END as category
FROM llm_interactions
WHERE (error IS NOT NULL AND error != '')
   OR ((error IS NULL OR error = '') AND (response IS NULL OR length(response) < 10))
ORDER BY time DESC LIMIT 5;

-- Delete error records (failed LLM calls)
DELETE FROM llm_interactions
WHERE error IS NOT NULL AND error != '';

-- Delete silent failures (no error but empty/short response)
DELETE FROM llm_interactions
WHERE (error IS NULL OR error = '') AND (response IS NULL OR length(response) < 10);

-- Count after
SELECT COUNT(*) AS total_after FROM llm_interactions;

-- Show remaining distribution by model
SELECT model_name, COUNT(*) as records,
       ROUND(AVG(length(response))) as avg_resp_len
FROM llm_interactions
GROUP BY model_name ORDER BY records DESC;

-- Show remaining distribution by task
SELECT task_name, COUNT(*) as records
FROM llm_interactions
GROUP BY task_name ORDER BY records DESC;

COMMIT;
