-- +goose Up
-- 1. Remove MODBUS_UDP and tighten devices to name, host, port, protocol (MODBUS_TCP)
UPDATE devices SET protocol = 'MODBUS_TCP' WHERE protocol <> 'MODBUS_TCP';
ALTER TABLE devices DROP CONSTRAINT IF EXISTS devices_protocol_check;
ALTER TABLE devices ADD CONSTRAINT devices_protocol_check CHECK (protocol IN ('MODBUS_TCP'));

-- 2. Add dynamic protocol_attributes JSONB to register_mappings and backfill existing rows
ALTER TABLE register_mappings
    ADD COLUMN IF NOT EXISTS protocol_attributes JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE register_mappings rm
SET protocol_attributes = jsonb_build_object(
    'slaveAddress', COALESCE(d.unit_id, 1),
    'functionCode', rm.function_code,
    'registerAddress', rm.register_address,
    'dataType', rm.data_type,
    'wordOrder', rm.word_order,
    'accessMode', CASE WHEN rm.access_mode = 'READ_WRITE' THEN 'READ_WRITE' ELSE 'READ' END
)
FROM devices d
WHERE d.id = rm.device_id AND rm.protocol_attributes = '{}'::jsonb;

-- 3. Add sort_order to dashboards for reordering
ALTER TABLE dashboards
    ADD COLUMN IF NOT EXISTS sort_order INTEGER NOT NULL DEFAULT 0;

-- 4. Remove chart widget type (keep display, lamp, switch, button, number_input)
DELETE FROM widgets WHERE widget_type = 'chart';
ALTER TABLE widgets DROP CONSTRAINT IF EXISTS widgets_widget_type_check;
ALTER TABLE widgets ADD CONSTRAINT widgets_widget_type_check
    CHECK (widget_type IN ('lamp', 'display', 'switch', 'button', 'number_input'));

-- 5. Per-register history entries (selected keys + poll interval + retention)
CREATE TABLE IF NOT EXISTS history_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id UUID NOT NULL,
    register_mapping_id UUID NOT NULL REFERENCES register_mappings(id) ON DELETE CASCADE,
    poll_interval_seconds INTEGER NOT NULL DEFAULT 60 CHECK (poll_interval_seconds BETWEEN 1 AND 86400),
    retention_days INTEGER NOT NULL DEFAULT 30 CHECK (retention_days BETWEEN 1 AND 365),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (site_id, register_mapping_id)
);

CREATE INDEX IF NOT EXISTS idx_history_entries_site ON history_entries (site_id);

-- +goose Down
DROP TABLE IF EXISTS history_entries;
ALTER TABLE dashboards DROP COLUMN IF EXISTS sort_order;
ALTER TABLE register_mappings DROP COLUMN IF EXISTS protocol_attributes;
