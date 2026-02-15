// pkg/ociplugins/fetcher.go
package ociplugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"

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

	// Configure for insecure (HTTP) registries if needed
	if f.isInsecureRegistry(ref.Registry) {
		repo.PlainHTTP = true
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

	// Check if this is a multi-arch image index
	switch descriptor.MediaType {
	case ocispec.MediaTypeImageIndex, "application/vnd.docker.distribution.manifest.list.v2+json":
		// Handle multi-arch index - resolve to platform-specific manifest
		return f.pullFromIndex(ctx, store, &descriptor, params)
	}

	// Handle single-platform manifest
	return f.pullFromManifest(ctx, store, &descriptor, params)
}

// pullFromManifest extracts plugin data from a single-platform manifest
func (f *ORASFetcher) pullFromManifest(ctx context.Context, store content.Storage, descriptor *ocispec.Descriptor, params *OCIPluginParams) (*ocispec.Descriptor, *PluginConfig, []byte, error) {
	// Read and parse manifest
	manifestData, err := content.FetchAll(ctx, store, *descriptor)
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

	return descriptor, pluginConfig, binaryData, nil
}

// pullFromIndex handles multi-arch image indexes by selecting the appropriate platform manifest
func (f *ORASFetcher) pullFromIndex(ctx context.Context, store content.Storage, indexDesc *ocispec.Descriptor, params *OCIPluginParams) (*ocispec.Descriptor, *PluginConfig, []byte, error) {
	// Fetch and parse the index
	indexData, err := content.FetchAll(ctx, store, *indexDesc)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to fetch index: %w", err)
	}

	var index ocispec.Index
	if err := json.Unmarshal(indexData, &index); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse index: %w", err)
	}

	// Normalize target architecture
	targetArch := params.Architecture
	if targetArch == "" {
		targetArch = runtime.GOOS + "/" + runtime.GOARCH
	}

	// Find matching platform manifest
	manifestDesc, err := f.selectPlatformManifest(&index, targetArch)
	if err != nil {
		return nil, nil, nil, err
	}

	// The platform-specific manifest should already be in the store from oras.Copy()
	return f.pullFromManifest(ctx, store, manifestDesc, params)
}

// selectPlatformManifest selects the appropriate manifest from an index based on target architecture
func (f *ORASFetcher) selectPlatformManifest(index *ocispec.Index, targetArch string) (*ocispec.Descriptor, error) {
	if len(index.Manifests) == 0 {
		return nil, fmt.Errorf("no manifests found in index")
	}

	// First pass: look for exact platform match
	for i := range index.Manifests {
		manifest := &index.Manifests[i]
		if manifest.Platform != nil {
			manifestPlatform := manifest.Platform.OS + "/" + manifest.Platform.Architecture
			if manifestPlatform == targetArch {
				return manifest, nil
			}
		}
	}

	// Second pass: look for compatible platform (e.g., arm64 can run amd64 via emulation)
	for i := range index.Manifests {
		manifest := &index.Manifests[i]
		if manifest.Platform != nil {
			manifestPlatform := manifest.Platform.OS + "/" + manifest.Platform.Architecture
			if f.isCompatibleArchitecture(manifestPlatform, targetArch) {
				return manifest, nil
			}
		}
	}

	// Build list of available platforms for error message
	var availablePlatforms []string
	for _, manifest := range index.Manifests {
		if manifest.Platform != nil {
			availablePlatforms = append(availablePlatforms, manifest.Platform.OS+"/"+manifest.Platform.Architecture)
		}
	}

	return nil, &ErrIncompatibleArchitecture{
		PluginArch:  strings.Join(availablePlatforms, ", "),
		RuntimeArch: targetArch,
	}
}

