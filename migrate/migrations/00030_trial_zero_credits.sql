-- +goose Up
-- Trial (free) tier should only prove out the bare-minimum platform flow,
-- not gate AI-credit-metered features. Zero it; paid tiers unaffected.
UPDATE public.plans SET credits_per_month = 0 WHERE name = 'Trial';

-- +goose Down
UPDATE public.plans SET credits_per_month = 100 WHERE name = 'Trial';
