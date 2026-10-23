// pkg/ociplugins/keys_test.go
package ociplugins

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
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

func TestResolvePublicKey(t *testing.T) {
	// Save and restore environment
	originalEnv := saveEnvironment()
	defer restoreEnvironment(originalEnv)
	clearOCIPluginEnvVars()

	// No temp files are written while resolving keys.
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	keyDir := t.TempDir()
	keyFile := filepath.Join(keyDir, "signing.pub")
	require.NoError(t, os.WriteFile(keyFile, []byte(testPublicKeyPEM2), 0o600))
	badFile := filepath.Join(keyDir, "bad.pub")
	require.NoError(t, os.WriteFile(badFile, []byte("not a key"), 0o600))
	namedFile := filepath.Join(keyDir, "named.pem")
	require.NoError(t, os.WriteFile(namedFile, []byte(testPublicKeyPEM), 0o600))

	os.Setenv("OCI_PLUGINS_PUBKEY_1", testPublicKeyPEM)
	os.Setenv("OCI_PLUGINS_PUBKEY_CI", testPublicKeyPEM2)
	os.Setenv("OCI_PLUGINS_PUBKEY_FILE_DEV", namedFile)
	os.Setenv("CUSTOM_KEY_VAR", testPublicKeyPEM)
	os.Setenv("OCI_PLUGINS_PUBKEY_2", "not a key")

	config := &OCIConfig{DefaultPublicKeys: LoadPublicKeysFromEnv()}

	tests := []struct {
		name     string
		cfg      *OCIConfig
		keyID    string
		want     string
		wantPath string
		wantErr  bool
	}{
		{name: "empty ID uses the first default key (embedded)", cfg: config, keyID: "", want: DefaultTykPublicKey},
		{name: "empty ID with no default keys", cfg: &OCIConfig{}, keyID: "", wantErr: true},
		{name: "embedded", cfg: config, keyID: "embedded:default", want: DefaultTykPublicKey},
		{name: "numeric env", cfg: config, keyID: "1", want: testPublicKeyPEM},
		{name: "numeric env missing", cfg: config, keyID: "99", wantErr: true},
		{name: "numeric env with invalid PEM", cfg: config, keyID: "2", wantErr: true},
		{name: "named env", cfg: config, keyID: "CI", want: testPublicKeyPEM2},
		{name: "named env, lower case", cfg: config, keyID: "ci", want: testPublicKeyPEM2},
		{name: "named PUBKEY_FILE env", cfg: config, keyID: "DEV", want: testPublicKeyPEM, wantPath: namedFile},
		{name: "named missing", cfg: config, keyID: "NONEXISTENT", wantErr: true},
		{name: "env: prefix", cfg: config, keyID: "env:CUSTOM_KEY_VAR", want: testPublicKeyPEM},
		{name: "env: prefix missing", cfg: config, keyID: "env:NO_SUCH_VAR", wantErr: true},
		{name: "file: prefix", cfg: config, keyID: "file:" + keyFile, want: testPublicKeyPEM2, wantPath: keyFile},
		{name: "file: prefix missing", cfg: config, keyID: "file:" + keyDir + "/missing.pub", wantErr: true},
		{name: "direct path", cfg: config, keyID: keyFile, want: testPublicKeyPEM2, wantPath: keyFile},
		{name: "direct path with invalid PEM", cfg: config, keyID: badFile, wantErr: true},
		{name: "configured-key search", cfg: &OCIConfig{DefaultPublicKeys: []string{"env:CUSTOM_KEY_VAR"}}, keyID: "CUSTOM", want: testPublicKeyPEM},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolvePublicKey(tt.cfg, tt.keyID)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))

			path, err := PublicKeyPath(tt.cfg, tt.keyID)
			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, path)
		})
	}

	entries, err := os.ReadDir(tmp)
	require.NoError(t, err)
	assert.Empty(t, entries, "resolving keys must not write temp files")
}

func TestParsePublicKey(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})

	key, err := ParsePublicKey(pemBytes)
	require.NoError(t, err)
	assert.True(t, priv.PublicKey.Equal(key))

	_, err = ParsePublicKey([]byte(DefaultTykPublicKey))
	assert.NoError(t, err, "the embedded Tyk key must parse")

	_, err = ParsePublicKey([]byte("not a key"))
	assert.Error(t, err)
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

	t.Run("resolves to the embedded PEM", func(t *testing.T) {
		clearOCIPluginEnvVars()

		config := &OCIConfig{
			DefaultPublicKeys: LoadPublicKeysFromEnv(),
		}
		key, err := ResolvePublicKey(config, "embedded:default")
		require.NoError(t, err)
		assert.Equal(t, DefaultTykPublicKey, string(key))
	})

	t.Run("an empty key ID uses the embedded key when no specific key requested", func(t *testing.T) {
		clearOCIPluginEnvVars()

		config := &OCIConfig{
			DefaultPublicKeys: LoadPublicKeysFromEnv(),
		}
		// Empty pubKeyID should fall back to first key, which is the embedded default
		key, err := ResolvePublicKey(config, "")
		require.NoError(t, err)
		assert.Equal(t, DefaultTykPublicKey, string(key))
	})
}

func TestLoadPublicKeysFromDirectoryAndValidate(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"cosign.pub", "a.pem", "b.key", "notes.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(testPublicKeyPEM), 0o600))
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub.pub"), 0o700))

	paths, err := LoadPublicKeysFromDirectory(dir)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{dir + "/cosign.pub", dir + "/a.pem", dir + "/b.key"}, paths)

	assert.NoError(t, ValidatePublicKey(dir+"/cosign.pub"))
	assert.Error(t, ValidatePublicKey(dir+"/missing.pub"))
	_, err = LoadPublicKeysFromDirectory(dir + "/missing")
	assert.Error(t, err)
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