// configureAuth sets up authentication for the registry
func (f *ORASFetcher) configureAuth(repo *remote.Repository, registry, authConfigName string) error {
	// Get auth config
	var authConfig RegistryAuth
	var exists bool

	fmt.Printf("[OCI Auth] configureAuth: registry=%q authConfigName=%q available_auths=%d\n", registry, authConfigName, len(f.config.RegistryAuth))
	for k := range f.config.RegistryAuth {
		fmt.Printf("[OCI Auth]   available key: %q\n", k)
	}

	if authConfigName != "" {
		authConfig, exists = f.config.RegistryAuth[authConfigName]
	} else {
		// Look for registry-specific auth
		authConfig, exists = f.config.RegistryAuth[registry]
	}

	if !exists {
		fmt.Printf("[OCI Auth] NO auth found for registry %q - using anonymous access\n", registry)
		return nil
	}

	fmt.Printf("[OCI Auth] auth found: entitlement=%t entitlementEnv=%t token=%t username=%t\n",
		authConfig.Entitlement != "", authConfig.EntitlementEnv != "", authConfig.Token != "", authConfig.Username != "")

	// Create credential function
	var creds auth.Credential
	if authConfig.Entitlement != "" || authConfig.EntitlementEnv != "" {
		// Entitlement token authentication (e.g. Cloudsmith)
		// Sent as basic auth with username "token" and the entitlement token as password
		entitlement := authConfig.Entitlement
		if authConfig.EntitlementEnv != "" {
			entitlement = os.Getenv(authConfig.EntitlementEnv)
		}
		if entitlement == "" {
			return fmt.Errorf("entitlement token authentication configured but token is empty")
		}

		username := authConfig.EntitlementUsername
		if username == "" {
			username = "token"
		}

		creds = auth.Credential{
			Username: username,
			Password: entitlement,
		}
	} else if authConfig.Token != "" || authConfig.TokenEnv != "" {
		// Token-based authentication (OAuth2 style)
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

	// For entitlement auth (e.g. Cloudsmith), bypass the Docker token exchange
	// entirely and send Basic auth on every request. This avoids issues where
	// Cloudsmith's token endpoint requests multiple scopes (including namespace-level
	// scopes) that a repository-scoped entitlement cannot satisfy.
	if authConfig.Entitlement != "" || authConfig.EntitlementEnv != "" {
		repo.Client = &basicAuthHTTPClient{
			username: creds.Username,
			password: creds.Password,
			inner:    http.DefaultClient,
		}
	} else {
		// Standard Docker token exchange for username/password and token auth
		repo.Client = &auth.Client{
			Credential: func(ctx context.Context, hostport string) (auth.Credential, error) {
				return creds, nil
			},
		}
	}

	return nil
}

// basicAuthHTTPClient is an http.Client wrapper that injects Basic auth on every request,
// bypassing the Docker V2 token exchange. Required for registries like Cloudsmith
// where entitlement tokens are repository-scoped and the token exchange requests
// additional namespace-level scopes that the entitlement cannot satisfy.
type basicAuthHTTPClient struct {
	username string
	password string
	inner    *http.Client
}

func (c *basicAuthHTTPClient) Do(req *http.Request) (*http.Response, error) {
	req.SetBasicAuth(c.username, c.password)
	return c.inner.Do(req)
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

	// Configure for insecure (HTTP) registries if needed
	if f.isInsecureRegistry(ref.Registry) {
		repo.PlainHTTP = true
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

	// Fetch manifest/index content
	manifestReader, err := repo.Fetch(ctx, descriptor)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer manifestReader.Close()

	manifestData, err := content.ReadAll(manifestReader, descriptor)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	// Check if this is a multi-arch image index
	switch descriptor.MediaType {
	case ocispec.MediaTypeImageIndex, "application/vnd.docker.distribution.manifest.list.v2+json":
		// Handle multi-arch index - resolve to platform-specific manifest
		return f.pullManifestFromIndex(ctx, repo, manifestData, params)
	}

	// Parse as single-platform manifest
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return &descriptor, &manifest, nil
}

// pullManifestFromIndex resolves a multi-arch index to the appropriate platform manifest
func (f *ORASFetcher) pullManifestFromIndex(ctx context.Context, repo *remote.Repository, indexData []byte, params *OCIPluginParams) (*ocispec.Descriptor, *ocispec.Manifest, error) {
	var index ocispec.Index
	if err := json.Unmarshal(indexData, &index); err != nil {
		return nil, nil, fmt.Errorf("failed to parse index: %w", err)
	}

	// Normalize target architecture
	targetArch := params.Architecture
	if targetArch == "" {
		targetArch = runtime.GOOS + "/" + runtime.GOARCH
	}

	// Find matching platform manifest
	manifestDesc, err := f.selectPlatformManifest(&index, targetArch)
	if err != nil {
		return nil, nil, err
	}

	// Fetch the platform-specific manifest
	manifestReader, err := repo.Fetch(ctx, *manifestDesc)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch platform manifest: %w", err)
	}
	defer manifestReader.Close()

	manifestData, err := content.ReadAll(manifestReader, *manifestDesc)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read platform manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, nil, fmt.Errorf("failed to parse platform manifest: %w", err)
	}

	return manifestDesc, &manifest, nil
}

// CheckExists verifies if an artifact exists in the registry without downloading
func (f *ORASFetcher) CheckExists(ctx context.Context, ref *OCIReference, params *OCIPluginParams) (bool, error) {
	// Create remote repository
	repo, err := remote.NewRepository(ref.FullRepo())
	if err != nil {
		return false, fmt.Errorf("failed to create remote repository: %w", err)
	}

	// Configure for insecure (HTTP) registries if needed
	if f.isInsecureRegistry(ref.Registry) {
		repo.PlainHTTP = true
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

// isInsecureRegistry checks if a registry should use HTTP instead of HTTPS
func (f *ORASFetcher) isInsecureRegistry(registry string) bool {
	// Check if registry is in the insecure registries list
	for _, insecureRegistry := range f.config.InsecureRegistries {
		if registry == insecureRegistry {
			return true
		}
	}

	// Localhost registries are typically insecure for development
	if strings.HasPrefix(registry, "localhost:") || strings.HasPrefix(registry, "127.0.0.1:") {
		return true
	}

	return false
}