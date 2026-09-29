//go:build enterprise
// +build enterprise

package main

import (
	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/scripting/engine"

	// The filter script engine and guardrail providers register themselves
	// with scripting/engine and the guardrails registry. Without them the
	// edge would pass every filter through.
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/filters"
	// The Semantic Router engine registers itself with pkg/semanticrouting.
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/semantic_router/engine"
)

// An enterprise edge must enforce filters: fail at start rather than pass
// them through if the registrations above ever go missing.
func init() {
	if !engine.Available() || !guardrails.Available() {
		panic("enterprise microgateway built without the filter engine: import github.com/TykTechnologies/ai-studio-enterprise/v2/features/filters")
	}
}
