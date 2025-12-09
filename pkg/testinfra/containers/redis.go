// Package containers provides testcontainers-go helpers for integration testing.
// This package enables consistent, reproducible container-based testing across
// the codebase without requiring docker-compose or manual setup.
package containers

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// RedisContainer wraps a testcontainers Redis instance with convenience methods.
type RedisContainer struct {
	testcontainers.Container
	host     string
	port     string
	password string
}

// RedisConfig holds configuration for creating a Redis container.
type RedisConfig struct {
	// Password sets the Redis password. Empty string means no authentication.
	Password string
	// Version specifies the Redis image tag (e.g., "7.2-alpine"). Defaults to "7.2-alpine".
	Version string
	// Database selects the Redis database number. Defaults to 0.
	Database int
}

// DefaultRedisConfig returns a default Redis container configuration.
func DefaultRedisConfig() *RedisConfig {
	return &RedisConfig{
		Password: "",
		Version:  "7.2-alpine",
		Database: 0,
	}
}

// NewRedisContainer creates and starts a new Redis container.
// The container is ready for use when this function returns.
// Call Close() when done to clean up resources.
func NewRedisContainer(ctx context.Context, cfg *RedisConfig) (*RedisContainer, error) {
	if cfg == nil {
		cfg = DefaultRedisConfig()
	}

	if cfg.Version == "" {
		cfg.Version = "7.2-alpine"
	}

	req := testcontainers.ContainerRequest{
		Image:        fmt.Sprintf("redis:%s", cfg.Version),
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor: wait.ForAll(
			wait.ForLog("Ready to accept connections").WithStartupTimeout(60*time.Second),
			wait.ForListeningPort("6379/tcp"),
		),
	}

	// Add password if specified
	if cfg.Password != "" {
		req.Cmd = []string{"redis-server", "--requirepass", cfg.Password}
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start Redis container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	port, err := container.MappedPort(ctx, "6379")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get mapped port: %w", err)
	}

	return &RedisContainer{
		Container: container,
		host:      host,
		port:      port.Port(),
		password:  cfg.Password,
	}, nil
}

// Host returns the container's host address.
func (r *RedisContainer) Host() string {
	return r.host
}

// Port returns the mapped port for Redis (as a string).
func (r *RedisContainer) Port() string {
	return r.port
}

// Addr returns the full address in "host:port" format.
func (r *RedisContainer) Addr() string {
	return fmt.Sprintf("%s:%s", r.host, r.port)
}

// Password returns the configured password (empty string if no auth).
func (r *RedisContainer) Password() string {
	return r.password
}

// Close terminates the Redis container and releases resources.
func (r *RedisContainer) Close(ctx context.Context) error {
	if r.Container == nil {
		return nil
	}
	return r.Container.Terminate(ctx)
}

// FlushAll clears all data from Redis. Useful between tests.
func (r *RedisContainer) FlushAll(ctx context.Context) error {
	cmd := []string{"redis-cli", "FLUSHALL"}
	if r.password != "" {
		cmd = []string{"redis-cli", "-a", r.password, "FLUSHALL"}
	}

	exitCode, _, err := r.Container.Exec(ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to execute FLUSHALL: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("FLUSHALL returned exit code %d", exitCode)
	}
	return nil
}

// Ping verifies the Redis connection is working.
func (r *RedisContainer) Ping(ctx context.Context) error {
	cmd := []string{"redis-cli", "PING"}
	if r.password != "" {
		cmd = []string{"redis-cli", "-a", r.password, "PING"}
	}

	exitCode, _, err := r.Container.Exec(ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to execute PING: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("PING returned exit code %d", exitCode)
	}
	return nil
}
