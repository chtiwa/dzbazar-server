-- +goose Up
-- A delivery rate with both prices at 0 is never an intentional free-shipping
-- setup (the admin form has no such affordance) -- deactivate any that slipped
-- in before UpdateDeliveryRate/BulkUpdateDeliveryRates started enforcing this.
UPDATE public.delivery_rates
SET is_active = false
WHERE is_active = true
  AND doorstep_rate = 0
  AND stopdesk_rate = 0;

-- +goose Down
-- Not reversible: cannot tell which of these rows was active before this
-- migration touched it.
