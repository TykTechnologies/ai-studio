-- migrations/004_add_allowed_models.down.sql

-- Remove allowed_models column from llms table
ALTER TABLE llms DROP COLUMN IF EXISTS allowed_models;