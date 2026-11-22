-- NO_TRANSACTION
-- (CREATE MATERIALIZED VIEW ... WITH (timescaledb.continuous) cannot run inside
--  a transaction block.)
--
-- 003_continuous_aggregates.sql
-- Rollups so wide chart ranges never touch raw data:
--   <= 6h  -> data_value_histories (raw)
--   <= 7d  -> data_value_1m
--   >  7d  -> data_value_1h
-- Both aggregates are built directly from raw to avoid unweighted average-of-averages skew.

SET ROLE svc_webview_history;

CREATE MATERIALIZED VIEW IF NOT EXISTS platform_webview_history.data_value_1m
WITH (timescaledb.continuous) AS
SELECT
  time_bucket('1 minute', time) AS bucket,
  register_mapping_id,
  site_id,
  avg(value)   AS avg_value,
  min(value)   AS min_value,
  max(value)   AS max_value,
  last(value, time) AS last_value,
  count(*)     AS sample_count
FROM platform_webview_history.data_value_histories
GROUP BY bucket, register_mapping_id, site_id
WITH NO DATA;

SELECT add_continuous_aggregate_policy(
  'platform_webview_history.data_value_1m',
  start_offset      => INTERVAL '3 hours',
  end_offset        => INTERVAL '1 minute',
  schedule_interval => INTERVAL '1 minute',
  if_not_exists     => TRUE
);

CREATE MATERIALIZED VIEW IF NOT EXISTS platform_webview_history.data_value_1h
WITH (timescaledb.continuous) AS
SELECT
  time_bucket('1 hour', time) AS bucket,
  register_mapping_id,
  site_id,
  avg(value)   AS avg_value,
  min(value)   AS min_value,
  max(value)   AS max_value,
  last(value, time) AS last_value,
  count(*)     AS sample_count
FROM platform_webview_history.data_value_histories
GROUP BY bucket, register_mapping_id, site_id
WITH NO DATA;

SELECT add_continuous_aggregate_policy(
  'platform_webview_history.data_value_1h',
  start_offset      => INTERVAL '3 days',
  end_offset        => INTERVAL '1 hour',
  schedule_interval => INTERVAL '1 hour',
  if_not_exists     => TRUE
);

-- Aggregate retention: 1 year (D13).
SELECT add_retention_policy(
  'platform_webview_history.data_value_1m',
  INTERVAL '1 year',
  if_not_exists => TRUE
);
SELECT add_retention_policy(
  'platform_webview_history.data_value_1h',
  INTERVAL '1 year',
  if_not_exists => TRUE
);

RESET ROLE;
