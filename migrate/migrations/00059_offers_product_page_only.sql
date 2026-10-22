-- +goose Up
-- Offer scoped to the product page only (shown on no landing page).
ALTER TABLE public.offers ADD COLUMN IF NOT EXISTS product_page_only boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE public.offers DROP COLUMN IF EXISTS product_page_only;
