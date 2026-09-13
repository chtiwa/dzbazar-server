-- +goose Up
CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    recipient_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    shop_id UUID NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    reference_id UUID NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_notifications_recipient_unread ON notifications (recipient_user_id) WHERE read_at IS NULL;
CREATE INDEX idx_notifications_recipient_created ON notifications (recipient_user_id, created_at DESC);

-- +goose Down
DROP TABLE notifications;
