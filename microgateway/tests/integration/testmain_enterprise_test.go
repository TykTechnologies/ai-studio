//go:build enterprise
// +build enterprise

package integration

import (
	"fmt"
	"os"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"github.com/TykTechnologies/midsommar/v2/scripting/engine"

	// Import enterprise features to register factories before tests run
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/filters"
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/plugin_security"
	// The enterprise microgateway main also registers the Semantic Router
	// engine (microgateway/cmd/microgateway/main_enterprise.go).
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/semantic_router/engine"
)

func TestMain(m *testing.M) {
	// Enterprise factories are now registered via init()

	// Fail fast if the registrations above go missing: the in-process
	// gateway would run the community runners and pass filters through.
	if !engine.Available() || !guardrails.Available() || !sr.Available() {
		fmt.Fprintln(os.Stderr, "enterprise integration tests built without the enterprise filter engine, guardrail providers or Semantic Router engine: import them as microgateway/cmd/microgateway/main_enterprise.go does")
		os.Exit(1)
	}
	os.Exit(m.Run())
}
