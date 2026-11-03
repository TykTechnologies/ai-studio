package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
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

// The Soften journey end to end on the hub, as the Filter Events dialog
// drives it (midsommar-clean-9u1j.7): the filter blocks "evil" for an app and
// records a detector event naming the filter; an exception is created from
// that event (its filter, detector and channel, source_event the event) and
// approved; the next request passes, logged and tagged with the exception.
func TestGuardrailException_SoftenFromEventJourney(t *testing.T) {
	api, db := setupTestAPI(t)
	require.NoError(t, analytics.Migrate(db))
	guardrails.RegisterSpec(guardrails.Spec{Name: "flag_evil_journey", DisplayName: "flag evil", Detectors: []guardrails.DetectorSpec{{Name: "prompt_attack", Category: "injection"}}})
	guardrails.RegisterFactory("flag_evil_journey", func(guardrails.Config) (guardrails.Provider, error) { return flagEvil{}, nil })
	filter := &models.Filter{Name: "Wolf", Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider": "flag_evil_journey", "detectors": []any{map[string]any{"name": "prompt_attack"}}, "on_detect": "block", "cache": false,
	}}
	require.NoError(t, db.Create(filter).Error)
	app := &models.App{Name: "Visor", UserID: 1, GuardrailCapability: "read_only"}
	require.NoError(t, app.Create(db))
	scripting.SetExceptionSource(api.service.GuardrailExceptionSource())
	t.Cleanup(func() { scripting.SetExceptionSource(nil) })

	body := `{"model":"m","messages":[{"role":"user","content":"review: evil"}]}`
	run := func() *scripting.ScriptOutput {
		out, err := scripting.NewFilterRunner(filter).RunScript(&scripting.ScriptInput{
			RawInput: body, Format: "openai", VendorName: "openai", Context: map[string]interface{}{"app_id": app.ID},
		}, nil)
		require.NoError(t, err)
		return out
	}

	// 1. Blocked, and the event names the filter (as the hub stores it).
	out := run()
	require.True(t, out.Block)
	require.NotEmpty(t, out.ComplianceEvents)
	first := out.ComplianceEvents[0]
	metaJSON, err := json.Marshal(first.Metadata)
	require.NoError(t, err)
	ev := &models.ComplianceEvent{AppID: app.ID, FilterName: filter.Name, FilterScope: "proxy_request", EventType: first.EventType,
		Severity: first.Severity, Metadata: string(metaJSON), TimeStamp: time.Now()}
	require.NoError(t, db.Create(ev).Error)

	// 2. The dialog's request: scope from the event's metadata.
	var meta map[string]any
	require.NoError(t, json.Unmarshal(metaJSON, &meta))
	segs := meta["segments"].([]any)
	seg := segs[0].(map[string]any)
	require.Equal(t, "user", seg["role"])
	base := fmt.Sprintf("/api/v1/apps/%d/guardrail-exceptions", app.ID)
	w := performRequest(api.router, http.MethodPost, base, map[string]any{
		"filter_id": meta["filter_id"], "detector": meta["detector"], "channel": "text_user", "action": "log",
		"justification": "PR content quotes injection test vectors", "expires_after_days": 30, "source_event": ev.ID,
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct {
		Data GuardrailExceptionResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotNil(t, created.Data.SourceEvent)
	assert.Equal(t, ev.ID, *created.Data.SourceEvent)
	// The test API has no user, so nobody is a full administrator: approve it.
	w = performRequest(api.router, http.MethodPost, fmt.Sprintf("%s/%d/approve", base, created.Data.ID), map[string]string{"content_hash": created.Data.ContentHash})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// 3. The next request passes, logged and tagged.
	out = run()
	assert.False(t, out.Block, "softened to log")
	require.NotEmpty(t, out.ComplianceEvents)
	assert.Equal(t, []uint{created.Data.ID}, out.ComplianceEvents[0].Metadata["exceptions"])
	assert.Equal(t, guardrails.ActionLog, out.ComplianceEvents[0].Metadata["action"])
	assert.Contains(t, out.ComplianceEvents[0].Description, fmt.Sprintf("softened by exception #%d", created.Data.ID))
}
