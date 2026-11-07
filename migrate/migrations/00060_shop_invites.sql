-- +goose Up
CREATE TABLE shop_invites (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    shop_id UUID NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL REFERENCES shop_roles(name),
    invited_by UUID REFERENCES users(id) ON DELETE SET NULL
);
-- one pending invite per (shop, user); accept/decline hard-deletes the row
CREATE UNIQUE INDEX idx_shop_invites_shop_user ON shop_invites (shop_id, user_id);
CREATE INDEX idx_shop_invites_user ON shop_invites (user_id);

-- +goose Down
DROP TABLE shop_invites;
