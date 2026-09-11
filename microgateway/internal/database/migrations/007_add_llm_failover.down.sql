-- migrations/007_add_llm_failover.down.sql

DROP INDEX IF EXISTS idx_analytics_failover_from;
ALTER TABLE analytics_events DROP COLUMN failover_attempt;
ALTER TABLE analytics_events DROP COLUMN failover_from_llm_id;
ALTER TABLE llms DROP COLUMN failover;
