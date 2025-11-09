-- Migration: Remove plugin_type column and related indexes
-- Date: 2025-10-29
-- Description: Migrates from plugin_type taxonomy to hook-based system
--
-- This migration removes the plugin_type field which has been replaced by:
-- - hook_types (JSON array of all hook types the plugin supports)
-- - hook_types_customized (boolean flag for user overrides)
--
-- The new fields are automatically created by GORM auto-migration.

-- Drop the plugin_type column and its index
ALTER TABLE plugins DROP COLUMN IF EXISTS plugin_type;

-- Note: The following fields are added by GORM auto-migration:
-- - hook_types (JSON/TEXT depending on database)
-- - hook_types_customized (BOOLEAN, default: false)
--
-- After running this migration, all plugins will need to have their
-- hook types configured through the manifest or UI.
