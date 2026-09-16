-- +goose Up
ALTER TABLE plans ADD COLUMN has_order_hourly_stats boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE plans DROP COLUMN has_order_hourly_stats;
