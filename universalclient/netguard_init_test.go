package universalclient

import "github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"

// The APIs in these tests are httptest servers on 127.0.0.1, and
// internal-range blocking is on by default: turn it off for the package.
// Tests of the default turn it back on (netguardtest.BlockInternal).
func init() { netguardtest.AllowInternalForPackage() }
