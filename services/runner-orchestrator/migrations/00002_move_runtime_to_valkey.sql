-- +goose Up
DROP INDEX IF EXISTS runners_status_idx;

ALTER TABLE runners
  DROP COLUMN IF EXISTS observed_state,
  DROP COLUMN IF EXISTS status,
  DROP COLUMN IF EXISTS handshake_state,
  DROP COLUMN IF EXISTS handshake_age_seconds,
  DROP COLUMN IF EXISTS last_handshake_at,
  DROP COLUMN IF EXISTS last_checked_at,
  DROP COLUMN IF EXISTS last_error,
  DROP COLUMN IF EXISTS last_reconciled_at;

-- +goose Down
ALTER TABLE runners
  ADD COLUMN IF NOT EXISTS observed_state TEXT,
  ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'UNKNOWN' CHECK (status IN ('ONLINE', 'DEGRADED', 'OFFLINE', 'UNKNOWN')),
  ADD COLUMN IF NOT EXISTS handshake_state TEXT NOT NULL DEFAULT 'UNSPECIFIED',
  ADD COLUMN IF NOT EXISTS handshake_age_seconds BIGINT,
  ADD COLUMN IF NOT EXISTS last_handshake_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS last_checked_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS last_error TEXT,
  ADD COLUMN IF NOT EXISTS last_reconciled_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS runners_status_idx ON runners (status);
