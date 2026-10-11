package vendorconformance

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/decisions"
)

// Decision-model conformance: the half of the suite that drives decision
// models (OpenAI's /v1/decisions and the System One wire format served by
// TypeSafe Jev and llama.cpp self-hosts) through every gateway surface and
// asserts the wire schema on both sides of the translation. See
// features/DecisionModels.md and features/VendorConformance.md §6.
//
// Decision targets are deliberately NOT VendorConfigs. The chat matrix,
// pre-flight and equivalence checks iterate every configured vendor with chat
// requests, and a System One model cannot answer one. They are seeded as their
// own routes and run by their own tests, like the guardrail half.
//
// Nothing here reads the network.

// Decision wire formats with a canonical schema.
const (
	// FormatOpenAIDecision is the OpenAI Decisions response: what the shim and
	// universal surfaces promise for every vendor, and what OpenAI answers
	// natively.
	FormatOpenAIDecision Format = "openai_decision"
	// FormatSystemOneDecision is the native System One response.
	FormatSystemOneDecision Format = "systemone_decision"
)

// DecisionScenario is one behaviour exercised on a (target, surface) pair.
type DecisionScenario string

const (
	// DecisionBasic asks one question of each type over string evidence.
	DecisionBasic DecisionScenario = "basic"
	// DecisionMessages sends the evidence as user messages with input_text
	// parts, which the System One translation flattens into state.
	DecisionMessages DecisionScenario = "messages"
)

// AllDecisionScenarios in reporting order.
var AllDecisionScenarios = []DecisionScenario{DecisionBasic, DecisionMessages}

// DecisionEvidence is the evidence every scenario asks about. It is
// unambiguous so a semantically wrong answer is worth a warning.
const DecisionEvidence = "Customer message: I was charged twice for my order last week. " +
	"Please refund the duplicate payment today, I need the money back for rent."

// DecisionQuestions is the canonical question set: one of each type, in an
// order that differs from any alphabetical order, so a translation that
// sorts questions or options is caught.
func DecisionQuestions() []decisions.Question {
	return []decisions.Question{
		{Type: decisions.TypePredicate, Name: "is_billing", Instructions: "Is this message about a billing or payment problem?"},
		{Type: decisions.TypeChoice, Name: "team", Instructions: "Which team should handle this message?",
			Choices: []decisions.Choice{
				{Value: "shipping", Description: "deliveries, tracking, lost parcels"},
				{Value: "billing", Description: "payments, charges, refunds, invoices"},
				{Value: "technical", Description: "bugs, outages, login problems"},
			}},
		{Type: decisions.TypeScore, Name: "urgency", Instructions: "How urgent is this message?",
			Levels: []decisions.Level{
				{Label: "can wait"},
				{Label: "this week"},
				{Label: "today", Description: "needs action before the end of the day"},
			}},
	}
}

// DecisionTarget is one fully-resolved, credentialed decision model, seeded as
// its own LLM route.
type DecisionTarget struct {
	// Key identifies the target: "openai" or "systemone".
	Key string
	// Vendor is the platform vendor the route is seeded as.
	Vendor models.Vendor
	// APIKey is stored on the route; empty for a self-host without auth.
	APIKey string
	// Endpoint is the route's API base, stored verbatim.
	Endpoint string
	// Model is the decision model ID.
	Model string
	// Wire is the native wire format the vendor speaks.
	Wire decisions.Format
	// EnvPrefix is the variable prefix the target was loaded from.
	EnvPrefix string
	// ModelEnv names the variable holding Model, for error messages.
	ModelEnv string
}

// Slug is the route slug. It is distinct from any chat route's.
func (d DecisionTarget) Slug() string {
	return "vt-dec-" + strings.ReplaceAll(d.Key, "_", "-")
}

// NativePath is the vendor-native suffix the sdk surface appends after
// /llm/call/{slug}/: what that vendor's own SDK would send against the
// configured endpoint.
func (d DecisionTarget) NativePath() string {
	endpoint := decisions.OpenAIEndpoint
	if d.Wire == decisions.FormatSystemOne {
		endpoint = decisions.SystemOneEndpoint
	}
	if strings.HasSuffix(strings.TrimSuffix(d.Endpoint, "/"), "/v1") {
		return strings.TrimPrefix(endpoint, "/v1/")
	}
	return strings.TrimPrefix(endpoint, "/")
}

