-- Remove cache pricing columns
ALTER TABLE model_prices DROP COLUMN cache_write_pt;
ALTER TABLE model_prices DROP COLUMN cache_read_pt;