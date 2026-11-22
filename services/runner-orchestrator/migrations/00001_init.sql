-- +goose Up
CREATE TABLE IF NOT EXISTS runners (
  id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  site_id                UUID NOT NULL UNIQUE,
  container_name         TEXT NOT NULL UNIQUE,
  container_id           TEXT,
  image                  TEXT NOT NULL,
  conf_fingerprint       TEXT,
  auth_token_encrypted   BYTEA,
  desired_state          TEXT NOT NULL DEFAULT 'RUNNING' CHECK (desired_state IN ('RUNNING', 'STOPPED')),
  observed_state         TEXT,
  status                 TEXT NOT NULL DEFAULT 'UNKNOWN' CHECK (status IN ('ONLINE', 'DEGRADED', 'OFFLINE', 'UNKNOWN')),
  handshake_state        TEXT NOT NULL DEFAULT 'UNSPECIFIED',
  handshake_age_seconds  BIGINT,
  last_handshake_at      TIMESTAMPTZ,
  last_checked_at        TIMESTAMPTZ,
  last_error             TEXT,
  last_reconciled_at     TIMESTAMPTZ,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS runners_status_idx ON runners (status);

CREATE TABLE IF NOT EXISTS runner_health_events (
  id           BIGSERIAL PRIMARY KEY,
  site_id      UUID NOT NULL,
  from_status  TEXT NOT NULL,
  to_status    TEXT NOT NULL,
  occurred_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  detail       TEXT
);
CREATE INDEX IF NOT EXISTS runner_health_events_site_time_idx
  ON runner_health_events (site_id, occurred_at DESC);

-- +goose Down
DROP TABLE IF EXISTS runner_health_events;
DROP TABLE IF EXISTS runners;
