package plugin_security

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// blockingService is a Service stub whose host validation always rejects,
// simulating the enterprise internal-network block.
type blockingService struct {
	communityService
}

func (s *blockingService) ValidateGRPCHost(host string) error {
	return ErrInternalNetworkBlocked
}

func (s *blockingService) IsInternalIP(host string) bool { return true }

// allowingService is a Service stub whose host validation always passes.
type allowingService struct {
	communityService
}

func (s *allowingService) ValidateGRPCHost(host string) error { return nil }

func (s *allowingService) IsInternalIP(host string) bool { return false }

func (s *allowingService) VerifySignature(ctx context.Context, ref *OCIReference, pubKeyID string) error {
	return nil
}

func TestValidateCommand_RejectsDangerousCommands(t *testing.T) {
	svc := &allowingService{}

	cases := []struct {
		name    string
		command string
	}{
		{"empty command", ""},
		{"too long", strings.Repeat("a", 1025)},
		{"path traversal", "./plugins/../../etc/evil"},
		{"windows path traversal", "..\\..\\evil.exe"},
		{"semicolon injection", "./plugins/my-plugin; rm -rf /"},
		{"pipe injection", "./plugins/my-plugin | cat /etc/passwd"},
		{"backtick injection", "./plugins/`whoami`"},
		{"subshell injection", "./plugins/$(whoami)"},
		{"ampersand injection", "./plugins/my-plugin && curl evil.com"},
		{"newline injection", "./plugins/my-plugin\nrm -rf /"},
		{"null byte", "./plugins/my-plugin\x00evil"},
		{"file outside safe dirs", "file:///etc/passwd"},
		{"file relative outside plugins", "file://secret/binary"},
		{"disallowed scheme", "ftp://example.com/plugin"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateCommand(tc.command, svc); err == nil {
				t.Errorf("expected %q to be rejected, but it was allowed", tc.command)
			}
		})
	}
}

func TestValidateCommand_AllowsSafeCommands(t *testing.T) {
	svc := &allowingService{}

	cases := []struct {
		name    string
		command string
	}{
		{"relative plugins path", "./plugins/my-plugin"},
		{"plugins path no dot", "plugins/my-plugin"},
		{"file in opt plugins", "file:///opt/plugins/my-plugin"},
		{"file in usr local bin", "file:///usr/local/bin/my-plugin"},
		{"oci reference", "oci://registry.example.com/plugins/my-plugin:1.0.0"},
		{"grpc external host", "grpc://plugins.example.com:50051"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateCommand(tc.command, svc); err != nil {
				t.Errorf("expected %q to be allowed, got error: %v", tc.command, err)
			}
		})
	}
}

func TestValidateCommand_BlocksInternalHosts(t *testing.T) {
	svc := &blockingService{}

	err := ValidateCommand("grpc://10.0.0.5:50051", svc)
	if err == nil {
		t.Fatal("expected internal host to be rejected when the security service blocks it")
	}
	if !errors.Is(err, ErrInternalNetworkBlocked) && !strings.Contains(err.Error(), "internal network") {
		t.Errorf("error should indicate internal network blocking, got: %v", err)
	}
}

func TestValidateCommand_NilServiceSkipsHostCheck(t *testing.T) {
	// A nil service must not panic; scheme and injection checks still apply.
	if err := ValidateCommand("oci://registry.example.com/repo:tag", nil); err != nil {
		t.Errorf("expected valid command to pass with nil service, got: %v", err)
	}
	if err := ValidateCommand("./plugins/x; rm -rf /", nil); err == nil {
		t.Error("expected injection to be rejected even with nil service")
	}
}
