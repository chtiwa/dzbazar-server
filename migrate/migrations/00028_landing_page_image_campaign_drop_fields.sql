-- +goose Up
-- Free-text prompt replaces the 6 structured fields entirely; campaign row
-- is gutted to ShopID/UserID/ImageCount, nothing fills the persona/brand
-- columns anymore.
ALTER TABLE public.landing_page_image_campaigns
    DROP COLUMN IF EXISTS product_name,
    DROP COLUMN IF EXISTS category,
    DROP COLUMN IF EXISTS primary_benefit,
    DROP COLUMN IF EXISTS pain_point,
    DROP COLUMN IF EXISTS audience,
    DROP COLUMN IF EXISTS offer_details,
    DROP COLUMN IF EXISTS age_range,
    DROP COLUMN IF EXISTS gender,
    DROP COLUMN IF EXISTS desires,
    DROP COLUMN IF EXISTS objections,
    DROP COLUMN IF EXISTS brand_colors,
    DROP COLUMN IF EXISTS mood,
    DROP COLUMN IF EXISTS reference_style,
    DROP COLUMN IF EXISTS custom_notes;

-- +goose Down
ALTER TABLE public.landing_page_image_campaigns
    ADD COLUMN product_name text NOT NULL DEFAULT '',
    ADD COLUMN category text NOT NULL DEFAULT '',
    ADD COLUMN primary_benefit text NOT NULL DEFAULT '',
    ADD COLUMN pain_point text NOT NULL DEFAULT '',
    ADD COLUMN audience text NOT NULL DEFAULT '',
    ADD COLUMN offer_details text NOT NULL DEFAULT '',
    ADD COLUMN age_range text,
    ADD COLUMN gender text,
    ADD COLUMN desires text,
    ADD COLUMN objections text,
    ADD COLUMN brand_colors text,
    ADD COLUMN mood text,
    ADD COLUMN reference_style text,
    ADD COLUMN custom_notes text;
