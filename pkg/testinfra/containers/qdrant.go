// Package containers provides testcontainers-go helpers for integration testing.
// This package enables consistent, reproducible container-based testing across
// the codebase without requiring docker-compose or manual setup.
package containers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// QdrantContainer wraps a testcontainers Qdrant instance with convenience methods.
type QdrantContainer struct {
	testcontainers.Container
	host     string
	httpPort string
	grpcPort string
}

// QdrantConfig holds configuration for creating a Qdrant container.
type QdrantConfig struct {
	// Version specifies the Qdrant image tag. Defaults to "v1.16.1".
	Version string
}

// DefaultQdrantConfig returns a default Qdrant container configuration.
func DefaultQdrantConfig() *QdrantConfig {
	return &QdrantConfig{
		Version: "v1.16.1",
	}
}

// NewQdrantContainer creates and starts a new Qdrant container.
// The container is ready for use when this function returns.
// Call Close() when done to clean up resources.
func NewQdrantContainer(ctx context.Context, cfg *QdrantConfig) (*QdrantContainer, error) {
	if cfg == nil {
		cfg = DefaultQdrantConfig()
	}

	if cfg.Version == "" {
		cfg.Version = "v1.16.1"
	}

	req := testcontainers.ContainerRequest{
		Image:        fmt.Sprintf("qdrant/qdrant:%s", cfg.Version),
		ExposedPorts: []string{"6333/tcp", "6334/tcp"},
		WaitingFor: wait.ForAll(
			wait.ForHTTP("/healthz").WithPort("6333/tcp").WithStartupTimeout(60*time.Second),
			wait.ForListeningPort("6333/tcp"),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start Qdrant container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	httpPort, err := container.MappedPort(ctx, "6333")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get HTTP mapped port: %w", err)
	}

	grpcPort, err := container.MappedPort(ctx, "6334")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get gRPC mapped port: %w", err)
	}

	return &QdrantContainer{
		Container: container,
		host:      host,
		httpPort:  httpPort.Port(),
		grpcPort:  grpcPort.Port(),
	}, nil
}

// Host returns the container's host address.
func (q *QdrantContainer) Host() string {
	return q.host
}

// HTTPPort returns the mapped HTTP port for Qdrant (as a string).
func (q *QdrantContainer) HTTPPort() string {
	return q.httpPort
}

// GRPCPort returns the mapped gRPC port for Qdrant (as a string).
func (q *QdrantContainer) GRPCPort() string {
	return q.grpcPort
}

// Addr returns the full HTTP address in "http://host:port" format.
func (q *QdrantContainer) Addr() string {
	return fmt.Sprintf("http://%s:%s", q.host, q.httpPort)
}

// GRPCAddr returns the gRPC address in "host:port" format.
func (q *QdrantContainer) GRPCAddr() string {
	return fmt.Sprintf("%s:%s", q.host, q.grpcPort)
}

// Close terminates the Qdrant container and releases resources.
func (q *QdrantContainer) Close(ctx context.Context) error {
	if q.Container == nil {
		return nil
	}
	return q.Container.Terminate(ctx)
}

// Ping verifies the Qdrant connection is working via the healthz endpoint.
func (q *QdrantContainer) Ping(ctx context.Context) error {
	url := fmt.Sprintf("%s/healthz", q.Addr())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to ping Qdrant: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Qdrant healthz returned status %d", resp.StatusCode)
	}

	return nil
}

// CreateCollection creates a new collection in Qdrant with the specified vector dimensions.
func (q *QdrantContainer) CreateCollection(ctx context.Context, name string, dimensions int) error {
	url := fmt.Sprintf("%s/collections/%s", q.Addr(), name)

	payload := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     dimensions,
			"distance": "Cosine",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create collection, status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// DeleteCollection deletes a collection from Qdrant.
func (q *QdrantContainer) DeleteCollection(ctx context.Context, name string) error {
	url := fmt.Sprintf("%s/collections/%s", q.Addr(), name)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete collection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete collection, status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// CollectionExists checks if a collection exists in Qdrant.
func (q *QdrantContainer) CollectionExists(ctx context.Context, name string) (bool, error) {
	url := fmt.Sprintf("%s/collections/%s", q.Addr(), name)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to check collection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return false, fmt.Errorf("failed to check collection, status %d: %s", resp.StatusCode, string(respBody))
}

// CollectionPointCount returns the number of points in a collection.
func (q *QdrantContainer) CollectionPointCount(ctx context.Context, name string) (int, error) {
	url := fmt.Sprintf("%s/collections/%s", q.Addr(), name)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to get collection info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("failed to get collection info, status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Result struct {
			PointsCount int `json:"points_count"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Result.PointsCount, nil
}

// ListCollections returns the names of all collections in Qdrant.
func (q *QdrantContainer) ListCollections(ctx context.Context) ([]string, error) {
	url := fmt.Sprintf("%s/collections", q.Addr())

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

	var result struct {
		Result struct {
			Collections []struct {
				Name string `json:"name"`
			} `json:"collections"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	names := make([]string, len(result.Result.Collections))
	for i, c := range result.Result.Collections {
		names[i] = c.Name
	}

	return names, nil
}

// UpsertPoints inserts or updates points in a collection.
func (q *QdrantContainer) UpsertPoints(ctx context.Context, collection string, points []QdrantPoint) error {
	url := fmt.Sprintf("%s/collections/%s/points?wait=true", q.Addr(), collection)

	qdrantPoints := make([]map[string]interface{}, len(points))
	for i, p := range points {
		qdrantPoints[i] = map[string]interface{}{
			"id":      p.ID,
			"vector":  p.Vector,
			"payload": p.Payload,
		}
	}

	payload := map[string]interface{}{
		"points": qdrantPoints,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upsert points: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to upsert points, status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// SearchPoints searches for similar vectors in a collection.
func (q *QdrantContainer) SearchPoints(ctx context.Context, collection string, vector []float32, limit int) ([]QdrantSearchResult, error) {
	url := fmt.Sprintf("%s/collections/%s/points/search", q.Addr(), collection)

	payload := map[string]interface{}{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to search points: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to search points, status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Result []struct {
			ID      string                 `json:"id"`
			Score   float32                `json:"score"`
			Payload map[string]interface{} `json:"payload"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	results := make([]QdrantSearchResult, len(result.Result))
	for i, r := range result.Result {
		results[i] = QdrantSearchResult{
			ID:      r.ID,
			Score:   r.Score,
			Payload: r.Payload,
		}
	}

	return results, nil
}

// DeletePoints deletes points from a collection by filter.
func (q *QdrantContainer) DeletePoints(ctx context.Context, collection string, filter map[string]interface{}) error {
	url := fmt.Sprintf("%s/collections/%s/points/delete?wait=true", q.Addr(), collection)

	payload := map[string]interface{}{
		"filter": filter,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete points: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete points, status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// QdrantPoint represents a point to be stored in Qdrant.
type QdrantPoint struct {
	ID      string                 `json:"id"`
	Vector  []float32              `json:"vector"`
	Payload map[string]interface{} `json:"payload"`
}

// QdrantSearchResult represents a search result from Qdrant.
type QdrantSearchResult struct {
	ID      string                 `json:"id"`
	Score   float32                `json:"score"`
	Payload map[string]interface{} `json:"payload"`
}
