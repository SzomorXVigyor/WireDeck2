-- NO_TRANSACTION
-- (TimescaleDB compression and retention policy functions cannot execute inside
--  a single transaction block.)
--
-- 002_history_hypertable.sql
-- Remote sensor & register time-series hypertable.

SET ROLE svc_webview_history;

CREATE TABLE IF NOT EXISTS platform_webview_history.data_value_histories (
  time                 TIMESTAMPTZ      NOT NULL,
  register_mapping_id  UUID             NOT NULL,
  site_id              UUID             NOT NULL,
  value                DOUBLE PRECISION NOT NULL
);

SELECT create_hypertable(
  'platform_webview_history.data_value_histories',
  'time',
  chunk_time_interval => INTERVAL '1 day',
  if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS data_value_histories_mapping_time_idx
  ON platform_webview_history.data_value_histories (register_mapping_id, time DESC);

CREATE INDEX IF NOT EXISTS data_value_histories_site_time_idx
  ON platform_webview_history.data_value_histories (site_id, time DESC);

ALTER TABLE platform_webview_history.data_value_histories SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'site_id, register_mapping_id',
  timescaledb.compress_orderby   = 'time DESC'
);

SELECT add_compression_policy(
  'platform_webview_history.data_value_histories',
  INTERVAL '7 days',
  if_not_exists => TRUE
);

-- Raw retention: 30 days (D13). Continuous aggregates are retained for 1 year (003).
SELECT add_retention_policy(
  'platform_webview_history.data_value_histories',
  INTERVAL '30 days',
  if_not_exists => TRUE
);

RESET ROLE;
