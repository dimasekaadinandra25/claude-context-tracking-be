ALTER TABLE core.users ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE core.users SET must_change_password = TRUE WHERE username = 'admin';
