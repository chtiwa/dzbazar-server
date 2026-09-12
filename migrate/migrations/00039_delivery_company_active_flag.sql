-- +goose Up
ALTER TABLE public.delivery_companies
    ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE public.delivery_companies DROP COLUMN is_active;
