# Database Configuration

The microgateway supports both SQLite and PostgreSQL databases with comprehensive configuration options for development and production environments.

## Overview

Database configuration features:
- **Multiple Database Types**: SQLite for development, PostgreSQL for production
- **Connection Pooling**: Efficient database connection management
- **Auto-Migration**: Automatic schema updates and version management
- **Performance Tuning**: Configurable connection limits and timeouts
- **High Availability**: Support for database clustering and replication
- **Security**: SSL/TLS encryption and credential management

## Database Types

### SQLite (Development)
```bash
# SQLite configuration
DATABASE_TYPE=sqlite
DATABASE_DSN=file:./data/microgateway.db?cache=shared&mode=rwc

# SQLite connection string options:
# - cache=shared: Allow multiple connections
# - mode=rwc: Read-write-create mode
# - _journal_mode=WAL: Write-ahead logging
# - _foreign_keys=on: Enable foreign key constraints
```

#### SQLite Pros and Cons
**Pros:**
- Zero external dependencies
- Simple setup and deployment
- Good for development and testing
- File-based storage

**Cons:**
- Limited concurrent write performance
- Not suitable for high-concurrency production
- Limited scalability options

### PostgreSQL (Production)
```bash
# PostgreSQL configuration
DATABASE_TYPE=postgres
DATABASE_DSN=postgres://username:password@hostname:port/database?sslmode=require

# PostgreSQL connection string options:
# - sslmode=require: Require SSL connection
# - sslmode=disable: Disable SSL (development only)
# - application_name=microgateway: Application identifier
# - connect_timeout=10: Connection timeout in seconds
```

#### PostgreSQL Pros and Cons
**Pros:**
- High-performance ACID transactions
- Excellent concurrency support
- Advanced features and extensions
- Scalability and clustering support

**Cons:**
- Requires external database service
- More complex setup and maintenance
- Additional operational overhead

## Connection Configuration

### Basic Connection Settings
```bash
# Database connection
DATABASE_TYPE=postgres
DATABASE_DSN=postgres://mgw_user:secure_password@postgres:5432/microgateway?sslmode=require

# Connection validation
DB_PING_TIMEOUT=10s
DB_CONNECT_TIMEOUT=30s
```

### Connection Pool Settings
```bash
# Connection pool configuration
DB_MAX_OPEN_CONNS=25        # Maximum open connections
DB_MAX_IDLE_CONNS=25        # Maximum idle connections
DB_CONN_MAX_LIFETIME=5m     # Maximum connection lifetime
DB_CONN_MAX_IDLE_TIME=10m   # Maximum idle time before closure
```

### Advanced Connection Options
```bash
# PostgreSQL advanced settings
DATABASE_DSN=postgres://user:pass@host:port/db?\
sslmode=require&\
pool_max_conns=25&\
pool_min_conns=5&\
pool_max_conn_lifetime=1h&\
pool_max_conn_idle_time=30m&\
application_name=microgateway&\
connect_timeout=10&\
statement_timeout=30000
```

## Database Setup

### SQLite Setup
```bash
# SQLite requires no external setup
# Database file created automatically
mkdir -p data
DATABASE_TYPE=sqlite
DATABASE_DSN=file:./data/microgateway.db?cache=shared&mode=rwc

# Run migrations
./microgateway -migrate
```

### PostgreSQL Setup
```bash
# Create database and user
sudo -u postgres psql
postgres=# CREATE DATABASE microgateway;
postgres=# CREATE USER mgw_user WITH PASSWORD 'secure_password';
postgres=# GRANT ALL PRIVILEGES ON DATABASE microgateway TO mgw_user;
postgres=# \q

# Configure connection
DATABASE_TYPE=postgres
DATABASE_DSN=postgres://mgw_user:secure_password@localhost:5432/microgateway?sslmode=require

# Run migrations
./microgateway -migrate
```

### Database Initialization
```bash
# Automatic migration on startup
DB_AUTO_MIGRATE=true

# Manual migration
./microgateway -migrate

# Check migration status
mgw system health | grep database
```

## Performance Tuning

### Connection Pool Optimization
```bash
# High-concurrency settings
DB_MAX_OPEN_CONNS=50
DB_MAX_IDLE_CONNS=50
DB_CONN_MAX_LIFETIME=10m

# Conservative settings
DB_MAX_OPEN_CONNS=10
DB_MAX_IDLE_CONNS=10
DB_CONN_MAX_LIFETIME=1h

# Monitor connection usage
mgw system metrics | grep db_connections
```

