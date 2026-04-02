# Pre-Campaign Checklist: 30-Day Multi-Instance Data Collection

Run through this checklist before starting the training data collection campaign.
All items must pass before data collection begins.

## 1. Schema & Migration

- [ ] All instances on schema v8+ (migration 008: `instance_id`, 009: `space_id` backfill)
- [ ] Verify column exists on all 3 tables:
  ```bash
  docker compose exec -T timescaledb psql -U mdemg -d mdemg_metrics -c "
    SELECT table_name, column_name FROM information_schema.columns
    WHERE column_name = 'instance_id'
      AND table_name IN ('llm_interactions', 'embedding_events', 'retrieval_events');
  "
  # Expected: 3 rows
  ```

## 2. Instance Identity

- [ ] Each participating instance has a unique `MDEMG_INSTANCE_ID` configured
- [ ] Verify instance ID is being written:
  ```bash
  docker compose exec -T timescaledb psql -U mdemg -d mdemg_metrics -tAc "
    SELECT DISTINCT instance_id FROM llm_interactions
    WHERE time > now() - interval '1 hour' AND instance_id != '';
  "
  ```
- [ ] Document all instance IDs:
  | Instance | MDEMG_INSTANCE_ID | Owner |
  |----------|-------------------|-------|
  | Dev laptop | `MacBook-Pro-reh3376-mdemg-dev` | reh3376 |

## 3. ULTS Spec Verification

- [ ] Run hash verification — all 16 specs pass:
  ```bash
  python3 scripts/verify_ults_hashes.py --specs-dir docs/tests/ults/specs/ --repo-root .
  ```
- [ ] ULTS self-check passes:
  ```bash
  python3 docs/tests/utds/runners/utds_runner.py self-check
  ```

## 4. Multi-Table Export

- [ ] Full multi-table export succeeds with 0 privacy violations:
  ```bash
  TSDB_PORT=5433 ./bin/mdemg data export --space-id mdemg-dev --output /tmp/pre-campaign-export.tar.gz
  ```
- [ ] UTDS validation passes:
  ```bash
  python3 docs/tests/utds/runners/utds_runner.py validate --archive /tmp/pre-campaign-export.tar.gz --strict
  ```

## 5. Curation Pipeline

- [ ] Quality filter runs without errors:
  ```bash
  PYTHONPATH=neural python3 -m training.quality_filter \
    --input /tmp/extracted/llm_interactions.jsonl \
    --output /tmp/filtered.jsonl \
    --ults-dir docs/tests/ults/specs/ \
    --report /tmp/filter-report.json
  ```
- [ ] Dataset versioner produces valid output:
  ```bash
  PYTHONPATH=neural python3 -m training.dataset_versioner \
    --input-dir /tmp/ \
    --output-dir /tmp/dataset/v0/ \
    --version v0-precheck \
    --first-cycle \
    --min-per-task 1
  ```

## 6. Monitoring

- [ ] `data status` shows per-task accumulation rates:
  ```bash
  TSDB_PORT=5433 ./bin/mdemg data status --space-id mdemg-dev
  ```
- [ ] `data status --warn` exits non-zero for under-sampled tasks (expected pre-campaign):
  ```bash
  TSDB_PORT=5433 ./bin/mdemg data status --space-id mdemg-dev --warn
  echo "Exit code: $?"
  ```

## 7. E2E Pipeline Test

- [ ] Synthetic multi-instance E2E test passes:
  ```bash
  python3 -m pytest tests/e2e/training_pipeline/test_full_pipeline.py -v
  # Expected: 6 passed
  ```

## 8. Data Quality Baseline

- [ ] Run data review to establish baseline metrics:
  ```bash
  cd scripts && uv run python tsdb_data_review.py
  ```
- [ ] Record baseline row counts per task (for comparison at campaign end)

## Campaign Parameters

| Parameter | Value |
|-----------|-------|
| Duration | 30 days |
| Target rows per task | 500 minimum |
| Dedup mode (SFT) | `--dedup-key prompt` |
| Dedup mode (DPO) | `--dedup-key prompt-response` |
| First cycle | `--first-cycle` (all data is exogenous) |
| Monitoring frequency | Every 12 hours: `./bin/mdemg data status --warn` |
