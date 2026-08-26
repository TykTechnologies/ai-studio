package plugin_security

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// ValidateCommand performs security validation on a plugin command string.
// It is shared by the API layer (create/update requests) and the plugin
// manager (load time), so records that enter the database without passing
// through the API (direct DB writes, migrations, imports, replication) are
// still validated before a process is spawned.
//
// securityService is used for internal-network host validation on URL
// commands (enforced in ENT, and configurable in CE). A nil service skips
// the host check but all other checks still apply.
func ValidateCommand(command string, securityService Service) error {
	if command == "" {
		return fmt.Errorf("🔒 SECURITY: plugin command cannot be empty")
	}

	// Length limit to prevent extremely long commands
	if len(command) > 1024 {
		return fmt.Errorf("🔒 SECURITY: plugin command exceeds maximum length of 1024 characters")
	}

	// Check for path traversal attacks
	if strings.Contains(command, "../") || strings.Contains(command, "..\\") {
		return fmt.Errorf("🔒 SECURITY: plugin command contains path traversal attempt: %s", sanitizeForLogging(command))
	}

	// Check for command injection patterns
	dangerousPatterns := []string{
		";", "|", "&", "&&", "||", "`", "$(",
		"\n", "\r", "\x00", // Control characters
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(command, pattern) {
			return fmt.Errorf("🔒 SECURITY: plugin command contains potentially dangerous character sequence: %s", sanitizeForLogging(command))
		}
	}

	// Validate file:// schemes for proper path format
	if strings.HasPrefix(command, "file://") {
		filePath := strings.TrimPrefix(command, "file://")

		// Check for path traversal in file paths
		if strings.Contains(filePath, "..") {
			return fmt.Errorf("🔒 SECURITY: file:// command contains path traversal: %s", sanitizeForLogging(command))
		}

		// Canonicalize path to check for directory traversal
		cleanPath := filepath.Clean(filePath)
		if !strings.HasPrefix(cleanPath, "/") && !strings.Contains(cleanPath, ":") { // Allow Windows drive letters
			// Relative path that could escape intended directory
			if strings.Contains(cleanPath, "..") {
				return fmt.Errorf("🔒 SECURITY: file:// command resolves to unsafe path: %s", sanitizeForLogging(command))
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
			return fmt.Errorf("🔒 SECURITY: file:// command path not in allowed directories: %s", sanitizeForLogging(command))
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
					return fmt.Errorf("🔒 SECURITY: plugin command contains invalid URL: %s", sanitizeForLogging(command))
				}

				// Check for internal/private network addresses using the plugin security service
				// ENT: Blocks internal IPs unless development bypass is enabled
				if securityService != nil && parsedURL.Hostname() != "" {
					host := parsedURL.Hostname()
					if err := securityService.ValidateGRPCHost(host); err != nil {
						return fmt.Errorf("🔒 SECURITY: plugin command targets internal network address %s: %w", sanitizeForLogging(command), err)
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
					return fmt.Errorf("🔒 SECURITY: plugin command uses disallowed URL scheme: %s", sanitizeForLogging(command))
				}
			}
		}
	}

	return nil
}

// sanitizeForLogging sanitizes strings for safe logging by escaping control
// characters that could break log format, and truncating long values.
func sanitizeForLogging(input string) string {
	sanitized := strings.ReplaceAll(input, "\n", "\\n")
	sanitized = strings.ReplaceAll(sanitized, "\r", "\\r")
	sanitized = strings.ReplaceAll(sanitized, "\t", "\\t")

	if len(sanitized) > 256 {
		sanitized = sanitized[:256] + "..."
	}

	return sanitized
}
