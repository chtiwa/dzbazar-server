-- +goose Up
ALTER TABLE google_sheets_integrations DROP CONSTRAINT google_sheets_integrations_shop_id_key;
ALTER TABLE google_sheets_integrations
    ADD COLUMN kind TEXT NOT NULL DEFAULT 'orders' CHECK (kind IN ('orders','abandoned')),
    ADD COLUMN columns JSONB NOT NULL DEFAULT '[]'::jsonb,
    ALTER COLUMN service_account_json SET DEFAULT '';
CREATE UNIQUE INDEX idx_google_sheets_integrations_shop_kind ON google_sheets_integrations(shop_id, kind);
UPDATE google_sheets_integrations SET last_error = '', columns = '[{"key":"id","header":"Order ID"},{"key":"date","header":"Date"},{"key":"name","header":"Customer Name"},{"key":"phone","header":"Phone"},{"key":"wilaya","header":"Wilaya"},{"key":"commune","header":"City"},{"key":"stopdesk","header":"Stopdesk Point"},{"key":"products","header":"Products"},{"key":"total","header":"Total"},{"key":"status","header":"Status"}]'::jsonb;

-- +goose Down
-- Forward-only: reverting would delete every non-orders sheet integration.
SELECT 1;