### Query Performance
```bash
# Enable query logging for analysis
DB_LOG_LEVEL=info

# PostgreSQL specific optimizations
# Add to postgresql.conf:
shared_buffers = 256MB
effective_cache_size = 1GB
random_page_cost = 1.1
checkpoint_completion_target = 0.9
wal_buffers = 16MB
```

### Index Optimization
```sql
-- Key indexes for performance
CREATE INDEX CONCURRENTLY idx_analytics_events_app_id_created_at 
ON analytics_events(app_id, created_at);

CREATE INDEX CONCURRENTLY idx_analytics_events_llm_id_created_at 
ON analytics_events(llm_id, created_at);

CREATE INDEX CONCURRENTLY idx_budget_usage_app_id_period 
ON budget_usage(app_id, period_start, period_end);

-- Namespace indexes for hub-and-spoke
CREATE INDEX CONCURRENTLY idx_llms_namespace 
ON llms(namespace);

CREATE INDEX CONCURRENTLY idx_apps_namespace 
ON apps(namespace);
```

## Security Configuration

### SSL/TLS Configuration
```bash
# Enable SSL for PostgreSQL
DATABASE_DSN=postgres://user:pass@host:port/db?sslmode=require

# SSL modes:
# - disable: No SSL
# - require: Require SSL (default for production)
# - verify-ca: Verify CA certificate
# - verify-full: Verify CA and hostname

# Client certificates
DATABASE_DSN=postgres://user:pass@host:port/db?\
sslmode=verify-full&\
sslcert=/path/to/client.crt&\
sslkey=/path/to/client.key&\
sslrootcert=/path/to/ca.crt
```

### Credential Management
```bash
# Database credentials
DB_USERNAME=mgw_user
DB_PASSWORD=secure_password
DB_HOST=postgres.company.com
DB_PORT=5432
DB_NAME=microgateway

# Construct DSN from components
DATABASE_DSN=postgres://${DB_USERNAME}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=require

# Use secret management
export DB_PASSWORD=$(vault kv get -field=password secret/microgateway/db)
```

### Database Access Control
```sql
-- Create restricted database user
CREATE USER mgw_app WITH PASSWORD 'app_password';

-- Grant minimal required permissions
GRANT CONNECT ON DATABASE microgateway TO mgw_app;
GRANT USAGE ON SCHEMA public TO mgw_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO mgw_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO mgw_app;

-- Grant permissions on future tables
ALTER DEFAULT PRIVILEGES IN SCHEMA public 
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO mgw_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public 
GRANT USAGE, SELECT ON SEQUENCES TO mgw_app;
```

## Database Maintenance

### Backup Configuration
```bash
# PostgreSQL backup
pg_dump $DATABASE_DSN > microgateway_backup_$(date +%Y%m%d).sql

# Automated backup script
#!/bin/bash
BACKUP_DIR="/var/backups/microgateway"
DATE=$(date +%Y%m%d_%H%M%S)
mkdir -p $BACKUP_DIR

pg_dump $DATABASE_DSN | gzip > $BACKUP_DIR/microgateway_$DATE.sql.gz

# Cleanup old backups (keep 30 days)
find $BACKUP_DIR -name "microgateway_*.sql.gz" -mtime +30 -delete
```

### Database Monitoring
```bash
# Monitor database connections
mgw system metrics | grep db_

# Key database metrics:
# - db_connections_open
# - db_connections_idle
# - db_connections_in_use
# - db_queries_total
# - db_query_duration_seconds

# PostgreSQL-specific monitoring
SELECT * FROM pg_stat_activity WHERE application_name = 'microgateway';
SELECT * FROM pg_stat_database WHERE datname = 'microgateway';
```

### Cleanup and Maintenance
```bash
# Analytics data cleanup (if not automatic)
DB_ANALYTICS_CLEANUP_ENABLED=true
DB_ANALYTICS_CLEANUP_INTERVAL=24h
DB_ANALYTICS_RETENTION_DAYS=90

# Manual cleanup
DELETE FROM analytics_events WHERE created_at < NOW() - INTERVAL '90 days';

# Database optimization
VACUUM ANALYZE analytics_events;
REINDEX INDEX idx_analytics_events_app_id_created_at;
```

## High Availability

### Database Clustering
```bash
# PostgreSQL streaming replication
# Primary database
DATABASE_DSN=postgres://user:pass@postgres-primary:5432/microgateway?sslmode=require

# Read replica for analytics queries
ANALYTICS_DB_DSN=postgres://user:pass@postgres-replica:5432/microgateway?sslmode=require

# Failover configuration
DATABASE_DSN=postgres://user:pass@postgres-primary:5432/microgateway?\
sslmode=require&\
target_session_attrs=read-write&\
host=postgres-secondary
```

