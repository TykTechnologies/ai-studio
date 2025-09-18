-- migrations/005_add_namespace_support.down.sql
-- Remove namespace support for hub-and-spoke architecture

-- Drop new tables first
DROP TABLE IF EXISTS configuration_changes;
DROP TABLE IF EXISTS edge_instances;

-- Drop composite indexes
DROP INDEX IF EXISTS idx_token_namespace_app;
DROP INDEX IF EXISTS idx_app_namespace_owner;
DROP INDEX IF EXISTS idx_llm_namespace_vendor;

-- Drop namespace indexes
DROP INDEX IF EXISTS idx_plugin_namespace;
DROP INDEX IF EXISTS idx_filter_namespace;
DROP INDEX IF EXISTS idx_model_price_namespace;
DROP INDEX IF EXISTS idx_token_namespace;
DROP INDEX IF EXISTS idx_app_namespace;
DROP INDEX IF EXISTS idx_llm_namespace;

-- Remove namespace columns from existing tables
ALTER TABLE plugins DROP COLUMN IF EXISTS namespace;
ALTER TABLE filters DROP COLUMN IF EXISTS namespace;
ALTER TABLE model_prices DROP COLUMN IF EXISTS namespace;
ALTER TABLE api_tokens DROP COLUMN IF EXISTS namespace;
ALTER TABLE apps DROP COLUMN IF EXISTS namespace;
ALTER TABLE llms DROP COLUMN IF EXISTS namespace;