// NativeFormat is the canonical schema of the vendor's own response.
func (d DecisionTarget) NativeFormat() Format {
	if d.Wire == decisions.FormatSystemOne {
		return FormatSystemOneDecision
	}
	return FormatOpenAIDecision
}

// DecisionsConfig is the decision half of the suite configuration.
type DecisionsConfig struct {
	Enabled     bool
	Targets     []DecisionTarget
	Skipped     []SkippedVendor
	Scenarios   []DecisionScenario
	ArtifactDir string
}

// Target returns the configured target with the given key.
func (c *DecisionsConfig) Target(key string) (DecisionTarget, bool) {
	for _, t := range c.Targets {
		if t.Key == key {
			return t, true
		}
	}
	return DecisionTarget{}, false
}

// Summary renders the decision targets and skips for the run log.
func (c *DecisionsConfig) Summary() string {
	var b strings.Builder
	b.WriteString("=== DECISION MODEL CONFORMANCE ===\n")
	if len(c.Targets) == 0 {
		b.WriteString("  (no decision models configured)\n")
	}
	for _, t := range c.Targets {
		fmt.Fprintf(&b, "  %-10s %-10s model=%s wire=%s endpoint=%s\n", t.Key, t.Vendor, t.Model, t.Wire, t.Endpoint)
	}
	for _, s := range c.Skipped {
		fmt.Fprintf(&b, "  %-10s SKIPPED - %s\n", s.Key, s.Reason)
	}
	return b.String()
}

// decisionTargetKeys are the decision targets the suite knows, in report
// order. VENDOR_TESTS_VENDORS accepts them alongside the chat vendors.
var decisionTargetKeys = []string{"openai", "systemone"}

// LoadDecisions reads the decision targets from the credentials file and the
// environment. Like the vendor half, a target without credentials is skipped
// with its reason, never failed.
//
//	VT_OPENAI_MODEL_DECISION   OpenAI decision model (reuses VT_OPENAI_API_KEY / _ENDPOINT)
//	VT_SYSTEMONE_MODEL         System One model (jev-latest, or a self-hosted model name)
//	VT_SYSTEMONE_ENDPOINT      default https://api.typesafe.ai; a llama.cpp URL for self-hosts
//	VT_SYSTEMONE_API_KEY       required for the hosted API, optional for a self-host
func LoadDecisions() (*DecisionsConfig, error) {
	root, err := RepoRoot()
	if err != nil {
		return nil, err
	}
	if err := loadEnvFile(root); err != nil {
		return nil, err
	}

	cfg := &DecisionsConfig{Enabled: envBool("VENDOR_TESTS_ENABLED", false)}
	cfg.ArtifactDir = strings.TrimSpace(os.Getenv("VENDOR_TESTS_ARTIFACT_DIR"))
	if cfg.ArtifactDir == "" {
		cfg.ArtifactDir = filepath.Join(root, "test-results", "vendor-conformance")
	} else if !filepath.IsAbs(cfg.ArtifactDir) {
		cfg.ArtifactDir = filepath.Join(root, cfg.ArtifactDir)
	}

	if cfg.Scenarios, err = parseDecisionScenarios(os.Getenv("VENDOR_TESTS_DECISION_SCENARIOS")); err != nil {
		return nil, err
	}
	filter, err := parseVendorFilter(os.Getenv("VENDOR_TESTS_VENDORS"))
	if err != nil {
		return nil, err
	}

	for _, key := range decisionTargetKeys {
		if filter != nil && !filter[key] {
			continue
		}
		var target DecisionTarget
		var skip string
		switch key {
		case "openai":
			target, skip = loadOpenAIDecision()
		case "systemone":
			target, skip = loadSystemOneDecision()
		}
		if skip != "" {
			cfg.Skipped = append(cfg.Skipped, SkippedVendor{Key: key, Reason: skip})
			continue
		}
		cfg.Targets = append(cfg.Targets, target)
	}
	sort.Slice(cfg.Skipped, func(i, j int) bool { return cfg.Skipped[i].Key < cfg.Skipped[j].Key })
	return cfg, nil
}

