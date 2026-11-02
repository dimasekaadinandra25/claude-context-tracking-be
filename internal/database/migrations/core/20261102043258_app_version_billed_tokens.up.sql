ALTER TABLE core.api_keys ADD COLUMN app_version TEXT;

ALTER TABLE usage.session_hours
    ADD COLUMN input_tokens BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN cache_write_tokens BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN cache_read_tokens BIGINT NOT NULL DEFAULT 0;
