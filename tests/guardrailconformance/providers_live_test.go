//go:build vendorlive && enterprise

// Package guardrailconformance drives every configured guardrail provider
// against its real service and asserts the verdict our provider code derives.
//
// Gated twice: the `vendorlive` build tag and VENDOR_TESTS_ENABLED=true, like
// the vendor suite. Credentials come from test-secrets/vendors.env (the
// VT_GUARD_* section); a provider with blank required variables is skipped
// with a reason, never silently.
//
//	make test-guardrails
//
// Every verdict is written to test-results/vendor-conformance/guardrails/ so
// a real vendor response can be reviewed after the run.
package guardrailconformance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	// Registers the enterprise provider implementations (builtin and remote).
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/guardrails/builtin"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/guardrails/providers"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	vc "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/vendorconformance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	sharedOnce sync.Once
	shared     *vc.GuardrailsConfig
	sharedSkip string
)

func setup(t *testing.T) *vc.GuardrailsConfig {
	t.Helper()
	sharedOnce.Do(func() {
		cfg, err := vc.LoadGuardrails()
		switch {
		case err != nil:
			sharedSkip = "guardrail conformance config error: " + err.Error()
		case !cfg.Enabled:
			sharedSkip = "VENDOR_TESTS_ENABLED is not true (set it in test-secrets/vendors.env)"
		default:
			shared = cfg
			fmt.Fprint(os.Stderr, "\n"+cfg.Summary()+"\n")
			_ = os.MkdirAll(cfg.ArtifactDir, 0o755)
		}
	})
	if sharedSkip != "" {
		t.Skip(sharedSkip)
	}
	return shared
}

