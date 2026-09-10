-- +goose Up
-- Per-shop, per-carrier stopdesk desk ("bureau") names for the carriers that
-- have no live hub API: Osen, Leopard, Anderson. ZR Express is excluded --
-- it resolves its hubs live via POST /hubs/search (see resolveZrHubID in
-- server/controllers/zrGeoController.go), so it never reads this table.
--
-- delivery_company_id points at the shop's own connected carrier row
-- (public.delivery_companies), not the global available_delivery_companies
-- catalog, so two shops on the same carrier keep independent desk lists.
--
-- Deliberately NO foreign key from clients.stopdesk_point to this table:
-- that column stays a plain stored string so deleting a bureau never
-- rewrites or breaks a past order.
CREATE TABLE public.bureaux (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    shop_id UUID NOT NULL REFERENCES public.shops(id) ON DELETE CASCADE,
    delivery_company_id UUID NOT NULL REFERENCES public.delivery_companies(id) ON DELETE CASCADE,
    wilaya_id INTEGER NOT NULL REFERENCES public.wilayas(id),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

-- The order-form dropdown reads by shop + wilaya (union across carriers);
-- the management page reads by shop + carrier. One composite index covers
-- the leading shop_id for both.
CREATE INDEX idx_bureaux_shop_wilaya ON public.bureaux(shop_id, wilaya_id);
CREATE INDEX idx_bureaux_delivery_company ON public.bureaux(delivery_company_id);
CREATE INDEX idx_bureaux_deleted_at ON public.bureaux(deleted_at);

-- Multiple distinct desk names per wilaya are expected (Osen has 4 desks in
-- some wilayas), so the uniqueness is only on the exact name within one
-- shop + carrier + wilaya. Partial on deleted_at IS NULL so a soft-deleted
-- row never blocks re-adding the same name.
CREATE UNIQUE INDEX idx_bureaux_unique_name
    ON public.bureaux(shop_id, delivery_company_id, wilaya_id, name)
    WHERE deleted_at IS NULL;

INSERT INTO public.permission_actions (name, resource, label) VALUES
    ('bureaux.view', 'bureaux', 'View carrier bureaux'),
    ('bureaux.edit', 'bureaux', 'Edit carrier bureaux')
    ON CONFLICT (name) DO UPDATE SET resource = EXCLUDED.resource, label = EXCLUDED.label;

INSERT INTO public.role_action_defaults (role, action, allow) VALUES
    ('owner', 'bureaux.view', true),
    ('owner', 'bureaux.edit', true),
    ('moderator', 'bureaux.view', true),
    ('moderator', 'bureaux.edit', true),
    ('confirmation', 'bureaux.view', true)
    ON CONFLICT (role, action) DO NOTHING;

-- +goose Down
DELETE FROM public.role_action_defaults WHERE action IN ('bureaux.view', 'bureaux.edit');
DELETE FROM public.permission_actions WHERE name IN ('bureaux.view', 'bureaux.edit');
DROP TABLE public.bureaux;
