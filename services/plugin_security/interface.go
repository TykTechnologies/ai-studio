package plugin_security

import (
	"context"
)

// Service defines the interface for plugin security operations
// CE provides stub implementations that allow all operations
// ENT provides full security enforcement
type Service interface {
	// ValidateGRPCHost validates that a GRPC host is not targeting internal networks
	// CE: Always returns nil (allows all hosts)
	// ENT: Blocks internal IP addresses unless development override is enabled
	ValidateGRPCHost(host string) error

	// IsInternalIP checks if a host resolves to an internal/private IP address
	// CE: Always returns false
	// ENT: Checks against private IP ranges (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, etc.)
	IsInternalIP(host string) bool

	// VerifySignature verifies the signature of an OCI artifact using a public key
	// CE: Always returns nil (no verification)
	// ENT: Performs full Cosign signature verification
	VerifySignature(ctx context.Context, ref *OCIReference, pubKeyID string) error

	// VerifyBundle verifies a signature bundle for keyless signing
	// CE: Always returns nil (no verification)
	// ENT: Verifies using certificate identity and OIDC issuer
	VerifyBundle(ctx context.Context, ref *OCIReference, issuer, subject string) error

	// VerifyWithPolicy verifies a signature using a policy file
	// CE: Always returns nil (no verification)
	// ENT: Verifies using Cosign policy file
	VerifyWithPolicy(ctx context.Context, ref *OCIReference, policyPath string) error

	// GetPublicKeyPath retrieves the path to a public key for verification
	// CE: Always returns empty string
	// ENT: Resolves key references to file paths
	GetPublicKeyPath(pubKeyID string) (string, error)

	// ValidatePublicKey checks if a public key file exists and is accessible
	// CE: Always returns nil
	// ENT: Validates the key file
	ValidatePublicKey(keyPath string) error

	// LoadPublicKeysFromDirectory loads all public keys from a directory
	// CE: Always returns empty slice
	// ENT: Scans directory for public key files
	LoadPublicKeysFromDirectory(dir string) ([]string, error)
}
