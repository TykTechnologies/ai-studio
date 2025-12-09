// pkg/ociplugins/fetcher_test.go
package ociplugins

import (
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectPlatformManifest(t *testing.T) {
	fetcher := &ORASFetcher{config: DefaultOCIConfig()}

	tests := []struct {
		name           string
		index          *ocispec.Index
		targetArch     string
		expectedDigest string
		expectError    bool
		errorContains  string
	}{
		{
			name: "exact match linux/amd64",
			index: &ocispec.Index{
				Manifests: []ocispec.Descriptor{
					{
						Digest: "sha256:amd64digest",
						Platform: &ocispec.Platform{
							OS:           "linux",
							Architecture: "amd64",
						},
					},
					{
						Digest: "sha256:arm64digest",
						Platform: &ocispec.Platform{
							OS:           "linux",
							Architecture: "arm64",
						},
					},
				},
			},
			targetArch:     "linux/amd64",
			expectedDigest: "sha256:amd64digest",
			expectError:    false,
		},
		{
			name: "exact match darwin/arm64",
			index: &ocispec.Index{
				Manifests: []ocispec.Descriptor{
					{
						Digest: "sha256:linuxamd64",
						Platform: &ocispec.Platform{
							OS:           "linux",
							Architecture: "amd64",
						},
					},
					{
						Digest: "sha256:darwinarm64",
						Platform: &ocispec.Platform{
							OS:           "darwin",
							Architecture: "arm64",
						},
					},
				},
			},
			targetArch:     "darwin/arm64",
			expectedDigest: "sha256:darwinarm64",
			expectError:    false,
		},
		{
			name: "compatible fallback arm64 to amd64",
			index: &ocispec.Index{
				Manifests: []ocispec.Descriptor{
					{
						Digest: "sha256:amd64only",
						Platform: &ocispec.Platform{
							OS:           "darwin",
							Architecture: "amd64",
						},
					},
				},
			},
			targetArch:     "darwin/arm64",
			expectedDigest: "sha256:amd64only",
			expectError:    false,
		},
		{
			name: "no compatible architecture",
			index: &ocispec.Index{
				Manifests: []ocispec.Descriptor{
					{
						Digest: "sha256:linuxonly",
						Platform: &ocispec.Platform{
							OS:           "linux",
							Architecture: "amd64",
						},
					},
				},
			},
			targetArch:    "darwin/arm64",
			expectError:   true,
			errorContains: "darwin/arm64",
		},
		{
			name: "empty manifests list",
			index: &ocispec.Index{
				Manifests: []ocispec.Descriptor{},
			},
			targetArch:    "linux/amd64",
			expectError:   true,
			errorContains: "no manifests found",
		},
		{
			name: "manifest without platform info - skip",
			index: &ocispec.Index{
				Manifests: []ocispec.Descriptor{
					{
						Digest:   "sha256:noplatform",
						Platform: nil,
					},
					{
						Digest: "sha256:withplatform",
						Platform: &ocispec.Platform{
							OS:           "linux",
							Architecture: "amd64",
						},
					},
				},
			},
			targetArch:     "linux/amd64",
			expectedDigest: "sha256:withplatform",
			expectError:    false,
		},
		{
			name: "prefers exact match over compatible",
			index: &ocispec.Index{
				Manifests: []ocispec.Descriptor{
					{
						Digest: "sha256:amd64",
						Platform: &ocispec.Platform{
							OS:           "darwin",
							Architecture: "amd64",
						},
					},
					{
						Digest: "sha256:arm64",
						Platform: &ocispec.Platform{
							OS:           "darwin",
							Architecture: "arm64",
						},
					},
				},
			},
			targetArch:     "darwin/arm64",
			expectedDigest: "sha256:arm64",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc, err := fetcher.selectPlatformManifest(tt.index, tt.targetArch)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, desc)
			} else {
				require.NoError(t, err)
				require.NotNil(t, desc)
				assert.Equal(t, tt.expectedDigest, string(desc.Digest))
			}
		})
	}
}

func TestIsCompatibleArchitectureFetcher(t *testing.T) {
	fetcher := &ORASFetcher{config: DefaultOCIConfig()}

	tests := []struct {
		name        string
		layerArch   string
		targetArch  string
		expected    bool
	}{
		{
			name:       "exact match",
			layerArch:  "linux/amd64",
			targetArch: "linux/amd64",
			expected:   true,
		},
		{
			name:       "arm64 can run amd64",
			layerArch:  "darwin/amd64",
			targetArch: "darwin/arm64",
			expected:   true,
		},
		{
			name:       "amd64 cannot run arm64",
			layerArch:  "linux/arm64",
			targetArch: "linux/amd64",
			expected:   false,
		},
		{
			name:       "different OS not compatible",
			layerArch:  "linux/amd64",
			targetArch: "darwin/amd64",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fetcher.isCompatibleArchitecture(tt.layerArch, tt.targetArch)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewORASFetcher(t *testing.T) {
	config := DefaultOCIConfig()
	fetcher, err := NewORASFetcher(config)

	require.NoError(t, err)
	assert.NotNil(t, fetcher)
	assert.Equal(t, config, fetcher.config)
}

func TestIsInsecureRegistry(t *testing.T) {
	config := DefaultOCIConfig()
	config.InsecureRegistries = []string{"my-insecure-reg:5000"}

	fetcher := &ORASFetcher{config: config}

	tests := []struct {
		name     string
		registry string
		expected bool
	}{
		{
			name:     "localhost is insecure",
			registry: "localhost:5000",
			expected: true,
		},
		{
			name:     "127.0.0.1 is insecure",
			registry: "127.0.0.1:5000",
			expected: true,
		},
		{
			name:     "configured insecure registry",
			registry: "my-insecure-reg:5000",
			expected: true,
		},
		{
			name:     "regular registry is secure",
			registry: "ghcr.io",
			expected: false,
		},
		{
			name:     "docker.io is secure",
			registry: "docker.io",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fetcher.isInsecureRegistry(tt.registry)
			assert.Equal(t, tt.expected, result)
		})
	}
}
