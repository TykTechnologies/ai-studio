package proxy

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// priceTable answers GetModelPriceByModelNameAndVendor from a map: an unknown
// model gets the unpriced (all zero) row the real services return.
type priceTable struct {
	services.ServiceInterface
	prices  map[string]float64
	lookups []string
}

func (p *priceTable) GetModelPriceByModelNameAndVendor(model, vendor string) (*models.ModelPrice, error) {
	p.lookups = append(p.lookups, model)
	return &models.ModelPrice{ModelName: model, Vendor: vendor, CPT: p.prices[model], CPIT: p.prices[model], Currency: "USD"}, nil
}

// A Bedrock model can be addressed by its id, by a cross-region
// inference-profile id and by ARN, and Bedrock reports no model back, so a
// call was priced by whatever string the client sent: a call by ARN was
// recorded at zero cost and never charged a budget, although the profile or
// the model had a price (midsommar-clean-6bg5.8). Every spelling of a priced
// model now costs the same; an identifier that names no model stays unpriced.
func TestBedrockPricing_EverySpellingOfAModelIsPriced(t *testing.T) {
	const (
		modelID    = "amazon.nova-lite-v1:0"
		profileID  = "apac.amazon.nova-lite-v1:0"
		arn        = "arn:aws:bedrock:ap-southeast-2:123456789012:inference-profile/apac.amazon.nova-lite-v1:0"
		fmARN      = "arn:aws:bedrock:ap-southeast-2::foundation-model/amazon.nova-lite-v1:0"
		appProfile = "arn:aws:bedrock:ap-southeast-2:123456789012:application-inference-profile/abc123"
		fakeCheap  = "arn:aws:bedrock:ap-southeast-2:123456789012:application-inference-profile/apac.amazon.nova-lite-v1:0"
	)
	usage := &MockTokenResponse{promptTokens: 100, responseTokens: 20}
	llm := &models.LLM{ID: 3, Vendor: models.BEDROCK}
	app := &models.App{ID: 7}

	for _, tc := range []struct {
		name      string
		prices    map[string]float64
		requested string
		wantRate  float64
		wantName  string
	}{
		{"model id priced, called by model id", map[string]float64{modelID: 2}, modelID, 2, modelID},
		{"profile id priced, called by profile ARN (the report's repro)", map[string]float64{profileID: 2}, arn, 2, profileID},
		{"model id priced, called by profile ARN", map[string]float64{modelID: 2}, arn, 2, modelID},
		{"model id priced, called by profile id", map[string]float64{modelID: 2}, profileID, 2, modelID},
		{"model id priced, called by foundation-model ARN", map[string]float64{modelID: 2}, fmARN, 2, modelID},
		{"profile id and model id priced: the profile's price for a call by ARN", map[string]float64{profileID: 3, modelID: 2}, arn, 3, profileID},
		{"the ARN's own price wins", map[string]float64{arn: 5, profileID: 3, modelID: 2}, arn, 5, arn},
		{"the profile id's own price wins over the model's", map[string]float64{profileID: 3, modelID: 2}, profileID, 3, profileID},
		{"application inference profile: unpriced", map[string]float64{modelID: 2, profileID: 3}, appProfile, 0, appProfile},
		{"application inference profile named like a priced model: unpriced", map[string]float64{modelID: 2, profileID: 3}, fakeCheap, 0, fakeCheap},
		{"no price under any name: unpriced", nil, arn, 0, arn},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &priceTable{prices: tc.prices}
			rec := buildChatRecordFor(svc, llm, app, usage, tc.requested, time.Now())
			assert.InDelta(t, models.ChatRecordCost(string(models.BEDROCK), tc.wantRate, tc.wantRate, 0, 0, 100, 20, 0, 0), rec.Cost, 1e-9)
			assert.Equal(t, tc.requested, rec.RequestedModel, "requested_model keeps what the client sent")
			assert.Equal(t, tc.wantName, rec.Name, "name is the model the price was found under")
			if tc.wantRate > 0 {
				assert.Greater(t, rec.Cost, 0.0)
			}
		})
	}
}

// The mapping is Bedrock's: another vendor's model that looks like a profile
// id or an ARN is not priced under a derived name.
func TestBedrockPricing_OtherVendorsAreNotMapped(t *testing.T) {
	svc := &priceTable{prices: map[string]float64{"amazon.nova-lite-v1:0": 2}}
	rec := buildChatRecordFor(svc, &models.LLM{ID: 3, Vendor: models.OPENAI}, &models.App{ID: 7},
		&MockTokenResponse{promptTokens: 100, responseTokens: 20}, "us.amazon.nova-lite-v1:0", time.Now())
	assert.Zero(t, rec.Cost)
	assert.Equal(t, []string{"us.amazon.nova-lite-v1:0"}, svc.lookups)
}

// Through the gateway: a call by profile ARN costs what the same call by
// model id costs, on the OpenAI-shaped bridge (streamed or not) and on the
// native Bedrock path with the ARN escaped as the AWS SDKs send it.
func TestBedrockPricing_ARNCallCostsTheSameAsModelIDCall(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(h *failoverHarness, model, escaped string) (*http.Response, []byte)
	}{
		{"bridge", func(h *failoverHarness, model, _ string) (*http.Response, []byte) {
			return h.post("/ai/primary/v1/chat/completions", `{"model":"`+model+`","messages":[{"role":"user","content":"hi"}]}`)
		}},
		{"bridge, streamed", func(h *failoverHarness, model, _ string) (*http.Response, []byte) {
			return h.post("/ai/primary/v1/chat/completions", `{"stream":true,"model":"`+model+`","messages":[{"role":"user","content":"hi"}]}`)
		}},
		{"native converse", func(h *failoverHarness, _, escaped string) (*http.Response, []byte) {
			return h.postBedrock("/llm/rest/primary/model/"+escaped+"/converse", `{"messages":[{"role":"user","content":[{"text":"hi"}]}]}`)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cost := func(model, escaped string) models.LLMChatRecord {
				h, _ := converseHarness(t, false)
				h.addPrice("anthropic.claude-3", models.BEDROCK, 0.001)
				resp, out := tc.call(h, model, escaped)
				require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
				var recs []models.LLMChatRecord
				require.Eventually(t, func() bool { recs = h.chatRecords(); return len(recs) == 1 }, 5*time.Second, 20*time.Millisecond)
				return recs[0]
			}
			byID := cost("anthropic.claude-3", "anthropic.claude-3")
			byARN := cost(profileARN, sdkEscapedARN)
			require.Greater(t, byID.Cost, 0.0)
			assert.InDelta(t, byID.Cost, byARN.Cost, 1e-9, "the ARN call is charged like the model id call")
			assert.Equal(t, "anthropic.claude-3", byARN.Name)
			assert.Equal(t, profileARN, byARN.RequestedModel)
		})
	}
}
