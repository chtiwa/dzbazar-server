-- Orders search does `full_name ILIKE '%q%' OR phone_number LIKE '%q%'`;
-- leading wildcards can't use btree, so it seq-scans clients. pg_trgm (enabled in 00001) fixes that.
-- +goose Up
CREATE INDEX idx_clients_full_name_trgm ON clients USING gin (full_name gin_trgm_ops);
CREATE INDEX idx_clients_phone_number_trgm ON clients USING gin (phone_number gin_trgm_ops);

-- +goose Down
DROP INDEX IF EXISTS idx_clients_full_name_trgm;
DROP INDEX IF EXISTS idx_clients_phone_number_trgm;
