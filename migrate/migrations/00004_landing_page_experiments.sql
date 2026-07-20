-- +goose Up
-- An experiment groups 2+ existing landing_pages rows as "sets" under strict
-- round-robin traffic split. Sets keep their own Views/ConversionRate exactly
-- as any standalone landing page does (page_visits/orders stay the source of
-- truth) — this table only adds the routing/decision state on top.
CREATE TABLE public.landing_page_experiments (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    shop_id uuid NOT NULL,
    product_id uuid NOT NULL,
    name text NOT NULL,
    target_conversions integer NOT NULL DEFAULT 100,
    status text NOT NULL DEFAULT 'running',
    winner_landing_page_id uuid,
    assignment_cursor bigint NOT NULL DEFAULT 0,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone
);

ALTER TABLE ONLY public.landing_page_experiments
    ADD CONSTRAINT landing_page_experiments_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.landing_page_experiments
    ADD CONSTRAINT landing_page_experiments_status_check
    CHECK (status IN ('running', 'decided', 'stopped'));

ALTER TABLE ONLY public.landing_page_experiments
    ADD CONSTRAINT fk_experiments_shop FOREIGN KEY (shop_id) REFERENCES public.shops(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.landing_page_experiments
    ADD CONSTRAINT fk_experiments_winner FOREIGN KEY (winner_landing_page_id) REFERENCES public.landing_pages(id) ON DELETE SET NULL;

CREATE INDEX idx_landing_page_experiments_shop_id ON public.landing_page_experiments USING btree (shop_id);
CREATE INDEX idx_landing_page_experiments_deleted_at ON public.landing_page_experiments USING btree (deleted_at);

-- experiment_id nullable: most landing pages never belong to a test.
-- experiment_position is the stable set ordering the round-robin walks.
ALTER TABLE public.landing_pages ADD COLUMN experiment_id uuid;
ALTER TABLE public.landing_pages ADD COLUMN experiment_position integer NOT NULL DEFAULT 0;

ALTER TABLE ONLY public.landing_pages
    ADD CONSTRAINT fk_landing_pages_experiment FOREIGN KEY (experiment_id) REFERENCES public.landing_page_experiments(id) ON DELETE SET NULL;

CREATE INDEX idx_landing_pages_experiment_id ON public.landing_pages USING btree (experiment_id);

-- Sticky visitor->set assignment, keyed by the same persistent visitor_id
-- page_visits already uses. Guarantees a reload never reassigns a visitor
-- to a different set mid-session.
CREATE TABLE public.landing_page_experiment_assignments (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    experiment_id uuid NOT NULL,
    visitor_id text NOT NULL,
    landing_page_id uuid NOT NULL,
    created_at timestamp with time zone
);

ALTER TABLE ONLY public.landing_page_experiment_assignments
    ADD CONSTRAINT landing_page_experiment_assignments_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.landing_page_experiment_assignments
    ADD CONSTRAINT fk_assignments_experiment FOREIGN KEY (experiment_id) REFERENCES public.landing_page_experiments(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.landing_page_experiment_assignments
    ADD CONSTRAINT fk_assignments_landing_page FOREIGN KEY (landing_page_id) REFERENCES public.landing_pages(id) ON DELETE CASCADE;

CREATE UNIQUE INDEX idx_lpea_experiment_visitor ON public.landing_page_experiment_assignments USING btree (experiment_id, visitor_id);

-- +goose Down
DROP TABLE IF EXISTS public.landing_page_experiment_assignments;

ALTER TABLE public.landing_pages DROP CONSTRAINT IF EXISTS fk_landing_pages_experiment;
DROP INDEX IF EXISTS public.idx_landing_pages_experiment_id;
ALTER TABLE public.landing_pages DROP COLUMN IF EXISTS experiment_position;
ALTER TABLE public.landing_pages DROP COLUMN IF EXISTS experiment_id;

DROP TABLE IF EXISTS public.landing_page_experiments;
