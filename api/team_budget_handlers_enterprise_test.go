//go:build enterprise
// +build enterprise

package api

import (
	"net/http"
	"testing"
	"time"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/team_budget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/budget"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/team_budget"
)

func TestTeamBudgetsEnterprise_API(t *testing.T) {
	db := sharedMemoryDB(t)
	service := apitest.SetupTestService(db)
	authCfg := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	api := NewAPI(service, true, authService, authCfg, nil, emptyFile, nil)
	router := api.router

	admin := models.NewUser()
	admin.Email, admin.Name, admin.Password = "admin@tyk.io", "admin", "hash"
	admin.IsAdmin, admin.EmailVerified = true, true
	require.NoError(t, admin.Create(db))

	eng := &models.Group{Name: "Engineering"}
	ops := &models.Group{Name: "Ops"}
	require.NoError(t, db.Create(eng).Error)
	require.NoError(t, db.Create(ops).Error)
	dev := &models.User{Email: "dev@tyk.io", Name: "dev", EmailVerified: true}
	require.NoError(t, db.Create(dev).Error)
	require.NoError(t, db.Model(eng).Association("Users").Append(dev))
	require.NoError(t, db.Model(ops).Association("Users").Append(dev))

	do := func(method, path string, body interface{}) (int, map[string]interface{}) {
		w := apitest.PerformAuthRequest(router, method, path, body, admin.APIKey)
		var out map[string]interface{}
		if w.Body.Len() > 0 {
			decodeWebhookJSON(t, w, &out)
		}
		return w.Code, out
	}

	// Switch on; the Default team becomes an empty pool.
	code, out := do("PUT", "/api/v1/team-budgets/settings", map[string]interface{}{"enabled": true})
	require.Equal(t, http.StatusOK, code, out)
	assert.Equal(t, true, out["enabled"])

	// Budget validation.
	code, _ = do("PUT", "/api/v1/groups/"+itoa(eng.ID)+"/budget", map[string]interface{}{"monthly_budget": 10, "enforcement": "nope"})
	assert.Equal(t, http.StatusBadRequest, code)
	code, _ = do("PUT", "/api/v1/groups/9999/budget", map[string]interface{}{"monthly_budget": 10})
	assert.Equal(t, http.StatusNotFound, code)

	code, out = do("PUT", "/api/v1/groups/"+itoa(eng.ID)+"/budget", map[string]interface{}{
		"monthly_budget": 10, "default_app_allocation": 4, "enforcement": "hard_block",
	})
	require.Equal(t, http.StatusOK, code, out)
	assert.Equal(t, true, out["managed"])

	// The dev's budget team is Ops; switch it to Engineering.
	code, _ = do("PUT", "/api/v1/users/"+itoa(dev.ID)+"/budget-team", map[string]interface{}{"team_id": eng.ID})
	require.Equal(t, http.StatusNoContent, code)
	code, _ = do("PUT", "/api/v1/users/"+itoa(dev.ID)+"/budget-team", map[string]interface{}{"team_id": 9999})
	assert.Equal(t, http.StatusBadRequest, code)

	createApp := func(name string, budget interface{}, teamID interface{}) (int, map[string]interface{}) {
		attrs := map[string]interface{}{"name": name, "user_id": dev.ID, "monthly_budget": budget}
		if teamID != nil {
			attrs["team_id"] = teamID
		}
		return do("POST", "/api/v1/apps", map[string]interface{}{"data": map[string]interface{}{"type": "apps", "attributes": attrs}})
	}

	code, out = createApp("a1", nil, nil)
	require.Equal(t, http.StatusCreated, code, out)
	attrs := out["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	assert.Equal(t, float64(eng.ID), attrs["team_id"])
	assert.Equal(t, 4.0, attrs["monthly_budget"])

	code, out = createApp("too-big", 7, nil)
	assert.Equal(t, http.StatusBadRequest, code, out)

	// An explicit 0 is kept as a zero budget.
	code, out = createApp("zero", 0, nil)
	require.Equal(t, http.StatusCreated, code, out)
	assert.Equal(t, 0.0, out["data"].(map[string]interface{})["attributes"].(map[string]interface{})["monthly_budget"])

	// Explicit team, empty Ops pool (unmanaged: no allocation).
	code, out = createApp("ops-app", nil, ops.ID)
	require.Equal(t, http.StatusCreated, code, out)
	attrs = out["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	assert.Equal(t, float64(ops.ID), attrs["team_id"])
	assert.Nil(t, attrs["monthly_budget"], "unmanaged team: no limit")

	code, out = do("GET", "/api/v1/groups/"+itoa(eng.ID)+"/budget", nil)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, 4.0, out["allocated"])
	assert.Equal(t, 6.0, out["unallocated"])

	code, _ = do("POST", "/api/v1/groups/"+itoa(eng.ID)+"/budget/reset", nil)
	assert.Equal(t, http.StatusNoContent, code)

	day := time.Now().UTC().Format("2006-01-02")
	code, out = do("GET", "/api/v1/analytics/team-costs?start_date="+day+"&end_date="+day, nil)
	require.Equal(t, http.StatusOK, code, out)
	assert.NotEmpty(t, out["teams"])
	code, _ = do("GET", "/api/v1/analytics/team-costs?start_date=yesterday", nil)
	assert.Equal(t, http.StatusBadRequest, code)
	code, _ = do("GET", "/api/v1/analytics/team-costs?start_date=2024-01-01&end_date=2026-01-01", nil)
	assert.Equal(t, http.StatusBadRequest, code, "at most 366 days")
	code, _ = do("GET", "/api/v1/analytics/team-costs?start_date=2026-02-01&end_date=2026-01-01", nil)
	assert.Equal(t, http.StatusBadRequest, code, "end before start")

	code, _ = do("DELETE", "/api/v1/groups/"+itoa(eng.ID)+"/budget", nil)
	assert.Equal(t, http.StatusNoContent, code)
	tb, err := service.TeamBudget.GetTeamBudget(eng.ID)
	require.NoError(t, err)
	assert.Nil(t, tb)
	assert.True(t, team_budget.IsEnterpriseAvailable())
}
