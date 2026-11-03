package api

import (
	"context"
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting/llmclient"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// judgeOnlySvc gives the judge its LLM and the app; the capability comes
// from the real source under test.
type judgeOnlySvc struct {
	services.ServiceInterface
	app *models.App
}

func (s *judgeOnlySvc) CheckLLMBudget(*models.LLM) error { return nil }
func (s *judgeOnlySvc) GetLLMByID(id uint) (*models.LLM, error) {
	// 70 is the judge; any other is the LLM the traffic went to, scored
	// below it (the judge may not read text above its own privacy score).
	l := &models.LLM{Name: "judge", Vendor: models.OPENAI, Active: true, DefaultModel: "judge-mini", PrivacyScore: 80}
	if id != 70 {
		l.Name, l.PrivacyScore = "gpt", 50
	}
	l.ID = id
	return l, nil
}
func (s *judgeOnlySvc) GetAppByID(uint) (*models.App, error) { return s.app, nil }
func (s *judgeOnlySvc) GetModelPriceByModelNameAndVendor(string, string) (*models.ModelPrice, error) {
	return nil, errors.New("no price")
}

// .58 on the hub: a read_only app with no exceptions, judged benign, is
// softened to the read_only floor (log) through Studio's capability source;
// after its attestation is reset it is acting (quarantine).
func TestJudgeSoften_HubCapabilityWithoutExceptions(t *testing.T) {
	api, db := setupTestAPI(t)
	guardrails.RegisterSpec(guardrails.Spec{Name: "flag_evil_judge", DisplayName: "flag evil", Detectors: []guardrails.DetectorSpec{{Name: "prompt_attack", Category: "injection"}}})
	guardrails.RegisterFactory("flag_evil_judge", func(guardrails.Config) (guardrails.Provider, error) { return flagEvil{}, nil })
	filter := &models.Filter{Name: "Wolf judged", Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider": "flag_evil_judge", "detectors": []any{map[string]any{"name": "prompt_attack"}}, "on_detect": "block", "cache": false,
		"judge": map[string]any{"mode": "soften", "llm_id": 70, "timeout_ms": 2000},
	}}
	require.NoError(t, db.Create(filter).Error)
	app := &models.App{Name: "Reviewer", Description: "Reviews pull requests", UserID: 1}
	require.NoError(t, app.Create(db))
	_, err := api.service.SetAppGuardrailCapability(app.ID, "read_only", 1)
	require.NoError(t, err)

	scripting.SetExceptionSource(api.service.GuardrailExceptionSource())
	scripting.SetCapabilitySource(api.service.GuardrailExceptionSource())
	t.Cleanup(func() { scripting.SetExceptionSource(nil); scripting.SetCapabilitySource(nil) })
	t.Cleanup(scripting.SetJudgeCallForTesting(func(_ context.Context, _ *models.LLM, model, _, _ string) (llmclient.Completion, error) {
		return llmclient.Completion{Text: `{"verdict":"benign","reason":"code_or_data"}`, Model: model}, nil
	}))

	run := func() string {
		t.Helper()
		out, err := scripting.NewFilterRunner(filter).RunScript(&scripting.ScriptInput{
			RawInput: `{"model":"m","messages":[{"role":"user","content":"test vector: evil"}]}`, Format: "openai", VendorName: "openai",
			Context: map[string]interface{}{"app_id": app.ID, "llm_id": int64(5)},
		}, &judgeOnlySvc{app: app})
		require.NoError(t, err)
		for _, e := range out.ComplianceEvents {
			if e.EventType == "guardrail.prompt_attack" {
				a, _ := e.Metadata["action"].(string)
				return a
			}
		}
		t.Fatalf("no detector event: %+v", out.ComplianceEvents)
		return ""
	}
	assert.Equal(t, guardrails.ActionLog, run(), "read_only with no exceptions softens to log")

	_, err = api.service.SetAppGuardrailCapability(app.ID, "", 1)
	require.NoError(t, err)
	assert.Equal(t, guardrails.ActionQuarantine, run(), "after a reset, the acting floor")
}