func loadOpenAIDecision() (DecisionTarget, string) {
	key := env("VT_OPENAI_API_KEY")
	model := env("VT_OPENAI_MODEL_DECISION")
	switch {
	case key == "":
		return DecisionTarget{}, "VT_OPENAI_API_KEY not set"
	case model == "":
		return DecisionTarget{}, "VT_OPENAI_MODEL_DECISION not set"
	}
	return DecisionTarget{
		Key:       "openai",
		Vendor:    models.OPENAI,
		APIKey:    key,
		Endpoint:  envOr("VT_OPENAI_ENDPOINT", "https://api.openai.com/v1"),
		Model:     model,
		Wire:      decisions.FormatOpenAI,
		EnvPrefix: "VT_OPENAI",
		ModelEnv:  "VT_OPENAI_MODEL_DECISION",
	}, ""
}

// systemOneHostedEndpoint is TypeSafe's API, the vendor's default endpoint.
const systemOneHostedEndpoint = "https://api.typesafe.ai"

func loadSystemOneDecision() (DecisionTarget, string) {
	model := env("VT_SYSTEMONE_MODEL")
	endpoint := envOr("VT_SYSTEMONE_ENDPOINT", systemOneHostedEndpoint)
	key := env("VT_SYSTEMONE_API_KEY")
	switch {
	case model == "":
		return DecisionTarget{}, "VT_SYSTEMONE_MODEL not set"
	case key == "" && strings.TrimSuffix(endpoint, "/") == systemOneHostedEndpoint:
		return DecisionTarget{}, "VT_SYSTEMONE_API_KEY not set (required for the hosted API; set VT_SYSTEMONE_ENDPOINT for a self-host)"
	}
	return DecisionTarget{
		Key:       "systemone",
		Vendor:    models.SYSTEMONE,
		APIKey:    key,
		Endpoint:  endpoint,
		Model:     model,
		Wire:      decisions.FormatSystemOne,
		EnvPrefix: "VT_SYSTEMONE",
		ModelEnv:  "VT_SYSTEMONE_MODEL",
	}, ""
}

func parseDecisionScenarios(raw string) ([]DecisionScenario, error) {
	if strings.TrimSpace(raw) == "" {
		return append([]DecisionScenario(nil), AllDecisionScenarios...), nil
	}
	known := map[DecisionScenario]bool{}
	for _, s := range AllDecisionScenarios {
		known[s] = true
	}
	var out []DecisionScenario
	for _, item := range splitList(raw) {
		s := DecisionScenario(item)
		if !known[s] {
			return nil, fmt.Errorf("VENDOR_TESTS_DECISION_SCENARIOS: unknown scenario %q", item)
		}
		out = append(out, s)
	}
	return out, nil
}

// DecisionRequest builds the OpenAI-format request for a scenario. model is
// the model string the surface needs ("{slug}/{model}" on universal).
func DecisionRequest(scenario DecisionScenario, model, ref string) *decisions.Request {
	evidence := DecisionEvidence
	if ref != "" {
		evidence += " (" + ref + ")"
	}
	req := &decisions.Request{Model: model, Questions: DecisionQuestions()}
	switch scenario {
	case DecisionMessages:
		half := strings.Index(evidence, ". ") + 1
		req.Input, _ = json.Marshal([]map[string]any{{
			"role": "user",
			"content": []map[string]string{
				{"type": "input_text", "text": evidence[:half]},
				{"type": "input_text", "text": strings.TrimSpace(evidence[half:])},
			},
		}})
	default:
		req.Input, _ = json.Marshal(evidence)
	}
	return req
}

// NativeDecisionBody is the body the sdk surface sends: the request in the
// target's own wire format, exactly as that vendor's SDK would.
func NativeDecisionBody(t DecisionTarget, req *decisions.Request) ([]byte, error) {
	if t.Wire == decisions.FormatSystemOne {
		so, err := decisions.ToSystemOne(req)
		if err != nil {
			return nil, err
		}
		return json.Marshal(so)
	}
	return json.Marshal(req)
}

// DecisionEnvelope is the shape of a decision response, independent of wire
// format and of the model's (non-deterministic) values. Two surfaces that
// translated correctly produce the same envelope.
type DecisionEnvelope struct {
	Names    []string
	Types    []string
	HasUsage bool
}

// EnvelopeOf normalizes a parsed response.
func EnvelopeOf(resp *decisions.Response) DecisionEnvelope {
	env := DecisionEnvelope{HasUsage: resp.Usage != nil}
	for _, a := range resp.Answers {
		env.Names = append(env.Names, a.Name)
		env.Types = append(env.Types, a.Type)
	}
	return env
}

