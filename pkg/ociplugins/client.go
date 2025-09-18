// pkg/ociplugins/client.go
package ociplugins

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// OCIPluginClient handles OCI-based plugin operations
type OCIPluginClient struct {
	config    *OCIConfig
	storage   *ContentStorage
	fetcher   *ORASFetcher
	verifier  *SignatureVerifier

	// Background processing
	gcCancel  context.CancelFunc
	gcStopped chan struct{}
}

// NewOCIPluginClient creates a new OCI plugin client
func NewOCIPluginClient(config *OCIConfig) (*OCIPluginClient, error) {
	if config == nil {
		config = DefaultOCIConfig()
	}

	// Initialize storage
	storage, err := NewContentStorage(config.CacheDir, config.MaxCacheSize, config.GCInterval)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Initialize ORAS fetcher
	fetcher, err := NewORASFetcher(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ORAS fetcher: %w", err)
	}

	// Initialize signature verifier
	verifier, err := NewSignatureVerifier(config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize signature verifier: %w", err)
	}

	client := &OCIPluginClient{
		config:    config,
		storage:   storage,
		fetcher:   fetcher,
		verifier:  verifier,
		gcStopped: make(chan struct{}),
	}

	// Start background garbage collection if configured
	if config.GCInterval > 0 {
		ctx, cancel := context.WithCancel(context.Background())
		client.gcCancel = cancel
		go client.startGarbageCollection(ctx)
	}

	return client, nil
}

// FetchPlugin fetches a plugin by OCI reference, verifies it, and returns local information
func (c *OCIPluginClient) FetchPlugin(ctx context.Context, ref *OCIReference, params *OCIPluginParams) (*LocalPlugin, error) {
	startTime := time.Now()

	log.Info().
		Str("reference", ref.FullReference()).
		Str("architecture", params.Architecture).
		Msg("Fetching OCI plugin")

	// Validate reference against configuration
	if err := ValidateOCIReference(ref, c.config); err != nil {
		return nil, fmt.Errorf("reference validation failed: %w", err)
	}

	// Normalize architecture if not specified
	if params.Architecture == "" {
		params.Architecture = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	}

	// Check if plugin already exists in cache
	if c.HasPlugin(ref.Digest, params.Architecture) {
		log.Debug().
			Str("reference", ref.FullReference()).
			Str("architecture", params.Architecture).
			Msg("Plugin found in cache")

		return c.getLocalPlugin(ref, params)
	}

	// Fetch the plugin manifest and layers
	_, pluginConfig, binaryData, err := c.fetcher.Pull(ctx, ref, params)
	if err != nil {
		return nil, fmt.Errorf("failed to pull plugin: %w", err)
	}

	// Verify signature if required
	if c.config.RequireSignature {
		if err := c.verifier.Verify(ctx, ref, params.PublicKey); err != nil {
			return nil, &ErrSignatureVerificationFailed{
				Reference: ref.FullReference(),
				Reason:    err.Error(),
			}
		}
		log.Debug().
			Str("reference", ref.FullReference()).
			Msg("Signature verification passed")
	} else {
		log.Debug().
			Str("reference", ref.FullReference()).
			Msg("Signature verification skipped (disabled)")
	}

	// Validate architecture compatibility
	if pluginConfig != nil && pluginConfig.Arch != "" {
		if !isCompatibleArchitecture(pluginConfig.OS+"/"+pluginConfig.Arch, params.Architecture) {
			return nil, &ErrIncompatibleArchitecture{
				PluginArch:  pluginConfig.OS + "/" + pluginConfig.Arch,
				RuntimeArch: params.Architecture,
			}
		}
	}

	// Store the binary data as executable
	execPath, err := c.storage.StoreExecutable(ref.Digest, params.Architecture, binaryData)
	if err != nil {
		return nil, fmt.Errorf("failed to store executable: %w", err)
	}

	// Create and store metadata
	fetchTime := time.Now()
	version := ""
	if pluginConfig != nil && pluginConfig.Version != "" {
		version = pluginConfig.Version
	}

	// Track whether signature was actually verified
	signatureVerified := c.config.RequireSignature && params.PublicKey != ""

	metadata := &PluginMetadata{
		Reference:    ref,
		Params:       params,
		FetchTime:    fetchTime,
		Config:       pluginConfig,
		Verified:     signatureVerified,
		Size:         int64(len(binaryData)),
		LastAccessed: fetchTime,
		Version:      version,
	}

	if err := c.storage.StoreMetadata(ref.Digest, params.Architecture, metadata); err != nil {
		log.Warn().Err(err).Msg("Failed to store plugin metadata")
	}

	// Create local plugin info
	localPlugin := &LocalPlugin{
		Reference:      ref,
		Params:         params,
		ExecutablePath: execPath,
		CacheDir:       c.config.CacheDir,
		Verified:       metadata.Verified,
		FetchTime:      fetchTime,
		Config:         pluginConfig,
	}

	log.Info().
		Str("reference", ref.FullReference()).
		Str("executable_path", execPath).
		Dur("fetch_time", time.Since(startTime)).
		Msg("OCI plugin fetched successfully")

	return localPlugin, nil
}

