-- Migration 004: Add continuous aggregate refresh policies
-- The metrics_hourly and metrics_daily aggregates were created with WITH NO DATA
-- and had no refresh policies, so they remained permanently empty.

-- Hourly aggregate: refresh data from 3h ago to 1h ago, every hour
SELECT add_continuous_aggregate_policy('metrics_hourly',
    start_offset  => INTERVAL '3 hours',
    end_offset    => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour',
    if_not_exists => true);

-- Daily aggregate: refresh data from 3d ago to 1d ago, every day
SELECT add_continuous_aggregate_policy('metrics_daily',
    start_offset  => INTERVAL '3 days',
    end_offset    => INTERVAL '1 day',
    schedule_interval => INTERVAL '1 day',
    if_not_exists => true);

UPDATE tsdb_schema_meta SET value = '4' WHERE key = 'schema_version';
