-- +goose Up
-- Groups the images from one generate-image-set call so the merchant can
-- fetch the set as a unit and regenerate it later. Inputs (product context,
-- persona, brand prefs) are persisted here — nowhere else — since regenerate
-- needs them; the existing per-image usage rows stay flat (just campaign_id
-- + image_index) rather than duplicating these columns onto every image row.
CREATE TABLE public.landing_page_image_campaigns (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id uuid NOT NULL,
    user_id uuid,
    product_name text NOT NULL,
    category text NOT NULL,
    primary_benefit text NOT NULL,
    pain_point text NOT NULL,
    audience text NOT NULL,
    offer_details text NOT NULL,
    age_range text,
    gender text,
    desires text,
    objections text,
    brand_colors text,
    mood text,
    reference_style text,
    custom_notes text,
    image_count integer NOT NULL
);
ALTER TABLE ONLY public.landing_page_image_campaigns
    ADD CONSTRAINT landing_page_image_campaigns_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.landing_page_image_campaigns
    ADD CONSTRAINT fk_landing_page_image_campaigns_shop FOREIGN KEY (shop_id) REFERENCES public.shops(id) ON DELETE CASCADE;
CREATE INDEX idx_landing_page_image_campaigns_shop_created ON public.landing_page_image_campaigns USING btree (shop_id, created_at);

-- ON DELETE SET NULL (not CASCADE): usage rows are the quota ledger
-- (CheckLandingPageImageGenBudget counts them). Deleting a campaign must
-- never delete usage rows, or a merchant could reset their own quota by
-- deleting campaigns.
ALTER TABLE public.landing_page_image_gen_usages ADD COLUMN campaign_id uuid;
ALTER TABLE public.landing_page_image_gen_usages ADD COLUMN image_index integer DEFAULT 0 NOT NULL;
ALTER TABLE ONLY public.landing_page_image_gen_usages
    ADD CONSTRAINT fk_landing_page_image_gen_usages_campaign FOREIGN KEY (campaign_id) REFERENCES public.landing_page_image_campaigns(id) ON DELETE SET NULL;
CREATE INDEX idx_landing_page_image_gen_usages_campaign ON public.landing_page_image_gen_usages USING btree (campaign_id, image_index);

-- +goose Down
DROP INDEX IF EXISTS idx_landing_page_image_gen_usages_campaign;
ALTER TABLE public.landing_page_image_gen_usages DROP CONSTRAINT IF EXISTS fk_landing_page_image_gen_usages_campaign;
ALTER TABLE public.landing_page_image_gen_usages DROP COLUMN IF EXISTS image_index;
ALTER TABLE public.landing_page_image_gen_usages DROP COLUMN IF EXISTS campaign_id;
DROP TABLE IF EXISTS public.landing_page_image_campaigns;
