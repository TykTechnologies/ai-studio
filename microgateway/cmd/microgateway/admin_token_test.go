package main

import (
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
)

// An edge serves no management API, so -create-admin-token is refused there
// instead of minting a token nothing accepts (lsj.16); standalone allows it.
func TestAdminTokenAllowed(t *testing.T) {
	edge := &config.Config{}
	edge.HubSpoke.Mode = "edge"
	err := adminTokenAllowed(edge)
	if err == nil || !strings.Contains(err.Error(), "edge mode") {
		t.Fatalf("edge: err = %v, want the edge-mode refusal", err)
	}

	for _, mode := range []string{"", "standalone"} {
		cfg := &config.Config{}
		cfg.HubSpoke.Mode = mode
		if err := adminTokenAllowed(cfg); err != nil {
			t.Errorf("mode %q: err = %v, want nil", mode, err)
		}
	}
}
