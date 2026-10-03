-- +goose Up
CREATE INDEX idx_clients_phone_number ON clients(phone_number);

-- +goose Down
DROP INDEX IF EXISTS idx_clients_phone_number;
