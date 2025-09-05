-- migrations/001_initial.down.sql

-- Drop tables in reverse order of creation to respect foreign key constraints
DROP TABLE IF EXISTS llm_filters;
DROP TABLE IF EXISTS filters;
DROP TABLE IF EXISTS analytics_events;
DROP TABLE IF EXISTS budget_usage;
DROP TABLE IF EXISTS model_prices;
DROP TABLE IF EXISTS app_llms;
DROP TABLE IF EXISTS credentials;
DROP TABLE IF EXISTS apps;
DROP TABLE IF EXISTS llms;
DROP TABLE IF EXISTS token_cache;
DROP TABLE IF EXISTS api_tokens;