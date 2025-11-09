package marketplace

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

// Fetcher handles fetching marketplace indexes and manifests
type Fetcher struct {
	httpClient *http.Client
	userAgent  string
}

// NewFetcher creates a new marketplace fetcher
func NewFetcher(timeout time.Duration) *Fetcher {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Fetcher{
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		userAgent: "TykAIStudio/1.0 Marketplace-Fetcher",
	}
}

// FetchIndex fetches and parses the marketplace index.yaml
func (f *Fetcher) FetchIndex(ctx context.Context, indexURL string) (*MarketplaceIndex, *FetchMetadata, error) {
	log.Info().Str("url", indexURL).Msg("Fetching marketplace index")

	req, err := http.NewRequestWithContext(ctx, "GET", indexURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "application/x-yaml, text/yaml, application/yaml")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse YAML
	var index MarketplaceIndex
	if err := yaml.Unmarshal(body, &index); err != nil {
		return nil, nil, fmt.Errorf("failed to parse index YAML: %w", err)
	}

	// Extract metadata from HTTP headers
	metadata := &FetchMetadata{
		ETag:         resp.Header.Get("ETag"),
		LastModified: parseHTTPDate(resp.Header.Get("Last-Modified")),
		ContentLength: resp.ContentLength,
		FetchedAt:    time.Now(),
	}

	log.Info().
		Str("url", indexURL).
		Int("plugin_count", countPlugins(&index)).
		Msg("Successfully fetched marketplace index")

	return &index, metadata, nil
}

// FetchManifest fetches and parses a plugin manifest.yaml
func (f *Fetcher) FetchManifest(ctx context.Context, manifestURL string) (*PluginManifest, error) {
	log.Debug().Str("url", manifestURL).Msg("Fetching plugin manifest")

	req, err := http.NewRequestWithContext(ctx, "GET", manifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "application/x-yaml, text/yaml, application/yaml")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse YAML
	var manifest PluginManifest
	if err := yaml.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest YAML: %w", err)
	}

	log.Debug().
		Str("url", manifestURL).
		Str("plugin_id", manifest.ID).
		Str("version", manifest.Version).
		Msg("Successfully fetched plugin manifest")

	return &manifest, nil
}

// FetchIndexConditional fetches the index only if it has been modified
func (f *Fetcher) FetchIndexConditional(ctx context.Context, indexURL, etag string, lastModified time.Time) (*MarketplaceIndex, *FetchMetadata, bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", indexURL, nil)
	if err != nil {
		return nil, nil, false, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "application/x-yaml, text/yaml, application/yaml")

	// Add conditional headers
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if !lastModified.IsZero() {
		req.Header.Set("If-Modified-Since", lastModified.Format(http.TimeFormat))
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, nil, false, fmt.Errorf("failed to fetch index: %w", err)
	}
	defer resp.Body.Close()

	// Not modified
	if resp.StatusCode == http.StatusNotModified {
		log.Debug().Str("url", indexURL).Msg("Marketplace index not modified")
		return nil, nil, false, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, nil, false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read and parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, false, fmt.Errorf("failed to read response body: %w", err)
	}

	var index MarketplaceIndex
	if err := yaml.Unmarshal(body, &index); err != nil {
		return nil, nil, false, fmt.Errorf("failed to parse index YAML: %w", err)
	}

	metadata := &FetchMetadata{
		ETag:         resp.Header.Get("ETag"),
		LastModified: parseHTTPDate(resp.Header.Get("Last-Modified")),
		ContentLength: resp.ContentLength,
		FetchedAt:    time.Now(),
	}

	log.Info().
		Str("url", indexURL).
		Int("plugin_count", countPlugins(&index)).
		Msg("Marketplace index updated")

	return &index, metadata, true, nil
}

// FetchMetadata contains HTTP response metadata
type FetchMetadata struct {
	ETag          string
	LastModified  time.Time
	ContentLength int64
	FetchedAt     time.Time
}

// parseHTTPDate parses HTTP date header
func parseHTTPDate(dateStr string) time.Time {
	if dateStr == "" {
		return time.Time{}
	}
	t, err := http.ParseTime(dateStr)
	if err != nil {
		log.Warn().Str("date", dateStr).Err(err).Msg("Failed to parse HTTP date")
		return time.Time{}
	}
	return t
}

// countPlugins counts total plugins in index
func countPlugins(index *MarketplaceIndex) int {
	count := 0
	for _, versions := range index.Plugins {
		count += len(versions)
	}
	return count
}

// ValidateIndex performs basic validation on the marketplace index
func ValidateIndex(index *MarketplaceIndex) error {
	if index == nil {
		return fmt.Errorf("index is nil")
	}

	if index.APIVersion == "" {
		return fmt.Errorf("index missing apiVersion")
	}

	if index.Plugins == nil {
		return fmt.Errorf("index missing plugins map")
	}

	// Validate each plugin entry
	for pluginID, versions := range index.Plugins {
		if len(versions) == 0 {
			return fmt.Errorf("plugin %s has no versions", pluginID)
		}

		for i, plugin := range versions {
			if plugin.ID != pluginID {
				return fmt.Errorf("plugin %s version %d: ID mismatch (expected %s, got %s)",
					pluginID, i, pluginID, plugin.ID)
			}
			if plugin.Version == "" {
				return fmt.Errorf("plugin %s version %d: missing version", pluginID, i)
			}
			if plugin.OCIRegistry == "" || plugin.OCIRepository == "" {
				return fmt.Errorf("plugin %s version %s: incomplete OCI reference", pluginID, plugin.Version)
			}
		}
	}

	return nil
}

// ValidateManifest performs basic validation on a plugin manifest
func ValidateManifest(manifest *PluginManifest) error {
	if manifest == nil {
		return fmt.Errorf("manifest is nil")
	}

	if manifest.ID == "" {
		return fmt.Errorf("manifest missing id")
	}
	if manifest.Name == "" {
		return fmt.Errorf("manifest missing name")
	}
	if manifest.Version == "" {
		return fmt.Errorf("manifest missing version")
	}

	// Validate OCI info
	if manifest.OCI.Registry == "" {
		return fmt.Errorf("manifest missing OCI registry")
	}
	if manifest.OCI.Repository == "" {
		return fmt.Errorf("manifest missing OCI repository")
	}
	if manifest.OCI.Tag == "" && manifest.OCI.Digest == "" {
		return fmt.Errorf("manifest missing OCI tag or digest")
	}

	// Validate capabilities
	if len(manifest.Capabilities.Hooks) == 0 {
		return fmt.Errorf("manifest missing capabilities.hooks")
	}

	return nil
}
