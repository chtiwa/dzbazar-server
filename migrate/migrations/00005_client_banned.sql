-- +goose Up
-- Phone-based ban fallback: BanOrderClient's fbp/ttp mechanism only works for
-- orders with a platform click-id. Organic orders (no fbp/ttp) ban the
-- Client row directly instead.
ALTER TABLE public.clients ADD COLUMN banned boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE public.clients DROP COLUMN IF EXISTS banned;
