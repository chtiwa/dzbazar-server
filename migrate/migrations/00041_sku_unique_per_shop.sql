-- +goose Up
ALTER TABLE public.product_variant_combinations ADD COLUMN shop_id uuid;

UPDATE public.product_variant_combinations pvc
SET shop_id = p.shop_id
FROM public.products p
WHERE p.id = pvc.product_id;

ALTER TABLE public.product_variant_combinations ALTER COLUMN shop_id SET NOT NULL;

DO $$
DECLARE idx_name text;
BEGIN
  SELECT indexname INTO idx_name
  FROM pg_indexes
  WHERE tablename = 'product_variant_combinations'
    AND indexdef LIKE '%UNIQUE%(sku)%';
  IF idx_name IS NOT NULL THEN
    EXECUTE format('DROP INDEX IF EXISTS %I', idx_name);
  END IF;
END $$;

CREATE UNIQUE INDEX idx_pvc_shop_sku ON public.product_variant_combinations (shop_id, sku);

-- +goose Down
DROP INDEX IF EXISTS idx_pvc_shop_sku;
ALTER TABLE public.product_variant_combinations DROP COLUMN IF EXISTS shop_id;
CREATE UNIQUE INDEX idx_product_variant_combinations_sku ON public.product_variant_combinations (sku);
