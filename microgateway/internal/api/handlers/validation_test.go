//go:build enterprise
// +build enterprise

package handlers

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/services/plugin_security"
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

	// Use enterprise plugin security service for validation
	securityService := plugin_security.NewService(&plugin_security.Config{
		AllowInternalNetworkAccess: false, // Enterprise mode - block internal IPs
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePluginCommand(tc.command, securityService)

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
		{"Loopback IPv4 range", "127.255.255.255", true},
		{"Private 192.168", "192.168.1.1", true},
		{"Private 192.168 edge", "192.168.255.255", true},
		{"Private 10.x", "10.0.0.1", true},
		{"Private 10.x edge", "10.255.255.255", true},
		{"Private 172.16", "172.16.0.1", true},
		{"Private 172.31 (edge)", "172.31.255.255", true},
		{"Link-local", "169.254.1.1", true},
		{"IPv6 loopback", "::1", true},
		{"IPv6 private fc00", "fc00::1", true},
		{"IPv6 private fd00", "fd00::1", true},
		{"IPv6 link-local", "fe80::1", true},
		// Edge cases that should NOT be internal
		{"Not private 172.32", "172.32.0.1", false},
		{"Not private 172.15", "172.15.255.255", false},
		{"Public IP", "8.8.8.8", false},
		{"Public Domain", "example.com", false},
		{"External Service", "api.external.com", false},
		{"Edge 192.169", "192.169.0.1", false},
		{"Edge 11.0.0.1", "11.0.0.1", false},
	}

	// Use enterprise plugin security service for internal IP validation
	securityService := plugin_security.NewService(&plugin_security.Config{
		AllowInternalNetworkAccess: false, // Enterprise mode - block internal IPs
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := securityService.ValidateGRPCHost(tc.host)
			result := (err != nil) // If validation fails, it's internal

			if result != tc.isInternal {
				t.Errorf("Expected ValidateGRPCHost(%q) blocked = %v, got %v", tc.host, tc.isInternal, result)
			}
		})
	}
}