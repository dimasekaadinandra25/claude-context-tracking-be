CREATE TABLE core.audit_logs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id    UUID,
    actor_email VARCHAR(255) NOT NULL DEFAULT '',
    action      VARCHAR(64) NOT NULL,
    target_type VARCHAR(64) NOT NULL DEFAULT '',
    target_id   VARCHAR(64) NOT NULL DEFAULT '',
    summary     TEXT NOT NULL DEFAULT '',
    ip          VARCHAR(64) NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_created_at ON core.audit_logs (created_at DESC);