### Connection Failover
```bash
# Multiple database hosts
DATABASE_DSN=postgres://user:pass@postgres-1:5432,postgres-2:5432/microgateway?sslmode=require

# Failover behavior
DB_FAILOVER_ENABLED=true
DB_FAILOVER_TIMEOUT=30s
DB_MAX_RETRIES=3
```

## Migration Management

### Schema Migrations
```bash
# Run migrations
./microgateway -migrate

# Check migration status
mgw system health | grep migration

# Migration configuration
DB_MIGRATION_TIMEOUT=300s
DB_MIGRATION_LOCK_TIMEOUT=60s
```

### Migration Files
```sql
-- Example migration: 001_add_namespace_support.sql
ALTER TABLE llms ADD COLUMN namespace VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE apps ADD COLUMN namespace VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE api_tokens ADD COLUMN namespace VARCHAR(255) NOT NULL DEFAULT '';

CREATE INDEX idx_llms_namespace ON llms(namespace);
CREATE INDEX idx_apps_namespace ON apps(namespace);
```

### Version Control
```bash
# Migration version tracking
SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1;

# Rollback support (manual)
# Create rollback scripts for each migration
# Test rollback procedures in development
```

## Environment-Specific Configuration

### Development Database
```bash
# SQLite for development
DATABASE_TYPE=sqlite
DATABASE_DSN=file:./data/dev.db?cache=shared&mode=rwc
DB_AUTO_MIGRATE=true
DB_LOG_LEVEL=info
```

### Staging Database
```bash
# PostgreSQL for staging
DATABASE_TYPE=postgres
DATABASE_DSN=postgres://staging_user:${STAGING_DB_PASS}@staging-db:5432/microgateway_staging?sslmode=require
DB_MAX_OPEN_CONNS=15
DB_AUTO_MIGRATE=true
```

### Production Database
```bash
# PostgreSQL for production
DATABASE_TYPE=postgres
DATABASE_DSN=postgres://prod_user:${PROD_DB_PASS}@prod-db-cluster:5432/microgateway?sslmode=require
DB_MAX_OPEN_CONNS=50
DB_MAX_IDLE_CONNS=25
DB_CONN_MAX_LIFETIME=10m
DB_AUTO_MIGRATE=false  # Manual migration control
```

## Database Monitoring

### Performance Metrics
```bash
# Monitor database performance
mgw system metrics | grep -E "db_|database_"

# PostgreSQL monitoring queries
-- Active connections
SELECT count(*) FROM pg_stat_activity 
WHERE application_name = 'microgateway';

-- Slow queries
SELECT query, mean_exec_time, calls 
FROM pg_stat_statements 
WHERE query LIKE '%microgateway%' 
ORDER BY mean_exec_time DESC LIMIT 10;

-- Database size
SELECT pg_size_pretty(pg_database_size('microgateway'));
```

### Health Monitoring
```bash
# Database health check
mgw system ready

# Connection pool status
mgw system metrics | grep db_connections

# Query performance
mgw system metrics | grep db_query_duration
```

## Troubleshooting

### Connection Issues
```bash
# Test database connectivity
psql $DATABASE_DSN

# Check connection string format
echo $DATABASE_DSN

# Verify credentials
psql -h host -U user -d database -c "SELECT 1;"
```

### Performance Issues
```bash
# Monitor connection pool exhaustion
mgw system metrics | grep db_connections_in_use

# Check for slow queries
tail -f /var/log/postgresql/postgresql.log | grep "slow query"

# Analyze query performance
EXPLAIN ANALYZE SELECT * FROM analytics_events WHERE app_id = 1;
```

### Migration Issues
```bash
# Check migration status
./microgateway -migrate --dry-run

# Manual migration rollback
# Restore from backup and replay transactions

# Migration lock issues
SELECT * FROM schema_migrations WHERE version = 'current';
```

## Best Practices

### Database Design
- Use PostgreSQL for production deployments
- Enable SSL/TLS for all database connections
- Implement proper backup and recovery procedures
- Monitor database performance and connection usage
- Use connection pooling for efficient resource utilization

### Security
- Use strong database passwords
- Limit database user permissions
- Enable SSL/TLS encryption
- Regular security updates
- Monitor database access patterns

### Performance
- Tune connection pool settings for your workload
- Monitor and optimize slow queries
- Implement proper indexing strategy
- Regular database maintenance (VACUUM, ANALYZE)
- Use read replicas for analytics queries

### Operations
- Automate database backups
- Test disaster recovery procedures
- Monitor database health and performance
- Implement database monitoring and alerting
- Plan for database scaling and growth

---

Database configuration is critical for microgateway performance and reliability. For security settings, see [Security Configuration](security.md). For performance tuning, see [Performance Tuning](performance.md).
