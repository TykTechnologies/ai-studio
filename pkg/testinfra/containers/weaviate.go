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

// WeaviateContainer wraps a testcontainers Weaviate instance with convenience methods.
type WeaviateContainer struct {
	testcontainers.Container
	host     string
	httpPort string
	grpcPort string
}

// WeaviateConfig holds configuration for creating a Weaviate container.
type WeaviateConfig struct {
	// Version specifies the Weaviate image tag. Defaults to "1.34.0".
	Version string
}

// DefaultWeaviateConfig returns a default Weaviate container configuration.
func DefaultWeaviateConfig() *WeaviateConfig {
	return &WeaviateConfig{
		Version: "1.34.0",
	}
}

// NewWeaviateContainer creates and starts a new Weaviate container.
// The container is ready for use when this function returns.
// Call Close() when done to clean up resources.
func NewWeaviateContainer(ctx context.Context, cfg *WeaviateConfig) (*WeaviateContainer, error) {
	if cfg == nil {
		cfg = DefaultWeaviateConfig()
	}

	if cfg.Version == "" {
		cfg.Version = "1.34.0"
	}

	req := testcontainers.ContainerRequest{
		Image:        fmt.Sprintf("semitechnologies/weaviate:%s", cfg.Version),
		ExposedPorts: []string{"8080/tcp", "50051/tcp"},
		Env: map[string]string{
			"AUTHENTICATION_ANONYMOUS_ACCESS_ENABLED": "true",
			"PERSISTENCE_DATA_PATH":                   "/var/lib/weaviate",
			"DEFAULT_VECTORIZER_MODULE":               "none",
			"CLUSTER_HOSTNAME":                        "node1",
		},
		WaitingFor: wait.ForAll(
			wait.ForHTTP("/v1/.well-known/ready").WithPort("8080/tcp").WithStartupTimeout(60*time.Second),
			wait.ForListeningPort("8080/tcp"),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start Weaviate container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	httpPort, err := container.MappedPort(ctx, "8080")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get HTTP mapped port: %w", err)
	}

	grpcPort, err := container.MappedPort(ctx, "50051")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get gRPC mapped port: %w", err)
	}

	return &WeaviateContainer{
		Container: container,
		host:      host,
		httpPort:  httpPort.Port(),
		grpcPort:  grpcPort.Port(),
	}, nil
}

// Host returns the container's host address.
func (w *WeaviateContainer) Host() string {
	return w.host
}

// HTTPPort returns the mapped HTTP port for Weaviate (as a string).
func (w *WeaviateContainer) HTTPPort() string {
	return w.httpPort
}

// GRPCPort returns the mapped gRPC port for Weaviate (as a string).
func (w *WeaviateContainer) GRPCPort() string {
	return w.grpcPort
}

// Addr returns the full HTTP address in "http://host:port" format.
func (w *WeaviateContainer) Addr() string {
	return fmt.Sprintf("http://%s:%s", w.host, w.httpPort)
}

// GRPCAddr returns the gRPC address in "host:port" format.
func (w *WeaviateContainer) GRPCAddr() string {
	return fmt.Sprintf("%s:%s", w.host, w.grpcPort)
}

// Close terminates the Weaviate container and releases resources.
func (w *WeaviateContainer) Close(ctx context.Context) error {
	if w.Container == nil {
		return nil
	}
	return w.Container.Terminate(ctx)
}

// Ping verifies the Weaviate connection is working via the ready endpoint.
func (w *WeaviateContainer) Ping(ctx context.Context) error {
	url := fmt.Sprintf("%s/v1/.well-known/ready", w.Addr())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to ping Weaviate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Weaviate ready endpoint returned status %d", resp.StatusCode)
	}

	return nil
}