// HasPlugin checks if a plugin exists in the cache
func (c *OCIPluginClient) HasPlugin(digest, arch string) bool {
	return c.storage.HasExecutable(digest, arch)
}

// GetPlugin returns a local plugin if it exists in cache
func (c *OCIPluginClient) GetPlugin(ref *OCIReference, params *OCIPluginParams) (*LocalPlugin, error) {
	if !c.HasPlugin(ref.Digest, params.Architecture) {
		return nil, &ErrPluginNotFound{Reference: ref.FullReference()}
	}

	return c.getLocalPlugin(ref, params)
}

// getLocalPlugin creates a LocalPlugin instance for a cached plugin
func (c *OCIPluginClient) getLocalPlugin(ref *OCIReference, params *OCIPluginParams) (*LocalPlugin, error) {
	execPath := c.storage.GetExecutablePath(ref.Digest, params.Architecture)

	// Load metadata from cache if available
	if c.storage.HasMetadata(ref.Digest, params.Architecture) {
		metadata, err := c.storage.LoadMetadata(ref.Digest, params.Architecture)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to load cached metadata, using defaults")
		} else {
			// Update last accessed time
			if err := c.storage.UpdateLastAccessed(ref.Digest, params.Architecture); err != nil {
				log.Warn().Err(err).Msg("Failed to update last accessed time")
			}

			return &LocalPlugin{
				Reference:      ref,
				Params:         params,
				ExecutablePath: execPath,
				CacheDir:       c.config.CacheDir,
				Verified:       metadata.Verified,
				FetchTime:      metadata.FetchTime,
				Config:         metadata.Config,
			}, nil
		}
	}

	// Fallback to basic info if metadata not available
	return &LocalPlugin{
		Reference:      ref,
		Params:         params,
		ExecutablePath: execPath,
		CacheDir:       c.config.CacheDir,
		Verified:       true, // Assume verified if in cache
		FetchTime:      time.Now(), // Unknown fetch time
	}, nil
}

// SetActivePlugin creates a symlink to mark a plugin as the active version
func (c *OCIPluginClient) SetActivePlugin(name string, ref *OCIReference, params *OCIPluginParams) error {
	return c.storage.CreateActiveLink(name, ref.Digest, params.Architecture)
}

// GetActivePlugin returns the path to the active version of a plugin
func (c *OCIPluginClient) GetActivePlugin(name string) (string, error) {
	return c.storage.GetActivePath(name)
}

// ListCached returns information about all cached plugins
func (c *OCIPluginClient) ListCached() ([]*LocalPlugin, error) {
	// Get all metadata from storage
	allMetadata, err := c.storage.ListAllMetadata()
	if err != nil {
		return nil, fmt.Errorf("failed to list metadata: %w", err)
	}

	var localPlugins []*LocalPlugin
	for _, metadata := range allMetadata {
		// Verify executable still exists
		execPath := c.storage.GetExecutablePath(metadata.Reference.Digest, metadata.Params.Architecture)
		if _, err := os.Stat(execPath); err != nil {
			// Executable missing, skip this entry
			continue
		}

		localPlugin := &LocalPlugin{
			Reference:      metadata.Reference,
			Params:         metadata.Params,
			ExecutablePath: execPath,
			CacheDir:       c.config.CacheDir,
			Verified:       metadata.Verified,
			FetchTime:      metadata.FetchTime,
			Config:         metadata.Config,
		}

		localPlugins = append(localPlugins, localPlugin)
	}

	return localPlugins, nil
}

