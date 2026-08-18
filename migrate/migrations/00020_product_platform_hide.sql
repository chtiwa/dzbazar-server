-- +goose Up
-- Lets a super admin force-hide a single fraudulent product listing without
-- suspending the whole shop (ListProducts in
-- controllers/superadmin/productsController.go is deliberately read-only
-- otherwise — bulk-editing another tenant's catalog is a correctness risk).
-- Platform-owned, not a merchant field — distinct from
-- product_variant_combinations.retired (migration 00012), which is a
-- merchant-owned SKU-retirement concept at the combination level, not this
-- product-level moderation flag.
ALTER TABLE public.products ADD COLUMN hidden_by_platform_at timestamptz;

-- +goose Down
ALTER TABLE public.products DROP COLUMN IF EXISTS hidden_by_platform_at;
