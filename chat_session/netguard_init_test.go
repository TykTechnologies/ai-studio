package chat_session

import "github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"

// The tool APIs in these tests are httptest servers on 127.0.0.1, and tool
// calls refuse internal addresses by default (w4e): turn that off for the
// package. Tests of the default turn it back on (netguardtest.BlockInternal).
func init() { netguardtest.AllowInternalForPackage() }
