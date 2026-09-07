-- +goose Up
-- The landing-page AI image generator is removed. Its tables held generated
-- image URLs and campaign grouping rows. Dropped rather than left orphaned.
DROP TABLE IF EXISTS public.landing_page_image_gen_usages;
DROP TABLE IF EXISTS public.landing_page_image_campaigns;

-- +goose Down
-- Irreversible by design: the rows are gone, recreating empty tables would
-- claim a rollback that doesn't restore anything.
SELECT 1;
