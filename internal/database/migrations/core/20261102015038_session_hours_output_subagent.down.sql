ALTER TABLE usage.session_hours
    DROP COLUMN IF EXISTS output_tokens,
    DROP COLUMN IF EXISTS subagent_requests,
    DROP COLUMN IF EXISTS subagent_output_tokens;
