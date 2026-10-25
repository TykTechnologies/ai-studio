package services

import (
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
)

// The config-only path dispatches on plugin_security.SourceOf like the load
// path: a grpc:// command is refused explicitly, never run as a local
// executable.
func TestCreateConfigOnlyPluginClient_RefusesGRPC(t *testing.T) {
	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "true") // pass load-time validation of 127.0.0.1
	m := NewAIStudioPluginManager(nil, nil)

	client, err := m.createConfigOnlyPluginClient("grpc://127.0.0.1:1234", netguard.Exemption{})
	if client != nil {
		client.Kill()
	}
	if err == nil || !strings.Contains(err.Error(), "grpc plugins have no config-only mode") {
		t.Fatalf("err = %v, want the explicit grpc refusal", err)
	}
}
