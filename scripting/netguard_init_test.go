package scripting

import "github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"

// The guardrail providers and tyk.makeHTTPRequest targets in these tests are
// httptest servers on 127.0.0.1, and internal-range blocking is on by
// default: turn it off for the package.
func init() { netguardtest.AllowInternalForPackage() }
