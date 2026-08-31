-- +goose Up
-- 00025 seeded separate Starter/Growth/Pro rows alongside the existing
-- Basic/Standard/Premium DZD plans. Merchant direction: fold them into one
-- catalog — rename the original three in place and switch them to USD so
-- existing ShopSubscriptions (which point at these ids) keep working
-- untouched. Drop the now-redundant duplicates 00025 created FIRST (by
-- their known ids — nothing references them yet since 00025 only just
-- shipped) since plans.name is unique and the renames below collide with
-- them otherwise.
DELETE FROM public.plans WHERE id IN (
    '018ead7d-844a-4af7-adc4-0086ba4fbcf0', -- Starter (00025 duplicate)
    '4484bc3f-7bb7-4734-857c-524594dd02b6', -- Growth (00025 duplicate)
    'e9c0bce4-8017-4573-affe-437558d5059f'  -- Pro (00025 duplicate)
);

UPDATE public.plans SET name = 'Starter', price = 9.99 WHERE name = 'Basic';
UPDATE public.plans SET name = 'Growth', price = 19.99 WHERE name = 'Standard';
UPDATE public.plans SET name = 'Pro', price = 29.99 WHERE name = 'Premium';

-- +goose Down
UPDATE public.plans SET name = 'Basic', price = 2000 WHERE name = 'Starter';
UPDATE public.plans SET name = 'Standard', price = 4000 WHERE name = 'Growth';
UPDATE public.plans SET name = 'Premium', price = 9000 WHERE name = 'Pro';
