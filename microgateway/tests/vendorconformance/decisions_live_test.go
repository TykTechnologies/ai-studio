//go:build vendorlive

package vendorconformance

// Decision-model conformance: OpenAI's /v1/decisions and the System One wire
// format (TypeSafe Jev, llama.cpp self-hosts), driven through every gateway
// surface against the real models. It proves what the unit tests cannot: that
// the vendors answer in the shapes pkg/decisions translates, that the
// translation holds on the real wire, and that the data plane records,
// prices and counts the calls.
//
//	make test-vendors-decisions
//
// Targets come from test-secrets/vendors.env (VT_OPENAI_MODEL_DECISION,
// VT_SYSTEMONE_*); see features/VendorConformance.md §6.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	mgwdb "github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/pkg/decisions"
	vc "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/vendorconformance"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decisionTestPrice is the input price seeded for every decision model, so
// the analytics test can assert a priced call. Its value is irrelevant.
const decisionTestPrice = 0.0000001

// seedDecisionTargets seeds one route per decision target, grants it to the
// conformance App and prices its model. Like seedLLM it must run before the
// server is built.
func seedDecisionTargets(db *gorm.DB, container *services.ServiceContainer, cfg *vc.DecisionsConfig, appID uint, maxTokens int) (map[string]*mgwdb.LLM, error) {
	routes := map[string]*mgwdb.LLM{}
	if cfg == nil {
		return routes, nil
	}
	for _, t := range cfg.Targets {
		llm := &mgwdb.LLM{
			Name:           "VT decision " + t.Key,
			Slug:           t.Slug(),
			Vendor:         string(t.Vendor),
			Endpoint:       t.Endpoint,
			DefaultModel:   t.Model,
			MaxTokens:      maxTokens,
			TimeoutSeconds: 300,
			IsActive:       true,
		}
		if t.APIKey != "" {
			enc, err := container.Crypto.Encrypt(t.APIKey)
			if err != nil {
				return nil, fmt.Errorf("encrypting %s key: %w", t.Key, err)
			}
			llm.APIKeyEncrypted = enc
		}
		if err := db.Create(llm).Error; err != nil {
			return nil, err
		}
		if err := db.Create(&mgwdb.AppLLM{AppID: appID, LLMID: llm.ID, IsActive: true}).Error; err != nil {
			return nil, err
		}
		price := &mgwdb.ModelPrice{ModelName: t.Model, Vendor: string(t.Vendor), CPIT: decisionTestPrice, CPT: 0, Currency: "USD"}
		if err := db.Create(price).Error; err != nil {
			return nil, err
		}
		routes[t.Key] = llm
	}
	return routes, nil
}

// decisionTargets returns the configured targets, skipping the test when
// there are none.
func decisionTargets(t *testing.T, h *harness) []vc.DecisionTarget {
	t.Helper()
	if h.decisions == nil || len(h.decisions.Targets) == 0 {
		msg := "no decision models configured"
		if h.decisions != nil {
			msg += ":\n" + h.decisions.Summary()
		}
		t.Skip(msg)
	}
	return h.decisions.Targets
}

// decisionSurfaces are the gateway surfaces a decision call can take, in the
// configured set.
func decisionSurfaces(h *harness) []vc.Surface {
	var out []vc.Surface
	for _, s := range []vc.Surface{vc.SurfaceShim, vc.SurfaceSDK, vc.SurfaceUniversal} {
		if h.cfg.HasSurface(s) {
			out = append(out, s)
		}
	}
	return out
}

// decisionCall is one decision request on one surface: where it goes, what it
// sends, and the format it must come back in.
type decisionCall struct {
	url    string
	body   []byte
	format vc.Format
	// req is the OpenAI-shape request with the bare model, for parsing and
	// checking the answers whatever the surface's wire format.
	req *decisions.Request
}

func (h *harness) decisionCall(t *testing.T, target vc.DecisionTarget, surface vc.Surface, scenario vc.DecisionScenario, ref string) decisionCall {
	t.Helper()
	req := vc.DecisionRequest(scenario, target.Model, ref)
	call := decisionCall{req: req, format: vc.FormatOpenAIDecision}
	var err error
	switch surface {
	case vc.SurfaceShim:
		call.url = fmt.Sprintf("%s/ai/%s/v1/decisions", h.baseURL, target.Slug())
		call.body, err = json.Marshal(req)
	case vc.SurfaceUniversal:
		call.url = h.baseURL + "/v1/decisions"
		routed := *req
		routed.Model = target.Slug() + "/" + target.Model
		call.body, err = json.Marshal(&routed)
	case vc.SurfaceSDK:
		call.url = fmt.Sprintf("%s/llm/call/%s/%s", h.baseURL, target.Slug(), target.NativePath())
		call.body, err = vc.NativeDecisionBody(target, req)
		call.format = target.NativeFormat()
	default:
		t.Fatalf("no decision URL for surface %s", surface)
	}
	require.NoError(t, err)
	return call
}

