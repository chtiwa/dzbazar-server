-- +goose Up
ALTER TABLE public.shops ADD COLUMN auto_assign_enabled boolean NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE public.shops DROP COLUMN IF EXISTS auto_assign_enabled;
