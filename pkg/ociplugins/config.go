// pkg/ociplugins/config.go
package ociplugins

import (
	"os"
	"strings"
	"time"
)

// OCIConfig holds the configuration for OCI plugin operations
type OCIConfig struct {
	// Cache settings
	CacheDir     string `yaml:"cache_dir" json:"cache_dir"`
	MaxCacheSize int64  `yaml:"max_cache_size" json:"max_cache_size"`

	// Default security settings
	DefaultPublicKeys []string `yaml:"default_public_keys" json:"default_public_keys"`
	AllowedRegistries []string `yaml:"allowed_registries" json:"allowed_registries"`

	// Authentication
	RegistryAuth map[string]RegistryAuth `yaml:"registry_auth" json:"registry_auth"`

	// Network settings
	Timeout       time.Duration `yaml:"timeout" json:"timeout"`
	RetryAttempts int          `yaml:"retry_attempts" json:"retry_attempts"`

	// Security settings
	RequireSignature bool `yaml:"require_signature" json:"require_signature"`

	// Garbage collection
	GCInterval   time.Duration `yaml:"gc_interval" json:"gc_interval"`
	KeepVersions int          `yaml:"keep_versions" json:"keep_versions"`
}

// RegistryAuth holds authentication configuration for a specific registry
type RegistryAuth struct {
	Username    string `yaml:"username" json:"username"`
	PasswordEnv string `yaml:"password_env" json:"password_env"` // Environment variable name
	Token       string `yaml:"token" json:"token"`               // Direct token (less secure)
	TokenEnv    string `yaml:"token_env" json:"token_env"`       // Token from environment
}

// OCIReference represents a parsed OCI reference
type OCIReference struct {
	Registry   string            `json:"registry"`   // nexus.example.com
	Repository string            `json:"repository"` // plugins/ner
	Digest     string            `json:"digest"`     // sha256:...
	Tag        string            `json:"tag"`        // optional, mainly for development
	Params     map[string]string `json:"params"`     // Query parameters
}

// OCIPluginParams represents parsed parameters from OCI command
type OCIPluginParams struct {
	Architecture string `json:"architecture"` // linux/amd64, linux/arm64
	PublicKey    string `json:"public_key"`   // Key identifier or path
	AuthConfig   string `json:"auth_config"`  // Auth configuration name
}

// LocalPlugin represents a plugin that has been fetched and is available locally
type LocalPlugin struct {
	Reference      *OCIReference    `json:"reference"`
	Params         *OCIPluginParams `json:"params"`
	ExecutablePath string           `json:"executable_path"` // Path to executable binary
	ConfigPath     string           `json:"config_path"`     // Optional config JSON path
	CacheDir       string           `json:"cache_dir"`       // Storage location
	Verified       bool             `json:"verified"`        // Cosign verification status
	FetchTime      time.Time        `json:"fetch_time"`      // When downloaded
	Config         *PluginConfig    `json:"config"`          // Parsed from OCI config layer
}

// PluginMetadata represents cached metadata about a plugin
type PluginMetadata struct {
	Reference    *OCIReference    `json:"reference"`
	Params       *OCIPluginParams `json:"params"`
	FetchTime    time.Time        `json:"fetch_time"`
	Config       *PluginConfig    `json:"config"`
	Verified     bool             `json:"verified"`
	Size         int64            `json:"size"`
	LastAccessed time.Time        `json:"last_accessed"`
	Version      string           `json:"version"` // From plugin config
}

// PluginConfig represents the configuration metadata stored in the OCI artifact
type PluginConfig struct {
	Name           string   `json:"name"`
	Version        string   `json:"version"`
	PluginAPI      string   `json:"plugin_api"`
	OS             string   `json:"os"`
	Arch           string   `json:"arch"`
	LibC           string   `json:"libc,omitempty"`
	HostMinVersion string   `json:"host_min_version,omitempty"`
	Capabilities   []string `json:"capabilities,omitempty"`
	Notes          string   `json:"notes,omitempty"`
}

// FullRepo returns the full repository path (registry/repository)
func (r *OCIReference) FullRepo() string {
	return r.Registry + "/" + r.Repository
}

// FullReference returns the complete OCI reference
func (r *OCIReference) FullReference() string {
	if r.Digest != "" {
		return r.FullRepo() + "@" + r.Digest
	}
	if r.Tag != "" {
		return r.FullRepo() + ":" + r.Tag
	}
	return r.FullRepo() + ":latest"
}

