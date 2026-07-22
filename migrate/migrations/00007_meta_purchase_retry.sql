-- +goose Up
-- Retry/reconciliation for the Meta CAPI Purchase send (orderEvents.go):
-- meta_purchase_sent_at already existed but was write-only — nothing ever
-- read it to retry a failed send. meta_purchase_attempts caps how many times
-- the periodic sweep (StartMetaPurchaseRetrySweep) will retry a given order
-- before giving up, so a permanently-misconfigured pixel doesn't retry
-- forever.
ALTER TABLE public.orders ADD COLUMN meta_purchase_attempts integer NOT NULL DEFAULT 0;

-- Actual page URL the customer's browser was on at checkout, captured
-- client-side (window.location.href) and sent up with the order. Replaces
-- the hardcoded literal domains previously baked into sendFacebookPurchase.go
-- (event_source_url) and sendTiktokPurchase.go (page.url) — this app is a
-- single multi-tenant client serving shops by slug, not one storefront
-- domain per shop, so there is no per-shop "domain" column to read instead;
-- the browser's own URL at order time is the only accurate source.
ALTER TABLE public.orders ADD COLUMN page_url text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE public.orders DROP COLUMN IF EXISTS page_url;
ALTER TABLE public.orders DROP COLUMN IF EXISTS meta_purchase_attempts;
