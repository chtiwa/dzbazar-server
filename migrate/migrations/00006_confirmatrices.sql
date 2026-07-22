-- +goose Up
-- Product scoping for confirmatrices (shop_members with role='confirmation'):
-- no rows for a member means "not scoped to anything", handled in app code as
-- "sees nothing until scoped" (an empty allow-list, not a wildcard).
CREATE TABLE public.confirmatrice_products (
    shop_member_id uuid NOT NULL,
    product_id uuid NOT NULL
);

ALTER TABLE ONLY public.confirmatrice_products
    ADD CONSTRAINT confirmatrice_products_pkey PRIMARY KEY (shop_member_id, product_id);

ALTER TABLE ONLY public.confirmatrice_products
    ADD CONSTRAINT fk_confirmatrice_products_member FOREIGN KEY (shop_member_id) REFERENCES public.shop_members(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.confirmatrice_products
    ADD CONSTRAINT fk_confirmatrice_products_product FOREIGN KEY (product_id) REFERENCES public.products(id) ON DELETE CASCADE;

CREATE INDEX idx_confirmatrice_products_product ON public.confirmatrice_products USING btree (product_id);

-- Order -> confirmatrice assignment. ON DELETE SET NULL so removing a staff
-- member never deletes/orphans the order, just clears its assignment.
ALTER TABLE public.orders ADD COLUMN assigned_member_id uuid;
ALTER TABLE public.orders ADD COLUMN assigned_at timestamp with time zone;

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT fk_orders_assigned_member FOREIGN KEY (assigned_member_id) REFERENCES public.shop_members(id) ON DELETE SET NULL;

CREATE INDEX idx_orders_assigned_member ON public.orders USING btree (assigned_member_id);

-- Single shop-level round-robin cursor, same pattern as
-- landing_page_experiments.assignment_cursor.
ALTER TABLE public.shops ADD COLUMN confirmatrice_cursor bigint NOT NULL DEFAULT 0;

INSERT INTO public.permission_actions (name, resource, label) VALUES
    ('orders.assign', 'orders', 'Assign orders to confirmatrices')
    ON CONFLICT (name) DO NOTHING;

INSERT INTO public.role_action_defaults (role, action, allow) VALUES
    ('owner', 'orders.assign', true),
    ('moderator', 'orders.assign', true),
    ('confirmation', 'orders.assign', false)
    ON CONFLICT (role, action) DO NOTHING;

-- +goose Down
DELETE FROM public.role_action_defaults WHERE action = 'orders.assign';
DELETE FROM public.permission_actions WHERE name = 'orders.assign';

ALTER TABLE public.shops DROP COLUMN IF EXISTS confirmatrice_cursor;

ALTER TABLE public.orders DROP CONSTRAINT IF EXISTS fk_orders_assigned_member;
DROP INDEX IF EXISTS public.idx_orders_assigned_member;
ALTER TABLE public.orders DROP COLUMN IF EXISTS assigned_at;
ALTER TABLE public.orders DROP COLUMN IF EXISTS assigned_member_id;

DROP TABLE IF EXISTS public.confirmatrice_products;
