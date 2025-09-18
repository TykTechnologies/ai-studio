// pkg/ociplugins/config_test.go
package ociplugins

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRegistryAuthFromEnv(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	for _, env := range os.Environ() {
		if len(env) > 0 {
			parts := splitEnv(env)
			if len(parts) == 2 {
				originalEnv[parts[0]] = parts[1]
			}
		}
	}

	// Clean up after test
	defer func() {
		// Clear test env vars
		testVars := []string{
			"OCI_PLUGINS_REGISTRY_NEXUS_USERNAME",
			"OCI_PLUGINS_REGISTRY_NEXUS_PASSWORDENV",
			"OCI_PLUGINS_REGISTRY_HARBOR_TOKENENV",
		}
		for _, v := range testVars {
			os.Unsetenv(v)
		}
	}()

	// Set up test environment variables
	os.Setenv("OCI_PLUGINS_REGISTRY_NEXUS_USERNAME", "plugin-ci")
	os.Setenv("OCI_PLUGINS_REGISTRY_NEXUS_PASSWORDENV", "NEXUS_PASSWORD")
	os.Setenv("OCI_PLUGINS_REGISTRY_HARBOR_TOKENENV", "HARBOR_TOKEN")

	// Load registry auth
	registryAuth := LoadRegistryAuthFromEnv()

	// Debug: print what we got
	t.Logf("Found %d registry auth configs: %+v", len(registryAuth), registryAuth)

	// Verify nexus config
	nexusAuth, exists := registryAuth["nexus"]
	require.True(t, exists, "nexus registry auth should exist")
	assert.Equal(t, "plugin-ci", nexusAuth.Username)
	assert.Equal(t, "NEXUS_PASSWORD", nexusAuth.PasswordEnv)

	// Verify harbor config
	harborAuth, exists := registryAuth["harbor"]
	require.True(t, exists, "harbor registry auth should exist")
	assert.Equal(t, "HARBOR_TOKEN", harborAuth.TokenEnv)
}

func TestLoadRegistryAuthForRegistry(t *testing.T) {
	// Clean up after test
	defer func() {
		os.Unsetenv("OCI_PLUGINS_REGISTRY_EXAMPLE_COM_USERNAME")
		os.Unsetenv("OCI_PLUGINS_REGISTRY_EXAMPLE_COM_TOKEN_ENV")
	}()

	// Test registry with dots and dashes
	os.Setenv("OCI_PLUGINS_REGISTRY_EXAMPLE_COM_USERNAME", "test-user")
	os.Setenv("OCI_PLUGINS_REGISTRY_EXAMPLE_COM_TOKEN_ENV", "TEST_TOKEN")

	auth := LoadRegistryAuthForRegistry("example.com")
	require.NotNil(t, auth)
	assert.Equal(t, "test-user", auth.Username)
	assert.Equal(t, "TEST_TOKEN", auth.TokenEnv)

	// Test non-existent registry
	auth2 := LoadRegistryAuthForRegistry("nonexistent.com")
	assert.Nil(t, auth2)
}

func TestPluginMetadata(t *testing.T) {
	metadata := &PluginMetadata{
		Reference: &OCIReference{
			Registry:   "test.com",
			Repository: "plugins/test",
			Digest:     "sha256:abc123",
		},
		Params: &OCIPluginParams{
			Architecture: "linux/amd64",
		},
		Config: &PluginConfig{
			Name:    "test-plugin",
			Version: "1.0.0",
		},
		Verified: true,
		Size:     1024,
	}

	// Test JSON serialization
	data, err := json.Marshal(metadata)
	require.NoError(t, err)

	var unmarshaled PluginMetadata
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, metadata.Reference.Registry, unmarshaled.Reference.Registry)
	assert.Equal(t, metadata.Config.Name, unmarshaled.Config.Name)
	assert.Equal(t, metadata.Verified, unmarshaled.Verified)
}

func TestEnhancedArchitectureCompatibility(t *testing.T) {
	tests := []struct {
		name        string
		pluginArch  string
		runtimeArch string
		expected    bool
	}{
		{
			name:        "exact match linux/amd64",
			pluginArch:  "linux/amd64",
			runtimeArch: "linux/amd64",
			expected:    true,
		},
		{
			name:        "amd64 on arm64 (emulation)",
			pluginArch:  "linux/amd64",
			runtimeArch: "linux/arm64",
			expected:    true,
		},
		{
			name:        "arm64 on amd64 (not compatible)",
			pluginArch:  "linux/arm64",
			runtimeArch: "linux/amd64",
			expected:    false,
		},
		{
			name:        "cross-OS not supported",
			pluginArch:  "darwin/amd64",
			runtimeArch: "linux/amd64",
			expected:    false,
		},
		{
			name:        "invalid plugin arch format",
			pluginArch:  "invalid",
			runtimeArch: "linux/amd64",
			expected:    false,
		},
		{
			name:        "invalid runtime arch format",
			pluginArch:  "linux/amd64",
			runtimeArch: "invalid",
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

// splitEnv splits environment variable string "KEY=VALUE" into ["KEY", "VALUE"]
func splitEnv(env string) []string {
	idx := strings.Index(env, "=")
	if idx == -1 {
		return []string{env}
	}
	return []string{env[:idx], env[idx+1:]}
}