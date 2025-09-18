// pkg/ociplugins/verifier.go
package ociplugins

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// SignatureVerifier handles cosign signature verification
type SignatureVerifier struct {
	config *OCIConfig
}

// NewSignatureVerifier creates a new signature verifier
func NewSignatureVerifier(config *OCIConfig) (*SignatureVerifier, error) {
	return &SignatureVerifier{
		config: config,
	}, nil
}

// Verify checks the cosign signature of an OCI artifact using cosign CLI
func (v *SignatureVerifier) Verify(ctx context.Context, ref *OCIReference, pubKeyID string) error {
	// Get public key path for verification
	pubKeyPath, err := v.getPublicKeyPath(pubKeyID)
	if err != nil {
		return fmt.Errorf("failed to get public key path: %w", err)
	}

	// Build cosign verify command
	// cosign verify --key /path/to/key.pub registry/repo@digest
	cmd := exec.CommandContext(ctx, "cosign", "verify", "--key", pubKeyPath, ref.FullReference())

	// Run verification
	output, err := cmd.CombinedOutput()
	if err != nil {
		return &ErrSignatureVerificationFailed{
			Reference: ref.FullReference(),
			Reason:    fmt.Sprintf("cosign verify failed: %s", string(output)),
		}
	}

	return nil
}

// getPublicKeyPath retrieves the path to a public key for verification
// Supports multiple key resolution methods:
// - Numeric references (1, 2, 3) → OCI_PLUGINS_PUBKEY_<NUMBER>
// - Named references (CI, PROD) → OCI_PLUGINS_PUBKEY_<NAME>
// - Direct file paths
// - Environment variable names
func (v *SignatureVerifier) getPublicKeyPath(pubKeyID string) (string, error) {
	// If no specific key ID provided, use first available key
	if pubKeyID == "" {
		if len(v.config.DefaultPublicKeys) == 0 {
			return "", fmt.Errorf("no public key specified and no default keys configured")
		}
		return v.resolveKeyReference(v.config.DefaultPublicKeys[0])
	}

	// Try to resolve the key reference
	return v.resolveKeyReference(pubKeyID)
}

// resolveKeyReference resolves a key reference to a usable file path
func (v *SignatureVerifier) resolveKeyReference(keyRef string) (string, error) {
	// Case 1: Numeric reference (1, 2, 3...)
	if _, err := strconv.Atoi(keyRef); err == nil {
		envKey := fmt.Sprintf("OCI_PLUGINS_PUBKEY_%s", keyRef)
		if keyContent := os.Getenv(envKey); keyContent != "" {
			return v.writeKeyToTempFile(keyContent, fmt.Sprintf("pubkey-%s", keyRef))
		}
		return "", fmt.Errorf("public key %s not found in environment variable %s", keyRef, envKey)
	}

	// Case 2: Named reference (CI, PROD, DEV...)
	if isAlphaNumeric(keyRef) && len(keyRef) > 1 {
		envKey := fmt.Sprintf("OCI_PLUGINS_PUBKEY_%s", strings.ToUpper(keyRef))
		if keyContent := os.Getenv(envKey); keyContent != "" {
			return v.writeKeyToTempFile(keyContent, fmt.Sprintf("pubkey-%s", strings.ToLower(keyRef)))
		}
		// Don't treat this as an error - continue to other cases
	}

	// Case 3: Environment variable reference (env:OCI_PLUGINS_PUBKEY_CI)
	if strings.HasPrefix(keyRef, "env:") {
		envKey := strings.TrimPrefix(keyRef, "env:")
		if keyContent := os.Getenv(envKey); keyContent != "" {
			keyName := strings.ToLower(strings.TrimPrefix(envKey, "OCI_PLUGINS_PUBKEY_"))
			return v.writeKeyToTempFile(keyContent, fmt.Sprintf("pubkey-%s", keyName))
		}
		return "", fmt.Errorf("environment variable %s not found or empty", envKey)
	}

	// Case 4: File path reference (file:/path/to/key.pub)
	if strings.HasPrefix(keyRef, "file:") {
		filePath := strings.TrimPrefix(keyRef, "file:")
		if _, err := os.Stat(filePath); err == nil {
			return filePath, nil
		}
		return "", fmt.Errorf("public key file not found: %s", filePath)
	}

	// Case 5: Direct file path
	if strings.Contains(keyRef, "/") || strings.HasSuffix(keyRef, ".pub") || strings.HasSuffix(keyRef, ".pem") {
		if _, err := os.Stat(keyRef); err == nil {
			return keyRef, nil
		}
		return "", fmt.Errorf("public key file not found: %s", keyRef)
	}

	// Case 6: Search in configured keys only if keyRef looks like it could match
	// This avoids false positives when searching for specific named keys
	if keyRef != "" && !strings.Contains(keyRef, "/") && !strings.Contains(keyRef, ".") {
		for _, configuredKey := range v.config.DefaultPublicKeys {
			if configuredKey != keyRef { // Avoid infinite recursion
				// Only try configured keys if the keyRef could reasonably match them
				if strings.Contains(configuredKey, keyRef) || strings.Contains(configuredKey, strings.ToUpper(keyRef)) {
					if resolvedPath, err := v.resolveKeyReference(configuredKey); err == nil {
						return resolvedPath, nil
					}
				}
			}
		}
	}

	return "", fmt.Errorf("public key %s not found", keyRef)
}

