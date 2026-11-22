-- +goose Up
CREATE TABLE IF NOT EXISTS site_webview_settings (
    site_id UUID PRIMARY KEY,
    history_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS member_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id UUID NOT NULL,
    user_id UUID NOT NULL,
    can_write BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (site_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_member_policies_site ON member_policies (site_id);

CREATE TABLE IF NOT EXISTS devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id UUID NOT NULL,
    name TEXT NOT NULL,
    host INET NOT NULL,
    port INTEGER NOT NULL DEFAULT 502 CHECK (port BETWEEN 1 AND 65535),
    protocol TEXT NOT NULL DEFAULT 'MODBUS_TCP' CHECK (protocol IN ('MODBUS_TCP', 'MODBUS_UDP')),
    unit_id SMALLINT NOT NULL DEFAULT 1 CHECK (unit_id BETWEEN 0 AND 255),
    max_concurrent_requests INTEGER NOT NULL DEFAULT 1 CHECK (max_concurrent_requests BETWEEN 1 AND 8),
    request_timeout_ms INTEGER NOT NULL DEFAULT 2000 CHECK (request_timeout_ms BETWEEN 200 AND 30000),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (site_id, name)
);

CREATE INDEX IF NOT EXISTS idx_devices_site ON devices (site_id);

CREATE TABLE IF NOT EXISTS register_mappings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    site_id UUID NOT NULL,
    name TEXT NOT NULL,
    function_code TEXT NOT NULL CHECK (function_code IN ('READ_COIL', 'READ_DISCRETE', 'READ_HOLDING', 'READ_INPUT')),
    register_address INTEGER NOT NULL CHECK (register_address BETWEEN 0 AND 65535),
    register_count INTEGER NOT NULL DEFAULT 1 CHECK (register_count BETWEEN 1 AND 2),
    data_type TEXT NOT NULL CHECK (data_type IN ('BOOL', 'UINT16', 'INT16', 'UINT32', 'INT32', 'FLOAT32')),
    word_order TEXT NOT NULL DEFAULT 'BIG' CHECK (word_order IN ('BIG', 'LITTLE')),
    byte_order TEXT NOT NULL DEFAULT 'BIG' CHECK (byte_order IN ('BIG', 'LITTLE')),
    scale DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    offset_val DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    unit TEXT NOT NULL DEFAULT '',
    access_mode TEXT NOT NULL DEFAULT 'READ' CHECK (access_mode IN ('READ', 'WRITE', 'READ_WRITE')),
    write_function_code TEXT NOT NULL DEFAULT '' CHECK (write_function_code IN ('', 'WRITE_COIL', 'WRITE_REGISTER', 'WRITE_MULTIPLE_REGISTERS')),
    interactive_poll_interval_ms INTEGER NOT NULL DEFAULT 1000 CHECK (interactive_poll_interval_ms BETWEEN 250 AND 60000),
    archival_poll_interval_ms INTEGER NOT NULL DEFAULT 60000 CHECK (archival_poll_interval_ms BETWEEN 1000 AND 3600000),
    deadband DOUBLE PRECISION NOT NULL DEFAULT 0.0 CHECK (deadband >= 0.0),
    max_interval_seconds INTEGER NOT NULL DEFAULT 300 CHECK (max_interval_seconds BETWEEN 10 AND 86400),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (site_id, name)
);

CREATE INDEX IF NOT EXISTS idx_register_mappings_site ON register_mappings (site_id);
CREATE INDEX IF NOT EXISTS idx_register_mappings_device ON register_mappings (device_id);

CREATE TABLE IF NOT EXISTS dashboards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id UUID NOT NULL,
    name TEXT NOT NULL,
    layout JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (site_id, name)
);

CREATE INDEX IF NOT EXISTS idx_dashboards_site ON dashboards (site_id);

CREATE TABLE IF NOT EXISTS widgets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dashboard_id UUID NOT NULL REFERENCES dashboards(id) ON DELETE CASCADE,
    register_mapping_id UUID REFERENCES register_mappings(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    widget_type TEXT NOT NULL CHECK (widget_type IN ('lamp', 'display', 'switch', 'button', 'number_input', 'chart')),
    sort_order INTEGER NOT NULL DEFAULT 0,
    style JSONB NOT NULL DEFAULT '{}'::jsonb,
    extra JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_widgets_dashboard ON widgets (dashboard_id, sort_order);

-- +goose Down
DROP TABLE IF EXISTS widgets;
DROP TABLE IF EXISTS dashboards;
DROP TABLE IF EXISTS register_mappings;
DROP TABLE IF EXISTS devices;
DROP TABLE IF EXISTS member_policies;
DROP TABLE IF EXISTS site_webview_settings;
