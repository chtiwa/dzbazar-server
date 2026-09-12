-- +goose Up
ALTER TABLE public.shops ADD COLUMN free_delivery_enabled boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE public.shops DROP COLUMN IF EXISTS free_delivery_enabled;
