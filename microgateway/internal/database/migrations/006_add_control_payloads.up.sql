-- migrations/006_add_control_payloads.up.sql
-- Add control_payloads table for edge-to-control plugin data transmission

CREATE TABLE control_payloads (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    plugin_id INTEGER NOT NULL,
    payload BLOB NOT NULL,
    correlation_id VARCHAR(255),
    metadata JSON,
    sent BOOLEAN DEFAULT FALSE,
    sent_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for efficient querying
CREATE INDEX idx_control_payload_plugin ON control_payloads(plugin_id);
CREATE INDEX idx_control_payload_correlation ON control_payloads(correlation_id);
CREATE INDEX idx_control_payload_sent ON control_payloads(sent, created_at);
CREATE INDEX idx_control_payload_created ON control_payloads(created_at);
