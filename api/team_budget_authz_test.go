package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

// PUT /users/:id/budget-team only accepts one of the user's own teams: a
// real team the user is not in is refused and nothing is stored.
func TestSetUserBudgetTeam_RequiresMembership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := apitest.SetupTestDB(t)
	svc := apitest.SetupTestService(db)
	a := &API{service: svc, config: &auth.Config{}}
	r := gin.New()
	r.PUT("/users/:id/budget-team", a.setUserBudgetTeam)

	eng := &models.Group{Name: "Engineering"}
	ops := &models.Group{Name: "Ops"}
	require.NoError(t, db.Create(eng).Error)
	require.NoError(t, db.Create(ops).Error)
	user := &models.User{Email: "u@x.io"}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Model(eng).Association("Users").Append(user))

	put := func(body string) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/users/%d/budget-team", user.ID), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w.Code
	}
	budgetTeam := func() *uint {
		var u models.User
		require.NoError(t, db.First(&u, user.ID).Error)
		return u.BudgetTeamID
	}

	assert.Equal(t, http.StatusBadRequest, put(fmt.Sprintf(`{"team_id": %d}`, ops.ID)), "an existing team the user is not in")
	assert.Nil(t, budgetTeam(), "nothing stored")

	assert.Equal(t, http.StatusNoContent, put(fmt.Sprintf(`{"team_id": %d}`, eng.ID)))
	require.NotNil(t, budgetTeam())
	assert.Equal(t, eng.ID, *budgetTeam())

	assert.Equal(t, http.StatusBadRequest, put(fmt.Sprintf(`{"team_id": %d}`, ops.ID)))
	assert.Equal(t, eng.ID, *budgetTeam(), "a refused change keeps the previous team")

	assert.Equal(t, http.StatusNoContent, put(`{"team_id": null}`))
	assert.Nil(t, budgetTeam(), "clearing is always allowed")
}
