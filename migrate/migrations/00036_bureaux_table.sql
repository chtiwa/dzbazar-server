-- +goose Up
-- Per-shop, per-wilaya stopdesk desk ("bureau") names. NOT linked to a
-- delivery carrier: the same list is used no matter which carrier ends up
-- shipping the order, because the carrier is only chosen later, at ship time
-- (admin/src/pages/orders/BatchShipModal.tsx). ZR Express still resolves its
-- own hubs live via resolveZrHubID (server/controllers/zrGeoController.go)
-- and simply ignores this table -- that is a ZR-side detail, not a reason to
-- shape this table around carriers.
--
-- Seeded one row per wilaya per shop, exactly like public.delivery_rates:
-- new shops get theirs in shopsController.go's creation transaction, and the
-- backfill at the bottom of this migration gives every already-existing shop
-- the same 58 rows.
--
-- Deliberately NO foreign key from clients.stopdesk_point to this table:
-- that column stays a plain stored string so deleting a bureau never
-- rewrites or breaks a past order.
CREATE TABLE public.bureaux (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    shop_id UUID NOT NULL REFERENCES public.shops(id) ON DELETE CASCADE,
    wilaya_id INTEGER NOT NULL REFERENCES public.wilayas(id),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

-- Both readers -- the order-form dropdown (shop + wilaya) and the management
-- page (shop, all wilayas) -- lead with shop_id, so one composite index
-- covers both.
CREATE INDEX idx_bureaux_shop_wilaya ON public.bureaux(shop_id, wilaya_id);
CREATE INDEX idx_bureaux_deleted_at ON public.bureaux(deleted_at);

-- Multiple distinct desk names per wilaya are expected (a shop can add more
-- desks next to the seeded default), so uniqueness is only on the exact name
-- within one shop + wilaya. Partial on deleted_at IS NULL so a soft-deleted
-- row never blocks re-adding the same name.
CREATE UNIQUE INDEX idx_bureaux_unique_name
    ON public.bureaux(shop_id, wilaya_id, name)
    WHERE deleted_at IS NULL;

INSERT INTO public.permission_actions (name, resource, label) VALUES
    ('bureaux.view', 'bureaux', 'View bureaux'),
    ('bureaux.edit', 'bureaux', 'Edit bureaux')
    ON CONFLICT (name) DO UPDATE SET resource = EXCLUDED.resource, label = EXCLUDED.label;

INSERT INTO public.role_action_defaults (role, action, allow) VALUES
    ('owner', 'bureaux.view', true),
    ('owner', 'bureaux.edit', true),
    ('moderator', 'bureaux.view', true),
    ('moderator', 'bureaux.edit', true),
    ('confirmation', 'bureaux.view', true)
    ON CONFLICT (role, action) DO NOTHING;

-- Backfill: every shop that already exists gets the same one-row-per-wilaya
-- seed that shopsController.go now gives new shops, so no shop is ever left
-- with an empty stopdesk dropdown. Name defaults to the wilaya's own name
-- ("Alger"), which the shop owner can then rename or add alongside.
INSERT INTO public.bureaux (id, shop_id, wilaya_id, name)
SELECT uuid_generate_v4(), s.id, w.id, w.name
FROM public.shops s
CROSS JOIN public.wilayas w;

-- +goose Down
DELETE FROM public.role_action_defaults WHERE action IN ('bureaux.view', 'bureaux.edit');
DELETE FROM public.permission_actions WHERE name IN ('bureaux.view', 'bureaux.edit');
DROP TABLE public.bureaux;
