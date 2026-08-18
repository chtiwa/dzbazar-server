-- +goose Up
-- Lets a super admin/support agent dismiss a flagged_clients row as a false
-- positive from the new cross-tenant fraud review page. The row itself is
-- never deleted (it's the fraud history), but resolving it must stop
-- blocking that client's future orders — see the resolved_at IS NULL check
-- added to the fbp/ttp short-circuit in ordersController.go CreateOrder.
ALTER TABLE public.flagged_clients ADD COLUMN resolved_at timestamptz;
ALTER TABLE public.flagged_clients ADD COLUMN resolved_by uuid;

ALTER TABLE ONLY public.flagged_clients
    ADD CONSTRAINT fk_flagged_clients_resolver FOREIGN KEY (resolved_by) REFERENCES public.users(id);

-- +goose Down
ALTER TABLE public.flagged_clients DROP CONSTRAINT IF EXISTS fk_flagged_clients_resolver;
ALTER TABLE public.flagged_clients DROP COLUMN IF EXISTS resolved_by;
ALTER TABLE public.flagged_clients DROP COLUMN IF EXISTS resolved_at;
