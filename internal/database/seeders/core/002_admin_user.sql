CREATE EXTENSION IF NOT EXISTS pgcrypto;

INSERT INTO core.users (email, username, password_hash, full_name, is_active, is_email_verified, email_verified_at)
VALUES (:'admin_email', 'admin', crypt(:'admin_password', gen_salt('bf', 10)), 'Administrator', TRUE, TRUE, NOW())
ON CONFLICT DO NOTHING;

INSERT INTO core.user_roles (user_id, role_id)
SELECT id, '00000000-0000-0000-0000-000000000001'
FROM core.users
WHERE email = :'admin_email' AND deleted_at IS NULL
ON CONFLICT DO NOTHING;
