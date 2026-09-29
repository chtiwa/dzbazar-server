-- +goose Up
-- LAUNCH BLOCKER 13, OWNER DECIDED 2026-09-28: stock tracking is opt-in per
-- product. Existing products default to false — dropshippers who never set
-- stock (Variant quantity defaults to 0) must not have every order rejected
-- by the now-live server stock check. When false, both the server 409 check
-- (ordersController.go) and all client stock UI (sold-out badge, qty clamp,
-- disabled submit) are skipped; a merchant opts in via a toggle in admin
-- ProductForm.tsx once their stock numbers are actually accurate.
ALTER TABLE public.products ADD COLUMN track_stock boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE public.products DROP COLUMN track_stock;
