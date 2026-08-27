-- +goose Up
-- Per-call log of AI landing-page image generations, same shape/reasoning as
-- 00021_ai_description_usage.sql: one row per successful generation, read by
-- services.CheckLandingPageImageGenLimit (COUNT(*) for the shop's current
-- subscription period) against plans.max_ai_images_per_month. Image
-- generation costs far more per call than a text completion, so it gets its
-- own (tighter) cap rather than sharing max_ai_descriptions_per_month.
CREATE TABLE public.landing_page_image_gen_usages (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id uuid NOT NULL,
    user_id uuid
);

ALTER TABLE ONLY public.landing_page_image_gen_usages
    ADD CONSTRAINT landing_page_image_gen_usages_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.landing_page_image_gen_usages
    ADD CONSTRAINT fk_landing_page_image_gen_usages_shop FOREIGN KEY (shop_id) REFERENCES public.shops(id) ON DELETE CASCADE;

CREATE INDEX idx_landing_page_image_gen_usages_shop_created ON public.landing_page_image_gen_usages USING btree (shop_id, created_at);

-- Default 5, not -1: same backfill reasoning as max_ai_descriptions_per_month
-- — silently granting every existing tier unlimited $0.04/image generation
-- on deploy is the expensive failure mode.
ALTER TABLE public.plans ADD COLUMN max_ai_images_per_month integer DEFAULT 5 NOT NULL;

-- +goose Down
ALTER TABLE public.plans DROP COLUMN IF EXISTS max_ai_images_per_month;
DROP TABLE IF EXISTS public.landing_page_image_gen_usages;
