-- +goose Up
-- Trial plan every new shop is auto-subscribed to at creation (see
-- shopsController.CreateShop): 2-day window, capped at 100 orders, 2 landing
-- pages, 5 products, 1 user. Paid tiers ($9.99/$19.99/$29.99) are the
-- upgrade targets an approved invoice (see invoices table below) switches a
-- shop onto, same upsert-ShopSubscription logic as plan_switch_requests.
INSERT INTO public.plans (id, name, price, is_active, max_shops, max_products, max_orders, max_landing_pages, max_users, max_facebook_pixels, max_tik_tok_pixels, has_confirmation_orders, has_abandoned_orders, has_order_tracking, has_client_tracking)
VALUES
    (public.uuid_generate_v4(), 'Trial', 0, true, 1, 5, 100, 2, 1, 1, 1, true, false, false, false),
    (public.uuid_generate_v4(), 'Starter', 9.99, true, 1, 30, 500, 3, 2, 1, 1, true, false, false, false),
    (public.uuid_generate_v4(), 'Growth', 19.99, true, 1, 150, 2000, 10, 5, 2, 2, true, true, true, true),
    (public.uuid_generate_v4(), 'Pro', 29.99, true, 2, -1, -1, -1, -1, 3, 3, true, true, true, true);

-- Merchant-submitted proof of a manual Redot transfer. Mirrors
-- plan_switch_requests (status queue + super-admin approve/reject), plus the
-- amount/payment method/screenshot proof a real payment needs. Approval
-- upserts shop_subscriptions the same way ApprovePlanSwitchRequest does.
CREATE TABLE public.invoices (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id uuid NOT NULL,
    plan_id uuid NOT NULL,
    amount numeric NOT NULL,
    payment_method text DEFAULT 'redot'::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    proof_screenshot_url text,
    reviewed_by uuid,
    reviewed_at timestamp with time zone
);

ALTER TABLE ONLY public.invoices
    ADD CONSTRAINT invoices_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.invoices
    ADD CONSTRAINT fk_invoices_shop FOREIGN KEY (shop_id) REFERENCES public.shops(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.invoices
    ADD CONSTRAINT fk_invoices_plan FOREIGN KEY (plan_id) REFERENCES public.plans(id);

ALTER TABLE ONLY public.invoices
    ADD CONSTRAINT fk_invoices_reviewer FOREIGN KEY (reviewed_by) REFERENCES public.users(id);

CREATE INDEX idx_invoices_shop_id ON public.invoices USING btree (shop_id);

-- At most one pending invoice per shop at a time.
CREATE UNIQUE INDEX idx_invoices_shop_pending ON public.invoices USING btree (shop_id) WHERE (status = 'pending'::text AND deleted_at IS NULL);

-- +goose Down
DROP TABLE IF EXISTS public.invoices;
DELETE FROM public.plans WHERE name IN ('Trial', 'Starter', 'Growth', 'Pro');
