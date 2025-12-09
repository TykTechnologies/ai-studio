-- migrations/006_add_control_payloads.down.sql
-- Remove control_payloads table

DROP INDEX IF EXISTS idx_control_payload_created;
DROP INDEX IF EXISTS idx_control_payload_sent;
DROP INDEX IF EXISTS idx_control_payload_correlation;
DROP INDEX IF EXISTS idx_control_payload_plugin;
DROP TABLE IF EXISTS control_payloads;
