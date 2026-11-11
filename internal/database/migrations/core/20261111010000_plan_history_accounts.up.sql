CREATE TABLE usage.plan_limit_readings (
    user_id              UUID NOT NULL REFERENCES core.users(id) ON DELETE CASCADE,
    captured_at          TIMESTAMPTZ NOT NULL,
    five_hour_used       DOUBLE PRECISION,
    seven_day_used       DOUBLE PRECISION,
    seven_day_resets_at  TIMESTAMPTZ,
    PRIMARY KEY (user_id, captured_at)
);

INSERT INTO usage.plan_limit_readings (user_id, captured_at, five_hour_used, seven_day_used, seven_day_resets_at)
SELECT user_id, captured_at, five_hour_used, seven_day_used, seven_day_resets_at FROM usage.plan_limits;

CREATE TABLE usage.claude_accounts (
    user_id     UUID PRIMARY KEY REFERENCES core.users(id) ON DELETE CASCADE,
    email       VARCHAR(320) NOT NULL DEFAULT '',
    plan        VARCHAR(100) NOT NULL DEFAULT '',
    tier        VARCHAR(100) NOT NULL DEFAULT '',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
