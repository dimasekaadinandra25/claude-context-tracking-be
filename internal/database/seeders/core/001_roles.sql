INSERT INTO core.roles (id, code, name, description, permissions, is_system, is_active) VALUES
(
    '00000000-0000-0000-0000-000000000001',
    'super_admin',
    'Super Administrator',
    'Full access, bypasses all permission checks.',
    '{}'::jsonb,
    TRUE,
    TRUE
),
(
    '00000000-0000-0000-0000-000000000003',
    'member',
    'Member',
    'Sees only their own usage.',
    '{"usage":"viewer"}'::jsonb,
    TRUE,
    TRUE
)
ON CONFLICT DO NOTHING;
