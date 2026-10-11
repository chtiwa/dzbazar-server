-- +goose Up
-- Margin fix: 2000 credits/month on Pro (pooled across up to 5 shops) cost more
-- in image-gen spend than the plan earns. Image models bill 6-25 credits each.
UPDATE public.plans SET credits_per_month = 100 WHERE name = 'Starter';
UPDATE public.plans SET credits_per_month = 300 WHERE name = 'Growth';
UPDATE public.plans SET credits_per_month = 800 WHERE name = 'Pro';

-- +goose Down
UPDATE public.plans SET credits_per_month = 200  WHERE name = 'Starter';
UPDATE public.plans SET credits_per_month = 600  WHERE name = 'Growth';
UPDATE public.plans SET credits_per_month = 2000 WHERE name = 'Pro';
