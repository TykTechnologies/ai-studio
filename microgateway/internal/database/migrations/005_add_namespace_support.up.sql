-- migrations/005_add_namespace_support.up.sql
-- Add namespace support for hub-and-spoke architecture

-- Add namespace column to LLMs table
ALTER TABLE llms ADD COLUMN namespace VARCHAR(255) NOT NULL DEFAULT '';
CREATE INDEX idx_llm_namespace ON llms(namespace, is_active);

-- Add namespace column to Apps table
ALTER TABLE apps ADD COLUMN namespace VARCHAR(255) NOT NULL DEFAULT '';
CREATE INDEX idx_app_namespace ON apps(namespace, is_active);

-- Add namespace column to API tokens table
ALTER TABLE api_tokens ADD COLUMN namespace VARCHAR(255) NOT NULL DEFAULT '';
CREATE INDEX idx_token_namespace ON api_tokens(namespace, is_active);

-- Add namespace column to Model prices table
ALTER TABLE model_prices ADD COLUMN namespace VARCHAR(255) NOT NULL DEFAULT '';
CREATE INDEX idx_model_price_namespace ON model_prices(namespace);

-- Add namespace column to Filters table
ALTER TABLE filters ADD COLUMN namespace VARCHAR(255) NOT NULL DEFAULT '';
CREATE INDEX idx_filter_namespace ON filters(namespace, is_active);

-- Add namespace column to Plugins table
ALTER TABLE plugins ADD COLUMN namespace VARCHAR(255) NOT NULL DEFAULT '';
CREATE INDEX idx_plugin_namespace ON plugins(namespace, is_active);

-- Add composite indexes for efficient namespace-based queries
CREATE INDEX idx_llm_namespace_vendor ON llms(namespace, vendor, is_active);
CREATE INDEX idx_app_namespace_owner ON apps(namespace, owner_email, is_active);
CREATE INDEX idx_token_namespace_app ON api_tokens(namespace, app_id, is_active);

-- Create table to track edge instance registrations (for control instances)
CREATE TABLE edge_instances (
    id SERIAL PRIMARY KEY,
    edge_id VARCHAR(255) UNIQUE NOT NULL,
    namespace VARCHAR(255) NOT NULL DEFAULT '',
    version VARCHAR(100),
    build_hash VARCHAR(64),
    metadata JSON,
    last_heartbeat TIMESTAMP,
    status VARCHAR(50) DEFAULT 'registered', -- registered, connected, disconnected, unhealthy
    session_id VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_edge_instances_namespace ON edge_instances(namespace, status);
CREATE INDEX idx_edge_instances_heartbeat ON edge_instances(last_heartbeat, status);

-- Create table to track configuration changes for propagation
CREATE TABLE configuration_changes (
    id SERIAL PRIMARY KEY,
    change_type VARCHAR(20) NOT NULL, -- CREATE, UPDATE, DELETE
    entity_type VARCHAR(50) NOT NULL, -- LLM, APP, TOKEN, MODEL_PRICE, FILTER, PLUGIN
    entity_id INTEGER NOT NULL,
    entity_data JSON, -- Complete serialized entity data
    namespace VARCHAR(255) NOT NULL DEFAULT '',
    propagated_to_edges JSON, -- Array of edge_ids that have received this change
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    processed BOOLEAN DEFAULT FALSE
);

CREATE INDEX idx_config_changes_namespace ON configuration_changes(namespace, processed, created_at);
CREATE INDEX idx_config_changes_type ON configuration_changes(entity_type, entity_id, created_at);
CREATE INDEX idx_config_changes_processed ON configuration_changes(processed, created_at);