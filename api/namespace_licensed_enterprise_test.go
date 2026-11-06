//go:build enterprise

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/entitlements"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// licensedOnlyScope licenses the default namespace and team-a, as SQL.
type licensedOnlyScope struct{ entitlements.Service }

func (licensedOnlyScope) LicensedNamespaceScope(q *gorm.DB) *gorm.DB {
	return q.Where("namespace IN ?", append(models.NamespaceAliases(models.DefaultNamespace), "team-a"))
}

func (licensedOnlyScope) IsNamespaceLicensed(*gorm.DB, string) bool {
	panic("the namespace list must not check namespaces one by one")
}

// The namespace list marks a namespace whose edges are not licensed, from
// one query, and a namespace without edges as licensed (nothing to gate).
func TestListNamespacesMarksUnlicensed(t *testing.T) {
	p := setupPushTestAPI(t)
	p.api.service.Entitlements = licensedOnlyScope{Service: entitlements.NewCommunityService()}
	p.edge(t, "e-default", "default", models.EdgeStatusConnected)
	p.edge(t, "e-a", "team-a", models.EdgeStatusConnected)
	p.edge(t, "e-b", "team-b", models.EdgeStatusConnected)
	require.NoError(t, p.api.service.DB.Create(&models.App{Name: "only-apps", Namespace: "team-apps"}).Error)
	p.router.GET("/api/v1/namespaces", p.api.listNamespaces)

	w := httptest.NewRecorder()
	p.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/namespaces", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body NamespaceListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	got := map[string]bool{}
	for _, ns := range body.Data {
		got[ns.Attributes.Name] = ns.Attributes.Licensed
	}
	assert.True(t, got["team-a"])
	assert.False(t, got["team-b"])
	for name, licensed := range got {
		if name != "team-b" {
			assert.True(t, licensed, name)
		}
	}
}
