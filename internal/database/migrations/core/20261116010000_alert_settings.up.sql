CREATE TABLE usage.alert_settings (
    id            BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
    alerts        TEXT NOT NULL,
    runaway_share INTEGER NOT NULL CHECK (runaway_share BETWEEN 1 AND 100),
    project_share INTEGER NOT NULL CHECK (project_share BETWEEN 1 AND 100),
    updated_by    UUID REFERENCES core.users(id) ON DELETE SET NULL,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
