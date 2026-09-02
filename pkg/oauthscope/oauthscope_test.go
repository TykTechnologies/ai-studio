package oauthscope

import "testing"

func TestGrants(t *testing.T) {
	tests := []struct {
		name       string
		tokenScope string
		required   string
		want       bool
	}{
		{"exact match", "mcp", MCP, true},
		{"one of several", "openid profile mcp", MCP, true},
		{"leading and trailing space", "  mcp  ", MCP, true},
		{"tab separated", "openid\tmcp", MCP, true},
		{"identity scope only", "openid", MCP, false},
		{"empty scope", "", MCP, false},
		{"prefix is not a match", "mcpx", MCP, false},
		{"substring is not a match", "supermcp", MCP, false},
		{"case sensitive", "MCP", MCP, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Grants(tt.tokenScope, tt.required); got != tt.want {
				t.Errorf("Grants(%q, %q) = %v, want %v", tt.tokenScope, tt.required, got, tt.want)
			}
		})
	}
}

func TestUnsupported(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		want      []string
	}{
		{"all supported", "openid profile email mcp", nil},
		{"empty", "", nil},
		{"one bad", "mcp admin:everything", []string{"admin:everything"}},
		{"several bad", "read write", []string{"read", "write"}},
		{"the dropped resource scopes are no longer issued", "mcp_read mcp_write", []string{"mcp_read", "mcp_write"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Unsupported(tt.requested)
			if len(got) != len(tt.want) {
				t.Fatalf("Unsupported(%q) = %v, want %v", tt.requested, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Unsupported(%q)[%d] = %q, want %q", tt.requested, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// The resource document must not advertise a scope the gateway does not enforce:
// that mismatch is what made scopes decorative in the first place.
func TestResourceScopesAreEnforceable(t *testing.T) {
	for _, s := range Resource {
		if !IsSupported(s) {
			t.Errorf("resource scope %q is advertised but is not a supported scope", s)
		}
	}
}
