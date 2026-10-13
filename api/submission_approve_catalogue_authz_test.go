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

// midsommar-clean-2ml.50: approving a submission writes catalogue
// memberships from assigned_catalogues, so it needs the same write
// permission as editing those catalogues directly. A reviewer holding only
// submissions:write is refused, and nothing is approved or published.
func TestApproveSubmission_AssignedCataloguesNeedCatalogueWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := apitest.SetupTestDB(t)
	svc := apitest.SetupTestService(db)
	a := &API{service: svc, config: &auth.Config{}}

	submitter := &models.User{Email: "contrib@x.io", Name: "Contributor"}
	reviewer := &models.User{Email: "reviewer@x.io", Name: "Reviewer"}
	require.NoError(t, db.Create(submitter).Error)
	require.NoError(t, db.Create(reviewer).Error)

	toolCatalogue, err := svc.CreateToolCatalogue("Community tools", "", "", "")
	require.NoError(t, err)

	newToolSubmission := func(name string) *models.Submission {
		sub, err := svc.CreateSubmission(
			submitter.ID,
			models.SubmissionResourceTypeTool,
			models.SubmissionStatusSubmitted,
			models.JSONMap{"name": name, "tool_type": "REST"},
			nil, 0, "public data",
			"contrib@x.io", "", "", nil, "", "",
		)
		require.NoError(t, err)
		return sub
	}

	approve := func(perms authz.Set, submissionID uint, body string) int {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set("user", reviewer)
			authz.WithContext(c, authz.NewContext(reviewer.ID, false, func() (authz.Set, error) { return perms, nil }))
		})
		r.POST("/submissions/:id/approve", a.adminApproveSubmission)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/submissions/%d/approve", submissionID), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w.Code
	}
	withCatalogue := fmt.Sprintf(`{"data":{"attributes":{"final_privacy_score":10,"assigned_catalogues":{"ids":[%d]}}}}`, toolCatalogue.ID)
	withoutCatalogue := `{"data":{"attributes":{"final_privacy_score":10}}}`
	inCatalogue := func(sub *models.Submission) int64 {
		got, err := svc.GetSubmissionByID(sub.ID)
		require.NoError(t, err)
		if got.ResourceID == nil {
			return 0
		}
		var n int64
		require.NoError(t, db.Table("tool_catalogue_tools").
			Where("tool_catalogue_id = ? AND tool_id = ?", toolCatalogue.ID, *got.ResourceID).Count(&n).Error)
		return n
	}

	reviewerOnly := authz.NewSet(authz.Write("submissions"))
	catalogueEditor := authz.NewSet(authz.Write("submissions"), authz.Write("tool-catalogues"))
	wrongType := authz.NewSet(authz.Write("submissions"), authz.Write("data-catalogues"))

	t.Run("submissions:write alone cannot assign catalogues", func(t *testing.T) {
		sub := newToolSubmission("Refused")
		assert.Equal(t, http.StatusForbidden, approve(reviewerOnly, sub.ID, withCatalogue))
		got, err := svc.GetSubmissionByID(sub.ID)
		require.NoError(t, err)
		assert.NotEqual(t, models.SubmissionStatusApproved, got.Status, "nothing is approved")
		assert.Nil(t, got.ResourceID, "no resource is created")
	})

	t.Run("the other catalogue type's write permission does not count", func(t *testing.T) {
		sub := newToolSubmission("Wrong type")
		assert.Equal(t, http.StatusForbidden, approve(wrongType, sub.ID, withCatalogue))
	})

	t.Run("tool-catalogues:write may assign tool catalogues", func(t *testing.T) {
		sub := newToolSubmission("Published")
		assert.Equal(t, http.StatusOK, approve(catalogueEditor, sub.ID, withCatalogue))
		assert.EqualValues(t, 1, inCatalogue(sub))
	})

	t.Run("approving without assigned catalogues needs only submissions:write", func(t *testing.T) {
		sub := newToolSubmission("Plain")
		assert.Equal(t, http.StatusOK, approve(reviewerOnly, sub.ID, withoutCatalogue))
		assert.EqualValues(t, 0, inCatalogue(sub))
	})
}

// The permission each resource type needs to assign catalogues, including
// the fail-closed default for plugin resources (and anything unknown).
func TestCanAssignCatalogues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	can := func(resourceType string, perms ...authz.Permission) bool {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		set := authz.NewSet(perms...)
		authz.WithContext(c, authz.NewContext(7, false, func() (authz.Set, error) { return set, nil }))
		return canAssignCatalogues(c, resourceType)
	}
	toolW, dataW, llmW := authz.Write("tool-catalogues"), authz.Write("data-catalogues"), authz.Write("catalogues")

	assert.True(t, can(models.SubmissionResourceTypeTool, toolW))
	assert.True(t, can(models.SubmissionResourceTypeMCPServer, toolW))
	assert.False(t, can(models.SubmissionResourceTypeTool, dataW, llmW))

	assert.True(t, can(models.SubmissionResourceTypeDatasource, dataW))
	assert.False(t, can(models.SubmissionResourceTypeDatasource, toolW, llmW))

	assert.False(t, can(models.SubmissionResourceTypePlugin, toolW, dataW), "a plugin resource needs every catalogue write")
	assert.True(t, can(models.SubmissionResourceTypePlugin, toolW, dataW, llmW))
	assert.False(t, can("something-new", toolW, dataW))
}
