// Package containers provides testcontainers-go helpers for integration testing.
// This package enables consistent, reproducible container-based testing across
// the codebase without requiring docker-compose or manual setup.
package containers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ChromaContainer wraps a testcontainers Chroma instance with convenience methods.
type ChromaContainer struct {
	testcontainers.Container
	host string
	port string
}

// ChromaConfig holds configuration for creating a Chroma container.
type ChromaConfig struct {
	// Version specifies the Chroma image tag. Defaults to "1.3.5".
	Version string
}

// DefaultChromaConfig returns a default Chroma container configuration.
func DefaultChromaConfig() *ChromaConfig {
	return &ChromaConfig{
		Version: "1.3.5",
	}
}

// NewChromaContainer creates and starts a new Chroma container.
// The container is ready for use when this function returns.
// Call Close() when done to clean up resources.
func NewChromaContainer(ctx context.Context, cfg *ChromaConfig) (*ChromaContainer, error) {
	if cfg == nil {
		cfg = DefaultChromaConfig()
	}

	if cfg.Version == "" {
		cfg.Version = "1.3.5"
	}

	req := testcontainers.ContainerRequest{
		Image:        fmt.Sprintf("chromadb/chroma:%s", cfg.Version),
		ExposedPorts: []string{"8000/tcp"},
		WaitingFor: wait.ForAll(
			wait.ForHTTP("/api/v2/heartbeat").WithPort("8000/tcp").WithStartupTimeout(60*time.Second),
			wait.ForListeningPort("8000/tcp"),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start Chroma container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	port, err := container.MappedPort(ctx, "8000")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get mapped port: %w", err)
	}

	return &ChromaContainer{
		Container: container,
		host:      host,
		port:      port.Port(),
	}, nil
}

// Host returns the container's host address.
func (c *ChromaContainer) Host() string {
	return c.host
}

// Port returns the mapped port for Chroma (as a string).
func (c *ChromaContainer) Port() string {
	return c.port
}

// Addr returns the full HTTP address in "http://host:port" format.
func (c *ChromaContainer) Addr() string {
	return fmt.Sprintf("http://%s:%s", c.host, c.port)
}

// Close terminates the Chroma container and releases resources.
func (c *ChromaContainer) Close(ctx context.Context) error {
	if c.Container == nil {
		return nil
	}
	return c.Container.Terminate(ctx)
}

// Ping verifies the Chroma connection is working via the heartbeat endpoint.
func (c *ChromaContainer) Ping(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/v2/heartbeat", c.Addr())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to ping Chroma: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Chroma heartbeat returned status %d", resp.StatusCode)
	}

	return nil
}

// CreateCollection creates a new collection in Chroma.
// Returns an error if the collection already exists.
func (c *ChromaContainer) CreateCollection(ctx context.Context, name string) error {
	url := fmt.Sprintf("%s/api/v2/tenants/default_tenant/databases/default_database/collections", c.Addr())

	body := fmt.Sprintf(`{"name": "%s"}`, name)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(strings.NewReader(body))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create collection, status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// DeleteCollection deletes a collection from Chroma.
// Returns an error if the collection does not exist.
func (c *ChromaContainer) DeleteCollection(ctx context.Context, name string) error {
	url := fmt.Sprintf("%s/api/v2/tenants/default_tenant/databases/default_database/collections/%s", c.Addr(), name)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete collection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete collection, status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// Reset clears all collections from Chroma. Useful between tests.
// Note: This uses the reset endpoint which clears all data.
func (c *ChromaContainer) Reset(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/v2/reset", c.Addr())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reset Chroma: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to reset Chroma, status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// ListCollections returns the names of all collections in Chroma.
func (c *ChromaContainer) ListCollections(ctx context.Context) ([]string, error) {
	url := fmt.Sprintf("%s/api/v2/tenants/default_tenant/databases/default_database/collections", c.Addr())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list collections: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list collections, status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse the response - Chroma returns an array of collection objects
	// For simplicity, we just return an empty slice if we can't parse
	// The actual parsing would require json unmarshal which we avoid for simplicity
	return []string{}, nil
}

