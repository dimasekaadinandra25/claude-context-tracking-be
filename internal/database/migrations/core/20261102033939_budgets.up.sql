CREATE TABLE usage.budgets (
    user_id             UUID PRIMARY KEY REFERENCES core.users(id) ON DELETE CASCADE,
    monthly_new_context BIGINT NOT NULL CHECK (monthly_new_context > 0),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
