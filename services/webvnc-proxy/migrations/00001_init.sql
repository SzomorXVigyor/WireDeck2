-- +goose Up
CREATE TABLE IF NOT EXISTS vnc_targets (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  site_id             UUID NOT NULL,
  name                TEXT NOT NULL,
  host                INET NOT NULL,
  port                INTEGER NOT NULL DEFAULT 5900 CHECK (port BETWEEN 1 AND 65535),
  password_encrypted  BYTEA,
  is_active           BOOLEAN NOT NULL DEFAULT TRUE,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (site_id, name)
);
CREATE INDEX IF NOT EXISTS vnc_targets_site_idx ON vnc_targets (site_id);

CREATE TABLE IF NOT EXISTS vnc_sessions (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  site_id       UUID NOT NULL,
  target_id     UUID NOT NULL REFERENCES vnc_targets(id) ON DELETE CASCADE,
  user_id       UUID NOT NULL,
  started_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  ended_at      TIMESTAMPTZ,
  bytes_in      BIGINT NOT NULL DEFAULT 0,
  bytes_out     BIGINT NOT NULL DEFAULT 0,
  close_reason  TEXT
);
CREATE INDEX IF NOT EXISTS vnc_sessions_open_idx
  ON vnc_sessions (site_id, started_at DESC)
  WHERE ended_at IS NULL;
CREATE INDEX IF NOT EXISTS vnc_sessions_site_time_idx
  ON vnc_sessions (site_id, started_at DESC);

-- +goose Down
DROP TABLE IF EXISTS vnc_sessions;
DROP TABLE IF EXISTS vnc_targets;
