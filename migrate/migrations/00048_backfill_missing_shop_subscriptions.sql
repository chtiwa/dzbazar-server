-- +goose Up
-- Blocker 7 fix: shopSubscription() no longer gives a shop with no
-- shop_subscriptions row a generous free-tier fallback (services/planLimits.go
-- now returns expiredPlan, all caps 0, for ErrRecordNotFound) — cancelling a
-- subscription used to be exactly this state, which was strictly better than
-- Starter. The merchant-facing cancel route is gone, so post-migration no new
-- shop can reach "no row" through the app; this backfills any shop that got
-- there historically (pre-billing shops created before migration 00025, or
-- any other legacy gap) so it isn't instead read as "locked out" the moment
-- this deploy ships. OWNER DECIDED 2026-09-28: give each a fresh Trial
-- subscription starting now, same 2-day window shopsController.CreateShop
-- grants a brand-new shop (see line ~421-427: start := time.Now(); expires :=
-- start.AddDate(0, 0, 2)).
-- INSERT ... SELECT ... WHERE NOT EXISTS makes this idempotent-safe to rerun;
-- the pre-existing unique index on shop_subscriptions.shop_id
-- (idx_shop_subscriptions_shop_id) backstops it either way.
INSERT INTO public.shop_subscriptions (id, created_at, updated_at, shop_id, plan_id, started_at, expires_at)
SELECT
    public.uuid_generate_v4(),
    now(),
    now(),
    s.id,
    t.id,
    now(),
    now() + interval '2 days'
FROM public.shops s
-- join (not a scalar subselect) so a missing/renamed Trial plan inserts
-- nothing instead of a NULL plan_id that would fail the boot-time migration
JOIN (SELECT id FROM public.plans WHERE name = 'Trial' ORDER BY created_at LIMIT 1) t ON true
WHERE s.deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM public.shop_subscriptions sub WHERE sub.shop_id = s.id
  );

-- +goose Down
-- Only remove the rows this migration itself could have created (Trial,
-- started roughly at migration time) — never touch a subscription a merchant
-- has since renewed/upgraded onto a paid plan.
DELETE FROM public.shop_subscriptions
WHERE plan_id = (SELECT id FROM public.plans WHERE name = 'Trial' LIMIT 1)
  AND started_at >= now() - interval '1 day';
