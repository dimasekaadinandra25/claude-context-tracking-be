CREATE SCHEMA IF NOT EXISTS usage;

CREATE TABLE usage.sessions (
    id                  TEXT PRIMARY KEY,
    user_id             UUID NOT NULL REFERENCES core.users(id) ON DELETE CASCADE,
    machine             VARCHAR(255) NOT NULL,
    project             TEXT NOT NULL DEFAULT '',
    title               TEXT NOT NULL DEFAULT '',
    model               VARCHAR(100) NOT NULL,
    context_window      BIGINT NOT NULL DEFAULT 0,
    start_context       BIGINT NOT NULL DEFAULT 0,
    latest_context      BIGINT NOT NULL DEFAULT 0,
    peak_context        BIGINT NOT NULL DEFAULT 0,
    new_context         BIGINT NOT NULL DEFAULT 0,
    requests            INT NOT NULL DEFAULT 0,
    compactions         INT NOT NULL DEFAULT 0,
    first_request_at    TIMESTAMPTZ NOT NULL,
    last_request_at     TIMESTAMPTZ NOT NULL,
    last_reported_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_usage_sessions_user_last ON usage.sessions (user_id, last_request_at DESC);
CREATE INDEX idx_usage_sessions_last ON usage.sessions (last_request_at DESC);
CREATE INDEX idx_usage_sessions_model ON usage.sessions (model);

CREATE TABLE usage.session_hours (
    session_id          TEXT NOT NULL REFERENCES usage.sessions(id) ON DELETE CASCADE,
    hour                TIMESTAMPTZ NOT NULL,
    model               VARCHAR(100) NOT NULL,
    user_id             UUID NOT NULL REFERENCES core.users(id) ON DELETE CASCADE,
    new_context         BIGINT NOT NULL DEFAULT 0,
    requests            INT NOT NULL DEFAULT 0,
    PRIMARY KEY (session_id, hour, model)
);

CREATE INDEX idx_usage_session_hours_user_hour ON usage.session_hours (user_id, hour);
CREATE INDEX idx_usage_session_hours_hour ON usage.session_hours (hour);
CREATE INDEX idx_usage_session_hours_model_hour ON usage.session_hours (model, hour);
