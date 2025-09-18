-- Drop indexes
DROP INDEX IF EXISTS idx_llm_plugins_order;
DROP INDEX IF EXISTS idx_llm_plugins_llm_id;
DROP INDEX IF EXISTS idx_plugins_deleted_at;
DROP INDEX IF EXISTS idx_plugins_is_active;
DROP INDEX IF EXISTS idx_plugins_hook_type;

-- Drop tables in reverse order
DROP TABLE IF EXISTS llm_plugins;
DROP TABLE IF EXISTS plugins;