-- +goose Up
-- Collapse the two fixed AI counters (max_ai_descriptions_per_month,
-- max_ai_images_per_month) into one credits allowance. "Used" stays a
-- computed figure over the existing append-only usage logs — a weighted SUM
-- (description = 5, image = 10) since shop_subscriptions.started_at, the
-- same window every other period-scoped cap uses. No running balance, so no
-- reset job: approving an invoice already rewrites started_at, and that is
-- the reset. See services.CheckCreditBudget.
ALTER TABLE public.plans ADD COLUMN credits_per_month integer DEFAULT 0 NOT NULL;

-- Backfill by tier name, same UPDATE-in-place approach as 00026 so live
-- shop_subscriptions rows keep pointing at the same plan ids. Default 0 (not
-- -1): a tier added outside this list must be granted credits deliberately —
-- silently handing out unlimited image generation is the expensive failure.
UPDATE public.plans SET credits_per_month = 100  WHERE name = 'Trial';
UPDATE public.plans SET credits_per_month = 200  WHERE name = 'Starter';
UPDATE public.plans SET credits_per_month = 600  WHERE name = 'Growth';
UPDATE public.plans SET credits_per_month = 2000 WHERE name = 'Pro';

ALTER TABLE public.plans DROP COLUMN IF EXISTS max_ai_descriptions_per_month;
ALTER TABLE public.plans DROP COLUMN IF EXISTS max_ai_images_per_month;

-- +goose Down
ALTER TABLE public.plans ADD COLUMN max_ai_descriptions_per_month integer DEFAULT 30 NOT NULL;
ALTER TABLE public.plans ADD COLUMN max_ai_images_per_month integer DEFAULT 5 NOT NULL;
ALTER TABLE public.plans DROP COLUMN IF EXISTS credits_per_month;
