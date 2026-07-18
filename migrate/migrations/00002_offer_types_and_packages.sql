-- +goose Up
-- Additive only: two nullable columns on the existing offers table.
-- Existing rows get NULL offer_type (Action alone still fully describes
-- their behavior) and NULL quantity_packages (unused unless offer_type =
-- 'quantity_upsell'). No backfill needed, nothing existing changes shape.
ALTER TABLE public.offers ADD COLUMN offer_type text;
ALTER TABLE public.offers ADD COLUMN quantity_packages text;

CREATE INDEX idx_offers_offer_type ON public.offers (offer_type);

-- +goose Down
DROP INDEX IF EXISTS idx_offers_offer_type;
ALTER TABLE public.offers DROP COLUMN IF EXISTS quantity_packages;
ALTER TABLE public.offers DROP COLUMN IF EXISTS offer_type;
