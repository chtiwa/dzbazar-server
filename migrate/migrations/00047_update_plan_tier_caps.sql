-- +goose Up
-- Merchant-facing pricing update: new caps for Starter/Growth/Pro. Trial is
-- unchanged (still 2 days, enforced in shopsController.CreateShop — not a
-- plans-table field). Growth gains max_shops (1 -> 3); Pro's shop/product/
-- order/landing-page/user/pixel caps all move up per the new pricing sheet.
UPDATE public.plans SET
    max_shops = 1,
    max_orders = 500,
    max_users = 1,
    credits_per_month = 200
WHERE name = 'Starter';

UPDATE public.plans SET
    max_shops = 3,
    max_products = 20,
    max_orders = 3000,
    max_landing_pages = 20,
    max_users = 5,
    max_facebook_pixels = 3,
    max_tik_tok_pixels = 3,
    credits_per_month = 600
WHERE name = 'Growth';

UPDATE public.plans SET
    max_shops = 5,
    max_products = 100,
    max_orders = 20000,
    max_landing_pages = 100,
    max_users = 20,
    max_facebook_pixels = 10,
    max_tik_tok_pixels = 10,
    credits_per_month = 2000
WHERE name = 'Pro';

-- +goose Down
UPDATE public.plans SET
    max_shops = 1,
    max_products = 30,
    max_orders = 500,
    max_users = 1,
    credits_per_month = 200
WHERE name = 'Starter';

UPDATE public.plans SET
    max_shops = 1,
    max_products = 150,
    max_orders = 2000,
    max_landing_pages = 10,
    max_users = 5,
    max_facebook_pixels = 2,
    max_tik_tok_pixels = 2,
    credits_per_month = 600
WHERE name = 'Growth';

UPDATE public.plans SET
    max_shops = 2,
    max_products = -1,
    max_orders = -1,
    max_landing_pages = -1,
    max_users = -1,
    max_facebook_pixels = 3,
    max_tik_tok_pixels = 3,
    credits_per_month = 2000
WHERE name = 'Pro';
