//go:build vendorlive

package vendorconformance

import (
	"encoding/json"
	"fmt"

	mgwdb "github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/guardrails"
	vc "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/vendorconformance"
	"gorm.io/gorm"
)

// Seeding of the guardrailed route lives outside the enterprise-tagged test
// file so the harness, which builds under `vendorlive` alone, always
// compiles. Under a community data-plane build the guardrail filters are
// no-ops and the route is simply another copy of the first vendor.

// guardrailBlockPrefix is the block message every guardrail filter on the
// route carries, followed by the provider key, so a 400 says which provider
// blocked.
const guardrailBlockPrefix = "blocked by the conformance guardrail "

// seedGuardrailed seeds a second route for vendor v with one request-side
// guardrail filter per configured provider. Order matters: the built-in
// secrets and PII filters go first so the credential and PII probes are
// decided locally, then every remote provider that is expected to flag an
// injection (each restricted to its injection detectors so a PII-only policy
// cannot pre-empt the probe), then the built-in injection heuristics last so
// the injection probe is blocked even with no remote provider configured.
func seedGuardrailed(db *gorm.DB, container *services.ServiceContainer, v vc.VendorConfig, g *vc.GuardrailsConfig, maxTokens int) (*mgwdb.LLM, string, error) {
	guarded := v
	guarded.Key = v.Key + "-guarded"
	llm, err := seedLLM(db, container, guarded, maxTokens)
	if err != nil {
		return nil, "", err
	}
	model, _ := v.Model(vc.ModelLatest)

	builtin, ok := g.Provider("builtin")
	if !ok {
		return nil, "", fmt.Errorf("the built-in provider is always configured; VENDOR_TESTS_GUARDRAILS must include builtin")
	}

	type spec struct {
		name      string
		provider  vc.GuardrailProviderConfig
		action    string
		detectors []string
	}
	specs := []spec{
		{"vt-guard-builtin-secrets", builtin, guardrails.ActionBlock, []string{guardrails.CategorySecrets}},
		{"vt-guard-builtin-pii", builtin, guardrails.ActionRedact, []string{guardrails.CategoryPII}},
	}
	for _, p := range g.Remote() {
		if !p.Expect[vc.GuardrailInjection] {
			continue
		}
		specs = append(specs, spec{"vt-guard-" + p.Key, p, guardrails.ActionBlock, injectionDetectors(p)})
	}
	specs = append(specs, spec{"vt-guard-builtin-injection", builtin, guardrails.ActionBlock, []string{guardrails.CategoryInjection}})

	for i, s := range specs {
		cfg := s.provider.FilterConfig(s.action, s.detectors)
		cfg["block_message"] = guardrailBlockPrefix + s.provider.Key
		// The edge stores what the hub would have pushed: normalised, with
		// the connection references already resolved.
		parsed, err := guardrails.ParseConfig(cfg)
		if err != nil {
			return nil, "", fmt.Errorf("%s: %w", s.name, err)
		}
		normalised, err := guardrails.Normalize(parsed, false)
		if err != nil {
			return nil, "", fmt.Errorf("%s: %w", s.name, err)
		}
		raw, err := json.Marshal(normalised)
		if err != nil {
			return nil, "", err
		}
		filter := &mgwdb.Filter{
			Name:     s.name,
			Script:   "",
			Kind:     "guardrail",
			Config:   string(raw),
			IsActive: true,
		}
		if err := db.Create(filter).Error; err != nil {
			return nil, "", err
		}
		if err := db.Create(&mgwdb.LLMFilter{LLMID: llm.ID, FilterID: filter.ID, IsActive: true, OrderIndex: i}).Error; err != nil {
			return nil, "", err
		}
	}
	return llm, model, nil
}

// injectionDetectors narrows a provider to the detectors that classify prompt
// attacks, so its PII or moderation detectors cannot fire on the other probes.
func injectionDetectors(p vc.GuardrailProviderConfig) []string {
	switch p.Provider {
	case guardrails.ProviderLakera, guardrails.ProviderAzureContentSafety:
		return []string{"prompt_attack"}
	case guardrails.ProviderBedrockGuardrails:
		return []string{"content"}
	}
	return p.Detectors
}
