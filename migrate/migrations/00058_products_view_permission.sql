-- +goose Up
-- GET /products was gated by role (owner/moderator), so confirmation staff got
-- 403 in the order-edit product picker. Make it a grantable action.
INSERT INTO public.permission_actions (name, resource, label)
    VALUES ('products.view', 'products', 'View products')
    ON CONFLICT (name) DO NOTHING;

INSERT INTO public.role_action_defaults (role, action, allow) VALUES
    ('owner', 'products.view', true),
    ('moderator', 'products.view', true),
    ('confirmation', 'products.view', true)
    ON CONFLICT (role, action) DO NOTHING;

-- +goose Down
DELETE FROM public.shop_member_permissions WHERE action = 'products.view';
DELETE FROM public.role_action_defaults WHERE action = 'products.view';
DELETE FROM public.permission_actions WHERE name = 'products.view';
