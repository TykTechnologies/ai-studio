package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services/group_access"
)

// Workspaces .53: the admin nav counts Workspace roles, as the route gate
// does. A user whose only roles are scoped to one Workspace gets the admin
// surface and the entries for the Workspace resources they hold there;
// entries for global resources stay hidden.
func TestNavManifest_WorkspaceScopedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	service.GroupAccessService = group_access.NewService(db)
	cfg := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	a := NewAPI(service, true, authService, cfg, nil, emptyFile, nil)

	u := models.NewUser()
	u.Email, u.Name, u.Password, u.EmailVerified = "scoped@nav.test", "Scoped", "hash", true
	require.NoError(t, u.Create(db))

	manifest := func(grants authz.Grants) NavManifest {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/common/nav", nil)
		c.Set("user", u)
		authz.WithContext(c, authz.NewGrantsContext(u.ID, false, func() (authz.Grants, error) { return grants, nil }))
		a.getNavManifest(c)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var m NavManifest
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m))
		return m
	}

	scoped := manifest(authz.Grants{
		Global: authz.NewSet(),
		Workspaces: map[uint]authz.Set{2: authz.NewSet(
			authz.Read("llms"), authz.Write("llms"), authz.Read("tools"), authz.Read("datasources"),
			// A global permission in a Workspace set grants nothing (D2).
			authz.Read("users"),
		)},
	})
	assert.Contains(t, labels(scoped.Surfaces), "Admin", "a Workspace role shows the admin surface")
	for _, id := range []string{"llms", "tools", "datasources"} {
		assert.NotNil(t, find(scoped.Admin, id), "entry %s", id)
	}
	assert.Nil(t, find(scoped.Admin, "users"), "a global-only entry stays hidden")
	assert.Nil(t, find(scoped.Admin, "filters"), "an entry for a permission held nowhere stays hidden")

	none := manifest(authz.Grants{Global: authz.NewSet()})
	assert.NotContains(t, labels(none.Surfaces), "Admin")
	assert.Empty(t, none.Admin)

	global := manifest(authz.Grants{Global: authz.NewSet(authz.Read("users"))})
	assert.NotNil(t, find(global.Admin, "users"), "a global role still shows global entries")
}
