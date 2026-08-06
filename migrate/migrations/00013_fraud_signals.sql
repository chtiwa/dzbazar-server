-- +goose Up
ALTER TABLE public.shops ADD COLUMN ban_incognito_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE public.shops ADD COLUMN ban_vpn_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE public.shops ADD COLUMN ban_datacenter_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE public.orders ADD COLUMN hidden_reason text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE public.orders DROP COLUMN IF EXISTS hidden_reason;
ALTER TABLE public.shops DROP COLUMN IF EXISTS ban_datacenter_enabled;
ALTER TABLE public.shops DROP COLUMN IF EXISTS ban_vpn_enabled;
ALTER TABLE public.shops DROP COLUMN IF EXISTS ban_incognito_enabled;
