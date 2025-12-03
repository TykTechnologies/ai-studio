// Package containers provides testcontainers-go helpers for integration testing.
// This package enables consistent, reproducible container-based testing across
// the codebase without requiring docker-compose or manual setup.
package containers

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// PGVectorContainer wraps a testcontainers PostgreSQL+pgvector instance with convenience methods.
type PGVectorContainer struct {
	testcontainers.Container
	host     string
	port     string
	user     string
	password string
	database string
}

// PGVectorConfig holds configuration for creating a PGVector container.
type PGVectorConfig struct {
	// Version specifies the pgvector image tag. Defaults to "0.8.0-pg16".
	Version string
	// User specifies the PostgreSQL user. Defaults to "testuser".
	User string
	// Password specifies the PostgreSQL password. Defaults to "testpass".
	Password string
	// Database specifies the PostgreSQL database name. Defaults to "testdb".
	Database string
}

// DefaultPGVectorConfig returns a default PGVector container configuration.
func DefaultPGVectorConfig() *PGVectorConfig {
	return &PGVectorConfig{
		Version:  "0.8.0-pg16",
		User:     "testuser",
		Password: "testpass",
		Database: "testdb",
	}
}

// NewPGVectorContainer creates and starts a new PostgreSQL container with pgvector extension.
// The container is ready for use when this function returns.
// Call Close() when done to clean up resources.
func NewPGVectorContainer(ctx context.Context, cfg *PGVectorConfig) (*PGVectorContainer, error) {
	if cfg == nil {
		cfg = DefaultPGVectorConfig()
	}

	if cfg.Version == "" {
		cfg.Version = "0.8.0-pg16"
	}
	if cfg.User == "" {
		cfg.User = "testuser"
	}
	if cfg.Password == "" {
		cfg.Password = "testpass"
	}
	if cfg.Database == "" {
		cfg.Database = "testdb"
	}

	req := testcontainers.ContainerRequest{
		Image:        fmt.Sprintf("pgvector/pgvector:%s", cfg.Version),
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     cfg.User,
			"POSTGRES_PASSWORD": cfg.Password,
			"POSTGRES_DB":       cfg.Database,
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
			wait.ForListeningPort("5432/tcp"),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start PGVector container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get mapped port: %w", err)
	}

	pgContainer := &PGVectorContainer{
		Container: container,
		host:      host,
		port:      port.Port(),
		user:      cfg.User,
		password:  cfg.Password,
		database:  cfg.Database,
	}

	// Enable pgvector extension
	if err := pgContainer.EnableExtension(ctx); err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to enable pgvector extension: %w", err)
	}

	return pgContainer, nil
}

// Host returns the container's host address.
func (p *PGVectorContainer) Host() string {
	return p.host
}

// Port returns the mapped port for PostgreSQL (as a string).
func (p *PGVectorContainer) Port() string {
	return p.port
}

// User returns the PostgreSQL user.
func (p *PGVectorContainer) User() string {
	return p.user
}

// Password returns the PostgreSQL password.
func (p *PGVectorContainer) Password() string {
	return p.password
}

// Database returns the PostgreSQL database name.
func (p *PGVectorContainer) Database() string {
	return p.database
}

// ConnectionString returns the PostgreSQL connection string.
func (p *PGVectorContainer) ConnectionString() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		p.user, p.password, p.host, p.port, p.database)
}

// Close terminates the PGVector container and releases resources.
func (p *PGVectorContainer) Close(ctx context.Context) error {
	if p.Container == nil {
		return nil
	}
	return p.Container.Terminate(ctx)
}

// Ping verifies the PostgreSQL connection is working.
func (p *PGVectorContainer) Ping(ctx context.Context) error {
	db, err := sql.Open("postgres", p.ConnectionString())
	if err != nil {
		return fmt.Errorf("failed to open connection: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	return nil
}

// EnableExtension enables the pgvector extension in the database.
func (p *PGVectorContainer) EnableExtension(ctx context.Context) error {
	db, err := sql.Open("postgres", p.ConnectionString())
	if err != nil {
		return fmt.Errorf("failed to open connection: %w", err)
	}
	defer db.Close()

	_, err = db.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {
		return fmt.Errorf("failed to create vector extension: %w", err)
	}

	return nil
}

// CreateVectorTable creates a table with the standard pgvector schema used by the codebase.
// The table will have: id (UUID), content (TEXT), embedding (vector), metadata (JSONB).
func (p *PGVectorContainer) CreateVectorTable(ctx context.Context, tableName string, dimensions int) error {
	db, err := sql.Open("postgres", p.ConnectionString())
	if err != nil {
		return fmt.Errorf("failed to open connection: %w", err)
	}
	defer db.Close()

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id VARCHAR(36) PRIMARY KEY,
			content TEXT NOT NULL,
			embedding vector(%d),
			metadata JSONB
		)
	`, tableName, dimensions)

	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create table %s: %w", tableName, err)
	}

	return nil
}

// DropTable drops a table from the database.
func (p *PGVectorContainer) DropTable(ctx context.Context, tableName string) error {
	db, err := sql.Open("postgres", p.ConnectionString())
	if err != nil {
		return fmt.Errorf("failed to open connection: %w", err)
	}
	defer db.Close()

	query := fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", tableName)
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop table %s: %w", tableName, err)
	}

	return nil
}

// TableExists checks if a table exists in the database.
func (p *PGVectorContainer) TableExists(ctx context.Context, tableName string) (bool, error) {
	db, err := sql.Open("postgres", p.ConnectionString())
	if err != nil {
		return false, fmt.Errorf("failed to open connection: %w", err)
	}
	defer db.Close()

	var exists bool
	query := `SELECT EXISTS (
		SELECT FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = $1
	)`
	err = db.QueryRowContext(ctx, query, tableName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check table existence: %w", err)
	}

	return exists, nil
}

// TableRowCount returns the number of rows in a table.
func (p *PGVectorContainer) TableRowCount(ctx context.Context, tableName string) (int, error) {
	db, err := sql.Open("postgres", p.ConnectionString())
	if err != nil {
		return 0, fmt.Errorf("failed to open connection: %w", err)
	}
	defer db.Close()

	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
	err = db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count rows in %s: %w", tableName, err)
	}

	return count, nil
}

// Exec executes a query on the database.
func (p *PGVectorContainer) Exec(ctx context.Context, query string, args ...interface{}) error {
	db, err := sql.Open("postgres", p.ConnectionString())
	if err != nil {
		return fmt.Errorf("failed to open connection: %w", err)
	}
	defer db.Close()

	_, err = db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to execute query: %w", err)
	}

	return nil
}

// Query executes a query and returns rows.
func (p *PGVectorContainer) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	db, err := sql.Open("postgres", p.ConnectionString())
	if err != nil {
		return nil, fmt.Errorf("failed to open connection: %w", err)
	}
	// Note: caller is responsible for closing rows, which keeps the connection alive
	// For production use, consider connection pooling

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	return rows, nil
}
