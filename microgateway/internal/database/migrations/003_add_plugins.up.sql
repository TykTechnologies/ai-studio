-- Create plugins table
CREATE TABLE plugins (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,
    command VARCHAR(500) NOT NULL,
    checksum VARCHAR(255),
    config JSON,
    hook_type VARCHAR(50) NOT NULL,
    is_active BOOLEAN NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    deleted_at TIMESTAMP
);

-- Create indexes for plugins table
CREATE INDEX idx_plugins_hook_type ON plugins(hook_type);
CREATE INDEX idx_plugins_is_active ON plugins(is_active);
CREATE INDEX idx_plugins_deleted_at ON plugins(deleted_at);

-- Create llm_plugins association table
CREATE TABLE llm_plugins (
    llm_id INTEGER REFERENCES llms(id) ON DELETE CASCADE,
    plugin_id INTEGER REFERENCES plugins(id) ON DELETE CASCADE,
    order_index INTEGER DEFAULT 0,
    is_active BOOLEAN DEFAULT true,
    config_override JSON,
    created_at TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY (llm_id, plugin_id)
);

-- Create indexes for llm_plugins table
CREATE INDEX idx_llm_plugins_llm_id ON llm_plugins(llm_id);
CREATE INDEX idx_llm_plugins_order ON llm_plugins(llm_id, order_index);