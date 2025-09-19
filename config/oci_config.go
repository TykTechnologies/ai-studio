package config

import (
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
)

// OCIConfig holds OCI plugin configuration for AI Studio
// Uses AI_STUDIO_OCI_* environment variables to avoid conflicts with microgateway
type OCIConfig struct {
	// Cache settings
	CacheDir     string `env:"AI_STUDIO_OCI_CACHE_DIR" envDefault:""`
	MaxCacheSize int64  `env:"AI_STUDIO_OCI_MAX_CACHE_SIZE" envDefault:"1073741824"` // 1GB

	// Security settings
	AllowedRegistries []string `env:"AI_STUDIO_OCI_ALLOWED_REGISTRIES" envSeparator:","`
	RequireSignature  bool     `env:"AI_STUDIO_OCI_REQUIRE_SIGNATURE" envDefault:"false"` // More permissive default for AI Studio

	// Network settings
	Timeout       time.Duration `env:"AI_STUDIO_OCI_TIMEOUT" envDefault:"30s"`
	RetryAttempts int          `env:"AI_STUDIO_OCI_RETRY_ATTEMPTS" envDefault:"3"`

	// Garbage collection
	GCInterval   time.Duration `env:"AI_STUDIO_OCI_GC_INTERVAL" envDefault:"24h"`
	KeepVersions int          `env:"AI_STUDIO_OCI_KEEP_VERSIONS" envDefault:"3"`

	// Advanced settings
	InsecureRegistries []string `env:"AI_STUDIO_OCI_INSECURE_REGISTRIES" envSeparator:","`
}

// IsEnabled returns true if OCI support is enabled (cache directory is configured)
func (c *OCIConfig) IsEnabled() bool {
	return c.CacheDir != ""
}

// SetDefaults sets sensible default values for AI Studio
func (c *OCIConfig) SetDefaults() {
	if c.CacheDir == "" {
		// Don't set a default - OCI is opt-in
		return
	}

	if c.MaxCacheSize <= 0 {
		c.MaxCacheSize = 1024 * 1024 * 1024 // 1GB
	}

	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}

	if c.RetryAttempts <= 0 {
		c.RetryAttempts = 3
	}

	if c.GCInterval <= 0 {
		c.GCInterval = 24 * time.Hour
	}

	if c.KeepVersions <= 0 {
		c.KeepVersions = 3
	}
}

// Validate validates the OCI configuration
func (c *OCIConfig) Validate() error {
	if !c.IsEnabled() {
		return nil // OCI is optional
	}

	if c.MaxCacheSize <= 0 {
		return fmt.Errorf("AI_STUDIO_OCI_MAX_CACHE_SIZE must be positive")
	}

	if c.Timeout <= 0 {
		return fmt.Errorf("AI_STUDIO_OCI_TIMEOUT must be positive")
	}

	if c.RetryAttempts < 0 {
		return fmt.Errorf("AI_STUDIO_OCI_RETRY_ATTEMPTS must be non-negative")
	}

	if c.GCInterval <= 0 {
		return fmt.Errorf("AI_STUDIO_OCI_GC_INTERVAL must be positive")
	}

	if c.KeepVersions <= 0 {
		return fmt.Errorf("AI_STUDIO_OCI_KEEP_VERSIONS must be positive")
	}

	return nil
}

// ToOCILibConfig converts AI Studio OCI config to the library config format
// Reuses microgateway's registry auth and public key loading functions
func (c *OCIConfig) ToOCILibConfig() *ociplugins.OCIConfig {
	if !c.IsEnabled() {
		return nil
	}

	// Set defaults before conversion
	c.SetDefaults()

	return &ociplugins.OCIConfig{
		CacheDir:           c.CacheDir,
		MaxCacheSize:       c.MaxCacheSize,
		DefaultPublicKeys:  ociplugins.LoadPublicKeysFromEnv(),    // Reuse microgateway function
		AllowedRegistries:  c.AllowedRegistries,
		RegistryAuth:       ociplugins.LoadRegistryAuthFromEnv(),  // Reuse microgateway function
		Timeout:            c.Timeout,
		RetryAttempts:      c.RetryAttempts,
		RequireSignature:   c.RequireSignature,
		InsecureRegistries: c.InsecureRegistries,
		GCInterval:         c.GCInterval,
		KeepVersions:       c.KeepVersions,
	}
}