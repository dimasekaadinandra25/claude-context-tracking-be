ALTER TABLE usage.session_hours
    DROP COLUMN IF EXISTS input_tokens,
    DROP COLUMN IF EXISTS cache_write_tokens,
    DROP COLUMN IF EXISTS cache_read_tokens;

ALTER TABLE core.api_keys DROP COLUMN IF EXISTS app_version;
