-- Cleanup script for soft-deleted edge instances
-- This removes any soft-deleted edge instances to allow them to re-register
-- Run this if you encounter "duplicate key value violates unique constraint" errors
-- after implementing the hard delete feature

-- PostgreSQL
DELETE FROM edge_instances WHERE deleted_at IS NOT NULL;

-- SQLite (alternative syntax if using SQLite)
-- DELETE FROM edge_instances WHERE deleted_at IS NOT NULL;
