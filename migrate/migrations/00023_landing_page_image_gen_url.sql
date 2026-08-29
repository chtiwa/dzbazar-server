-- +goose Up
-- Every AI image generation already logs a usage row (00022). Store the
-- generated image's B2 URL on that same row so merchants can browse every
-- AI-generated image from one gallery, whether or not they went on to use it
-- in a landing page (the frontend re-uploads a chosen image as a fresh file,
-- so there's no reliable "used" signal to track without a bigger change).
ALTER TABLE public.landing_page_image_gen_usages ADD COLUMN url text;

-- +goose Down
ALTER TABLE public.landing_page_image_gen_usages DROP COLUMN IF EXISTS url;
