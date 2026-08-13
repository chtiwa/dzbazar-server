-- +goose Up
ALTER TABLE public.pixels ALTER COLUMN is_active SET DEFAULT false;

-- +goose Down
ALTER TABLE public.pixels ALTER COLUMN is_active SET DEFAULT true;
