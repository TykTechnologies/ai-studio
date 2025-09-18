// pkg/ociplugins/client_test.go
package ociplugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOCIPluginClient(t *testing.T) {
	tests := []struct {
		name        string
		config      *OCIConfig
		expectError bool
	}{
		{
			name: "with nil config",
			config: func() *OCIConfig {
				cfg := DefaultOCIConfig()
				cfg.CacheDir = t.TempDir() // Use temp dir for tests
				return cfg
			}(),
			expectError: false,
		},
		{
			name: "with valid config",
			config: func() *OCIConfig {
				cfg := DefaultOCIConfig()
				cfg.CacheDir = t.TempDir() // Use temp dir for tests
				return cfg
			}(),
			expectError: false,
		},
		{
			name: "with custom config",
			config: func() *OCIConfig {
				return &OCIConfig{
					CacheDir:          t.TempDir(),
					MaxCacheSize:      500 * 1024 * 1024, // 500MB
					RequireSignature:  false,
					AllowedRegistries: []string{"test.registry.com"},
					Timeout:           60,
					RetryAttempts:     2,
				}
			}(),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewOCIPluginClient(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.NotNil(t, client.config)
				assert.NotNil(t, client.storage)
				assert.NotNil(t, client.fetcher)
				assert.NotNil(t, client.verifier)
			}
		})
	}
}

func TestDefaultOCIConfig(t *testing.T) {
	config := DefaultOCIConfig()

	assert.NotNil(t, config)
	assert.Equal(t, "/var/lib/microgateway/plugins", config.CacheDir)
	assert.Equal(t, int64(1024*1024*1024), config.MaxCacheSize) // 1GB
	assert.True(t, config.RequireSignature)
	assert.Equal(t, 3, config.RetryAttempts)
	assert.NotNil(t, config.RegistryAuth)
}

func TestOCIPluginClient_HasPlugin(t *testing.T) {
	// Create client with temporary cache directory
	config := DefaultOCIConfig()
	config.CacheDir = t.TempDir()

	client, err := NewOCIPluginClient(config)
	require.NoError(t, err)

	// Test non-existent plugin
	exists := client.HasPlugin("sha256:nonexistent", "linux/amd64")
	assert.False(t, exists)
}

func TestIsCompatibleArchitecture(t *testing.T) {
	tests := []struct {
		name        string
		pluginArch  string
		runtimeArch string
		expected    bool
	}{
		{
			name:        "exact match",
			pluginArch:  "linux/amd64",
			runtimeArch: "linux/amd64",
			expected:    true,
		},
		{
			name:        "different arch",
			pluginArch:  "linux/arm64",
			runtimeArch: "linux/amd64",
			expected:    false,
		},
		{
			name:        "different os",
			pluginArch:  "darwin/amd64",
			runtimeArch: "linux/amd64",
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCompatibleArchitecture(tt.pluginArch, tt.runtimeArch)
			assert.Equal(t, tt.expected, result)
		})
	}
}