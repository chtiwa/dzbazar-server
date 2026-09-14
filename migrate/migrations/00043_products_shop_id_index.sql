-- +goose Up
CREATE INDEX idx_products_shop_id ON products(shop_id);

-- +goose Down
DROP INDEX IF EXISTS idx_products_shop_id;
