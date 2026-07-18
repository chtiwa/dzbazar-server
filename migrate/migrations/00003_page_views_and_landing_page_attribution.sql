-- +goose Up
-- page_visits: unique views per product / per landing page. Same discipline as
-- shop_visits (one row per unique visitor/day/page, deduped via the unique
-- index below), just with an extra page_type/entity_id dimension so a single
-- table covers both product pages and landing pages.
CREATE TABLE public.page_visits (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    shop_id uuid NOT NULL,
    page_type text NOT NULL,
    entity_id uuid NOT NULL,
    day date NOT NULL,
    visitor_id text NOT NULL,
    created_at timestamp with time zone
);

ALTER TABLE ONLY public.page_visits
    ADD CONSTRAINT page_visits_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.page_visits
    ADD CONSTRAINT page_visits_page_type_check CHECK (page_type IN ('product', 'landing_page'));

CREATE UNIQUE INDEX idx_page_visits_shop_type_entity_day_visitor
    ON public.page_visits USING btree (shop_id, page_type, entity_id, day, visitor_id);

-- Which landing page (if any) an order was placed from. Nullable — most
-- orders come from a plain product page. ON DELETE SET NULL so deleting a
-- landing page later never blocks/cascades into losing an order.
ALTER TABLE public.orders ADD COLUMN landing_page_id uuid;

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT fk_orders_landing_page FOREIGN KEY (landing_page_id) REFERENCES public.landing_pages(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE public.orders DROP CONSTRAINT IF EXISTS fk_orders_landing_page;
ALTER TABLE public.orders DROP COLUMN IF EXISTS landing_page_id;
DROP TABLE IF EXISTS public.page_visits;
