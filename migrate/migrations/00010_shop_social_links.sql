-- +goose Up
ALTER TABLE public.shops ADD COLUMN facebook_url text NOT NULL DEFAULT '';
ALTER TABLE public.shops ADD COLUMN instagram_url text NOT NULL DEFAULT '';
ALTER TABLE public.shops ADD COLUMN tiktok_url text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE public.shops DROP COLUMN IF EXISTS facebook_url;
ALTER TABLE public.shops DROP COLUMN IF EXISTS instagram_url;
ALTER TABLE public.shops DROP COLUMN IF EXISTS tiktok_url;