// sendDecision posts a call and returns the parsed OpenAI-shape response, or
// nil when the case was skipped or failed (the test is already marked).
func sendDecision(t *testing.T, h *harness, name string, call decisionCall) (*response, *decisions.Response) {
	t.Helper()
	resp := mustPost(t, h, call.url, call.body)
	if resp.Status != http.StatusOK {
		h.recorder.RecordFailure(name, fmt.Sprintf("HTTP %d", resp.Status), resp.Body)
		if skipIfThrottled(t, resp) {
			return resp, nil
		}
		t.Errorf("expected 200, got %d:\n%s", resp.Status, bodyExcerpt(resp.Body))
		return resp, nil
	}
	requireJSON(t, resp.Body)

	res, err := vc.Validate(call.format, resp.Body)
	require.NoError(t, err, "validating response")
	h.recorder.RecordDrift(name, res.Drift)
	if !res.OK() {
		h.recorder.RecordFailure(name, res.Error(), resp.Body)
		t.Errorf("%s\n\nresponse:\n%s", res.Error(), bodyExcerpt(resp.Body))
	}
	if h.cfg.StrictUnknownFields && len(res.Drift) > 0 {
		t.Errorf("strict mode: %d undocumented field(s): %v", len(res.Drift), res.Drift)
	} else if len(res.Drift) > 0 {
		t.Logf("drift (fields the schema does not describe): %v", res.Drift)
	}

	parsed, err := decisions.ParseResponse(resp.Body, call.req)
	if err != nil {
		h.recorder.RecordFailure(name, "unparseable decision response", resp.Body)
		t.Fatalf("not a decision response: %v\n%s", err, bodyExcerpt(resp.Body))
	}
	t.Cleanup(func() {
		if t.Failed() {
			h.recorder.RecordFailure(name, "assertion failed", resp.Body)
		}
	})
	return resp, parsed
}

// TestPreflightDecisionModelsCallable proves each decision model exists and
// its credentials work with one single-question call, so a later failure is
// a translation bug rather than a configuration one.
func TestPreflightDecisionModelsCallable(t *testing.T) {
	h := setup(t)
	for _, target := range decisionTargets(t, h) {
		target := target
		t.Run(target.Key, func(t *testing.T) {
			t.Parallel()
			body, err := json.Marshal(&decisions.Request{
				Model: target.Model,
				Input: json.RawMessage(`"The sky is blue."`),
				Questions: []decisions.Question{
					{Type: decisions.TypePredicate, Name: "about_sky", Instructions: "Is this about the sky?"},
				},
			})
			require.NoError(t, err)
			resp := mustPost(t, h, fmt.Sprintf("%s/ai/%s/v1/decisions", h.baseURL, target.Slug()), body)
			if resp.Status == http.StatusOK || skipIfThrottled(t, resp) {
				return
			}
			t.Errorf("decision model %q is not callable (HTTP %d).\nCheck %s and %s_ENDPOINT in test-secrets/vendors.env.\n\nResponse:\n%s",
				target.Model, resp.Status, target.ModelEnv, target.EnvPrefix, bodyExcerpt(resp.Body))
		})
	}
}

// TestDecisionConformance is the decision matrix: every target, on every
// gateway surface, for every scenario. Each response is validated against the
// canonical schema of the wire format the surface promised, its undocumented
// fields are recorded as drift, and the answers are checked against the
// questions asked.
func TestDecisionConformance(t *testing.T) {
	h := setup(t)
	targets := decisionTargets(t, h)
	t.Log(h.decisions.Summary())

	for _, target := range targets {
		for _, surface := range decisionSurfaces(h) {
			for _, scenario := range h.decisions.Scenarios {
				target, surface, scenario := target, surface, scenario
				name := fmt.Sprintf("decisions/%s/%s/%s", target.Key, surface, scenario)
				t.Run(fmt.Sprintf("%s/%s/%s", target.Key, surface, scenario), func(t *testing.T) {
					t.Parallel()
					call := h.decisionCall(t, target, surface, scenario, "")
					resp, parsed := sendDecision(t, h, name, call)
					if parsed == nil {
						return
					}
					for _, problem := range vc.CheckDecisionResponse(call.req, parsed) {
						t.Errorf("decision contract: %s", problem)
					}
					for _, warn := range vc.DecisionPlausibility(parsed) {
						t.Logf("WARNING (model quality, not a gateway failure): %s", warn)
					}
					if surface != vc.SurfaceSDK {
						assert.Equal(t, target.Slug(), resp.Header.Get("X-Tyk-Served-LLM"), "served-LLM header")
					}
				})
			}
		}
	}
}

