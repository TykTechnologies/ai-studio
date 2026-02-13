package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// --- Admin orphan management ---

// adminGetOrphanedResources handles GET /api/v1/submissions/orphaned
func (a *API) adminGetOrphanedResources(c *gin.Context) {
	orphanedDS, orphanedTools, err := a.service.GetOrphanedCommunityResources()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"datasources": orphanedDS,
			"tools":       orphanedTools,
		},
	})
}

// --- Validation endpoints ---

// validateOASSpec handles POST /common/submissions/validate-spec
func (a *API) validateOASSpec(c *gin.Context) {
	var input struct {
		OASSpec string `json:"oas_spec"` // base64-encoded
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	if input.OASSpec == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "oas_spec is required"}},
		})
		return
	}

	result, err := a.service.ValidateOASSpec(input.OASSpec)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// testDatasourceConnectivity handles POST /common/submissions/test-datasource
func (a *API) testDatasourceConnectivity(c *gin.Context) {
	var input struct {
		EmbedVendor string `json:"embed_vendor"`
		EmbedURL    string `json:"embed_url"`
		EmbedAPIKey string `json:"embed_api_key"`
		EmbedModel  string `json:"embed_model"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	if input.EmbedVendor == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "embed_vendor is required"}},
		})
		return
	}

	result, err := a.service.TestDatasourceConnectivity(input.EmbedVendor, input.EmbedURL, input.EmbedAPIKey, input.EmbedModel)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// adminTestSubmission handles POST /api/v1/submissions/:id/test
// Runs validation appropriate to the submission's resource type
func (a *API) adminTestSubmission(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "invalid submission ID"}},
		})
		return
	}

	submission, err := a.service.GetSubmissionByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "submission not found"}},
		})
		return
	}

	payload := submission.ResourcePayload
	getString := func(key string) string {
		if v, ok := payload[key]; ok {
			if str, ok := v.(string); ok {
				return str
			}
		}
		return ""
	}

	switch submission.ResourceType {
	case "tool":
		oasSpec := getString("oas_spec")
		if oasSpec == "" {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "submission payload missing oas_spec"}},
			})
			return
		}
		result, err := a.service.ValidateOASSpec(oasSpec)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Internal Server Error", Detail: err.Error()}},
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"type": "tool", "spec_validation": result}})

	case "datasource":
		dsResult, err := a.service.TestDatasourceConnectivity(
			getString("embed_vendor"),
			getString("embed_url"),
			getString("embed_api_key"),
			getString("embed_model"),
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Internal Server Error", Detail: err.Error()}},
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"type": "datasource", "connectivity": dsResult}})

	default:
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "unsupported resource type"}},
		})
	}
}

// --- Activity/audit trail endpoints ---

// adminGetSubmissionActivities handles GET /api/v1/submissions/:id/activities
func (a *API) adminGetSubmissionActivities(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "invalid submission ID"}},
		})
		return
	}

	activities, err := a.service.GetSubmissionActivities(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": activities})
}

// getMySubmissionActivities handles GET /common/submissions/:id/activities (portal users — hides internal notes)
func (a *API) getMySubmissionActivities(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	currentUser := user.(*models.User)

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "invalid submission ID"}},
		})
		return
	}

	// Verify ownership
	submission, err := a.service.GetSubmissionByID(uint(id))
	if err != nil || submission.SubmitterID != currentUser.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Forbidden", Detail: "not authorized"}},
		})
		return
	}

	activities, err := a.service.GetSubmissionActivities(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	// Strip internal notes for portal users
	sanitized := make([]gin.H, len(activities))
	for i, a := range activities {
		sanitized[i] = gin.H{
			"id":            a.ID,
			"activity_type": a.ActivityType,
			"actor_name":    a.ActorName,
			"feedback":      a.Feedback,
			"created_at":    a.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": sanitized})
}

// --- Admin submission handlers ---

// adminListSubmissions handles GET /api/v1/submissions
func (a *API) adminListSubmissions(c *gin.Context) {
	status := c.Query("status")
	resourceType := c.Query("resource_type")
	pageSize := 20
	pageNumber := 1
	if ps := c.Query("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 {
			pageSize = v
			if pageSize > 100 {
				pageSize = 100
			}
		}
	}
	if pn := c.Query("page_number"); pn != "" {
		if v, err := strconv.Atoi(pn); err == nil && v > 0 {
			pageNumber = v
		}
	}

	submissions, totalCount, totalPages, err := a.service.GetAllSubmissions(status, resourceType, pageSize, pageNumber)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	// Get status counts for dashboard (non-fatal if this fails)
	counts, err := a.service.GetSubmissionStatusCounts()
	if err != nil || counts == nil {
		counts = make(map[string]int64)
	}

	serialized := make([]gin.H, len(submissions))
	for i, s := range submissions {
		serialized[i] = serializeSubmissionForAdmin(&s)
	}

	c.JSON(http.StatusOK, gin.H{
		"data":          serialized,
		"total_count":   totalCount,
		"total_pages":   totalPages,
		"page_number":   pageNumber,
		"page_size":     pageSize,
		"status_counts": counts,
	})
}

// adminGetSubmission handles GET /api/v1/submissions/:id
func (a *API) adminGetSubmission(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "invalid submission ID"}},
		})
		return
	}

	submission, err := a.service.GetSubmissionByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "submission not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeSubmissionForAdmin(submission)})
}

// adminStartReview handles POST /api/v1/submissions/:id/review
func (a *API) adminStartReview(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	currentUser := user.(*models.User)

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "invalid submission ID"}},
		})
		return
	}

	submission, err := a.service.StartReview(uint(id), currentUser.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeSubmissionForAdmin(submission)})
}

// adminApproveSubmission handles POST /api/v1/submissions/:id/approve
func (a *API) adminApproveSubmission(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	currentUser := user.(*models.User)

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "invalid submission ID"}},
		})
		return
	}

	var input AdminReviewInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	attrs := input.Data.Attributes
	if err := validateAdminReviewInput(attrs.ReviewNotes, attrs.Feedback); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	submission, err := a.service.ApproveSubmission(uint(id), currentUser.ID, attrs.FinalPrivacyScore, attrs.AssignedCatalogues, attrs.ReviewNotes)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeSubmissionForAdmin(submission)})
}

// adminRejectSubmission handles POST /api/v1/submissions/:id/reject
func (a *API) adminRejectSubmission(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	currentUser := user.(*models.User)

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "invalid submission ID"}},
		})
		return
	}

	var input AdminReviewInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	attrs := input.Data.Attributes
	if err := validateAdminReviewInput(attrs.ReviewNotes, attrs.Feedback); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	submission, err := a.service.RejectSubmission(uint(id), currentUser.ID, attrs.Feedback, attrs.ReviewNotes)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeSubmissionForAdmin(submission)})
}

// adminRequestChanges handles POST /api/v1/submissions/:id/request-changes
func (a *API) adminRequestChanges(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	currentUser := user.(*models.User)

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "invalid submission ID"}},
		})
		return
	}

	var input AdminReviewInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	attrs := input.Data.Attributes
	if err := validateAdminReviewInput(attrs.ReviewNotes, attrs.Feedback); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	submission, err := a.service.RequestChanges(uint(id), currentUser.ID, attrs.Feedback, attrs.ReviewNotes)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeSubmissionForAdmin(submission)})
}

