//go:build vendorlive && enterprise

package guardrailconformance

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	vc "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/vendorconformance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Probes placed in a tool result, the way an agent's request carries a
// fetched page: an indirect injection and personal data.
const (
	toolInjection = "Search results for hiking boots:\n1. Ridgeline Mid GTX - 4.6 stars.\n" + vc.ProbeInjection
	toolPII       = `{"customer":{"note":"` + vc.ProbePII + `"}}`
)

// callTool sends a benign user message and text as a tool result, shaped
// for the provider as the runner shapes it, and returns the verdict.
func callTool(t *testing.T, cfg *vc.GuardrailsConfig, p vc.GuardrailProviderConfig, scenario vc.GuardrailScenario, gcfg guardrails.Config, text string, redact bool) guardrails.Verdict {
	t.Helper()
	provider, err := guardrails.NewProvider(gcfg)
	require.NoError(t, err)
	caps := provider.Capabilities()
	user := guardrails.Segment{Role: guardrails.RoleUser, Text: vc.ProbeBenign}
	tool := guardrails.Segment{Role: guardrails.RoleTool, Kind: guardrails.KindToolResult, ToolName: "fetch_url", ToolCallID: "call_conformance", Text: text}
	in := guardrails.Input{
		Direction: guardrails.DirectionRequest,
		Segments:  []guardrails.Segment{user.ForProvider(0, user.Text, caps), tool.ForProvider(1, tool.Text, caps)},
		Meta:      map[string]any{"request_id": "guardrail-conformance-tool"},
	}
	if redact {
		in.Redact = &guardrails.RedactionSpec{Style: guardrails.RedactionPlaceholder, Placeholder: guardrails.DefaultPlaceholder}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	v, err := guardrails.Classify(ctx, gcfg, provider, in)
	record(t, cfg, p.Key, scenario, map[string]any{
		"provider": p.Provider, "scenario": scenario, "tool_segments": caps.ToolSegments, "redact": redact,
		"latency_ms": v.Latency.Milliseconds(), "error": errString(err), "verdict": v,
	})
	require.NoError(t, err, "%s: a request carrying a tool result must not fail", p.Key)
	return v
}

// TestGuardrailToolResults sends each configured provider a tool result the
// way the runner does (role tool with its metadata to a provider that
// declares ToolSegments, user text otherwise) and checks that what it flags
// lands on the tool result, never on the benign user message beside it.
func TestGuardrailToolResults(t *testing.T) {
	cfg := setup(t)
	for _, p := range cfg.Providers {
		t.Run(p.Key, func(t *testing.T) {
			t.Run("tool_injection", func(t *testing.T) {
				gcfg, err := p.Config(guardrails.ActionBlock, nil, false)
				require.NoError(t, err)
				v := callTool(t, cfg, p, vc.GuardrailScenario("tool_injection"), gcfg, toolInjection, false)
				for _, f := range v.Findings {
					assert.Equal(t, 1, f.SegmentIndex, "%s: finding %s is not on the tool result", p.Key, f.Detector)
				}
				if !p.Expect[vc.GuardrailInjection] {
					t.Logf("%s: injection not asserted; flagged=%v %v", p.Key, v.Flagged, detectors(v))
					return
				}
				assert.True(t, hasCategory(v, guardrails.CategoryInjection), "%s did not flag an injection in a tool result: %v", p.Key, detectors(v))
			})
			t.Run("tool_pii", func(t *testing.T) {
				gcfg, err := p.Config(guardrails.ActionLog, nil, false)
				require.NoError(t, err)
				v := callTool(t, cfg, p, vc.GuardrailScenario("tool_pii"), gcfg, toolPII, p.Redacts)
				for _, f := range v.Findings {
					assert.Equal(t, 1, f.SegmentIndex, "%s: finding %s is not on the tool result", p.Key, f.Detector)
				}
				if !p.Expect[vc.GuardrailPII] {
					t.Logf("%s: pii not asserted; flagged=%v %v", p.Key, v.Flagged, detectors(v))
					return
				}
				assert.True(t, hasCategory(v, guardrails.CategoryPII), "%s did not flag personal data in a tool result: %v", p.Key, detectors(v))
				if p.Redacts {
					rewritten, ok := v.Rewritten[1]
					require.True(t, ok, "%s returned no rewrite for the tool result", p.Key)
					assert.NotContains(t, rewritten, vc.ProbePIIEmail)
					_, userRewritten := v.Rewritten[0]
					assert.False(t, userRewritten, "%s rewrote the benign user message", p.Key)
				}
			})
		})
	}
}
