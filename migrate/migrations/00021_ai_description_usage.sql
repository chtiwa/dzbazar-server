-- +goose Up
-- Per-call log of AI product-description generations. Two readers:
--   1. services.CheckAiDescriptionLimit — COUNT(*) for the shop's current
--      subscription period against plans.max_ai_descriptions_per_month.
--   2. superadmin GetShop — SUM(total_tokens) for the same window, so an
--      operator can see what a shop costs at the AI provider.
-- Per-call rows rather than a running per-period sum: same two queries
-- either way, no upsert race, and failed/rejected calls stay auditable.
-- Only successful generations get a row (see services/ai.go) — a provider
-- error or a rejected prompt must not consume the merchant's quota.
CREATE TABLE public.ai_description_usages (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id uuid NOT NULL,
    user_id uuid,
    prompt_tokens integer DEFAULT 0 NOT NULL,
    completion_tokens integer DEFAULT 0 NOT NULL,
    total_tokens integer DEFAULT 0 NOT NULL
);

ALTER TABLE ONLY public.ai_description_usages
    ADD CONSTRAINT ai_description_usages_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.ai_description_usages
    ADD CONSTRAINT fk_ai_description_usages_shop FOREIGN KEY (shop_id) REFERENCES public.shops(id) ON DELETE CASCADE;

-- Both readers filter shop_id + a created_at range.
CREATE INDEX idx_ai_description_usages_shop_created ON public.ai_description_usages USING btree (shop_id, created_at);

-- Default 30, not -1: existing plan rows are live and have no seeding
-- migration, so this default is the backfill. Silently granting every
-- existing tier unlimited AI on deploy is the expensive failure mode.
ALTER TABLE public.plans ADD COLUMN max_ai_descriptions_per_month integer DEFAULT 30 NOT NULL;

-- +goose Down
ALTER TABLE public.plans DROP COLUMN IF EXISTS max_ai_descriptions_per_month;
DROP TABLE IF EXISTS public.ai_description_usages;
