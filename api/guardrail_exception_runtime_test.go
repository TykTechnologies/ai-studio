package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flagEvil flags "evil" in any segment as a prompt attack.
type flagEvil struct{}

func (flagEvil) Capabilities() guardrails.Capabilities { return guardrails.Capabilities{} }

func (flagEvil) Classify(_ context.Context, in guardrails.Input) (guardrails.Verdict, error) {
	var v guardrails.Verdict
	for _, s := range in.Segments {
		if strings.Contains(s.Text, "evil") {
			v.Findings = append(v.Findings, guardrails.Finding{Detector: "prompt_attack", Category: "injection", Score: 1, Severity: "warning", SegmentIndex: s.Index})
		}
	}
	v.Flagged = len(v.Findings) > 0
	return v, nil
}

// An approved exception, served by Studio's source, softens the verdict on
// the hub's runtime and its hits show on the API.
func TestGuardrailException_RuntimeHitsVisibleOnAPI(t *testing.T) {
	api, db := setupTestAPI(t)
	guardrails.RegisterSpec(guardrails.Spec{Name: "flag_evil_api", DisplayName: "flag evil", Detectors: []guardrails.DetectorSpec{{Name: "prompt_attack", Category: "injection"}}})
	guardrails.RegisterFactory("flag_evil_api", func(guardrails.Config) (guardrails.Provider, error) { return flagEvil{}, nil })

	filter := &models.Filter{Name: "Wolf", Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider": "flag_evil_api", "detectors": []any{map[string]any{"name": "prompt_attack"}}, "on_detect": "block", "cache": false,
	}}
	require.NoError(t, db.Create(filter).Error)
	app := &models.App{Name: "Visor", UserID: 1, GuardrailCapability: "read_only"}
	require.NoError(t, app.Create(db))
	e, err := api.service.CreateGuardrailException(app.ID, services.GuardrailExceptionActor{UserID: 1, FullAdmin: true}, services.GuardrailExceptionSpec{
		FilterID: &filter.ID, Detector: ptrTo("prompt_attack"), Channel: ptrTo("text_user"), Action: ptrTo("log"), Justification: ptrTo("PR test vectors"),
	})
	require.NoError(t, err)

	scripting.SetExceptionSource(api.service.GuardrailExceptionSource())
	t.Cleanup(func() { scripting.SetExceptionSource(nil) })

	body := `{"model":"m","messages":[{"role":"user","content":"{\"role\":\"user\",\"content\":\"ok\",\"content\":\"evil\"}"}]}`
	run := func(appID uint) *scripting.ScriptOutput {
		out, err := scripting.NewFilterRunner(filter).RunScript(&scripting.ScriptInput{
			RawInput: body, Format: "openai", VendorName: "openai", Context: map[string]interface{}{"app_id": appID},
		}, nil)
		require.NoError(t, err)
		return out
	}
	out := run(app.ID)
	assert.False(t, out.Block, "softened to log")
	require.NotEmpty(t, out.ComplianceEvents)
	assert.Equal(t, []uint{e.ID}, out.ComplianceEvents[0].Metadata["exceptions"])
	assert.True(t, run(app.ID+1).Block, "another app is not softened")

	item := fmt.Sprintf("/api/v1/apps/%d/guardrail-exceptions/%d", app.ID, e.ID)
	assert.Eventually(t, func() bool {
		w := performRequest(api.router, http.MethodGet, item, nil)
		var resp struct {
			Data GuardrailExceptionResponse `json:"data"`
		}
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &resp) != nil {
			return false
		}
		return resp.Data.HitCount == 1 && resp.Data.LastHitAt != nil
	}, 2*time.Second, 20*time.Millisecond)
}

func ptrTo[T any](v T) *T { return &v }
