//go:build enterprise

package scripting

// The Enterprise build registers the guardrail provider implementations by
// importing them for their init() side effects. The Community build has no
// factories, and guardrail filters pass through exactly as script filters do.
import (
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/guardrails/builtin"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/guardrails/providers"
)
