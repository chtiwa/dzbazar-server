-- +goose Up
CREATE TABLE public.ai_chat_usages (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    shop_id uuid NOT NULL,
    user_id uuid,
    prompt_tokens integer DEFAULT 0 NOT NULL,
    completion_tokens integer DEFAULT 0 NOT NULL,
    total_tokens integer DEFAULT 0 NOT NULL
);

ALTER TABLE ONLY public.ai_chat_usages
    ADD CONSTRAINT ai_chat_usages_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.ai_chat_usages
    ADD CONSTRAINT fk_ai_chat_usages_shop FOREIGN KEY (shop_id) REFERENCES public.shops(id) ON DELETE CASCADE;
CREATE INDEX idx_ai_chat_usages_shop_created ON public.ai_chat_usages USING btree (shop_id, created_at);

-- Chat actions (the assistant reading live shop data) are Pro-tier only.
ALTER TABLE public.plans ADD COLUMN has_assistant_actions boolean NOT NULL DEFAULT false;
UPDATE public.plans SET has_assistant_actions = true WHERE name = 'Pro';

-- +goose Down
ALTER TABLE public.plans DROP COLUMN has_assistant_actions;
DROP TABLE IF EXISTS public.ai_chat_usages;
