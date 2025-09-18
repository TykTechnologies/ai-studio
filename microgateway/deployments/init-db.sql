-- deployments/init-db.sql
-- Database initialization script for PostgreSQL

-- Ensure the database exists
CREATE DATABASE IF NOT EXISTS microgateway;

-- Create user if not exists (PostgreSQL specific)
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_user WHERE usename = 'gateway') THEN
        CREATE USER gateway WITH PASSWORD 'gateway123';
    END IF;
END
$$;

-- Grant permissions
GRANT ALL PRIVILEGES ON DATABASE microgateway TO gateway;

-- Connect to the microgateway database
\c microgateway;

-- Grant schema permissions
GRANT ALL ON SCHEMA public TO gateway;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO gateway;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO gateway;