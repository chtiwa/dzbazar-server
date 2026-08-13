-- +goose Up
UPDATE public.pixels p
SET is_active = false
WHERE is_active = true
  AND p.id NOT IN (
    SELECT DISTINCT ON (shop_id, platform) id
    FROM public.pixels
    WHERE is_active = true
    ORDER BY shop_id, platform, created_at DESC
  );

CREATE UNIQUE INDEX idx_pixels_one_active_per_platform
  ON public.pixels (shop_id, platform)
  WHERE is_active = true;

-- +goose Down
DROP INDEX IF EXISTS idx_pixels_one_active_per_platform;
