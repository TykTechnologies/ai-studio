-- Add cache pricing columns to existing model_prices table
ALTER TABLE model_prices ADD COLUMN cache_write_pt DECIMAL(12,10) DEFAULT 0;
ALTER TABLE model_prices ADD COLUMN cache_read_pt DECIMAL(12,10) DEFAULT 0;