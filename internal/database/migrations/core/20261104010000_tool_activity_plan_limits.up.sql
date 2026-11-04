CREATE TABLE usage.session_tool_hours (
    session_id     TEXT NOT NULL REFERENCES usage.sessions(id) ON DELETE CASCADE,
    hour           TIMESTAMPTZ NOT NULL,
    tool           VARCHAR(100) NOT NULL,
    user_id        UUID NOT NULL REFERENCES core.users(id) ON DELETE CASCADE,
    calls          INT NOT NULL DEFAULT 0,
    lines_added    BIGINT NOT NULL DEFAULT 0,
    lines_removed  BIGINT NOT NULL DEFAULT 0,
    commits        INT NOT NULL DEFAULT 0,
    PRIMARY KEY (session_id, hour, tool)
);

CREATE INDEX idx_usage_session_tool_hours_user_hour ON usage.session_tool_hours (user_id, hour);
CREATE INDEX idx_usage_session_tool_hours_hour ON usage.session_tool_hours (hour);

CREATE TABLE usage.plan_limits (
    user_id              UUID PRIMARY KEY REFERENCES core.users(id) ON DELETE CASCADE,
    five_hour_used       DOUBLE PRECISION,
    five_hour_resets_at  TIMESTAMPTZ,
    seven_day_used       DOUBLE PRECISION,
    seven_day_resets_at  TIMESTAMPTZ,
    captured_at          TIMESTAMPTZ NOT NULL
);
