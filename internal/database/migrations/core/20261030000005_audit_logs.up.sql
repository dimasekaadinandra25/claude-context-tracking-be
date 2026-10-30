CREATE TABLE IF NOT EXISTS core.audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reff_type VARCHAR(100) NOT NULL,
    reff_id UUID NOT NULL,
    reff_no VARCHAR(50),
    action VARCHAR(40) NOT NULL,
    summary TEXT,
    changes JSONB,
    metadata JSONB,
    actor_id UUID,
    actor_name VARCHAR(150),
    actor_role VARCHAR(100),
    ip_address INET,
    user_agent TEXT,
    request_id VARCHAR(64),
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_audit_logs_action CHECK (action ~ '^[a-z][a-z0-9_]*$'),
    CONSTRAINT chk_audit_logs_reff_type CHECK (reff_type ~ '^[a-z][a-z0-9_.]*$')
);

CREATE INDEX idx_audit_logs_reff ON core.audit_logs (reff_type, reff_id, occurred_at DESC);
CREATE INDEX idx_audit_logs_actor ON core.audit_logs (actor_id, occurred_at DESC) WHERE actor_id IS NOT NULL;
CREATE INDEX idx_audit_logs_time ON core.audit_logs (occurred_at DESC);
CREATE INDEX idx_audit_logs_action ON core.audit_logs (action, occurred_at DESC);