// probabilitySumTolerance allows for rounding in reported distributions.
const probabilitySumTolerance = 0.05

// CheckDecisionResponse returns every way resp breaks the decision contract
// for req: the invariants a JSON schema cannot express. An empty result is a
// conforming response.
func CheckDecisionResponse(req *decisions.Request, resp *decisions.Response) []string {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	if len(resp.Answers) != len(req.Questions) {
		add("%d answers for %d questions", len(resp.Answers), len(req.Questions))
	}
	for i, q := range req.Questions {
		if i >= len(resp.Answers) {
			break
		}
		a := resp.Answers[i]
		where := fmt.Sprintf("answers[%d] (%s)", i, q.Name)
		if a.Name != q.Name {
			add("%s: name %q, want %q (answers must follow question order)", where, a.Name, q.Name)
		}
		if a.Type == decisions.TypeRefusal {
			continue
		}
		if a.Type != q.Type {
			add("%s: type %q, want %q or refusal", where, a.Type, q.Type)
			continue
		}
		checkUnit := func(field string, v *float64) {
			if v != nil && (*v < 0 || *v > 1 || math.IsNaN(*v)) {
				add("%s: %s %v outside [0,1]", where, field, *v)
			}
		}
		checkUnit("confidence", a.Confidence)
		switch q.Type {
		case decisions.TypePredicate:
			if a.Probability == nil {
				add("%s: predicate without probability", where)
			}
			checkUnit("probability", a.Probability)
		case decisions.TypeChoice:
			values := map[string]bool{}
			for _, c := range q.Choices {
				values[c.Value] = true
			}
			if !values[a.Choice] {
				add("%s: choice %q is not one of the options", where, a.Choice)
			}
			sum := 0.0
			for _, p := range a.Probabilities {
				v, _ := p.Value.(string)
				if !values[v] {
					add("%s: probability for unknown option %v", where, p.Value)
				}
				pv := p.Probability
				checkUnit("probability", &pv)
				sum += pv
			}
			if len(a.Probabilities) == len(q.Choices) && math.Abs(sum-1) > probabilitySumTolerance {
				add("%s: option probabilities sum to %.3f, want 1", where, sum)
			}
		case decisions.TypeScore:
			if a.Score == nil {
				add("%s: score without score", where)
			} else if *a.Score < 0 || *a.Score > float64(len(q.Levels)-1) {
				add("%s: score %v outside [0,%d]", where, *a.Score, len(q.Levels)-1)
			}
			sum := 0.0
			for _, p := range a.Probabilities {
				idx, ok := levelIndex(p.Value)
				if !ok || idx != math.Trunc(idx) || idx < 0 || int(idx) >= len(q.Levels) {
					add("%s: probability for unknown level %v", where, p.Value)
				}
				pv := p.Probability
				checkUnit("probability", &pv)
				sum += pv
			}
			if len(a.Probabilities) == len(q.Levels) && math.Abs(sum-1) > probabilitySumTolerance {
				add("%s: level probabilities sum to %.3f, want 1", where, sum)
			}
		}
	}

	if resp.Usage == nil {
		add("no usage: the gateway cannot price or budget this call")
	} else {
		if resp.Usage.InputTokens <= 0 {
			add("usage.input_tokens is %d; decision models are billed by input tokens", resp.Usage.InputTokens)
		}
		if resp.Usage.OutputTokens < 0 {
			add("usage.output_tokens is negative (%d)", resp.Usage.OutputTokens)
		}
	}
	return problems
}

// levelIndex reads a score probability's level index: a float64 after a JSON
// round trip, an int straight from the translator.
func levelIndex(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

// DecisionPlausibility returns the answers to the canonical questions that a
// competent decision model should not give. They are reported as warnings,
// not failures: they judge the model, not the gateway, but a translation that
// scrambled options or levels shows up here first.
func DecisionPlausibility(resp *decisions.Response) []string {
	var warn []string
	for _, a := range resp.Answers {
		switch a.Name {
		case "is_billing":
			if a.Probability != nil && *a.Probability < 0.5 {
				warn = append(warn, fmt.Sprintf("is_billing probability %.2f for an obvious billing complaint", *a.Probability))
			}
		case "team":
			if a.Type == decisions.TypeChoice && a.Choice != "billing" {
				warn = append(warn, fmt.Sprintf("team %q for an obvious billing complaint", a.Choice))
			}
		}
	}
	return warn
}
