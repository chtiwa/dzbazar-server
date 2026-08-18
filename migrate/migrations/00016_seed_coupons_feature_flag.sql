-- +goose Up
INSERT INTO public.feature_flags (id, created_at, updated_at, key, label, description, is_enabled)
VALUES (
    uuid_generate_v4(),
    now(),
    now(),
    'coupons_enabled',
    'Coupons',
    'Platform-wide kill switch for creating new coupons. Turning this off blocks merchants from creating new coupons (existing coupons keep working) — for use during an incident.',
    true
)
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM public.feature_flags WHERE key = 'coupons_enabled';
