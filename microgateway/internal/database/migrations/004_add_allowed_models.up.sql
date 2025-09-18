-- migrations/004_add_allowed_models.up.sql

-- Add allowed_models JSON column to llms table
ALTER TABLE llms ADD COLUMN allowed_models JSON;

-- Add comment for documentation
COMMENT ON COLUMN llms.allowed_models IS 'JSON array of regex patterns for allowed models';