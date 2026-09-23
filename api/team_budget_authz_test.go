package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An explicit team_id moves an App's spend (and allocation) into a team, so
// a team the owner is not in needs groups:write.
func TestAuthorizeAppTeam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := apitest.SetupTestDB(t)
	svc := apitest.SetupTestService(db)
	a := &API{service: svc, config: &auth.Config{}}

	eng := &models.Group{Name: "Engineering"}
	ops := &models.Group{Name: "Ops"}
	require.NoError(t, db.Create(eng).Error)
	require.NoError(t, db.Create(ops).Error)
	owner := &models.User{Email: "o@x.io"}
	require.NoError(t, db.Create(owner).Error)
	require.NoError(t, db.Model(eng).Association("Users").Append(owner))

	run := func(perms authz.Set, teamID *uint) (bool, int) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		authz.WithContext(c, authz.NewContext(99, false, func() (authz.Set, error) { return perms, nil }))
		ok := a.authorizeAppTeam(c, owner.ID, teamID)
		return ok, w.Code
	}
	editor := authz.NewSet(authz.Write("apps"))
	teamAdmin := authz.NewSet(authz.Write("apps"), authz.Write("groups"))

	ok, _ := run(editor, nil)
	assert.True(t, ok, "no team_id: nothing to check")
	ok, _ = run(editor, &eng.ID)
	assert.True(t, ok, "one of the owner's teams")
	ok, code := run(editor, &ops.ID)
	assert.False(t, ok, "a team the owner is not in")
	assert.Equal(t, http.StatusForbidden, code)
	ok, _ = run(teamAdmin, &ops.ID)
	assert.True(t, ok, "groups:write may assign any team")
}
