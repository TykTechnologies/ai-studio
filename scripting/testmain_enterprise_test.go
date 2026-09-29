//go:build enterprise

package scripting

// Register the enterprise script engine and guardrail providers, which the
// enterprise-tagged tests in this package exercise. The standalone and
// embedded builds get them from enterprise/all.
import _ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/filters"
