-- +goose Up
ALTER TABLE shop_invites ADD COLUMN expires_at TIMESTAMPTZ NOT NULL DEFAULT now() + interval '7 days';

-- +goose Down
ALTER TABLE shop_invites DROP COLUMN expires_at;
