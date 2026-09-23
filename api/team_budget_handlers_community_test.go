//go:build !enterprise
// +build !enterprise

package api

import (
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTeamBudgetsCommunity_PaymentRequired(t *testing.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	authCfg := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	api := NewAPI(service, true, authService, authCfg, nil, emptyFile, nil)

	admin := models.NewUser()
	admin.Email, admin.Name, admin.Password = "admin@tyk.io", "admin", "hash"
	admin.IsAdmin, admin.EmailVerified = true, true
	require.NoError(t, admin.Create(db))

	for _, r := range []struct{ method, path string }{
		{"GET", "/api/v1/groups/1/budget"},
		{"PUT", "/api/v1/team-budgets/settings"},
		{"GET", "/api/v1/analytics/team-costs"},
		{"DELETE", "/api/v1/groups/1/budget"},
	} {
		body := map[string]interface{}{"enabled": true}
		w := apitest.PerformAuthRequest(api.router, r.method, r.path, body, admin.APIKey)
		assert.Equal(t, http.StatusPaymentRequired, w.Code, "%s %s: %s", r.method, r.path, w.Body.String())
	}

	w := apitest.PerformAuthRequest(api.router, "GET", "/api/v1/team-budgets/settings", nil, admin.APIKey)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"enabled":false}`, w.Body.String())
}
