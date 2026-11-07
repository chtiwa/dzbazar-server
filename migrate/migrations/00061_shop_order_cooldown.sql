-- +goose Up
ALTER TABLE public.shops ADD COLUMN order_cooldown_hours integer NOT NULL DEFAULT 24;

-- +goose Down
ALTER TABLE public.shops DROP COLUMN IF EXISTS order_cooldown_hours;