// call runs one probe through the real provider with the filter's default
// timeout, records the verdict, and returns it.
func call(t *testing.T, cfg *vc.GuardrailsConfig, p vc.GuardrailProviderConfig, scenario vc.GuardrailScenario, gcfg guardrails.Config, text string, redact bool) (guardrails.Verdict, error) {
	t.Helper()
	provider, err := guardrails.NewProvider(gcfg)
	require.NoError(t, err, "%s: provider not registered in this build", p.Key)

	in := guardrails.Input{
		Segments:  []guardrails.Segment{{Index: 0, Role: "user", Text: text}},
		Direction: guardrails.DirectionRequest,
		Meta:      map[string]any{"request_id": "guardrail-conformance"},
	}
	if redact {
		in.Redact = &guardrails.RedactionSpec{Style: guardrails.RedactionPlaceholder, Placeholder: guardrails.DefaultPlaceholder}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	v, err := guardrails.Classify(ctx, gcfg, provider, in)

	record(t, cfg, p.Key, scenario, map[string]any{
		"provider": p.Provider, "scenario": scenario, "text": text, "redact": redact,
		"latency_ms": v.Latency.Milliseconds(), "error": errString(err), "verdict": v,
	})
	return v, err
}

func record(t *testing.T, cfg *vc.GuardrailsConfig, key string, scenario vc.GuardrailScenario, payload map[string]any) {
	t.Helper()
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return
	}
	path := filepath.Join(cfg.ArtifactDir, fmt.Sprintf("%s-%s.json", key, scenario))
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Logf("could not write %s: %v", path, err)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func detectors(v guardrails.Verdict) []string {
	out := []string{}
	for _, f := range v.Findings {
		out = append(out, f.Detector)
	}
	return out
}

func hasCategory(v guardrails.Verdict, category string) bool {
	for _, f := range v.Findings {
		if f.Category == category {
			return true
		}
	}
	return false
}

// TestGuardrailPreflight proves each configured provider is reachable with its
// credentials: a benign prompt must come back clean and without error. When
// this is green, a later failure is a contract problem, not a configuration one.
func TestGuardrailPreflight(t *testing.T) {
	cfg := setup(t)
	require.NotEmpty(t, cfg.Providers)
	for _, p := range cfg.Providers {
		p := p
		t.Run(p.Key, func(t *testing.T) {
			gcfg, err := p.Config(guardrails.ActionLog, nil, false)
			require.NoError(t, err)
			v, err := call(t, cfg, p, vc.GuardrailBenign, gcfg, vc.ProbeBenign, false)
			require.NoError(t, err, "%s: the provider must answer a benign prompt", p.Key)
			assert.False(t, v.Flagged, "%s flagged a benign prompt: %v", p.Key, detectors(v))
			t.Logf("%s: benign clean in %s", p.Key, v.Latency)
		})
	}
}

// TestGuardrailProviders runs the probe scenarios on every configured provider.
// Scenarios a provider is not expected to flag (its own policy decides) are
// still run and recorded, but only logged.
func TestGuardrailProviders(t *testing.T) {
	cfg := setup(t)
	for _, p := range cfg.Providers {
		p := p
		t.Run(p.Key, func(t *testing.T) {
			t.Run(string(vc.GuardrailInjection), func(t *testing.T) {
				gcfg, err := p.Config(guardrails.ActionBlock, nil, false)
				require.NoError(t, err)
				v, err := call(t, cfg, p, vc.GuardrailInjection, gcfg, vc.ProbeInjection, false)
				require.NoError(t, err)
				if !p.Expect[vc.GuardrailInjection] {
					t.Logf("%s: injection not asserted for this provider; verdict flagged=%v %v", p.Key, v.Flagged, detectors(v))
					return
				}
				require.True(t, v.Flagged, "%s did not flag an explicit instruction override: %v", p.Key, detectors(v))
				assert.True(t, hasCategory(v, guardrails.CategoryInjection), "%s flagged it but not as injection: %+v", p.Key, v.Findings)
				for _, f := range v.Findings {
					assert.NotContains(t, f.Label+f.Detector, "DAN", "findings must not carry the prompt text")
				}
			})

			t.Run(string(vc.GuardrailPII), func(t *testing.T) {
				gcfg, err := p.Config(guardrails.ActionLog, nil, false)
				require.NoError(t, err)
				v, err := call(t, cfg, p, vc.GuardrailPII, gcfg, vc.ProbePII, p.Redacts)
				require.NoError(t, err)
				if !p.Expect[vc.GuardrailPII] {
					t.Logf("%s: pii not asserted for this provider; verdict flagged=%v %v", p.Key, v.Flagged, detectors(v))
					return
				}
				require.True(t, v.Flagged, "%s did not flag an email and a phone number: %v", p.Key, detectors(v))
				assert.True(t, hasCategory(v, guardrails.CategoryPII), "%s flagged it but not as pii: %+v", p.Key, v.Findings)
				if p.Redacts {
					rewritten, ok := v.Rewritten[0]
					require.True(t, ok, "%s can redact but returned no rewrite", p.Key)
					assert.NotContains(t, rewritten, vc.ProbePIIEmail, "%s left the email in the rewrite: %q", p.Key, rewritten)
					assert.Contains(t, rewritten, "invoice", "%s rewrote more than the entities: %q", p.Key, rewritten)
					t.Logf("%s rewrite: %s", p.Key, rewritten)
				}
			})

			t.Run(string(vc.GuardrailSecret), func(t *testing.T) {
				gcfg, err := p.Config(guardrails.ActionBlock, nil, false)
				require.NoError(t, err)
				v, err := call(t, cfg, p, vc.GuardrailSecret, gcfg, vc.ProbeSecret, false)
				require.NoError(t, err)
				if !p.Expect[vc.GuardrailSecret] {
					t.Logf("%s: secret detection not asserted; verdict flagged=%v %v", p.Key, v.Flagged, detectors(v))
					return
				}
				require.True(t, v.Flagged, "%s did not flag an AWS access key", p.Key)
				for _, f := range v.Findings {
					assert.NotContains(t, f.Label, vc.ProbeSecretKey, "findings must never carry the secret")
				}
			})

			if p.Provider == guardrails.ProviderAzureContentSafety {
				t.Run(string(vc.GuardrailModeration), func(t *testing.T) {
					// Threshold 0 makes every category a finding, which proves
					// the response carries all four without sending harmful text.
					gcfg, err := p.Config(guardrails.ActionLog, []string{"Hate", "Sexual", "SelfHarm", "Violence"}, false)
					require.NoError(t, err)
					zero := 0.0
					for i := range gcfg.Detectors {
						gcfg.Detectors[i].Threshold = &zero
					}
					v, err := call(t, cfg, p, vc.GuardrailModeration, gcfg, vc.ProbeBenign, false)
					require.NoError(t, err)
					got := detectors(v)
					for _, c := range []string{"Hate", "Sexual", "SelfHarm", "Violence"} {
						assert.Contains(t, got, c, "text:analyze response is missing category %s", c)
					}
				})
			}

			if p.KeyField != "" {
				t.Run(string(vc.GuardrailAuthError), func(t *testing.T) {
					bad := p
					bad.Connection = map[string]string{}
					for k, v := range p.Connection {
						bad.Connection[k] = v
					}
					bad.Connection[p.KeyField] = "invalid-credential-for-conformance"
					gcfg, err := bad.Config(guardrails.ActionBlock, nil, false)
					require.NoError(t, err)
					_, err = call(t, cfg, bad, vc.GuardrailAuthError, gcfg, vc.ProbeBenign, false)
					require.Error(t, err, "%s accepted an invalid credential; a misconfigured filter would silently pass everything", p.Key)
					assert.False(t, errors.Is(err, context.DeadlineExceeded), "%s: auth failure must be a fast rejection, not a timeout", p.Key)
					assert.NotContains(t, err.Error(), "invalid-credential-for-conformance", "the error must not echo the credential")
					t.Logf("%s rejected a bad credential: %s", p.Key, truncate(err.Error(), 160))
				})
			}
		})
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// TestGuardrailProviderCatalogueMatchesRegistry pins the credential loaders
// to the provider registry: every remote loader names a provider this build
// implements, so a renamed provider cannot leave the suite silently skipping.
func TestGuardrailProviderCatalogueMatchesRegistry(t *testing.T) {
	cfg := setup(t)
	for _, p := range cfg.Providers {
		spec, ok := guardrails.ProviderSpec(p.Provider)
		require.True(t, ok, "%s names unknown provider %q", p.Key, p.Provider)
		assert.True(t, spec.Available, "%s: provider %q has no implementation in this build", p.Key, p.Provider)
		assert.Equal(t, spec.Redacts, p.Redacts, "%s: Redacts disagrees with the provider spec", p.Key)
	}
	_ = strings.TrimSpace
}
