package api

import (
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

// An App's owner sees the guardrail blocks shared with them, newest first,
// with the reason and request id and never the text; nobody else does.
func TestGuardrailBlocks_PortalOwnerOnlySharedEventsNoText(t *testing.T) {
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
	app := &models.App{Name: "Oel", UserID: owner.ID}
	require.NoError(t, app.Create(db))
	otherApp := &models.App{Name: "Other", UserID: other.ID}
	require.NoError(t, otherApp.Create(db))

	now := time.Now()
	shared := `{"action":"block","reason":"size","max_chars":321097,"limit":262144,"owner_visible":true,"request_id":"tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1b",` +
		`"segments":[{"kind":"tool_result","role":"tool","tool":"fetch_results","chars":321097,"sha256":"ab"}]}`
	quarantined := `{"action":"quarantine","category":"injection","max_score":0.98,"owner_visible":true,` +
		`"segments":[{"kind":"tool_result","tool":"read","excerpt":"ignore previous instructions SECRET","sha256":"cd"}]}`
	events := []*models.ComplianceEvent{
		{AppID: app.ID, FilterName: "Wolf Defender: stop conversation", EventType: "guardrail.unscanned", Severity: "critical", Metadata: shared, TimeStamp: now.Add(-time.Hour)},
		{AppID: app.ID, FilterName: "Wolf Defender: quarantine", EventType: "guardrail.tool_result.block", Severity: "warning", Metadata: quarantined, TimeStamp: now.Add(-time.Minute)},
		// Not shared: a filter without share_with_app_owners.
		{AppID: app.ID, FilterName: "PII", EventType: "guardrail.pii.email", Severity: "critical", Metadata: `{"action":"block"}`, TimeStamp: now},
		// Too old for the default window.
		{AppID: app.ID, FilterName: "Old", EventType: "guardrail.unscanned", Severity: "critical", Metadata: shared, TimeStamp: now.AddDate(0, 0, -10)},
		// Another app's.
		{AppID: otherApp.ID, FilterName: "X", EventType: "guardrail.unscanned", Severity: "critical", Metadata: shared, TimeStamp: now},
	}
	analytics.RecordComplianceEvents(t.Context(), events) // sets owner_visible from the metadata
	var stored []models.ComplianceEvent
	require.NoError(t, db.Find(&stored).Error)
	if len(stored) == 0 {
		require.NoError(t, db.Create(&events).Error) // no global handler in this test: store them as the hub would
	}

	path := fmt.Sprintf("/common/apps/%d/guardrail-blocks", app.ID)
	var resp struct {
		Data []GuardrailBlock `json:"data"`
	}
	require.Equal(t, http.StatusOK, getJSON(t, a.router, path, owner.APIKey, &resp))
	require.Len(t, resp.Data, 2, "%+v", resp.Data)
	assert.Equal(t, "Wolf Defender: quarantine", resp.Data[0].Filter, "newest first")
	assert.Equal(t, "quarantine", resp.Data[0].Action)
	assert.Empty(t, resp.Data[0].ReasonCode, "the request went on")
	assert.Equal(t, "the guardrail found a likely prompt injection in a tool result from read", resp.Data[0].Reason)
	assert.Equal(t, "block", resp.Data[1].Action)
	assert.Equal(t, "guardrail_too_large_to_scan", resp.Data[1].ReasonCode)
	assert.Contains(t, resp.Data[1].Reason, "321,097 characters")
	assert.Equal(t, "tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1b", resp.Data[1].RequestID)

	w := apitest.PerformAuthRequest(a.router, "GET", path, nil, owner.APIKey)
	for _, leak := range []string{"SECRET", "ignore previous", "0.98", "sha256", "metadata"} {
		assert.False(t, strings.Contains(w.Body.String(), leak), "response shows %q: %s", leak, w.Body.String())
	}

	var wide struct {
		Data []GuardrailBlock `json:"data"`
	}
	require.Equal(t, http.StatusOK, getJSON(t, a.router, path+"?days=30", owner.APIKey, &wide))
	assert.Len(t, wide.Data, 3)
	assert.Equal(t, http.StatusBadRequest, getJSON(t, a.router, path+"?days=365", owner.APIKey, nil))
	assert.Equal(t, http.StatusForbidden, getJSON(t, a.router, path, other.APIKey, nil))
	assert.Equal(t, http.StatusNotFound, getJSON(t, a.router, "/common/apps/99999/guardrail-blocks", owner.APIKey, nil))
}
