ALTER TABLE usage.sessions
    ADD COLUMN overhead_system_tokens BIGINT,
    ADD COLUMN overhead_memory_tokens BIGINT,
    ADD COLUMN overhead_agents_tokens BIGINT,
    ADD COLUMN overhead_skills_tokens BIGINT;
