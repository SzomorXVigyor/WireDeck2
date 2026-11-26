-- +goose Up
ALTER TABLE widgets DROP CONSTRAINT IF EXISTS widgets_widget_type_check;
ALTER TABLE widgets ADD CONSTRAINT widgets_widget_type_check
    CHECK (widget_type IN ('lamp', 'display', 'switch', 'button', 'number_input', 'graph'));

-- +goose Down
DELETE FROM widgets WHERE widget_type = 'graph';
ALTER TABLE widgets DROP CONSTRAINT IF EXISTS widgets_widget_type_check;
ALTER TABLE widgets ADD CONSTRAINT widgets_widget_type_check
    CHECK (widget_type IN ('lamp', 'display', 'switch', 'button', 'number_input'));
