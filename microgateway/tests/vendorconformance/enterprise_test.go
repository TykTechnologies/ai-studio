//go:build enterprise

package vendorconformance

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"github.com/TykTechnologies/midsommar/v2/scripting/engine"
	"github.com/TykTechnologies/midsommar/v2/services/plugin_security"

	// An enterprise build of the data plane panics if the community plugin
	// security service is constructed, so the enterprise factory has to be
	// registered before the harness boots. The import's init() does that, exactly
	// as microgateway/tests/integration/testmain_enterprise_test.go does for the
	// mock-backed integration suite.
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/plugin_security"

	// The rest mirrors microgateway/cmd/microgateway/main_enterprise.go: the
	// filter script engine and guardrail providers, and the Semantic Router
	// engine. Without them the in-process gateway runs the community runner.
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/filters"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/semantic_router/engine"
)

// requireEnterpriseRuntime fails the test unless this build registers what the
// enterprise microgateway's main registers (microgateway/cmd/microgateway/
// main_enterprise.go). Without the filter engine the in-process gateway runs
// the community script runner, which passes every filter through, and the
// live filter and guardrail suites would report the product as broken.
func requireEnterpriseRuntime(t *testing.T) {
	t.Helper()
	if !engine.Available() {
		t.Fatal("no filter script engine registered: the harness must import ai-studio-enterprise/v2/features/filters, as the enterprise microgateway main does")
	}
	if !guardrails.Available() {
		t.Fatal("no guardrail providers registered: the harness must import ai-studio-enterprise/v2/features/filters, as the enterprise microgateway main does")
	}
	if !sr.Available() {
		t.Fatal("no semantic router engine registered: the harness must import ai-studio-enterprise/v2/features/semantic_router/engine, as the enterprise microgateway main does")
	}
	if !plugin_security.IsEnterpriseAvailable() {
		t.Fatal("no enterprise plugin security service registered: the harness must import ai-studio-enterprise/v2/features/plugin_security")
	}
}

// TestEnterpriseHarnessRegistersRuntime runs in the enterprise unit tests (no
// vendorlive tag, no credentials), so a harness that drifts from the
// enterprise microgateway main fails in CI rather than in the next live run.
func TestEnterpriseHarnessRegistersRuntime(t *testing.T) {
	requireEnterpriseRuntime(t)
}
