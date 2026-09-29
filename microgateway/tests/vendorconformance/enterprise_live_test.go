//go:build vendorlive && enterprise

package vendorconformance

// An enterprise build of the data plane panics if the community plugin
// security service is constructed, so the enterprise factory has to be
// registered before the harness boots. The import's init() does that, exactly
// as microgateway/tests/integration/testmain_enterprise_test.go does for the
// mock-backed integration suite.
import _ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/plugin_security"
