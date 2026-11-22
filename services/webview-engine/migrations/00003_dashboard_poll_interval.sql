-- +goose Up
ALTER TABLE dashboards
    ADD COLUMN IF NOT EXISTS poll_interval_ms INTEGER NOT NULL DEFAULT 1000
    CHECK (poll_interval_ms BETWEEN 1000 AND 60000);

-- +goose Down
ALTER TABLE dashboards DROP COLUMN IF EXISTS poll_interval_ms;
