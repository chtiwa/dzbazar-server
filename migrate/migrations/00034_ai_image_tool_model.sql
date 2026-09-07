-- +goose Up
-- Per-call model choice, so credit cost can be billed per model instead of a
-- flat rate. Existing rows default to '' — services.creditCostForModel treats
-- an unrecognized/empty model as the priciest tier (conservative billing).
ALTER TABLE public.ai_image_tool_usages ADD COLUMN model text DEFAULT ''::text NOT NULL;

-- +goose Down
ALTER TABLE public.ai_image_tool_usages DROP COLUMN IF EXISTS model;
