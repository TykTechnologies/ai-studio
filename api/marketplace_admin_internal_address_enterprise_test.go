//go:build enterprise
// +build enterprise

package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/pkg/marketplace"
)

// Validating a marketplace URL makes AI Studio fetch a URL the caller
// chose. It was open to marketplace:read, which the read-only roles hold, so
// a Viewer or an Auditor could make Studio call addresses inside the
// network. It needs marketplace:write now.
func TestMarketplaceValidate_ReadOnlyRolesAreRefused(t *testing.T) {
	f := setupRBACFixture(t)
	db := f.api.service.DB

	auditor := models.NewUser()
	auditor.Email = "auditor@tyk.io"
	auditor.Name = "Auditor"
	auditor.Password = "hash"
	auditor.EmailVerified = true
	require.NoError(t, auditor.Create(db))
	f.bind(t, "user", auditor.ID, f.roles[models.SystemRoleAuditor])

	body := map[string]interface{}{"url": "http://127.0.0.1:5432/index.yaml"}
	for name, user := range map[string]*models.User{"viewer": f.viewer, "auditor": auditor} {
		w := f.do("POST", "/api/v1/admin/marketplaces/validate", body, user)
		require.Equal(t, http.StatusForbidden, w.Code, "%s: %s", name, w.Body.String())
		assert.Equal(t, authz.CodePermissionDenied, decodeErrorCode(t, w.Body.Bytes()), name)
		assert.Contains(t, w.Body.String(), "marketplace:write", name)
	}
}

// A marketplace on an internal address is refused when it is added, with
// the one fixed error, and nothing is stored.
func TestMarketplaceAdd_RefusesAnInternalAddress(t *testing.T) {
	f := setupRBACFixture(t)
	db := f.api.service.DB

	for _, u := range []string{
		"http://127.0.0.1:8080/index.yaml",
		"http://localhost:8080/index.yaml",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.5/index.yaml",
		"http://[::1]:8080/index.yaml",
		"file:///etc/passwd",
	} {
		w := f.do("POST", "/api/v1/admin/marketplaces", map[string]interface{}{"url": u}, f.owner)
		require.Equal(t, http.StatusBadRequest, w.Code, "%s: %s", u, w.Body.String())
		assert.Contains(t, w.Body.String(), marketplace.ErrFetchFailed.Error(), u)

		var count int64
		require.NoError(t, db.Model(&models.MarketplaceIndex{}).Where("source_url = ?", u).Count(&count).Error)
		assert.Zero(t, count, u)
	}
}
