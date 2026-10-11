-- +goose Up
-- Wilaya the order was placed for, frozen at creation. clients.state is
-- overwritten on every repeat order, so per-wilaya KPIs must not read it.
ALTER TABLE orders ADD COLUMN state text NOT NULL DEFAULT '';
UPDATE orders o SET state = c.state FROM clients c WHERE c.id = o.client_id;

-- +goose Down
ALTER TABLE orders DROP COLUMN state;