// CreateClass creates a new class (collection) in Weaviate with the specified vector dimensions.
func (w *WeaviateContainer) CreateClass(ctx context.Context, className string, dimensions int) error {
	url := fmt.Sprintf("%s/v1/schema", w.Addr())

	classSchema := map[string]interface{}{
		"class": className,
		"vectorIndexConfig": map[string]interface{}{
			"distance": "cosine",
		},
		"properties": []map[string]interface{}{
			{
				"name":     "content",
				"dataType": []string{"text"},
			},
			{
				"name":     "source",
				"dataType": []string{"text"},
			},
			{
				"name":     "doc_id",
				"dataType": []string{"text"},
			},
			{
				"name":     "category",
				"dataType": []string{"text"},
			},
		},
	}

	body, err := json.Marshal(classSchema)
	if err != nil {
		return fmt.Errorf("failed to marshal class schema: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create class: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create class, status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// DeleteClass deletes a class from Weaviate.
func (w *WeaviateContainer) DeleteClass(ctx context.Context, className string) error {
	url := fmt.Sprintf("%s/v1/schema/%s", w.Addr(), className)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete class: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete class, status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// ClassExists checks if a class exists in Weaviate.
func (w *WeaviateContainer) ClassExists(ctx context.Context, className string) (bool, error) {
	url := fmt.Sprintf("%s/v1/schema/%s", w.Addr(), className)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to check class: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	return false, fmt.Errorf("failed to check class, status %d: %s", resp.StatusCode, string(respBody))
}

// ClassObjectCount returns the number of objects in a class.
func (w *WeaviateContainer) ClassObjectCount(ctx context.Context, className string) (int, error) {
	url := fmt.Sprintf("%s/v1/graphql", w.Addr())

	query := map[string]interface{}{
		"query": fmt.Sprintf(`{
			Aggregate {
				%s {
					meta {
						count
					}
				}
			}
		}`, className),
	}

	body, err := json.Marshal(query)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to get object count: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("failed to get object count, status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data struct {
			Aggregate map[string][]struct {
				Meta struct {
					Count int `json:"count"`
				} `json:"meta"`
			} `json:"Aggregate"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	if agg, ok := result.Data.Aggregate[className]; ok && len(agg) > 0 {
		return agg[0].Meta.Count, nil
	}

	return 0, nil
}

// ListClasses returns the names of all classes in Weaviate.
func (w *WeaviateContainer) ListClasses(ctx context.Context) ([]string, error) {
	url := fmt.Sprintf("%s/v1/schema", w.Addr())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list classes: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list classes, status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Classes []struct {
			Class string `json:"class"`
		} `json:"classes"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	names := make([]string, len(result.Classes))
	for i, c := range result.Classes {
		names[i] = c.Class
	}

	return names, nil
}

// InsertObjects inserts objects into a class with vectors.
func (w *WeaviateContainer) InsertObjects(ctx context.Context, className string, objects []WeaviateObject) error {
	url := fmt.Sprintf("%s/v1/batch/objects", w.Addr())

	batchObjects := make([]map[string]interface{}, len(objects))
	for i, obj := range objects {
		batchObjects[i] = map[string]interface{}{
			"class":      className,
			"id":         obj.ID,
			"vector":     obj.Vector,
			"properties": obj.Properties,
		}
	}

	payload := map[string]interface{}{
		"objects": batchObjects,
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
		return fmt.Errorf("failed to insert objects: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to insert objects, status %d: %s", resp.StatusCode, string(respBody))
	}

	// Check response for errors - batch operations return an array of results
	var batchResult []struct {
		Result struct {
			Errors *struct {
				Error []struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"errors"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&batchResult); err != nil {
		return fmt.Errorf("failed to decode batch response: %w", err)
	}

	// Check if any objects had errors
	for i, r := range batchResult {
		if r.Result.Errors != nil && len(r.Result.Errors.Error) > 0 {
			return fmt.Errorf("failed to insert object %d: %s", i, r.Result.Errors.Error[0].Message)
		}
	}

	return nil
}

// SearchObjects searches for similar vectors in a class.
func (w *WeaviateContainer) SearchObjects(ctx context.Context, className string, vector []float32, limit int) ([]WeaviateSearchResult, error) {
	url := fmt.Sprintf("%s/v1/graphql", w.Addr())

	// Build vector string
	vectorStr := "["
	for i, v := range vector {
		if i > 0 {
			vectorStr += ","
		}
		vectorStr += fmt.Sprintf("%f", v)
	}
	vectorStr += "]"

	query := map[string]interface{}{
		"query": fmt.Sprintf(`{
			Get {
				%s(
					nearVector: {vector: %s}
					limit: %d
				) {
					content
					source
					doc_id
					category
					_additional {
						id
						distance
					}
				}
			}
		}`, className, vectorStr, limit),
	}

	body, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to search objects: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to search objects, status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data struct {
			Get map[string][]struct {
				Content    string `json:"content"`
				Source     string `json:"source"`
				DocID      string `json:"doc_id"`
				Category   string `json:"category"`
				Additional struct {
					ID       string  `json:"id"`
					Distance float32 `json:"distance"`
				} `json:"_additional"`
			} `json:"Get"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	objects, ok := result.Data.Get[className]
	if !ok {
		return []WeaviateSearchResult{}, nil
	}

	results := make([]WeaviateSearchResult, len(objects))
	for i, obj := range objects {
		results[i] = WeaviateSearchResult{
			ID:       obj.Additional.ID,
			Distance: obj.Additional.Distance,
			Properties: map[string]interface{}{
				"content":  obj.Content,
				"source":   obj.Source,
				"doc_id":   obj.DocID,
				"category": obj.Category,
			},
		}
	}

	return results, nil
}

// DeleteObjectsByFilter deletes objects from a class matching a filter.
func (w *WeaviateContainer) DeleteObjectsByFilter(ctx context.Context, className string, propertyName string, propertyValue string) error {
	url := fmt.Sprintf("%s/v1/batch/objects", w.Addr())

	payload := map[string]interface{}{
		"match": map[string]interface{}{
			"class": className,
			"where": map[string]interface{}{
				"path":     []string{propertyName},
				"operator": "Equal",
				"valueText": propertyValue,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete objects: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete objects, status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// WeaviateObject represents an object to be stored in Weaviate.
type WeaviateObject struct {
	ID         string                 `json:"id"`
	Vector     []float32              `json:"vector"`
	Properties map[string]interface{} `json:"properties"`
}

// WeaviateSearchResult represents a search result from Weaviate.
type WeaviateSearchResult struct {
	ID         string                 `json:"id"`
	Distance   float32                `json:"distance"`
	Properties map[string]interface{} `json:"properties"`
}