// TestDecisionEquivalence sends the same request through every surface and
// compares the answer shapes. The shim and universal surfaces must agree
// exactly (the universal router is a pre-auth rewrite), and the native sdk
// response, read through pkg/decisions, must describe the same answers: a
// translation that dropped, reordered or retyped an answer fails here even
// when every response is individually schema-valid.
func TestDecisionEquivalence(t *testing.T) {
	h := setup(t)
	surfaces := decisionSurfaces(h)
	if len(surfaces) < 2 {
		t.Skip("equivalence needs at least two surfaces")
	}
	for _, target := range decisionTargets(t, h) {
		target := target
		t.Run(target.Key, func(t *testing.T) {
			t.Parallel()
			envelopes := map[vc.Surface]vc.DecisionEnvelope{}
			for _, surface := range surfaces {
				call := h.decisionCall(t, target, surface, vc.DecisionBasic, "")
				_, parsed := sendDecision(t, h, fmt.Sprintf("decisions/%s/%s/equivalence", target.Key, surface), call)
				if parsed == nil {
					return
				}
				envelopes[surface] = vc.EnvelopeOf(parsed)
			}
			first := surfaces[0]
			for _, s := range surfaces[1:] {
				// A refusal on one surface and an answer on another is the
				// model, not the gateway; compare names and answered types.
				assert.Equal(t, envelopes[first].Names, envelopes[s].Names, "%s vs %s: answer names", first, s)
				assert.Equal(t, envelopes[first].HasUsage, envelopes[s].HasUsage, "%s vs %s: usage", first, s)
				for i := range envelopes[s].Types {
					a, b := envelopes[first].Types, envelopes[s].Types
					if i < len(a) && a[i] != decisions.TypeRefusal && b[i] != decisions.TypeRefusal {
						assert.Equal(t, a[i], b[i], "%s vs %s: answer %d type", first, s, i)
					}
				}
			}
		})
	}
}

// TestDecisionErrors checks the error contract on the OpenAI-format surfaces:
// a request the gateway can reject itself never reaches the vendor and names
// the offending parameter, and an upstream refusal reaches the caller as a
// 4xx OpenAI error envelope rather than our 5xx.
func TestDecisionErrors(t *testing.T) {
	h := setup(t)
	for _, target := range decisionTargets(t, h) {
		for _, surface := range []vc.Surface{vc.SurfaceShim, vc.SurfaceUniversal} {
			if !h.cfg.HasSurface(surface) {
				continue
			}
			target, surface := target, surface
			t.Run(fmt.Sprintf("%s/%s", target.Key, surface), func(t *testing.T) {
				t.Parallel()
				send := func(mutate func(map[string]any)) *response {
					call := h.decisionCall(t, target, surface, vc.DecisionBasic, "")
					var body map[string]any
					require.NoError(t, json.Unmarshal(call.body, &body))
					mutate(body)
					raw, err := json.Marshal(body)
					require.NoError(t, err)
					return mustPost(t, h, call.url, raw)
				}
				expectEnvelope := func(resp *response, status int, param string) {
					t.Helper()
					require.Equal(t, status, resp.Status, bodyExcerpt(resp.Body))
					res, err := vc.Validate(vc.FormatOpenAIError, resp.Body)
					require.NoError(t, err)
					assert.True(t, res.OK(), "%s\n%s", res.Error(), bodyExcerpt(resp.Body))
					if param != "" {
						var env struct {
							Error struct {
								Param string `json:"param"`
								Type  string `json:"type"`
							} `json:"error"`
						}
						require.NoError(t, json.Unmarshal(resp.Body, &env))
						assert.Equal(t, param, env.Error.Param)
						assert.Equal(t, "invalid_request_error", env.Error.Type)
					}
				}

				t.Run("one-option choice is rejected by the gateway", func(t *testing.T) {
					resp := send(func(b map[string]any) {
						qs := b["questions"].([]any)
						team := qs[1].(map[string]any)
						team["choices"] = team["choices"].([]any)[:1]
					})
					expectEnvelope(resp, http.StatusBadRequest, "questions[1].choices")
				})

				t.Run("streaming is rejected", func(t *testing.T) {
					resp := send(func(b map[string]any) { b["stream"] = true })
					expectEnvelope(resp, http.StatusBadRequest, "stream")
				})

				t.Run("unknown model is a 4xx envelope", func(t *testing.T) {
					resp := send(func(b map[string]any) {
						model := "vt-no-such-decision-model"
						if surface == vc.SurfaceUniversal {
							model = target.Slug() + "/" + model
						}
						b["model"] = model
					})
					if resp.Status == http.StatusOK {
						t.Skipf("%s serves whatever model it loaded and ignores the model name", target.Key)
					}
					if skipIfThrottled(t, resp) {
						return
					}
					assert.True(t, resp.Status >= 400 && resp.Status < 500,
						"an unknown model is the caller's error, got %d:\n%s", resp.Status, bodyExcerpt(resp.Body))
					res, err := vc.Validate(vc.FormatOpenAIError, resp.Body)
					require.NoError(t, err)
					assert.True(t, res.OK(), "%s\n%s", res.Error(), bodyExcerpt(resp.Body))
				})
			})
		}
	}
}

