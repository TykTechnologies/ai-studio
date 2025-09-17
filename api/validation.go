package api

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
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

// validatePluginCommand performs API-level security validation on plugin commands
// This replicates key validations from the service layer to catch attacks at source
func validatePluginCommand(command string) error {
	if command == "" {
		return fmt.Errorf("🔒 SECURITY: plugin command cannot be empty")
	}

	// Length limit to prevent extremely long commands
	if len(command) > 1024 {
		return fmt.Errorf("🔒 SECURITY: plugin command exceeds maximum length of 1024 characters")
	}

	// Check for path traversal attacks
	if strings.Contains(command, "../") || strings.Contains(command, "..\\") {
		return fmt.Errorf("🔒 SECURITY: plugin command contains path traversal attempt: %s", sanitizeStringForLogging(command))
	}

	// Check for command injection patterns
	dangerousPatterns := []string{
		";", "|", "&", "&&", "||", "`", "$(",
		"\n", "\r", "\x00", // Control characters
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(command, pattern) {
			return fmt.Errorf("🔒 SECURITY: plugin command contains potentially dangerous character sequence: %s", sanitizeStringForLogging(command))
		}
	}

	// Validate file:// schemes for proper path format
	if strings.HasPrefix(command, "file://") {
		filePath := strings.TrimPrefix(command, "file://")

		// Check for path traversal in file paths
		if strings.Contains(filePath, "..") {
			return fmt.Errorf("🔒 SECURITY: file:// command contains path traversal: %s", sanitizeStringForLogging(command))
		}

		// Canonicalize path to check for directory traversal
		cleanPath := filepath.Clean(filePath)
		if !strings.HasPrefix(cleanPath, "/") && !strings.Contains(cleanPath, ":") { // Allow Windows drive letters
			// Relative path that could escape intended directory
			if strings.Contains(cleanPath, "..") {
				return fmt.Errorf("🔒 SECURITY: file:// command resolves to unsafe path: %s", sanitizeStringForLogging(command))
			}
		}

		// Restrict to safe directories by default (can be overridden by service layer config)
		safeDirs := []string{
			"/usr/bin/", "/bin/", "/usr/local/bin/",
			"./plugins/", "plugins/", "/opt/plugins/",
		}

		isSafePath := false
		absPath := cleanPath
		if !filepath.IsAbs(cleanPath) {
			// For relative paths, they should be in allowed directories
			isSafePath = strings.HasPrefix(cleanPath, "plugins/") || strings.HasPrefix(cleanPath, "./plugins/")
		} else {
			// For absolute paths, check against safe directories
			for _, safeDir := range safeDirs {
				if strings.HasPrefix(absPath, safeDir) {
					isSafePath = true
					break
				}
			}
		}

		if !isSafePath {
			return fmt.Errorf("🔒 SECURITY: file:// command path not in allowed directories: %s", sanitizeStringForLogging(command))
		}
	}

	// Validate URLs (grpc://, http://, https://, oci://)
	if strings.Contains(command, "://") && !strings.HasPrefix(command, "file://") {
		// Extract URL from command
		parts := strings.Fields(command)
		for _, part := range parts {
			if strings.Contains(part, "://") {
				parsedURL, err := url.Parse(part)
				if err != nil {
					return fmt.Errorf("🔒 SECURITY: plugin command contains invalid URL: %s", sanitizeStringForLogging(command))
				}

				// Check for internal/private network addresses (bypass with ALLOW_INTERNAL_NETWORK_ACCESS=true)
				if parsedURL.Hostname() != "" {
					host := parsedURL.Hostname()
					if isInternalIP(host) && os.Getenv("ALLOW_INTERNAL_NETWORK_ACCESS") != "true" {
						return fmt.Errorf("🔒 SECURITY: plugin command targets internal network address: %s", sanitizeStringForLogging(command))
					}
				}

				// Validate allowed schemes
				allowedSchemes := []string{"grpc", "oci", "http", "https"}
				schemeAllowed := false
				for _, scheme := range allowedSchemes {
					if parsedURL.Scheme == scheme {
						schemeAllowed = true
						break
					}
				}
				if !schemeAllowed {
					return fmt.Errorf("🔒 SECURITY: plugin command uses disallowed URL scheme: %s", sanitizeStringForLogging(command))
				}
			}
		}
	}

	return nil
}

// isInternalIP checks if a hostname/IP is internal/private using proper CIDR validation
func isInternalIP(host string) bool {
	// Handle localhost variations
	lowerHost := strings.ToLower(host)
	if lowerHost == "localhost" || lowerHost == "::1" {
		return true
	}

	// Parse the IP address
	ip := net.ParseIP(host)
	if ip == nil {
		// If not an IP, could be a hostname - check for localhost patterns
		return strings.Contains(lowerHost, "localhost")
	}

	// Define private IP CIDR ranges
	privateCIDRs := []string{
		"10.0.0.0/8",        // Private Class A
		"172.16.0.0/12",     // Private Class B
		"192.168.0.0/16",    // Private Class C
		"127.0.0.0/8",       // IPv4 loopback
		"169.254.0.0/16",    // IPv4 link-local
		"::1/128",           // IPv6 loopback
		"fc00::/7",          // IPv6 unique local addresses
		"fe80::/10",         // IPv6 link-local
	}

	// Check if IP falls within any private CIDR range
	for _, cidrStr := range privateCIDRs {
		_, cidr, err := net.ParseCIDR(cidrStr)
		if err != nil {
			continue
		}
		if cidr.Contains(ip) {
			return true
		}
	}

	return false
}