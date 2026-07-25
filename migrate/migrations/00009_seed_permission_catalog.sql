-- +goose Up
-- Full catalog of every action gated by middleware.RequireShopPermission across
-- server/routes/*.go. Before this migration only orders.assign (00006) had rows,
-- so every other permission-gated route denied everyone (role_action_defaults has
-- no row = deny, and there's no owner-bypass in services.MemberCan).
INSERT INTO public.permission_actions (name, resource, label) VALUES
    ('dashboard.view',            'dashboard',          'View dashboard'),
    ('orders.view',               'orders',             'View orders'),
    ('orders.export',             'orders',             'Export orders'),
    ('orders.status_history',     'orders',             'View order status history'),
    ('orders.edit',               'orders',             'Edit orders'),
    ('orders.delete',             'orders',             'Delete orders'),
    ('orders.ban_client',         'orders',             'Ban a client'),
    ('orders.ship',               'orders',             'Ship orders (courier)'),
    ('orders.track',              'orders',             'Track courier shipments'),
    ('users.view',                'users',              'View users'),
    ('users.create',              'users',              'Create users'),
    ('users.edit',                'users',              'Edit users'),
    ('users.delete',              'users',              'Delete users'),
    ('products.create',           'products',           'Create products'),
    ('products.edit',             'products',           'Edit products'),
    ('products.delete',           'products',           'Delete products'),
    ('coupons.create',            'coupons',            'Create coupons'),
    ('coupons.edit',              'coupons',            'Edit coupons'),
    ('coupons.delete',            'coupons',            'Delete coupons'),
    ('clients.edit',              'clients',            'Edit clients'),
    ('clients.delete',            'clients',            'Delete clients'),
    ('delivery_rates.edit',       'delivery_rates',     'Edit delivery rates'),
    ('delivery_companies.view',   'delivery_companies', 'View delivery companies'),
    ('delivery_companies.edit',   'delivery_companies', 'Edit delivery companies'),
    ('landing_pages.create',      'landing_pages',      'Create landing pages'),
    ('landing_pages.edit',        'landing_pages',      'Edit landing pages'),
    ('landing_pages.delete',      'landing_pages',      'Delete landing pages'),
    ('pixels.view',               'pixels',             'View pixels'),
    ('pixels.edit',               'pixels',             'Edit pixels'),
    ('pixels.delete',             'pixels',             'Delete pixels'),
    ('offers.create',             'offers',             'Create offers'),
    ('offers.edit',               'offers',             'Edit offers'),
    ('offers.archive',            'offers',             'Archive offers'),
    ('offers.delete',             'offers',             'Delete offers'),
    ('settings.view',             'settings',           'View settings'),
    ('settings.edit',             'settings',           'Edit settings'),
    ('subscription.view',         'subscription',       'View subscription'),
    ('subscription.edit',         'subscription',       'Edit subscription')
    ON CONFLICT (name) DO UPDATE SET resource = EXCLUDED.resource, label = EXCLUDED.label;

-- Owner always has every action allowed (impersonation and real owner
-- memberships both resolve permission checks purely off this role default).
INSERT INTO public.role_action_defaults (role, action, allow)
    SELECT 'owner', name, true FROM public.permission_actions
    ON CONFLICT (role, action) DO NOTHING;

-- Moderator: full access except delete actions (matches CreateUserDialog copy).
INSERT INTO public.role_action_defaults (role, action, allow)
    SELECT 'moderator', name, true FROM public.permission_actions
    WHERE name NOT LIKE '%.delete'
    ON CONFLICT (role, action) DO NOTHING;

-- Confirmation: can only view and edit orders (matches CreateUserDialog copy).
-- orders.assign default for confirmation is already seeded false by 00006.
INSERT INTO public.role_action_defaults (role, action, allow) VALUES
    ('confirmation', 'orders.view', true),
    ('confirmation', 'orders.edit', true)
    ON CONFLICT (role, action) DO NOTHING;

-- +goose Down
DELETE FROM public.role_action_defaults
    WHERE action IN (SELECT name FROM public.permission_actions WHERE name <> 'orders.assign')
    AND role IN ('owner', 'moderator', 'confirmation');
DELETE FROM public.permission_actions WHERE name <> 'orders.assign';
