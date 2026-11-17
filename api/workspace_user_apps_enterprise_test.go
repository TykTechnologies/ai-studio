//go:build enterprise
// +build enterprise

package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GET /users/:id/apps and /users/:id/apps/count (working since #1015) are
// Workspace-scoped like the App list: a Workspace-scoped reader gets only
// the user's Apps in its Workspaces, and the count matches; a global reader
// gets them all.
func TestWorkspaceB4c_UserApps(t *testing.T) {
	f := setupWSB4(t)
	user := f.owner.ID
	other := f.inW1.ID

	f.created(t, f.do("POST", "/api/v1/apps", body(appAttrs("User W1 a", user, nil)), f.inW1))
	f.created(t, f.do("POST", "/api/v1/apps", body(appAttrs("User W1 b", user, nil)), f.inW1))
	f.created(t, f.do("POST", "/api/v1/apps", body(appAttrs("User W2", user, map[string]any{"workspace_id": f.w2})), f.owner))
	f.created(t, f.do("POST", "/api/v1/apps", body(appAttrs("Other W1", other, nil)), f.inW1))

	list := fmt.Sprintf("/api/v1/users/%d/apps", user)
	cnt := fmt.Sprintf("/api/v1/users/%d/apps/count", user)

	assert.ElementsMatch(t, []string{"User W1 a", "User W1 b"}, appNames(t, f.do("GET", list, nil, f.inW1)),
		"a W1 reader sees only the user's W1 Apps")
	w := f.do("GET", cnt, nil, f.inW1)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"count":2}`, w.Body.String(), "the count matches the list")

	assert.ElementsMatch(t, []string{"User W1 a", "User W1 b", "User W2"}, appNames(t, f.do("GET", list, nil, f.owner)),
		"a global reader sees all of the user's Apps")
	w = f.do("GET", cnt, nil, f.owner)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"count":3}`, w.Body.String())
}