// TestDecisionAnalytics proves the data plane records a decision call as one:
// the request kind, the question counts, the input tokens the vendor reported
// and a cost from the seeded price. This is the check that a decision model's
// spend reaches budgets (it did not before decision support: the response
// failed analysis and nothing was recorded).
func TestDecisionAnalytics(t *testing.T) {
	h := setup(t)
	for _, target := range decisionTargets(t, h) {
		for _, surface := range []vc.Surface{vc.SurfaceShim, vc.SurfaceSDK} {
			if !h.cfg.HasSurface(surface) {
				continue
			}
			target, surface := target, surface
			t.Run(fmt.Sprintf("%s/%s", target.Key, surface), func(t *testing.T) {
				t.Parallel()
				ref := newRef()
				call := h.decisionCall(t, target, surface, vc.DecisionBasic, ref)
				_, parsed := sendDecision(t, h, fmt.Sprintf("decisions/%s/%s/analytics", target.Key, surface), call)
				if parsed == nil {
					return
				}

				route := h.decisionRoutes[target.Key]
				require.NotNil(t, route, "no route seeded for %s", target.Key)
				ev := waitForLoggedRequestOn(t, h, route.ID, ref)

				assert.Equal(t, "decision", ev.RequestKind, "request kind")
				assert.Equal(t, len(call.req.Questions), ev.DecisionQuestions, "questions asked")
				assert.Equal(t, len(call.req.Questions), ev.Choices+ev.DecisionRefusals, "answered + refused = asked")
				if parsed.Usage != nil {
					// The record keeps OpenAI's cached share apart, as for chat.
					assert.Equal(t, parsed.Usage.InputTokens, ev.PromptTokens+ev.CacheReadPromptTokens+ev.CacheWritePromptTokens,
						"input tokens as the vendor reported them")
				}
				assert.Greater(t, ev.PromptTokens, 0, "a decision call is billed by input tokens")
				if ev.Cost <= 0 {
					t.Errorf("decision call recorded with no cost. The price was seeded for %q (%s); the event names the model %q. "+
						"If those differ the vendor reports a different model name than it was asked for, and price lookup misses it.",
						target.Model, target.Vendor, ev.Name)
				}
				assert.Equal(t, http.StatusOK, ev.StatusCode)
			})
		}
	}
}

// TestDecisionModelListing checks that /v1/models advertises each decision
// route with the decisions capability, which is how clients discover them.
func TestDecisionModelListing(t *testing.T) {
	h := setup(t)
	targets := decisionTargets(t, h)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, h.baseURL+"/v1/models", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+testAPIToken)
	resp, err := h.client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body := readAll(t, resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, bodyExcerpt(body))

	var list struct {
		Data []struct {
			ID           string   `json:"id"`
			Capabilities []string `json:"capabilities"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &list))
	caps := map[string][]string{}
	for _, m := range list.Data {
		caps[m.ID] = m.Capabilities
	}
	for _, target := range targets {
		id := target.Slug() + "/" + target.Model
		got, ok := caps[id]
		if assert.True(t, ok, "%s missing from /v1/models:\n%s", id, bodyExcerpt(body)) {
			assert.Contains(t, got, "decisions", "%s capabilities %v", id, got)
			if target.Vendor == "systemone" {
				assert.NotContains(t, got, "chat", "a System One route cannot serve chat")
			}
		}
	}
}
