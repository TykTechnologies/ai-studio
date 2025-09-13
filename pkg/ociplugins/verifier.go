// pkg/ociplugins/verifier.go
package ociplugins

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
func (v *SignatureVerifier) getPublicKeyPath(pubKeyID string) (string, error) {
	// If no specific key ID provided, use default keys
	if pubKeyID == "" {
		if len(v.config.DefaultPublicKeys) == 0 {
			return "", fmt.Errorf("no public key specified and no default keys configured")
		}
		pubKeyID = v.config.DefaultPublicKeys[0]
	}

	// Check if it's a file path that exists
	if _, err := os.Stat(pubKeyID); err == nil {
		return pubKeyID, nil
	}

	// Check if it's one of the configured keys
	for _, keyPath := range v.config.DefaultPublicKeys {
		if keyPath == pubKeyID {
			if _, err := os.Stat(keyPath); err == nil {
				return keyPath, nil
			}
			return "", fmt.Errorf("configured public key file not found: %s", keyPath)
		}
	}

	return "", fmt.Errorf("public key %s not found", pubKeyID)
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