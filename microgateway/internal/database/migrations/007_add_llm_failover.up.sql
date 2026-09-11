-- migrations/007_add_llm_failover.up.sql

-- Add the failover waterfall JSON column to llms (synced from the hub's LLM config).
ALTER TABLE llms ADD COLUMN failover JSON;

-- Analytics events carry the failover marker so the hub can tell a fallback
-- attempt from a primary one.
ALTER TABLE analytics_events ADD COLUMN failover_from_llm_id INTEGER;
ALTER TABLE analytics_events ADD COLUMN failover_attempt INTEGER DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_analytics_failover_from ON analytics_events(failover_from_llm_id);

COMMENT ON COLUMN llms.failover IS 'JSON LLMFailover waterfall: ordered (llm_id, model) targets plus triggers';
