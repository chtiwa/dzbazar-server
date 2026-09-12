-- +goose Up
ALTER TABLE public.product_variant_combinations ADD COLUMN shop_id uuid;

UPDATE public.product_variant_combinations pvc
SET shop_id = p.shop_id
FROM public.products p
WHERE p.id = pvc.product_id;

ALTER TABLE public.product_variant_combinations ALTER COLUMN shop_id SET NOT NULL;

ALTER TABLE public.product_variant_combinations DROP CONSTRAINT IF EXISTS uni_product_variant_combinations_sku;
DROP INDEX IF EXISTS idx_product_variant_combinations_sku;
DROP INDEX IF EXISTS product_variant_combinations_sku_key;

CREATE UNIQUE INDEX idx_pvc_shop_sku ON public.product_variant_combinations (shop_id, sku);

-- +goose Down
DROP INDEX IF EXISTS idx_pvc_shop_sku;
ALTER TABLE public.product_variant_combinations DROP COLUMN IF EXISTS shop_id;
CREATE UNIQUE INDEX idx_product_variant_combinations_sku ON public.product_variant_combinations (sku);
