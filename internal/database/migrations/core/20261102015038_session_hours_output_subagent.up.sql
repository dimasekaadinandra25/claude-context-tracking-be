ALTER TABLE usage.session_hours
    ADD COLUMN output_tokens BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN subagent_requests INT NOT NULL DEFAULT 0,
    ADD COLUMN subagent_output_tokens BIGINT NOT NULL DEFAULT 0;
