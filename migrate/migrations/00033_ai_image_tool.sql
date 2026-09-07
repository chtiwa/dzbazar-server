-- +goose Up
-- Per-call log of AI image-tool generations, same shape/reasoning as
-- 00021_ai_description_usage.sql: one row per successful generation. Reused
-- by services.CheckCreditBudget for the shop's current subscription period.
-- The merchant's reference photo is never persisted anywhere, including here
-- — only the prompt, for audit.
CREATE TABLE public.ai_image_tool_usages (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id uuid NOT NULL,
    user_id uuid,
    prompt text DEFAULT ''::text NOT NULL
);

ALTER TABLE ONLY public.ai_image_tool_usages
    ADD CONSTRAINT ai_image_tool_usages_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.ai_image_tool_usages
    ADD CONSTRAINT fk_ai_image_tool_usages_shop FOREIGN KEY (shop_id) REFERENCES public.shops(id) ON DELETE CASCADE;

CREATE INDEX idx_ai_image_tool_usages_shop_created ON public.ai_image_tool_usages USING btree (shop_id, created_at);

INSERT INTO public.permission_actions (name, resource, label) VALUES
    ('ai_image_tool.use', 'ai_image_tool', 'Use the AI image generator')
    ON CONFLICT (name) DO UPDATE SET resource = EXCLUDED.resource, label = EXCLUDED.label;

INSERT INTO public.role_action_defaults (role, action, allow) VALUES
    ('owner', 'ai_image_tool.use', true),
    ('moderator', 'ai_image_tool.use', true)
    ON CONFLICT (role, action) DO NOTHING;

-- +goose Down
DELETE FROM public.role_action_defaults WHERE action = 'ai_image_tool.use';
DELETE FROM public.permission_actions WHERE name = 'ai_image_tool.use';
DROP TABLE IF EXISTS public.ai_image_tool_usages;
