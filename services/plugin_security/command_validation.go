package plugin_security

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
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
	return ValidateCommandWith(command, securityService, netguard.Exemption{})
}

// ValidateCommandWith is ValidateCommand with the plugin's own "Allow
// internal host" exemption (netguard.CommandExemption): its hosts skip the
// internal-host check.
func ValidateCommandWith(command string, securityService Service, exempt netguard.Exemption) error {
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

	// Apply the safe-directory policy to executable paths. Both forms reach
	// exec.Command identically (the plugin manager strips the file:// prefix),
	// so a plain path must not bypass checks a file:// URI would fail.
	if strings.HasPrefix(command, "file://") {
		if err := validatePathInSafeDirs(strings.TrimPrefix(command, "file://"), command); err != nil {
			return err
		}
	} else if !strings.Contains(command, "://") {
		// Plain executable path (no URI scheme)
		if err := validatePathInSafeDirs(command, command); err != nil {
			return err
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
				if securityService != nil && parsedURL.Hostname() != "" && !exempt.Has(parsedURL.Hostname()) {
					host := parsedURL.Hostname()
					if err := securityService.ValidateGRPCHost(host); err != nil {
						return fmt.Errorf("🔒 SECURITY: plugin host %q in %s is an internal address, resolves to one, or does not resolve; add it to LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS to allow this host, or set ALLOW_INTERNAL_NETWORK_ACCESS=true to allow every internal host: %w", sanitizeForLogging(host), sanitizeForLogging(command), err)
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

// CheckCommandHosts applies the plugin host rule (netguard.CheckPluginHost)
// to the host of every URL in command (grpc://, oci://, http(s)://): unless
// ALLOW_INTERNAL_NETWORK_ACCESS=true, a host that is, or resolves to, an
// internal address, or does not resolve, is refused (a host named in
// LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS is exempt). It is the host check of
// ValidateCommand for paths that have no security service at hand (service
// layer creates and upgrades).
func CheckCommandHosts(command string) error {
	return CheckCommandHostsWith(command, netguard.Exemption{})
}

// CheckCommandHostsWith is CheckCommandHosts with the plugin's own "Allow
// internal host" exemption (netguard.CommandExemption).
func CheckCommandHostsWith(command string, exempt netguard.Exemption) error {
	for _, part := range strings.Fields(command) {
		if !strings.Contains(part, "://") || strings.HasPrefix(part, "file://") {
			continue
		}
		parsedURL, err := url.Parse(part)
		if err != nil {
			return fmt.Errorf("🔒 SECURITY: plugin command contains invalid URL: %s", sanitizeForLogging(command))
		}
		if host := parsedURL.Hostname(); host != "" {
			if err := netguard.CheckPluginHostWith(exempt, host); err != nil {
				return fmt.Errorf("🔒 SECURITY: %w", err)
			}
		}
	}
	return nil
}

// allowedDirsEnv lets operators extend the safe-directory allowlist for
// plugin executables. Shared with the plugin manager's executable-path
// scoping so one variable governs both layers.
const allowedDirsEnv = "AI_STUDIO_PLUGIN_ALLOWED_DIRS"

// validatePathInSafeDirs enforces the safe-directory policy on a plugin
// executable path. Relative paths must live under plugins/; absolute paths
// must be inside a built-in safe directory or one listed in
// AI_STUDIO_PLUGIN_ALLOWED_DIRS. Applied identically to file:// URIs and
// plain paths.
func validatePathInSafeDirs(filePath, command string) error {
	// Check for path traversal in file paths
	if strings.Contains(filePath, "..") {
		return fmt.Errorf("🔒 SECURITY: plugin command contains path traversal: %s", sanitizeForLogging(command))
	}

	cleanPath := filepath.Clean(filePath)

	// Restrict to safe directories by default; operators extend the list via
	// AI_STUDIO_PLUGIN_ALLOWED_DIRS
	safeDirs := []string{
		"/usr/bin/", "/bin/", "/usr/local/bin/", "/opt/plugins/",
	}
	for _, dir := range filepath.SplitList(os.Getenv(allowedDirsEnv)) {
		if dir == "" {
			continue
		}
		if !strings.HasSuffix(dir, string(filepath.Separator)) {
			dir += string(filepath.Separator)
		}
		safeDirs = append(safeDirs, dir)
	}

	if !filepath.IsAbs(cleanPath) {
		// Relative paths must live in the plugins directory
		if strings.HasPrefix(cleanPath, "plugins/") || strings.HasPrefix(cleanPath, "./plugins/") {
			return nil
		}
		return fmt.Errorf("🔒 SECURITY: plugin command path not in allowed directories (set %s to extend): %s", allowedDirsEnv, sanitizeForLogging(command))
	}

	for _, safeDir := range safeDirs {
		if strings.HasPrefix(cleanPath, safeDir) {
			return nil
		}
	}
	return fmt.Errorf("🔒 SECURITY: plugin command path not in allowed directories (set %s to extend): %s", allowedDirsEnv, sanitizeForLogging(command))
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
