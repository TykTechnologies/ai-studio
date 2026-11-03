package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An App's owner reports one of its guardrail blocks as a false positive
// from the portal: a proposed exception scoped from the event, softened to
// the softest action the App's capability allows, which the owner cannot
// approve (midsommar-clean-9u1j.7).
func TestGuardrailBlocks_ReportFalsePositive(t *testing.T) {
	db := apitest.SetupTestDB(t)
	require.NoError(t, analytics.Migrate(db))
	service := apitest.SetupTestService(db)
	cfg := apitest.SetupTestAuthConfig(db, service)
	cfg.TestMode = false
	authService := apitest.SetupTestAuthService(db, service)
	authService.Config.TestMode = false
	a := NewAPI(service, true, authService, cfg, nil, emptyFile, nil)

	mkUser := func(email string) *models.User {
		u := models.NewUser()
		u.Email, u.Name, u.Password, u.EmailVerified = email, email, "hash", true
		require.NoError(t, u.Create(db))
		return u
	}
	owner, other := mkUser("owner@tyk.io"), mkUser("other@tyk.io")
	app := &models.App{Name: "Visor", UserID: owner.ID, GuardrailCapability: "read_only"}
	require.NoError(t, app.Create(db))
	unattested := &models.App{Name: "Agent", UserID: owner.ID}
	require.NoError(t, unattested.Create(db))
	filter := &models.Filter{Name: "Wolf", Kind: models.FilterKindGuardrail, Config: models.JSONMap{"provider": "http", "on_detect": "block"}}
	require.NoError(t, db.Create(filter).Error)

	meta := func(extra string) string {
		return fmt.Sprintf(`{"filter_id":%d,"detector":"prompt_attack","action":"block","owner_visible":true,%s}`, filter.ID, extra)
	}
	now := time.Now()
	userText := &models.ComplianceEvent{AppID: app.ID, FilterName: "Wolf", FilterScope: "proxy_request", EventType: "guardrail.prompt_attack",
		Severity: "critical", Metadata: meta(`"segments":[{"index":0,"role":"user","kind":"text","sha256":"ab"}]`), TimeStamp: now, OwnerVisible: true}
	unscanned := &models.ComplianceEvent{AppID: app.ID, FilterName: "Wolf", FilterScope: "proxy_request", EventType: "guardrail.unscanned",
		Severity: "critical", Metadata: meta(`"segments":[{"index":0,"role":"user"}]`), TimeStamp: now, OwnerVisible: true}
	response := &models.ComplianceEvent{AppID: app.ID, FilterName: "Wolf", FilterScope: "proxy_response", EventType: "guardrail.prompt_attack",
		Severity: "critical", Metadata: meta(`"roles":["assistant"]`), TimeStamp: now, OwnerVisible: true}
	toolResult := &models.ComplianceEvent{AppID: unattested.ID, FilterName: "Wolf", FilterScope: "proxy_request", EventType: "guardrail.prompt_attack",
		Severity: "critical", Metadata: meta(`"segments":[{"index":2,"role":"tool","kind":"tool_result","tool":"read"}]`), TimeStamp: now, OwnerVisible: true}
	for _, e := range []*models.ComplianceEvent{userText, unscanned, response, toolResult} {
		require.NoError(t, db.Create(e).Error)
	}

	// The blocks list says which can be reported.
	var list struct {
		Data []GuardrailBlock `json:"data"`
	}
	require.Equal(t, http.StatusOK, getJSON(t, a.router, fmt.Sprintf("/common/apps/%d/guardrail-blocks", app.ID), owner.APIKey, &list))
	reportable := map[uint]bool{}
	for _, b := range list.Data {
		reportable[b.ID] = b.Reportable
	}
	assert.Equal(t, map[uint]bool{userText.ID: true, unscanned.ID: false, response.ID: false}, reportable)

	report := func(appID, eventID uint, key, why string) (int, GuardrailExceptionResponse, string) {
		w := apitest.PerformAuthRequest(a.router, http.MethodPost,
			fmt.Sprintf("/common/apps/%d/guardrail-blocks/%d/report", appID, eventID), map[string]string{"justification": why}, key)
		var resp struct {
			Data GuardrailExceptionResponse `json:"data"`
		}
		if w.Code == http.StatusCreated {
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		}
		return w.Code, resp.Data, w.Body.String()
	}

	code, _, _ := report(app.ID, userText.ID, other.APIKey, "not mine")
	assert.Equal(t, http.StatusForbidden, code, "only the App's owner")
	code, _, _ = report(app.ID, userText.ID, owner.APIKey, "  ")
	assert.Equal(t, http.StatusBadRequest, code, "a reason is required")

	code, e, body := report(app.ID, userText.ID, owner.APIKey, "These are test vectors in a PR")
	require.Equal(t, http.StatusCreated, code, body)
	assert.Equal(t, "proposed", e.Status)
	assert.Equal(t, filter.ID, e.FilterID)
	assert.Equal(t, "prompt_attack", e.Detector)
	assert.Equal(t, "text_user", e.Channel)
	assert.Equal(t, "log", e.Action, "the softest a read-only app allows")
	require.NotNil(t, e.SourceEvent)
	assert.Equal(t, userText.ID, *e.SourceEvent)
	assert.True(t, strings.HasPrefix(e.Justification, "Reported by the app owner: "), e.Justification)
	assert.Equal(t, owner.ID, e.ProposedBy)

	code, _, _ = report(app.ID, userText.ID, owner.APIKey, "again")
	assert.Equal(t, http.StatusConflict, code, "the same scope is already proposed")

	// The owner cannot approve it: approval is the admin API's, for full
	// administrators only.
	w := apitest.PerformAuthRequest(a.router, http.MethodPost,
		fmt.Sprintf("/api/v1/apps/%d/guardrail-exceptions/%d/approve", app.ID, e.ID), map[string]string{"content_hash": e.ContentHash}, owner.APIKey)
	assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, w.Code, w.Body.String())
	var stored models.GuardrailException
	require.NoError(t, db.First(&stored, e.ID).Error)
	assert.Equal(t, models.GuardrailExceptionProposed, stored.Status)

	// Not a detector's verdict on the request, or not this app's: refused.
	code, _, _ = report(app.ID, unscanned.ID, owner.APIKey, "x")
	assert.Equal(t, http.StatusBadRequest, code)
	code, _, _ = report(app.ID, response.ID, owner.APIKey, "x")
	assert.Equal(t, http.StatusBadRequest, code)
	code, _, _ = report(app.ID, toolResult.ID, owner.APIKey, "x")
	assert.Equal(t, http.StatusNotFound, code, "another app's event")

	// An unattested app (acting) cannot soften tool results at all.
	code, _, body = report(unattested.ID, toolResult.ID, owner.APIKey, "fixtures")
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, body, "allows no softening")
}
