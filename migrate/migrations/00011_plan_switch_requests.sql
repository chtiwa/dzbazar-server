-- +goose Up
-- Merchant plan switches now go through super-admin approval instead of
-- applying instantly: SubscribeShopToPlan creates a pending row here rather
-- than touching shop_subscriptions directly. Approval upserts
-- shop_subscriptions the same way the old direct-switch code did.
CREATE TABLE public.plan_switch_requests (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id uuid NOT NULL,
    plan_id uuid NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    reviewed_by uuid,
    reviewed_at timestamp with time zone
);

ALTER TABLE ONLY public.plan_switch_requests
    ADD CONSTRAINT plan_switch_requests_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.plan_switch_requests
    ADD CONSTRAINT fk_plan_switch_requests_shop FOREIGN KEY (shop_id) REFERENCES public.shops(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.plan_switch_requests
    ADD CONSTRAINT fk_plan_switch_requests_plan FOREIGN KEY (plan_id) REFERENCES public.plans(id);

ALTER TABLE ONLY public.plan_switch_requests
    ADD CONSTRAINT fk_plan_switch_requests_reviewer FOREIGN KEY (reviewed_by) REFERENCES public.users(id);

CREATE INDEX idx_plan_switch_requests_shop_id ON public.plan_switch_requests USING btree (shop_id);

-- At most one pending request per shop at a time.
CREATE UNIQUE INDEX idx_plan_switch_requests_shop_pending ON public.plan_switch_requests USING btree (shop_id) WHERE (status = 'pending'::text AND deleted_at IS NULL);

-- +goose Down
DROP TABLE IF EXISTS public.plan_switch_requests;
