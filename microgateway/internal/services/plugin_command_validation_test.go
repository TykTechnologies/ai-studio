package services

import "testing"

// The microgateway is the side that dials grpc:// plugin commands, so its
// copy of validatePluginCommand gets the same pin as Studio's: internal
// targets are blocked only with PLUGIN_BLOCK_INTERNAL_URLS=true. Address
// detection is covered in pkg/netguard.
func TestValidatePluginCommand_InternalTargets(t *testing.T) {
	s := &PluginService{}
	for _, tc := range []struct {
		command   string
		block     bool
		wantError bool
	}{
		{"grpc://2130706433:50051", true, true}, // decimal 127.0.0.1, missed by substring matching
		{"grpc://[::ffff:10.0.0.1]:50051", true, true},
		{"/usr/local/bin/plugin --upstream=grpc://169.254.169.254:80", true, true},
		{"grpc://127.0.0.1:50051", false, false}, // warning only
		{"grpc://plugins.example.com:50051", true, false},
		{"/usr/local/bin/plugin-v10.2 --listen=:9000", true, false}, // "10." and ":" are not an address
	} {
		t.Run(tc.command, func(t *testing.T) {
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
