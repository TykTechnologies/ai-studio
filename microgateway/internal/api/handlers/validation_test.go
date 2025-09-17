package handlers

import (
	"testing"
)

func TestMicrogatewayValidatePluginCommand(t *testing.T) {
	testCases := []struct {
		name        string
		command     string
		shouldError bool
		description string
	}{
		// Test a few key scenarios to ensure microgateway validation works
		{
			name:        "Path Traversal",
			command:     "file://../../../etc/passwd",
			shouldError: true,
			description: "Should block path traversal attacks",
		},
		{
			name:        "Command Injection",
			command:     "/usr/bin/plugin; rm -rf /",
			shouldError: true,
			description: "Should block command injection",
		},
		{
			name:        "Internal Network Access",
			command:     "grpc://127.0.0.1:8080/plugin",
			shouldError: true,
			description: "Should block internal network access",
		},
		{
			name:        "Valid Binary",
			command:     "/usr/bin/my-plugin",
			shouldError: false,
			description: "Should allow valid binary paths",
		},
		{
			name:        "Valid OCI",
			command:     "oci://registry.example.com/plugins/auth:v1.0",
			shouldError: false,
			description: "Should allow OCI registry URLs",
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

func TestMicrogatewayIsInternalIP(t *testing.T) {
	testCases := []struct {
		name       string
		host       string
		isInternal bool
	}{
		{"Localhost", "localhost", true},
		{"Loopback IPv4", "127.0.0.1", true},
		{"Private 192.168", "192.168.1.1", true},
		{"Private 10.x", "10.0.0.1", true},
		{"Private 172.16", "172.16.0.1", true},
		{"Public IP", "8.8.8.8", false},
		{"Public Domain", "example.com", false},
		{"External Service", "api.external.com", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isInternalIP(tc.host)
			if result != tc.isInternal {
				t.Errorf("Expected isInternalIP(%q) = %v, got %v", tc.host, tc.isInternal, result)
			}
		})
	}
}