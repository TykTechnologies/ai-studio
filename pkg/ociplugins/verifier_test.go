// pkg/ociplugins/verifier_test.go
package ociplugins

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Sample test public key (PEM format) - for testing only
const testPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA4qiw8PWe4N5yKnXNAneu
TGGw6Gi6zp0SUHmQPIeP3w+2aV5PpnpNf8QzVwXFyLHb8gj9pkpUlzALVVLLSU/i
U7A8Vd5pNX4gBwR9pnT8+XtQHqgA4Q4p2lPtXqDdFJY8xvj5TgE2LqNzrOhXqYf
H2Z9GQ7+3Qz2GjYZfFQoYu6FK2CvN0q2VnSrO+Y0vf1Hf9y8D0G3Zn8m9Lb8P3F
XJ2Z9nQ7K8g9L0E+M4C7Nz2GbE3P8CwXf4D5Z1F2H8K4P9LjNvXr4R+2QIDAQAB
-----END PUBLIC KEY-----`

const testPublicKeyPEM2 = `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAyV8l2Z7X3f+EQ8/QJ4qK
N5S8FfM4lK2V3gR4mE9L0W7H4R5P8C+X1Y6vN3Q2Z8F+4G9B1J3E7V9S2L5K4H8
P6F3A2Y8C1N7W5Q4T9R6G0B8D5E2M3L9K7S4F6H1P0X3V2N8C5A1Y4W7Q9R2T8
K3G6B9E5S7F1H4L0P2V8C6N4A9Y3W1Q7R5T2G8B0D4E6S3F9H7L1P5V0C2N8A4
YK7W6Q1R3T9G5B2D8E0S4F7H3L6P9V1C5N2A8Y4W0Q3R7T1G9B6D4E2S8F5H1P3
QIDAQAB
-----END PUBLIC KEY-----`

func TestLoadPublicKeysFromEnv(t *testing.T) {
	// Save and restore environment
	originalEnv := saveEnvironment()
	defer restoreEnvironment(originalEnv)

	// Clear any existing OCI plugin environment variables
	clearOCIPluginEnvVars()

	// Test 1: Numbered keys (embedded default always included)
	os.Setenv("OCI_PLUGINS_PUBKEY_1", testPublicKeyPEM)
	os.Setenv("OCI_PLUGINS_PUBKEY_2", testPublicKeyPEM2)

	keys := LoadPublicKeysFromEnv()
	assert.Len(t, keys, 3)
	assert.Equal(t, "embedded:default", keys[0], "embedded Tyk key should always be first")
	assert.Contains(t, keys, "env:OCI_PLUGINS_PUBKEY_1")
	assert.Contains(t, keys, "env:OCI_PLUGINS_PUBKEY_2")

	// Test 2: Named keys (embedded default always included)
	clearOCIPluginEnvVars()
	os.Setenv("OCI_PLUGINS_PUBKEY_CI", testPublicKeyPEM)
	os.Setenv("OCI_PLUGINS_PUBKEY_PROD", testPublicKeyPEM2)

	keys = LoadPublicKeysFromEnv()
	assert.Len(t, keys, 3)
	assert.Equal(t, "embedded:default", keys[0], "embedded Tyk key should always be first")
	assert.Contains(t, keys, "env:OCI_PLUGINS_PUBKEY_CI")
	assert.Contains(t, keys, "env:OCI_PLUGINS_PUBKEY_PROD")

	// Test 3: File-based keys (embedded default always included)
	clearOCIPluginEnvVars()
	tempFile1 := createTempKeyFile(t, testPublicKeyPEM)
	tempFile2 := createTempKeyFile(t, testPublicKeyPEM2)
	defer os.Remove(tempFile1)
	defer os.Remove(tempFile2)

	os.Setenv("OCI_PLUGINS_PUBKEY_FILE_CI", tempFile1)
	os.Setenv("OCI_PLUGINS_PUBKEY_FILE_PROD", tempFile2)

	keys = LoadPublicKeysFromEnv()
	assert.Len(t, keys, 3)
	assert.Equal(t, "embedded:default", keys[0], "embedded Tyk key should always be first")
	assert.Contains(t, keys, "file:"+tempFile1)
	assert.Contains(t, keys, "file:"+tempFile2)

	// Test 4: Mixed keys (embedded default always included)
	clearOCIPluginEnvVars()
	os.Setenv("OCI_PLUGINS_PUBKEY_1", testPublicKeyPEM)
	os.Setenv("OCI_PLUGINS_PUBKEY_CI", testPublicKeyPEM2)
	os.Setenv("OCI_PLUGINS_PUBKEY_FILE_DEV", tempFile1)

	keys = LoadPublicKeysFromEnv()
	assert.Len(t, keys, 4)
	assert.Equal(t, "embedded:default", keys[0], "embedded Tyk key should always be first")
	assert.Contains(t, keys, "env:OCI_PLUGINS_PUBKEY_1")
	assert.Contains(t, keys, "env:OCI_PLUGINS_PUBKEY_CI")
	assert.Contains(t, keys, "file:"+tempFile1)

	// Test 5: No env keys — embedded default is still present
	clearOCIPluginEnvVars()
	keys = LoadPublicKeysFromEnv()
	assert.Len(t, keys, 1)
	assert.Equal(t, "embedded:default", keys[0], "embedded Tyk key should be present even with no env vars")
}

func TestSignatureVerifier_resolveKeyReference(t *testing.T) {
	// Save and restore environment
	originalEnv := saveEnvironment()
	defer restoreEnvironment(originalEnv)

	clearOCIPluginEnvVars()

	// Set up test environment
	os.Setenv("OCI_PLUGINS_PUBKEY_1", testPublicKeyPEM)
	os.Setenv("OCI_PLUGINS_PUBKEY_CI", testPublicKeyPEM2)

	config := &OCIConfig{
		DefaultPublicKeys: LoadPublicKeysFromEnv(),
	}

	verifier, err := NewSignatureVerifier(config)
	require.NoError(t, err)

	tests := []struct {
		name        string
		keyRef      string
		setupFunc   func() string // Function to set up test-specific resources
		expectError bool
	}{
		{
			name:        "numeric reference",
			keyRef:      "1",
			expectError: false,
		},
		{
			name:        "named reference",
			keyRef:      "CI",
			expectError: false,
		},
		{
			name:        "environment variable reference",
			keyRef:      "env:OCI_PLUGINS_PUBKEY_1",
			expectError: false,
		},
		{
			name: "file reference",
			setupFunc: func() string {
				return createTempKeyFile(t, testPublicKeyPEM)
			},
			expectError: false,
		},
		{
			name: "file prefix reference",
			setupFunc: func() string {
				tempFile := createTempKeyFile(t, testPublicKeyPEM)
				return "file:" + tempFile
			},
			expectError: false,
		},
		{
			name:        "nonexistent numeric",
			keyRef:      "99",
			expectError: true,
		},
		{
			name:        "nonexistent named",
			keyRef:      "NONEXISTENT",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up test-specific resources
			keyRef := tt.keyRef
			var tempFile string
			if tt.setupFunc != nil {
				keyRef = tt.setupFunc()
				if strings.HasPrefix(keyRef, "file:") {
					tempFile = strings.TrimPrefix(keyRef, "file:")
				} else {
					tempFile = keyRef
				}
			}

			keyPath, err := verifier.resolveKeyReference(keyRef)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, keyPath)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, keyPath)

				// Verify the resolved file exists and is readable
				_, err := os.Stat(keyPath)
				assert.NoError(t, err)

				// Clean up temp file if created
				if strings.HasPrefix(keyPath, os.TempDir()) {
					defer os.Remove(keyPath)
				}
			}

			// Clean up setup temp file
			if tempFile != "" && tempFile != keyPath {
				os.Remove(tempFile)
			}
		})
	}
}

func TestSignatureVerifier_writeKeyToTempFile(t *testing.T) {
	config := &OCIConfig{}
	verifier, err := NewSignatureVerifier(config)
	require.NoError(t, err)

	// Test valid PEM content
	tempPath, err := verifier.writeKeyToTempFile(testPublicKeyPEM, "test")
	require.NoError(t, err)
	defer os.Remove(tempPath)

	// Verify file was created and contains correct content
	content, err := os.ReadFile(tempPath)
	require.NoError(t, err)
	assert.Equal(t, testPublicKeyPEM, string(content))

	// Test invalid PEM content
	_, err = verifier.writeKeyToTempFile("invalid content", "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid PEM content")
}

func TestPEMContentDetection(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "valid PEM",
			content:  testPublicKeyPEM,
			expected: true,
		},
		{
			name:     "invalid content",
			content:  "not a PEM key",
			expected: false,
		},
		{
			name:     "partial PEM",
			content:  "-----BEGIN PUBLIC KEY-----\nincomplete",
			expected: false,
		},
		{
			name:     "empty content",
			content:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPEMContent(tt.content)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultOCIConfigWithEmbeddedKeys(t *testing.T) {
	// Save and restore environment
	originalEnv := saveEnvironment()
	defer restoreEnvironment(originalEnv)

	clearOCIPluginEnvVars()

	// Set up test keys
	os.Setenv("OCI_PLUGINS_PUBKEY_1", testPublicKeyPEM)
	os.Setenv("OCI_PLUGINS_PUBKEY_CI", testPublicKeyPEM2)

	config := DefaultOCIConfig()

	// Should have embedded default key plus keys from environment
	assert.Len(t, config.DefaultPublicKeys, 3)
	assert.Equal(t, "embedded:default", config.DefaultPublicKeys[0], "embedded Tyk key should always be first")
	assert.Contains(t, config.DefaultPublicKeys, "env:OCI_PLUGINS_PUBKEY_1")
	assert.Contains(t, config.DefaultPublicKeys, "env:OCI_PLUGINS_PUBKEY_CI")
}

// TestEmbeddedDefaultKeyAlwaysPresent verifies that the embedded Tyk official
// signing key is always loaded and resolvable, regardless of environment config.
func TestEmbeddedDefaultKeyAlwaysPresent(t *testing.T) {
	originalEnv := saveEnvironment()
	defer restoreEnvironment(originalEnv)

	t.Run("present with no env keys", func(t *testing.T) {
		clearOCIPluginEnvVars()

		keys := LoadPublicKeysFromEnv()
		require.NotEmpty(t, keys)
		assert.Equal(t, "embedded:default", keys[0])
	})

	t.Run("present alongside env keys", func(t *testing.T) {
		clearOCIPluginEnvVars()
		os.Setenv("OCI_PLUGINS_PUBKEY_1", testPublicKeyPEM)

		keys := LoadPublicKeysFromEnv()
		assert.Equal(t, "embedded:default", keys[0], "embedded key must be first even when env keys are set")
		assert.Contains(t, keys, "env:OCI_PLUGINS_PUBKEY_1")
	})

	t.Run("resolves to valid PEM file", func(t *testing.T) {
		clearOCIPluginEnvVars()

		config := &OCIConfig{
			DefaultPublicKeys: LoadPublicKeysFromEnv(),
		}
		verifier, err := NewSignatureVerifier(config)
		require.NoError(t, err)

		keyPath, err := verifier.resolveKeyReference("embedded:default")
		require.NoError(t, err)
		defer os.Remove(keyPath)

		// File must exist and contain the expected PEM content
		content, err := os.ReadFile(keyPath)
		require.NoError(t, err)
		assert.Equal(t, DefaultTykPublicKey, string(content))
	})

	t.Run("getPublicKeyPath uses embedded key when no specific key requested", func(t *testing.T) {
		clearOCIPluginEnvVars()

		config := &OCIConfig{
			DefaultPublicKeys: LoadPublicKeysFromEnv(),
		}
		verifier, err := NewSignatureVerifier(config)
		require.NoError(t, err)

		// Empty pubKeyID should fall back to first key, which is the embedded default
		keyPath, err := verifier.getPublicKeyPath("")
		require.NoError(t, err)
		defer os.Remove(keyPath)

		content, err := os.ReadFile(keyPath)
		require.NoError(t, err)
		assert.Equal(t, DefaultTykPublicKey, string(content))
	})
}

// Helper functions

func saveEnvironment() map[string]string {
	env := make(map[string]string)
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "OCI_PLUGINS_") {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				env[parts[0]] = parts[1]
			}
		}
	}
	return env
}

func restoreEnvironment(env map[string]string) {
	// Clear all OCI plugin env vars
	clearOCIPluginEnvVars()

	// Restore original values
	for key, value := range env {
		os.Setenv(key, value)
	}
}

func clearOCIPluginEnvVars() {
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "OCI_PLUGINS_") {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) >= 1 {
				os.Unsetenv(parts[0])
			}
		}
	}
}

func createTempKeyFile(t *testing.T, content string) string {
	tempFile, err := os.CreateTemp("", "test-key-*.pub")
	require.NoError(t, err)

	_, err = tempFile.WriteString(content)
	require.NoError(t, err)

	err = tempFile.Close()
	require.NoError(t, err)

	return tempFile.Name()
}