// writeKeyToTempFile writes PEM content to a temporary file and returns the path
func (v *SignatureVerifier) writeKeyToTempFile(pemContent, keyName string) (string, error) {
	// Validate PEM content
	if !isPEMContent(pemContent) {
		return "", fmt.Errorf("invalid PEM content for key %s", keyName)
	}

	// Create temporary file
	tempFile, err := ioutil.TempFile("", fmt.Sprintf("oci-pubkey-%s-*.pub", keyName))
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for key %s: %w", keyName, err)
	}

	// Write PEM content
	if _, err := tempFile.WriteString(pemContent); err != nil {
		tempFile.Close()
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to write key content: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}

	return tempFile.Name(), nil
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

// VerifyBundle verifies a signature bundle for keyless signing
func (v *SignatureVerifier) VerifyBundle(ctx context.Context, ref *OCIReference, issuer, subject string) error {
	// Build cosign verify command for keyless verification
	// cosign verify --certificate-identity=<subject> --certificate-oidc-issuer=<issuer> <ref>
	args := []string{
		"verify",
		"--certificate-identity=" + subject,
		"--certificate-oidc-issuer=" + issuer,
		ref.FullReference(),
	}

	cmd := exec.CommandContext(ctx, "cosign", args...)

	// Run verification
	output, err := cmd.CombinedOutput()
	if err != nil {
		return &ErrSignatureVerificationFailed{
			Reference: ref.FullReference(),
			Reason:    fmt.Sprintf("cosign bundle verify failed: %s", string(output)),
		}
	}

	return nil
}

// VerifyWithPolicy verifies a signature using a policy file
func (v *SignatureVerifier) VerifyWithPolicy(ctx context.Context, ref *OCIReference, policyPath string) error {
	// Build cosign verify command with policy
	// cosign verify --policy=<policy> <ref>
	cmd := exec.CommandContext(ctx, "cosign", "verify", "--policy", policyPath, ref.FullReference())

	// Run verification
	output, err := cmd.CombinedOutput()
	if err != nil {
		return &ErrSignatureVerificationFailed{
			Reference: ref.FullReference(),
			Reason:    fmt.Sprintf("cosign policy verify failed: %s", string(output)),
		}
	}

	return nil
}

// LoadPublicKeysFromDirectory loads all public keys from a directory
func (v *SignatureVerifier) LoadPublicKeysFromDirectory(dir string) ([]string, error) {
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
func (v *SignatureVerifier) ValidatePublicKey(keyPath string) error {
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