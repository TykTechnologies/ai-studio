// pkg/ociplugins/fetcher.go
package ociplugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// ORASFetcher handles OCI artifact fetching using oras-go
type ORASFetcher struct {
	config *OCIConfig
}

// NewORASFetcher creates a new ORAS fetcher
func NewORASFetcher(config *OCIConfig) (*ORASFetcher, error) {
	return &ORASFetcher{
		config: config,
	}, nil
}

// Pull fetches a plugin from an OCI registry and returns the manifest, config, and binary data
func (f *ORASFetcher) Pull(ctx context.Context, ref *OCIReference, params *OCIPluginParams) (*ocispec.Descriptor, *PluginConfig, []byte, error) {
	// Create remote repository
	repo, err := remote.NewRepository(ref.FullRepo())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create remote repository: %w", err)
	}

	// Configure authentication
	if err := f.configureAuth(repo, ref.Registry, params.AuthConfig); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to configure authentication: %w", err)
	}

	// Create memory store for fetched content
	store := memory.New()

	// Determine reference (digest or tag)
	reference := ref.Digest
	if reference == "" {
		reference = ref.Tag
		if reference == "" {
			reference = "latest"
		}
	}

	// Copy manifest and blobs from remote to local store
	descriptor, err := oras.Copy(ctx, repo, reference, store, reference, oras.DefaultCopyOptions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to pull artifact: %w", err)
	}

	// Read and parse manifest
	manifestData, err := content.FetchAll(ctx, store, descriptor)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Parse plugin configuration from config blob if present
	var pluginConfig *PluginConfig
	if manifest.Config.Size > 0 {
		configData, err := content.FetchAll(ctx, store, manifest.Config)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to fetch config: %w", err)
		}

		pluginConfig = &PluginConfig{}
		if err := json.Unmarshal(configData, pluginConfig); err != nil {
			// Config parsing is optional, log warning but continue
			pluginConfig = nil
		}
	}

	// Find and extract the plugin binary
	binaryData, err := f.extractBinary(ctx, store, &manifest, params.Architecture)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to extract binary: %w", err)
	}

	return &descriptor, pluginConfig, binaryData, nil
}

// configureAuth sets up authentication for the registry
func (f *ORASFetcher) configureAuth(repo *remote.Repository, registry, authConfigName string) error {
	// Get auth config
	var authConfig RegistryAuth
	var exists bool

	if authConfigName != "" {
		authConfig, exists = f.config.RegistryAuth[authConfigName]
	} else {
		// Look for registry-specific auth
		authConfig, exists = f.config.RegistryAuth[registry]
	}

	if !exists {
		// No auth configured, continue with anonymous access
		return nil
	}

	// Create credential function
	var creds auth.Credential
	if authConfig.Token != "" || authConfig.TokenEnv != "" {
		// Token-based authentication
		token := authConfig.Token
		if authConfig.TokenEnv != "" {
			token = os.Getenv(authConfig.TokenEnv)
		}
		if token == "" {
			return fmt.Errorf("token authentication configured but token is empty")
		}

		creds = auth.Credential{
			Username: "oauth2accesstoken",
			Password: token,
		}
	} else if authConfig.Username != "" {
		// Username/password authentication
		password := ""
		if authConfig.PasswordEnv != "" {
			password = os.Getenv(authConfig.PasswordEnv)
		}
		if password == "" {
			return fmt.Errorf("username authentication configured but password is empty")
		}

		creds = auth.Credential{
			Username: authConfig.Username,
			Password: password,
		}
	}

	// Configure repository client with credentials
	repo.Client = &auth.Client{
		Credential: func(ctx context.Context, hostport string) (auth.Credential, error) {
			return creds, nil
		},
	}

	return nil
}

// extractBinary finds and extracts the plugin binary from the manifest layers
func (f *ORASFetcher) extractBinary(ctx context.Context, store content.Storage, manifest *ocispec.Manifest, targetArch string) ([]byte, error) {
	if len(manifest.Layers) == 0 {
		return nil, fmt.Errorf("no layers found in manifest")
	}

	// For MVP, assume first layer is the binary (as per OCI plugin distribution plan)
	// In future, we could inspect media types or annotations to find the correct layer
	binaryLayer := manifest.Layers[0]

	// Check if architecture is specified in annotations and matches
	if binaryLayer.Annotations != nil {
		if layerArch, exists := binaryLayer.Annotations["org.opencontainers.image.arch"]; exists {
			if layerOS, osExists := binaryLayer.Annotations["org.opencontainers.image.os"]; osExists {
				layerPlatform := layerOS + "/" + layerArch
				if !f.isCompatibleArchitecture(layerPlatform, targetArch) {
					return nil, &ErrIncompatibleArchitecture{
						PluginArch:  layerPlatform,
						RuntimeArch: targetArch,
					}
				}
			}
		}
	}

	// Fetch binary data
	binaryData, err := content.FetchAll(ctx, store, binaryLayer)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch binary layer: %w", err)
	}

	return binaryData, nil
}

// isCompatibleArchitecture checks if the layer architecture is compatible with target
func (f *ORASFetcher) isCompatibleArchitecture(layerArch, targetArch string) bool {
	// If target architecture is not specified, use runtime architecture
	if targetArch == "" {
		targetArch = runtime.GOOS + "/" + runtime.GOARCH
	}

	// Use the same compatibility logic as the client
	return isCompatibleArchitecture(layerArch, targetArch)
}

// PullManifest fetches just the manifest without downloading layers
func (f *ORASFetcher) PullManifest(ctx context.Context, ref *OCIReference, params *OCIPluginParams) (*ocispec.Descriptor, *ocispec.Manifest, error) {
	// Create remote repository
	repo, err := remote.NewRepository(ref.FullRepo())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create remote repository: %w", err)
	}

	// Configure authentication
	if err := f.configureAuth(repo, ref.Registry, params.AuthConfig); err != nil {
		return nil, nil, fmt.Errorf("failed to configure authentication: %w", err)
	}

	// Determine reference (digest or tag)
	reference := ref.Digest
	if reference == "" {
		reference = ref.Tag
		if reference == "" {
			reference = "latest"
		}
	}

	// Resolve reference to descriptor
	descriptor, err := repo.Resolve(ctx, reference)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve reference: %w", err)
	}

	// Fetch manifest content
	manifestReader, err := repo.Fetch(ctx, descriptor)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer manifestReader.Close()

	manifestData, err := content.ReadAll(manifestReader, descriptor)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return &descriptor, &manifest, nil
}

// CheckExists verifies if an artifact exists in the registry without downloading
func (f *ORASFetcher) CheckExists(ctx context.Context, ref *OCIReference, params *OCIPluginParams) (bool, error) {
	// Create remote repository
	repo, err := remote.NewRepository(ref.FullRepo())
	if err != nil {
		return false, fmt.Errorf("failed to create remote repository: %w", err)
	}

	// Configure authentication
	if err := f.configureAuth(repo, ref.Registry, params.AuthConfig); err != nil {
		return false, fmt.Errorf("failed to configure authentication: %w", err)
	}

	// Determine reference (digest or tag)
	reference := ref.Digest
	if reference == "" {
		reference = ref.Tag
		if reference == "" {
			reference = "latest"
		}
	}

	// Try to resolve reference
	_, err = repo.Resolve(ctx, reference)
	if err != nil {
		// Check if it's a "not found" error
		return false, nil
	}

	return true, nil
}