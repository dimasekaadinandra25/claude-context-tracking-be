CREATE TABLE IF NOT EXISTS core.roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(50) NOT NULL,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    permissions JSONB DEFAULT '{}',
    is_system BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    created_by UUID,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    updated_by UUID,
    deleted_at TIMESTAMPTZ,
    deleted_by UUID
);

CREATE UNIQUE INDEX roles_code_key ON core.roles(code) WHERE deleted_at IS NULL;
CREATE INDEX idx_roles_is_system ON core.roles(is_system) WHERE deleted_at IS NULL;
CREATE INDEX idx_roles_is_active ON core.roles(is_active) WHERE deleted_at IS NULL;
CREATE INDEX idx_roles_permissions ON core.roles USING GIN(permissions);

CREATE TABLE IF NOT EXISTS core.user_roles (
    user_id UUID NOT NULL REFERENCES core.users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES core.roles(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    created_by UUID,
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX idx_user_roles_role_id ON core.user_roles(role_id);

CREATE OR REPLACE FUNCTION core.has_permission(
    p_permissions JSONB,
    p_resource VARCHAR,
    p_level VARCHAR DEFAULT NULL
) RETURNS BOOLEAN AS $$
BEGIN
    IF p_level IS NULL THEN
        RETURN p_permissions ? p_resource;
    ELSE
        RETURN p_permissions->>p_resource = p_level;
    END IF;
END;
$$ LANGUAGE plpgsql IMMUTABLE;
