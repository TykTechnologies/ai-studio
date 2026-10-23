package ociplugins

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ResolvePublicKey returns the PEM bytes of the public key that keyID names,
// for verifying plugin signatures. An empty keyID means the first configured
// default key. keyID is resolved in this order:
//
//   - "embedded:<anything>": the built-in Tyk signing key (DefaultTykPublicKey);
//   - a number N: the PEM in OCI_PLUGINS_PUBKEY_N;
//   - a name: the PEM in OCI_PLUGINS_PUBKEY_<NAME>, else the file named by
//     OCI_PLUGINS_PUBKEY_FILE_<NAME>;
//   - "env:VAR": the PEM in VAR;
//   - "file:/path": the file at /path;
//   - a path (contains "/", or ends in .pub or .pem): that file;
//   - otherwise, a configured default key whose reference contains keyID.
//
// The key must be PEM; nothing is written to disk.
func ResolvePublicKey(cfg *OCIConfig, keyID string) ([]byte, error) {
	pemBytes, _, err := resolvePublicKey(cfg, keyID)
	return pemBytes, err
}

// PublicKeyPath returns the file a key ID resolves to, for callers that need
// a path (plugin_security.Service.GetPublicKeyPath). It is "" for a key that
// is not read from a file (embedded or environment keys); use
// ResolvePublicKey for the key itself.
func PublicKeyPath(cfg *OCIConfig, keyID string) (string, error) {
	_, path, err := resolvePublicKey(cfg, keyID)
	return path, err
}

// ParsePublicKey parses the first PEM block of a PKIX public key (ECDSA, RSA
// or Ed25519).
func ParsePublicKey(pemBytes []byte) (crypto.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in public key")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}
	return key, nil
}

func resolvePublicKey(cfg *OCIConfig, keyID string) ([]byte, string, error) {
	// If no specific key ID provided, use first available key
	if keyID == "" {
		if cfg == nil || len(cfg.DefaultPublicKeys) == 0 {
			return nil, "", fmt.Errorf("no public key specified and no default keys configured")
		}
		return resolveKeyReference(cfg, cfg.DefaultPublicKeys[0])
	}
	return resolveKeyReference(cfg, keyID)
}

