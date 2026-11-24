-- +goose Up
CREATE TABLE IF NOT EXISTS alarm_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id UUID NOT NULL,
    register_mapping_id UUID NOT NULL REFERENCES register_mappings(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    condition TEXT NOT NULL CHECK (condition IN ('GT', 'GTE', 'LT', 'LTE', 'EQ', 'NEQ')),
    threshold DOUBLE PRECISION NOT NULL,
    clear_deadband DOUBLE PRECISION NOT NULL DEFAULT 0.0 CHECK (clear_deadband >= 0.0),
    hold_seconds INTEGER NOT NULL DEFAULT 0 CHECK (hold_seconds BETWEEN 0 AND 86400),
    poll_interval_seconds INTEGER NOT NULL DEFAULT 10 CHECK (poll_interval_seconds BETWEEN 1 AND 3600),
    severity TEXT NOT NULL DEFAULT 'WARNING' CHECK (severity IN ('INFO', 'WARNING', 'CRITICAL')),
    notify_emails TEXT[] NOT NULL DEFAULT '{}'::text[],
    notify_on_clear BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (site_id, name)
);

CREATE INDEX IF NOT EXISTS idx_alarm_rules_site ON alarm_rules (site_id);
CREATE INDEX IF NOT EXISTS idx_alarm_rules_mapping ON alarm_rules (register_mapping_id);

CREATE TABLE IF NOT EXISTS alarm_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id UUID NOT NULL,
    rule_id UUID REFERENCES alarm_rules(id) ON DELETE SET NULL,
    register_mapping_id UUID REFERENCES register_mappings(id) ON DELETE SET NULL,
    rule_name TEXT NOT NULL,
    register_name TEXT NOT NULL,
    condition TEXT NOT NULL,
    threshold DOUBLE PRECISION NOT NULL,
    unit TEXT NOT NULL DEFAULT '',
    severity TEXT NOT NULL CHECK (severity IN ('INFO', 'WARNING', 'CRITICAL')),
    state TEXT NOT NULL CHECK (state IN ('ACTIVE', 'CLEARED')),
    is_stale BOOLEAN NOT NULL DEFAULT FALSE,
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    triggered_value DOUBLE PRECISION NOT NULL,
    cleared_at TIMESTAMPTZ,
    cleared_value DOUBLE PRECISION,
    acknowledged_at TIMESTAMPTZ,
    acknowledged_by_user_id UUID,
    acknowledged_by_email TEXT NOT NULL DEFAULT '',
    ack_note TEXT NOT NULL DEFAULT '',
    notified_at TIMESTAMPTZ,
    notify_failed BOOLEAN NOT NULL DEFAULT FALSE,
    notify_error TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_alarm_events_site_state ON alarm_events (site_id, state, triggered_at DESC);
CREATE INDEX IF NOT EXISTS idx_alarm_events_triggered ON alarm_events (triggered_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_alarm_events_one_active_per_rule
    ON alarm_events (rule_id)
    WHERE state = 'ACTIVE' AND rule_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS alarm_pending_states (
    rule_id UUID PRIMARY KEY REFERENCES alarm_rules(id) ON DELETE CASCADE,
    site_id UUID NOT NULL,
    since_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_value DOUBLE PRECISION NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_alarm_pending_site ON alarm_pending_states (site_id);

-- +goose Down
DROP TABLE IF EXISTS alarm_pending_states;
DROP TABLE IF EXISTS alarm_events;
DROP TABLE IF EXISTS alarm_rules;
