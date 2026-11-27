package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services/compliance"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// tkg.2: GET /compliance/events-by-request finds a gateway request's
// compliance events for an agent trace's guardrail outcome. It needs the
// same permission as the events list, and is Enterprise only.
func TestComplianceEventsByRequest(t *testing.T) {
	api, admin, member := setupAuthzAPI(t)
	db := api.service.DB
	require.NoError(t, db.AutoMigrate(&models.ComplianceEvent{}))
	if !compliance.IsEnterpriseAvailable() {
		// The lookup reads the table itself; only the edition gate matters.
		svc := compliance.NewService(db)
		compliance.RegisterEnterpriseFactory(func(*gorm.DB) compliance.Service { return svc })
		t.Cleanup(func() { compliance.RegisterEnterpriseFactory(nil) })
	}

	const id = "tyai_0123456789abcdef0123456789abcdef"
	const two = "tyai_22222222222222222222222222222222"
	at := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	add := func(req string, offset time.Duration) uint {
		e := models.ComplianceEvent{AppID: 7, FilterName: "no secrets", EventType: "guardrail.secrets.aws", Severity: "critical",
			Metadata: `{"action":"block","request_id":"` + req + `"}`, TimeStamp: at.Add(offset)}
		require.NoError(t, db.Create(&e).Error)
		return e.ID
	}
	one := add(id, time.Second)
	add(two, time.Second)
	add(two, 2*time.Second)

	get := func(key, req, app, when string) (int, []compliance.ComplianceEventRecord) {
		q := url.Values{"request_id": {req}, "app_id": {app}, "at": {when}}
		w := apitest.PerformAuthRequest(api.router, http.MethodGet, "/api/v1/compliance/events-by-request?"+q.Encode(), nil, key)
		var body complianceEventsByRequestResponse
		if w.Code == http.StatusOK {
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
		}
		return w.Code, body.Events
	}
	ts := at.Format(time.RFC3339)

	code, events := get(admin.APIKey, id, "7", ts)
	require.Equal(t, http.StatusOK, code)
	require.Len(t, events, 1, "one match: the UI opens ?event=<id>")
	assert.Equal(t, one, events[0].ID)

	code, events = get(admin.APIKey, two, "7", ts)
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, events, 2, "several matches: the UI lists them")

	code, events = get(admin.APIKey, "tyai_00000000000000000000000000000000", "7", ts)
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, events)

	code, _ = get(admin.APIKey, id, "8", ts)
	assert.Equal(t, http.StatusOK, code, "another App's request finds nothing")

	for name, q := range map[string][3]string{
		"invalid id":     {"tyai_<script>", "7", ts},
		"LIKE wildcard":  {"tyai_0123456789abcdef0123456789abcde%", "7", ts},
		"missing app":    {id, "", ts},
		"zero app":       {id, "0", ts},
		"missing window": {id, "7", ""},
		"bad time":       {id, "7", "yesterday"},
	} {
		code, _ := get(admin.APIKey, q[0], q[1], q[2])
		assert.Equal(t, http.StatusBadRequest, code, name)
	}

	code, _ = get(member.APIKey, id, "7", ts)
	assert.Equal(t, http.StatusForbidden, code, "a caller without compliance read")
	w := apitest.PerformRequest(api.router, http.MethodGet, "/api/v1/compliance/events-by-request?request_id="+id+"&app_id=7&at="+url.QueryEscape(ts), nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	e, ok := api.routePerms[routeKey(http.MethodGet, "/api/v1/compliance/events-by-request")]
	require.True(t, ok)
	assert.Equal(t, authz.Read("compliance"), e.perm, "the events list's permission")
}

func TestComplianceEventsByRequest_CommunityEdition(t *testing.T) {
	if compliance.IsEnterpriseAvailable() {
		t.Skip("enterprise build")
	}
	api, admin, _ := setupAuthzAPI(t)
	w := apitest.PerformAuthRequest(api.router, http.MethodGet,
		"/api/v1/compliance/events-by-request?request_id=tyai_0123456789abcdef0123456789abcdef&app_id=7&at=2026-11-27T10:00:00Z", nil, admin.APIKey)
	assert.Equal(t, http.StatusForbidden, w.Code, "Enterprise only, like the other compliance endpoints")
	assert.Contains(t, w.Body.String(), "Enterprise")
}
