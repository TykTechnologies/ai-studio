package data_session

import "github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"

// The embedders and stores in these tests are httptest servers on
// 127.0.0.1, and internal-range blocking is on by default: turn it off for
// the package.
func init() { netguardtest.AllowInternalForPackage() }
