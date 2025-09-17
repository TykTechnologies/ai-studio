package api

import (
	"fmt"
	"regexp"
	"strings"
)

// Security validation patterns and limits for API input parameters
var (
	// Edge ID validation: alphanumeric, hyphens, underscores, dots (max 64 chars)
	edgeIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9\-_.]*[a-zA-Z0-9]$`)

	// Namespace validation: alphanumeric, hyphens, underscores (max 64 chars)
	namespacePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9\-_]*[a-zA-Z0-9]$`)

	// Operation ID validation: alphanumeric, hyphens (max 64 chars)
	operationIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9]$`)

	// Maximum lengths for security
	maxEdgeIDLength = 64
	maxNamespaceLength = 64
	maxOperationIDLength = 64
)

// validateEdgeID validates edge_id path parameters
func validateEdgeID(edgeID string) error {
	if edgeID == "" {
		return fmt.Errorf("🔒 SECURITY: edge_id cannot be empty")
	}

	if len(edgeID) > maxEdgeIDLength {
		return fmt.Errorf("🔒 SECURITY: edge_id exceeds maximum length of %d characters", maxEdgeIDLength)
	}

	if !edgeIDPattern.MatchString(edgeID) {
		return fmt.Errorf("🔒 SECURITY: edge_id contains invalid characters. Must be alphanumeric with hyphens, underscores, or dots")
	}

	return nil
}

// validateNamespace validates namespace path parameters
func validateNamespace(namespace string) error {
	if namespace == "" {
		return fmt.Errorf("🔒 SECURITY: namespace cannot be empty")
	}

	// Special case: "global" is a valid namespace alias
	if namespace == "global" {
		return nil
	}

	if len(namespace) > maxNamespaceLength {
		return fmt.Errorf("🔒 SECURITY: namespace exceeds maximum length of %d characters", maxNamespaceLength)
	}

	if !namespacePattern.MatchString(namespace) {
		return fmt.Errorf("🔒 SECURITY: namespace contains invalid characters. Must be alphanumeric with hyphens or underscores")
	}

	return nil
}

// validateOperationID validates operation_id path parameters
func validateOperationID(operationID string) error {
	if operationID == "" {
		return fmt.Errorf("🔒 SECURITY: operation_id cannot be empty")
	}

	if len(operationID) > maxOperationIDLength {
		return fmt.Errorf("🔒 SECURITY: operation_id exceeds maximum length of %d characters", maxOperationIDLength)
	}

	if !operationIDPattern.MatchString(operationID) {
		return fmt.Errorf("🔒 SECURITY: operation_id contains invalid characters. Must be alphanumeric with hyphens")
	}

	return nil
}

// sanitizeStringForLogging sanitizes strings for safe logging by removing potential log injection characters
func sanitizeStringForLogging(input string) string {
	// Remove newlines, carriage returns, and other control characters that could break log format
	sanitized := strings.ReplaceAll(input, "\n", "\\n")
	sanitized = strings.ReplaceAll(sanitized, "\r", "\\r")
	sanitized = strings.ReplaceAll(sanitized, "\t", "\\t")

	// Limit length to prevent log bloat
	if len(sanitized) > 256 {
		sanitized = sanitized[:256] + "..."
	}

	return sanitized
}