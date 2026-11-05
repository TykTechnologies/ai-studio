//go:build !enterprise

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/entitlements"
	"github.com/TykTechnologies/midsommar/v2/services/group_access"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// reportingEntitlements answers the License page calls with fixed data.
type reportingEntitlements struct {
	entitlements.Service
	report     *entitlements.Report
	releaseErr error
	released   []string
}

func (r *reportingEntitlements) Report(*gorm.DB) (*entitlements.Report, error) { return r.report, nil }
func (r *reportingEntitlements) ReleaseNamespace(_ *gorm.DB, ns string) error {
	r.released = append(r.released, ns)
	return r.releaseErr
}

// The License page is the Owner's alone: in Community Edition that is the
// Super Admin. Others get 403, no nav entry and is_owner false; CE itself
// has no licence to report (402).
func TestLicenseEndpointsAreOwnerOnly(t *testing.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	service.GroupAccessService = group_access.NewService(db)
	cfg := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	r := NewAPI(service, true, authService, cfg, nil, emptyFile, nil).router

	mkAdmin := func(email string) (*models.User, string) {
		u := models.NewUser()
		u.Email = email
		u.Name = email
		u.Password = "hash"
		u.IsAdmin = true
		u.EmailVerified = true
		require.NoError(t, u.Create(db))
		return u, u.APIKey
	}
	owner, ownerKey := mkAdmin("owner@licence.test")
	require.Equal(t, models.SuperAdminID, owner.ID)
	_, adminKey := mkAdmin("admin@licence.test")

	w := apitest.PerformAuthRequest(r, "GET", "/api/v1/license", nil, ownerKey)
	assert.Equal(t, http.StatusPaymentRequired, w.Code, w.Body.String())

	for _, req := range [][2]string{{"GET", "/api/v1/license"}, {"DELETE", "/api/v1/license/namespaces/team-a"}} {
		w = apitest.PerformAuthRequest(r, req[0], req[1], nil, adminKey)
		assert.Equal(t, http.StatusForbidden, w.Code, "%s %s: %s", req[0], req[1], w.Body.String())
	}

	ent := &reportingEntitlements{
		Service: entitlements.NewCommunityService(),
		report: &entitlements.Report{Valid: true, SKUs: []string{"sku"}, Limits: []entitlements.Limit{{Key: "apps", Label: "Apps", Used: 1, Max: 2}}},
	}
	service.Entitlements = ent
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/license", nil, ownerKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Data entitlements.Report `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, *ent.report, body.Data)

	w = apitest.PerformAuthRequest(r, "DELETE", "/api/v1/license/namespaces/team-a", nil, ownerKey)
	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	ent.releaseErr = helpers.NewConflictError(`namespace "team-a" still has 1 connected edge gateway(s)`)
	w = apitest.PerformAuthRequest(r, "DELETE", "/api/v1/license/namespaces/team-a", nil, ownerKey)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "still has 1 connected edge gateway")
	assert.Equal(t, []string{"team-a", "team-a"}, ent.released)

	me := func(key string) bool {
		w := apitest.PerformAuthRequest(r, "GET", "/common/me?skip_entitlements=true", nil, key)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var m UserWithEntitlementsResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m))
		return m.Attributes.IsOwner
	}
	assert.True(t, me(ownerKey))
	assert.False(t, me(adminKey))

	nav := func(key string) NavManifest {
		w := apitest.PerformAuthRequest(r, "GET", "/common/nav", nil, key)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var m NavManifest
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m))
		return m
	}
	assert.NotNil(t, find(nav(ownerKey).Admin, "license"))
	assert.Nil(t, find(nav(adminKey).Admin, "license"))
}
