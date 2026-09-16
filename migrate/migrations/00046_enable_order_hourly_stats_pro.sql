-- +goose Up
-- 00045 added has_order_hourly_stats defaulting false on every plan row,
-- including the existing Pro plan — nothing backfilled it, so Pro shops
-- were locked out of a feature meant to be theirs. Enable it on Pro.
UPDATE public.plans SET has_order_hourly_stats = true WHERE name = 'Pro';

-- +goose Down
UPDATE public.plans SET has_order_hourly_stats = false WHERE name = 'Pro';
