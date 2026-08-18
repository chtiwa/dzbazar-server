-- +goose Up
ALTER TABLE public.shops ADD COLUMN suspend_reason text;

-- +goose Down
ALTER TABLE public.shops DROP COLUMN suspend_reason;