// DefaultOCIConfig returns a configuration with sensible defaults
func DefaultOCIConfig() *OCIConfig {
	config := &OCIConfig{
		CacheDir:          "/var/lib/microgateway/plugins",
		MaxCacheSize:      1024 * 1024 * 1024, // 1GB
		DefaultPublicKeys: []string{},
		AllowedRegistries: []string{},
		RegistryAuth:      make(map[string]RegistryAuth),
		Timeout:           30 * time.Second,
		RetryAttempts:     3,
		RequireSignature:  true,
		GCInterval:        24 * time.Hour,
		KeepVersions:      3,
	}

	// Load registry authentication from environment variables
	config.RegistryAuth = LoadRegistryAuthFromEnv()

	return config
}

// LoadRegistryAuthFromEnv loads registry authentication configuration from environment variables
// Environment variables pattern:
// OCI_PLUGINS_REGISTRY_<NAME>_USERNAME
// OCI_PLUGINS_REGISTRY_<NAME>_PASSWORD_ENV
// OCI_PLUGINS_REGISTRY_<NAME>_TOKEN_ENV
func LoadRegistryAuthFromEnv() map[string]RegistryAuth {
	registryAuth := make(map[string]RegistryAuth)

	// Scan environment variables for registry auth patterns
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "OCI_PLUGINS_REGISTRY_") {
			continue
		}

		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		// Parse: OCI_PLUGINS_REGISTRY_<REGISTRY_NAME>_<FIELD>
		keyParts := strings.Split(key, "_")
		if len(keyParts) < 5 {
			continue
		}

		// Extract registry name and field (handle multiple parts in registry name)
		var registryParts []string
		var field string

		for i := 3; i < len(keyParts); i++ {
			if i == len(keyParts)-1 {
				// Last part is the field
				field = strings.ToLower(keyParts[i])
			} else {
				// Part of registry name
				registryParts = append(registryParts, strings.ToLower(keyParts[i]))
			}
		}

		if len(registryParts) == 0 || field == "" {
			continue
		}

		registryName := strings.Join(registryParts, ".")

		// Initialize registry auth if not exists
		auth, exists := registryAuth[registryName]
		if !exists {
			auth = RegistryAuth{}
		}

		// Set field based on type
		switch field {
		case "username":
			auth.Username = value
		case "password":
			// Direct password (less secure)
			auth.Token = value // Store as token for direct password
		case "passwordenv":
			// Environment variable containing password
			auth.PasswordEnv = value
		case "token":
			// Direct token (less secure)
			auth.Token = value
		case "tokenenv":
			// Environment variable containing token
			auth.TokenEnv = value
		default:
			// Skip unknown fields
			continue
		}

		registryAuth[registryName] = auth
	}

	return registryAuth
}

// LoadRegistryAuthForRegistry loads authentication for a specific registry
func LoadRegistryAuthForRegistry(registryName string) *RegistryAuth {
	normalizedName := strings.ToUpper(strings.ReplaceAll(registryName, ".", "_"))
	normalizedName = strings.ReplaceAll(normalizedName, "-", "_")

	auth := &RegistryAuth{}
	hasAuth := false

	// Check for username
	if username := os.Getenv("OCI_PLUGINS_REGISTRY_" + normalizedName + "_USERNAME"); username != "" {
		auth.Username = username
		hasAuth = true
	}

	// Check for password environment variable
	if passwordEnv := os.Getenv("OCI_PLUGINS_REGISTRY_" + normalizedName + "_PASSWORD_ENV"); passwordEnv != "" {
		auth.PasswordEnv = passwordEnv
		hasAuth = true
	}

	// Check for token environment variable
	if tokenEnv := os.Getenv("OCI_PLUGINS_REGISTRY_" + normalizedName + "_TOKEN_ENV"); tokenEnv != "" {
		auth.TokenEnv = tokenEnv
		hasAuth = true
	}

	// Check for direct token (less secure, but sometimes needed)
	if token := os.Getenv("OCI_PLUGINS_REGISTRY_" + normalizedName + "_TOKEN"); token != "" {
		auth.Token = token
		hasAuth = true
	}

	if !hasAuth {
		return nil
	}

	return auth
}