// resolveKeyReference resolves a key reference to its PEM bytes and, for a
// key read from a file, the file's path.
func resolveKeyReference(cfg *OCIConfig, keyRef string) ([]byte, string, error) {
	// Case 0: Embedded key reference (the built-in Tyk official signing key)
	if strings.HasPrefix(keyRef, "embedded:") {
		return pemKey(DefaultTykPublicKey, "tyk-default")
	}

	// Case 1: Numeric reference (1, 2, 3...)
	if _, err := strconv.Atoi(keyRef); err == nil {
		envKey := fmt.Sprintf("OCI_PLUGINS_PUBKEY_%s", keyRef)
		if keyContent := os.Getenv(envKey); keyContent != "" {
			return pemKey(keyContent, fmt.Sprintf("pubkey-%s", keyRef))
		}
		return nil, "", fmt.Errorf("public key %s not found in environment variable %s", keyRef, envKey)
	}

	// Case 2: Named reference (CI, PROD, DEV, LIVE...)
	if isAlphaNumeric(keyRef) && len(keyRef) > 1 {
		envKey := fmt.Sprintf("OCI_PLUGINS_PUBKEY_%s", strings.ToUpper(keyRef))
		if keyContent := os.Getenv(envKey); keyContent != "" {
			return pemKey(keyContent, fmt.Sprintf("pubkey-%s", strings.ToLower(keyRef)))
		}
		// Also check file-based key variant (OCI_PLUGINS_PUBKEY_FILE_<NAME>)
		fileEnvKey := fmt.Sprintf("OCI_PLUGINS_PUBKEY_FILE_%s", strings.ToUpper(keyRef))
		if filePath := os.Getenv(fileEnvKey); filePath != "" {
			if _, err := os.Stat(filePath); err == nil {
				return pemKeyFile(filePath)
			}
		}
		// Don't treat this as an error - continue to other cases
	}

	// Case 3: Environment variable reference (env:OCI_PLUGINS_PUBKEY_CI)
	if strings.HasPrefix(keyRef, "env:") {
		envKey := strings.TrimPrefix(keyRef, "env:")
		if keyContent := os.Getenv(envKey); keyContent != "" {
			keyName := strings.ToLower(strings.TrimPrefix(envKey, "OCI_PLUGINS_PUBKEY_"))
			return pemKey(keyContent, fmt.Sprintf("pubkey-%s", keyName))
		}
		return nil, "", fmt.Errorf("environment variable %s not found or empty", envKey)
	}

	// Case 4: File path reference (file:/path/to/key.pub)
	if strings.HasPrefix(keyRef, "file:") {
		filePath := strings.TrimPrefix(keyRef, "file:")
		if _, err := os.Stat(filePath); err == nil {
			return pemKeyFile(filePath)
		}
		return nil, "", fmt.Errorf("public key file not found: %s", filePath)
	}

	// Case 5: Direct file path
	if strings.Contains(keyRef, "/") || strings.HasSuffix(keyRef, ".pub") || strings.HasSuffix(keyRef, ".pem") {
		if _, err := os.Stat(keyRef); err == nil {
			return pemKeyFile(keyRef)
		}
		return nil, "", fmt.Errorf("public key file not found: %s", keyRef)
	}

	// Case 6: Search in configured keys only if keyRef looks like it could match
	// This avoids false positives when searching for specific named keys
	if cfg != nil && keyRef != "" && !strings.Contains(keyRef, "/") && !strings.Contains(keyRef, ".") {
		for _, configuredKey := range cfg.DefaultPublicKeys {
			if configuredKey != keyRef { // Avoid infinite recursion
				// Only try configured keys if the keyRef could reasonably match them
				if strings.Contains(configuredKey, keyRef) || strings.Contains(configuredKey, strings.ToUpper(keyRef)) {
					if pemBytes, path, err := resolveKeyReference(cfg, configuredKey); err == nil {
						return pemBytes, path, nil
					}
				}
			}
		}
	}

	return nil, "", fmt.Errorf("public key %s not found", keyRef)
}

// pemKey returns PEM content as key bytes, after checking it is PEM.
func pemKey(pemContent, keyName string) ([]byte, string, error) {
	if !isPEMContent(pemContent) {
		return nil, "", fmt.Errorf("invalid PEM content for key %s", keyName)
	}
	return []byte(pemContent), "", nil
}

// pemKeyFile reads a PEM key file.
func pemKeyFile(path string) ([]byte, string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read public key file %s: %w", path, err)
	}
	if !isPEMContent(string(content)) {
		return nil, "", fmt.Errorf("invalid PEM content in public key file %s", path)
	}
	return content, path, nil
}

// isAlphaNumeric checks if string contains only letters and numbers
func isAlphaNumeric(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return len(s) > 0
}

// LoadPublicKeysFromDirectory returns the paths of the public key files in
// dir (cosign.pub, key.pub, *.pub, *.pem, *.key).
func LoadPublicKeysFromDirectory(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	var keyPaths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Look for common public key file extensions
		if name == "cosign.pub" ||
			name == "key.pub" ||
			hasExtension(name, ".pub", ".pem", ".key") {
			keyPaths = append(keyPaths, fmt.Sprintf("%s/%s", dir, name))
		}
	}

	return keyPaths, nil
}

// hasExtension checks if filename has any of the given extensions
func hasExtension(filename string, extensions ...string) bool {
	for _, ext := range extensions {
		if len(filename) >= len(ext) && filename[len(filename)-len(ext):] == ext {
			return true
		}
	}
	return false
}

// ValidatePublicKey checks if a public key file exists and is accessible
func ValidatePublicKey(keyPath string) error {
	_, err := os.Stat(keyPath)
	if err != nil {
		return fmt.Errorf("public key file not accessible: %w", err)
	}

	// Try to read the file
	_, err = os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("failed to read public key file: %w", err)
	}

	return nil
}
