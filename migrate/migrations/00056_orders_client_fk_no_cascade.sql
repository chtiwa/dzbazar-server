-- +goose Up
-- Deleting a client used to cascade-delete every order they ever placed
-- (shipped/COD history, plan order quota). NO ACTION (not RESTRICT) so a
-- shop-level cascade, which removes clients and orders in one statement, still works.
ALTER TABLE public.orders DROP CONSTRAINT fk_clients_orders;
ALTER TABLE public.orders
    ADD CONSTRAINT fk_clients_orders FOREIGN KEY (client_id) REFERENCES public.clients(id);

-- +goose Down
ALTER TABLE public.orders DROP CONSTRAINT fk_clients_orders;
ALTER TABLE public.orders
    ADD CONSTRAINT fk_clients_orders FOREIGN KEY (client_id) REFERENCES public.clients(id) ON DELETE CASCADE;
