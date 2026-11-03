ALTER TABLE usage.sessions
    DROP COLUMN IF EXISTS overhead_system_tokens,
    DROP COLUMN IF EXISTS overhead_memory_tokens,
    DROP COLUMN IF EXISTS overhead_agents_tokens,
    DROP COLUMN IF EXISTS overhead_skills_tokens;
