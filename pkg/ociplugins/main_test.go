package ociplugins

import (
	"os"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"
)

// Tests name made-up registries (registry.com, registry.example.com): they
// resolve to a public address, so the plugin host rule passes them without
// DNS. Tests of the rule stub internal names with netguardtest.StubLookupIP.
func TestMain(m *testing.M) {
	restore := netguardtest.ResolveAllPublic()
	code := m.Run()
	restore()
	os.Exit(code)
}
