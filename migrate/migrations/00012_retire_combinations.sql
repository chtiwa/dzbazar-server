-- +goose Up
ALTER TABLE public.product_variant_combinations ADD COLUMN retired boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE public.product_variant_combinations DROP COLUMN IF EXISTS retired;
