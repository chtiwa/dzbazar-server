-- +goose Up
-- Lets an owner/moderator temporarily pull a confirmatrice out of the
-- round-robin (sick day, day off) without deleting her account or touching
-- her product scope/history. Defaults true so every existing member stays
-- eligible after this migration runs.
ALTER TABLE public.shop_members ADD COLUMN active boolean NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE public.shop_members DROP COLUMN IF EXISTS active;
