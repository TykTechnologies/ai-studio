package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
)

// An agent stored with workspace_id 0 (not yet backfilled) is reported as
// the Default's id on the admin API, as every other stamped object is (.52),
// so the UI never reads it as another Workspace or a move.
func TestRedactAgent_ReportsStoredZeroAsDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	a := &API{service: service}
	def, err := service.DefaultWorkspaceID()
	require.NoError(t, err)
	require.NotZero(t, def)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/agents/1", nil)
	c.Set(authzGatedKey, true)
	authz.WithContext(c, authz.NewContext(1, false, func() (authz.Set, error) { return authz.NewSet(authz.Read("agents")), nil }))

	ag := &models.AgentConfig{WorkspaceID: 0}
	a.redactAgent(c, ag)
	assert.Equal(t, def, ag.WorkspaceID)

	other := &models.AgentConfig{WorkspaceID: def + 41}
	a.redactAgent(c, other)
	assert.Equal(t, def+41, other.WorkspaceID, "a set Workspace is reported as stored")
}
