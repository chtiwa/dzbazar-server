-- +goose Up
-- Per-variant-item image (e.g. "black" item shows the black-shoe photo).
-- UpdateProductByShop deletes and recreates all variant_items on every save
-- (see server/CLAUDE.md "Product variant update constraint"), so this column
-- is never a stable store keyed by item identity — the client payload must
-- resend the existing imageUrl on every update or it's lost.
ALTER TABLE public.variant_items ADD COLUMN image_url text;

-- +goose Down
ALTER TABLE public.variant_items DROP COLUMN IF EXISTS image_url;
