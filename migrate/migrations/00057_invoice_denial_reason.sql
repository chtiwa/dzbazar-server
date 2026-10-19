-- +goose Up
ALTER TABLE public.invoices ADD COLUMN denial_reason text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE public.invoices DROP COLUMN denial_reason;
