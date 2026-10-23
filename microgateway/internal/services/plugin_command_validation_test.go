package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"
)

// The microgateway is the side that dials grpc:// plugin commands, so its
// copy of validatePluginCommand gets the same pin as Studio's: plugin and
// registry hosts that are, or resolve to, internal addresses are refused
// unless ALLOW_INTERNAL_NETWORK_ACCESS=true (midsommar-clean-c1g.32); with
// internal hosts allowed, PLUGIN_BLOCK_INTERNAL_URLS still decides between
// refusing and warning about literal internal addresses. Address detection
// is covered in pkg/netguard.
func TestValidatePluginCommand_InternalTargets(t *testing.T) {
	netguardtest.StubLookupIP(t, map[string]string{
		"registry.corp.internal": "10.0.0.5",
		"unresolvable.example":   "",
	})
	s := &PluginService{}
	for _, tc := range []struct {
		command       string
		allowInternal bool
		block         bool
		wantError     bool
	}{
		{"grpc://2130706433:50051", false, false, true}, // decimal 127.0.0.1
		{"grpc://[::ffff:10.0.0.1]:50051", false, false, true},
		{"/usr/local/bin/plugin --upstream=grpc://169.254.169.254:80", true, true, true},
		{"grpc://127.0.0.1:50051", false, false, true},
		{"grpc://127.0.0.1:50051", true, false, false}, // warning only
		{"oci://registry.corp.internal/plugins/demo:1.0.0", false, false, true},
		{"oci://registry.corp.internal/plugins/demo:1.0.0", true, false, false},
		{"oci://unresolvable.example/plugins/demo:1.0.0", false, false, true},
		{"grpc://plugins.example.com:50051", false, true, false},
		{"/usr/local/bin/plugin-v10.2 --listen=:9000", false, true, false}, // "10." and ":" are not an address
	} {
		t.Run(tc.command, func(t *testing.T) {
			if tc.allowInternal {
				t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "true")
			} else {
				t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "")
			}
			if tc.block {
				t.Setenv("PLUGIN_BLOCK_INTERNAL_URLS", "true")
			} else {
				t.Setenv("PLUGIN_BLOCK_INTERNAL_URLS", "")
			}
			err := s.validatePluginCommand(tc.command)
			if (err != nil) != tc.wantError {
				t.Errorf("validatePluginCommand(%q) error = %v, want error %v", tc.command, err, tc.wantError)
			}
		})
	}
}
