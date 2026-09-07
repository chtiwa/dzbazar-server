-- +goose Up
CREATE TABLE google_sheets_integrations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    shop_id UUID NOT NULL UNIQUE REFERENCES shops(id) ON DELETE CASCADE,
    service_account_json TEXT NOT NULL,
    spreadsheet_id TEXT NOT NULL,
    sheet_name TEXT NOT NULL DEFAULT 'Orders',
    is_active BOOLEAN NOT NULL DEFAULT true,
    last_synced_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_google_sheets_integrations_deleted_at ON google_sheets_integrations(deleted_at);

ALTER TABLE orders ADD COLUMN sheets_export_sent_at TIMESTAMPTZ;
ALTER TABLE orders ADD COLUMN sheets_export_attempts INT NOT NULL DEFAULT 0;

INSERT INTO public.permission_actions (name, resource, label) VALUES
    ('sheets.view', 'sheets', 'View Google Sheets integration'),
    ('sheets.edit', 'sheets', 'Edit Google Sheets integration')
    ON CONFLICT (name) DO UPDATE SET resource = EXCLUDED.resource, label = EXCLUDED.label;

INSERT INTO public.role_action_defaults (role, action, allow) VALUES
    ('owner', 'sheets.view', true),
    ('owner', 'sheets.edit', true),
    ('moderator', 'sheets.view', true),
    ('moderator', 'sheets.edit', true)
    ON CONFLICT (role, action) DO NOTHING;

-- +goose Down
DELETE FROM public.role_action_defaults WHERE action IN ('sheets.view', 'sheets.edit');
DELETE FROM public.permission_actions WHERE name IN ('sheets.view', 'sheets.edit');
ALTER TABLE orders DROP COLUMN sheets_export_attempts;
ALTER TABLE orders DROP COLUMN sheets_export_sent_at;
DROP TABLE google_sheets_integrations;
