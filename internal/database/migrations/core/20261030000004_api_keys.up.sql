CREATE TYPE core.api_key_environment AS ENUM ('live', 'test');

CREATE TABLE IF NOT EXISTS core.api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES core.users(id) ON DELETE CASCADE,
    key_id VARCHAR(16) NOT NULL,
    secret_hash VARCHAR(64) NOT NULL,
    key_prefix VARCHAR(50) NOT NULL,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    environment core.api_key_environment DEFAULT 'live',
    scoped_permissions JSONB DEFAULT '[]',
    ip_whitelist TEXT[],
    rate_limit INT DEFAULT 1000,
    rate_limit_window INT DEFAULT 3600,
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    revoked_by UUID REFERENCES core.users(id),
    revoke_reason TEXT,
    last_used_at TIMESTAMPTZ,
    last_used_ip VARCHAR(45),
    total_requests BIGINT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    created_by UUID,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    updated_by UUID,
    deleted_at TIMESTAMPTZ,
    deleted_by UUID
);

CREATE UNIQUE INDEX api_keys_key_id_key ON core.api_keys(key_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX api_keys_secret_hash_key ON core.api_keys(secret_hash) WHERE deleted_at IS NULL;
CREATE INDEX idx_api_keys_user_id ON core.api_keys(user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_api_keys_environment ON core.api_keys(environment) WHERE deleted_at IS NULL;
CREATE INDEX idx_api_keys_expires_at ON core.api_keys(expires_at) WHERE deleted_at IS NULL AND expires_at IS NOT NULL;
CREATE INDEX idx_api_keys_scoped_permissions ON core.api_keys USING GIN(scoped_permissions);
