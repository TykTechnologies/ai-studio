// pkg/ociplugins/parser_test.go
package ociplugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOCICommand(t *testing.T) {
	tests := []struct {
		name          string
		command       string
		expectedRef   *OCIReference
		expectedParams *OCIPluginParams
		expectError   bool
	}{
		{
			name:    "basic OCI reference with digest",
			command: "oci://nexus.example.com/plugins/ner@sha256:0123deadbeef456",
			expectedRef: &OCIReference{
				Registry:   "nexus.example.com",
				Repository: "plugins/ner",
				Digest:     "sha256:0123deadbeef456",
				Tag:        "",
				Params:     make(map[string]string),
			},
			expectedParams: &OCIPluginParams{
				Architecture: "linux/amd64", // default
				PublicKey:    "",
				AuthConfig:   "",
			},
			expectError: false,
		},
		{
			name:    "OCI reference with tag",
			command: "oci://registry.com/myorg/plugin:v1.2.3",
			expectedRef: &OCIReference{
				Registry:   "registry.com",
				Repository: "myorg/plugin",
				Digest:     "",
				Tag:        "v1.2.3",
				Params:     make(map[string]string),
			},
			expectedParams: &OCIPluginParams{
				Architecture: "linux/amd64",
				PublicKey:    "",
				AuthConfig:   "",
			},
			expectError: false,
		},
		{
			name:    "OCI reference with parameters",
			command: "oci://registry.com/plugins/test@sha256:abc123?arch=linux/arm64&pubkey=test.pub&auth=prod",
			expectedRef: &OCIReference{
				Registry:   "registry.com",
				Repository: "plugins/test",
				Digest:     "sha256:abc123",
				Tag:        "",
				Params: map[string]string{
					"arch":   "linux/arm64",
					"pubkey": "test.pub",
					"auth":   "prod",
				},
			},
			expectedParams: &OCIPluginParams{
				Architecture: "linux/arm64",
				PublicKey:    "test.pub",
				AuthConfig:   "prod",
			},
			expectError: false,
		},
		{
			name:        "invalid scheme",
			command:     "https://registry.com/plugins/test",
			expectError: true,
		},
		{
			name:        "missing registry",
			command:     "oci://",
			expectError: true,
		},
		{
			name:        "invalid architecture",
			command:     "oci://registry.com/plugins/test@sha256:abc123?arch=invalid-arch",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, params, err := ParseOCICommand(tt.command)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedRef.Registry, ref.Registry)
			assert.Equal(t, tt.expectedRef.Repository, ref.Repository)
			assert.Equal(t, tt.expectedRef.Digest, ref.Digest)
			assert.Equal(t, tt.expectedRef.Tag, ref.Tag)
			assert.Equal(t, tt.expectedParams.Architecture, params.Architecture)
			assert.Equal(t, tt.expectedParams.PublicKey, params.PublicKey)
			assert.Equal(t, tt.expectedParams.AuthConfig, params.AuthConfig)
		})
	}
}

func TestOCIReference_FullReference(t *testing.T) {
	tests := []struct {
		name     string
		ref      *OCIReference
		expected string
	}{
		{
			name: "with digest",
			ref: &OCIReference{
				Registry:   "registry.com",
				Repository: "plugins/test",
				Digest:     "sha256:abc123",
			},
			expected: "registry.com/plugins/test@sha256:abc123",
		},
		{
			name: "with tag",
			ref: &OCIReference{
				Registry:   "registry.com",
				Repository: "plugins/test",
				Tag:        "v1.0.0",
			},
			expected: "registry.com/plugins/test:v1.0.0",
		},
		{
			name: "without digest or tag",
			ref: &OCIReference{
				Registry:   "registry.com",
				Repository: "plugins/test",
			},
			expected: "registry.com/plugins/test:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ref.FullReference()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateOCIReference(t *testing.T) {
	tests := []struct {
		name        string
		ref         *OCIReference
		config      *OCIConfig
		expectError bool
	}{
		{
			name: "allowed registry",
			ref: &OCIReference{
				Registry: "allowed.registry.com",
			},
			config: &OCIConfig{
				AllowedRegistries: []string{"allowed.registry.com", "another.registry.com"},
			},
			expectError: false,
		},
		{
			name: "disallowed registry",
			ref: &OCIReference{
				Registry: "malicious.registry.com",
			},
			config: &OCIConfig{
				AllowedRegistries: []string{"allowed.registry.com"},
			},
			expectError: true,
		},
		{
			name: "no allowlist configured",
			ref: &OCIReference{
				Registry: "any.registry.com",
			},
			config: &OCIConfig{
				AllowedRegistries: []string{},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOCIReference(tt.ref, tt.config)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}