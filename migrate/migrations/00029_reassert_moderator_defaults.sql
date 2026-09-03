-- +goose Up
-- Moderator should mirror owner on every non-delete action. Re-assert to
-- UPDATE (not just insert) in case a role default got toggled off after
-- 00009 seeded it.
UPDATE public.role_action_defaults
    SET allow = true
    WHERE role = 'moderator' AND action NOT LIKE '%.delete';

INSERT INTO public.role_action_defaults (role, action, allow)
    SELECT 'moderator', name, true FROM public.permission_actions
    WHERE name NOT LIKE '%.delete'
    ON CONFLICT (role, action) DO NOTHING;

-- +goose Down
-- No-op: reverting to a possibly-wrong prior state isn't meaningful here.