// ListCachedByRegistry returns cached plugins filtered by registry
func (c *OCIPluginClient) ListCachedByRegistry(registry string) ([]*LocalPlugin, error) {
	allPlugins, err := c.ListCached()
	if err != nil {
		return nil, err
	}

	var filtered []*LocalPlugin
	for _, plugin := range allPlugins {
		if plugin.Reference.Registry == registry {
			filtered = append(filtered, plugin)
		}
	}

	return filtered, nil
}

// ListCachedByArchitecture returns cached plugins filtered by architecture
func (c *OCIPluginClient) ListCachedByArchitecture(arch string) ([]*LocalPlugin, error) {
	allPlugins, err := c.ListCached()
	if err != nil {
		return nil, err
	}

	var filtered []*LocalPlugin
	for _, plugin := range allPlugins {
		if plugin.Params.Architecture == arch {
			filtered = append(filtered, plugin)
		}
	}

	return filtered, nil
}

// GarbageCollect removes old plugin versions from cache
func (c *OCIPluginClient) GarbageCollect(ctx context.Context, keepVersions int) error {
	log.Info().
		Int("keep_versions", keepVersions).
		Msg("Starting plugin cache garbage collection")

	if err := c.storage.GarbageCollect(keepVersions); err != nil {
		return fmt.Errorf("garbage collection failed: %w", err)
	}

	// Calculate cache size after cleanup
	size, err := c.storage.CalculateSize()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to calculate cache size after GC")
	} else {
		log.Info().
			Int64("cache_size_bytes", size).
			Msg("Garbage collection completed")
	}

	return nil
}

// GetCacheSize returns the current size of the plugin cache
func (c *OCIPluginClient) GetCacheSize() (int64, error) {
	return c.storage.CalculateSize()
}

// startGarbageCollection runs periodic garbage collection in the background
func (c *OCIPluginClient) startGarbageCollection(ctx context.Context) {
	ticker := time.NewTicker(c.config.GCInterval)
	defer ticker.Stop()
	defer close(c.gcStopped)

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping background garbage collection")
			return
		case <-ticker.C:
			gcCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			if err := c.GarbageCollect(gcCtx, c.config.KeepVersions); err != nil {
				log.Error().Err(err).Msg("Background garbage collection failed")
			}
			cancel()
		}
	}
}

// Close shuts down the OCI client and cleans up resources
func (c *OCIPluginClient) Close() error {
	// Stop background garbage collection if running
	if c.gcCancel != nil {
		log.Info().Msg("Stopping OCI plugin client...")
		c.gcCancel()

		// Wait for garbage collection to stop (with timeout)
		select {
		case <-c.gcStopped:
			log.Info().Msg("Background garbage collection stopped")
		case <-time.After(5 * time.Second):
			log.Warn().Msg("Timed out waiting for garbage collection to stop")
		}
	}

	return nil
}

// isCompatibleArchitecture checks if plugin architecture is compatible with runtime architecture
func isCompatibleArchitecture(pluginArch, runtimeArch string) bool {
	// Exact match is always compatible
	if pluginArch == runtimeArch {
		return true
	}

	// Split architectures into OS/ARCH components
	pluginParts := strings.Split(pluginArch, "/")
	runtimeParts := strings.Split(runtimeArch, "/")

	if len(pluginParts) != 2 || len(runtimeParts) != 2 {
		return false
	}

	pluginOS, pluginCPU := pluginParts[0], pluginParts[1]
	runtimeOS, runtimeCPU := runtimeParts[0], runtimeParts[1]

	// OS must match (no cross-OS compatibility)
	if pluginOS != runtimeOS {
		return false
	}

	// Architecture compatibility matrix
	compatMatrix := map[string][]string{
		// arm64 can run amd64 binaries via emulation on many systems
		"arm64": {"amd64", "arm64"},
		// amd64 can only run amd64 binaries reliably
		"amd64": {"amd64"},
		// arm can run arm binaries
		"arm": {"arm"},
		// 386 can run on amd64 with compatibility layer
		"386": {"386"},
	}

	// Check if runtime CPU can execute plugin CPU
	compatibleCPUs, exists := compatMatrix[runtimeCPU]
	if !exists {
		return false
	}

	for _, compatCPU := range compatibleCPUs {
		if pluginCPU == compatCPU {
			return true
		}
	}

	return false
}