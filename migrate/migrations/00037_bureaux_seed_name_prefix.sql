-- +goose Up
-- 00036 seeded bureaux named exactly after the wilaya ("Alger"). The seed
-- format is now "Bureau {wilaya}" (shopsController.go), so retag every row
-- that still matches its wilaya's bare name -- i.e. still exactly as seeded,
-- untouched by a shop owner since. A row a shop owner already renamed no
-- longer matches w.name and is left alone.
UPDATE public.bureaux b
SET name = 'Bureau ' || w.name
FROM public.wilayas w
WHERE b.wilaya_id = w.id
  AND b.name = w.name;

-- +goose Down
UPDATE public.bureaux b
SET name = w.name
FROM public.wilayas w
WHERE b.wilaya_id = w.id
  AND b.name = 'Bureau ' || w.name;
