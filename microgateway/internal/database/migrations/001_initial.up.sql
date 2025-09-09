-- migrations/001_initial.up.sql

-- API Tokens table for gateway authentication
CREATE TABLE api_tokens (
    id SERIAL PRIMARY KEY,
    token VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    app_id INTEGER NOT NULL,
    scopes TEXT, -- JSON array of scopes
    is_active BOOLEAN DEFAULT true,
    expires_at TIMESTAMP,
    last_used_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);
CREATE INDEX idx_token_active ON api_tokens(token, is_active);
CREATE INDEX idx_app_tokens ON api_tokens(app_id, is_active);

-- Note: Token caching removed for simplicity - using direct database queries

-- Extended LLMs table
CREATE TABLE llms (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) UNIQUE NOT NULL,
    vendor VARCHAR(100) NOT NULL,
    endpoint VARCHAR(500),
    api_key_encrypted TEXT, -- Encrypted API key
    default_model VARCHAR(255),
    max_tokens INTEGER DEFAULT 4096,
    timeout_seconds INTEGER DEFAULT 30,
    retry_count INTEGER DEFAULT 3,
    is_active BOOLEAN DEFAULT true,
    monthly_budget DECIMAL(10,2),
    rate_limit_rpm INTEGER, -- Requests per minute
    metadata JSON,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);
CREATE INDEX idx_llm_active ON llms(is_active, slug);
CREATE INDEX idx_llm_vendor ON llms(vendor, is_active);

-- Apps table
CREATE TABLE apps (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    owner_email VARCHAR(255),
    is_active BOOLEAN DEFAULT true,
    monthly_budget DECIMAL(10,2),
    budget_start_date DATE,
    budget_reset_day INTEGER DEFAULT 1, -- Day of month to reset
    rate_limit_rpm INTEGER,
    allowed_ips TEXT, -- JSON array of allowed IPs
    metadata JSON,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);
CREATE INDEX idx_app_active ON apps(is_active);
CREATE INDEX idx_app_owner ON apps(owner_email);

-- Add foreign key constraints
ALTER TABLE api_tokens ADD CONSTRAINT fk_api_tokens_app_id FOREIGN KEY (app_id) REFERENCES apps(id) ON DELETE CASCADE;

-- App-LLM associations
CREATE TABLE app_llms (
    app_id INTEGER NOT NULL,
    llm_id INTEGER NOT NULL,
    is_active BOOLEAN DEFAULT true,
    custom_budget DECIMAL(10,2), -- Override app budget for specific LLM
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (app_id, llm_id)
);
ALTER TABLE app_llms ADD CONSTRAINT fk_app_llms_app_id FOREIGN KEY (app_id) REFERENCES apps(id) ON DELETE CASCADE;
ALTER TABLE app_llms ADD CONSTRAINT fk_app_llms_llm_id FOREIGN KEY (llm_id) REFERENCES llms(id) ON DELETE CASCADE;

-- Model pricing table
CREATE TABLE model_prices (
    id SERIAL PRIMARY KEY,
    vendor VARCHAR(100) NOT NULL,
    model_name VARCHAR(255) NOT NULL,
    prompt_price DECIMAL(10,8) NOT NULL, -- Price per token
    completion_price DECIMAL(10,8) NOT NULL,
    currency VARCHAR(3) DEFAULT 'USD',
    per_tokens INTEGER DEFAULT 1000, -- Price per X tokens
    effective_date DATE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX idx_model_price ON model_prices(vendor, model_name, effective_date);
CREATE INDEX idx_price_lookup ON model_prices(vendor, model_name);

-- Budget tracking table
CREATE TABLE budget_usage (
    id SERIAL PRIMARY KEY,
    app_id INTEGER NOT NULL,
    llm_id INTEGER,
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    tokens_used BIGINT DEFAULT 0,
    requests_count INTEGER DEFAULT 0,
    total_cost DECIMAL(10,4) DEFAULT 0,
    prompt_tokens BIGINT DEFAULT 0,
    completion_tokens BIGINT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
ALTER TABLE budget_usage ADD CONSTRAINT fk_budget_usage_app_id FOREIGN KEY (app_id) REFERENCES apps(id) ON DELETE CASCADE;
ALTER TABLE budget_usage ADD CONSTRAINT fk_budget_usage_llm_id FOREIGN KEY (llm_id) REFERENCES llms(id) ON DELETE SET NULL;
CREATE UNIQUE INDEX idx_budget_period ON budget_usage(app_id, llm_id, period_start, period_end);
CREATE INDEX idx_budget_app ON budget_usage(app_id, period_start);

-- Analytics events table
CREATE TABLE analytics_events (
    id BIGSERIAL PRIMARY KEY,
    request_id VARCHAR(100) UNIQUE NOT NULL,
    app_id INTEGER NOT NULL,
    llm_id INTEGER,
    credential_id INTEGER,
    endpoint VARCHAR(500),
    method VARCHAR(10),
    status_code INTEGER,
    request_tokens INTEGER,
    response_tokens INTEGER,
    total_tokens INTEGER,
    cost DECIMAL(10,6),
    latency_ms INTEGER,
    error_message TEXT,
    metadata JSON,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
ALTER TABLE analytics_events ADD CONSTRAINT fk_analytics_events_app_id FOREIGN KEY (app_id) REFERENCES apps(id) ON DELETE CASCADE;
ALTER TABLE analytics_events ADD CONSTRAINT fk_analytics_events_llm_id FOREIGN KEY (llm_id) REFERENCES llms(id) ON DELETE SET NULL;
CREATE INDEX idx_analytics_app ON analytics_events(app_id, created_at);
CREATE INDEX idx_analytics_request ON analytics_events(request_id);

-- Filters table
CREATE TABLE filters (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL, -- 'request', 'response', 'both'
    script TEXT NOT NULL, -- Filter script (Tengo or similar)
    is_active BOOLEAN DEFAULT true,
    order_index INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);
CREATE INDEX idx_filter_active ON filters(is_active, order_index);

-- LLM-Filter associations
CREATE TABLE llm_filters (
    llm_id INTEGER NOT NULL,
    filter_id INTEGER NOT NULL,
    is_active BOOLEAN DEFAULT true,
    order_index INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (llm_id, filter_id)
);
ALTER TABLE llm_filters ADD CONSTRAINT fk_llm_filters_llm_id FOREIGN KEY (llm_id) REFERENCES llms(id) ON DELETE CASCADE;
ALTER TABLE llm_filters ADD CONSTRAINT fk_llm_filters_filter_id FOREIGN KEY (filter_id) REFERENCES filters(id) ON DELETE CASCADE;