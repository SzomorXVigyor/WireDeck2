-- +goose Up
ALTER TABLE alarm_events
    ADD COLUMN IF NOT EXISTS last_value DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS peak_delta DOUBLE PRECISION;

UPDATE alarm_events
SET last_value = COALESCE(last_value, triggered_value),
    peak_delta = COALESCE(peak_delta, triggered_value - threshold)
WHERE last_value IS NULL OR peak_delta IS NULL;

-- +goose Down
ALTER TABLE alarm_events
    DROP COLUMN IF EXISTS peak_delta,
    DROP COLUMN IF EXISTS last_value;
