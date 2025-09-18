// pkg/ociplugins/errors.go
package ociplugins

import (
	"fmt"
)

// ErrInvalidOCIReference indicates an invalid OCI reference format
type ErrInvalidOCIReference struct {
	Reference string
	Reason    string
}

func (e *ErrInvalidOCIReference) Error() string {
	return fmt.Sprintf("invalid OCI reference %q: %s", e.Reference, e.Reason)
}

// ErrRegistryNotAllowed indicates the registry is not in the allowlist
type ErrRegistryNotAllowed struct {
	Registry        string
	AllowedRegistries []string
}

func (e *ErrRegistryNotAllowed) Error() string {
	return fmt.Sprintf("registry %q is not allowed (allowed: %v)", e.Registry, e.AllowedRegistries)
}

// ErrSignatureVerificationFailed indicates cosign signature verification failed
type ErrSignatureVerificationFailed struct {
	Reference string
	Reason    string
}

func (e *ErrSignatureVerificationFailed) Error() string {
	return fmt.Sprintf("signature verification failed for %q: %s", e.Reference, e.Reason)
}

// ErrPluginNotFound indicates the plugin was not found in the registry
type ErrPluginNotFound struct {
	Reference string
}

func (e *ErrPluginNotFound) Error() string {
	return fmt.Sprintf("plugin not found: %q", e.Reference)
}

// ErrIncompatibleArchitecture indicates the plugin is not compatible with the current architecture
type ErrIncompatibleArchitecture struct {
	PluginArch   string
	RuntimeArch  string
}

func (e *ErrIncompatibleArchitecture) Error() string {
	return fmt.Sprintf("plugin architecture %q is incompatible with runtime architecture %q",
		e.PluginArch, e.RuntimeArch)
}

// ErrCacheCorrupted indicates the plugin cache is corrupted
type ErrCacheCorrupted struct {
	Path   string
	Reason string
}

func (e *ErrCacheCorrupted) Error() string {
	return fmt.Sprintf("cache corrupted at %q: %s", e.Path, e.Reason)
}

// ErrNetworkTimeout indicates a network operation timed out
type ErrNetworkTimeout struct {
	Operation string
	Registry  string
}

func (e *ErrNetworkTimeout) Error() string {
	return fmt.Sprintf("network timeout during %s with registry %q", e.Operation, e.Registry)
}