ALTER TABLE usage.plan_limits ADD COLUMN IF NOT EXISTS model_limits JSONB NOT NULL DEFAULT '[]'::jsonb;
