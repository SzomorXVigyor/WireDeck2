-- +goose Up
DROP TABLE IF EXISTS alarm_pending_states;

-- +goose Down
CREATE TABLE IF NOT EXISTS alarm_pending_states (
    rule_id UUID PRIMARY KEY REFERENCES alarm_rules(id) ON DELETE CASCADE,
    site_id UUID NOT NULL,
    since_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_value DOUBLE PRECISION NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_alarm_pending_site ON alarm_pending_states (site_id);
