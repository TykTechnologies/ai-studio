package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// --- Input types ---

type SubmissionInput struct {
	Data struct {
		Attributes struct {
			ResourceType         string         `json:"resource_type"`
			Status               string         `json:"status"` // draft or submitted
			ResourcePayload      models.JSONMap `json:"resource_payload"`
			Attestations         models.JSONMap `json:"attestations"`
			SuggestedPrivacy     int            `json:"suggested_privacy"`
			PrivacyJustification string         `json:"privacy_justification"`
			PrimaryContact       string         `json:"primary_contact"`
			SecondaryContact     string         `json:"secondary_contact"`
			SLAExpectation       string         `json:"sla_expectation"`
			DataCutoffDate       *string        `json:"data_cutoff_date"` // ISO 8601 string
			DocumentationURL     string         `json:"documentation_url"`
			Notes                string         `json:"notes"`
		} `json:"attributes"`
	} `json:"data"`
}

type AdminReviewInput struct {
	Data struct {
		Attributes struct {
			FinalPrivacyScore  int            `json:"final_privacy_score"`
			AssignedCatalogues models.JSONMap `json:"assigned_catalogues"`
			ReviewNotes        string         `json:"review_notes"`
			Feedback           string         `json:"feedback"` // submitter-facing feedback (for reject/changes_requested)
		} `json:"attributes"`
	} `json:"data"`
}

type UpdateSubmissionInput struct {
	Data struct {
		Attributes struct {
			ResourceType         string         `json:"resource_type"`
			TargetResourceID     uint           `json:"target_resource_id"`
			Status               string         `json:"status"`
			ResourcePayload      models.JSONMap `json:"resource_payload"`
			Attestations         models.JSONMap `json:"attestations"`
			SuggestedPrivacy     int            `json:"suggested_privacy"`
			PrivacyJustification string         `json:"privacy_justification"`
			PrimaryContact       string         `json:"primary_contact"`
			SecondaryContact     string         `json:"secondary_contact"`
			SLAExpectation       string         `json:"sla_expectation"`
			DataCutoffDate       *string        `json:"data_cutoff_date"`
			DocumentationURL     string         `json:"documentation_url"`
			Notes                string         `json:"notes"`
		} `json:"attributes"`
	} `json:"data"`
}

// --- User-facing submission handlers ---

// createSubmission handles POST /common/submissions
func (a *API) createSubmission(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	currentUser := user.(*models.User)

	var input SubmissionInput
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
	if attrs.ResourceType == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "resource_type is required"}},
		})
		return
	}

	var dataCutoff *time.Time
	if attrs.DataCutoffDate != nil && *attrs.DataCutoffDate != "" {
		t, err := time.Parse(time.RFC3339, *attrs.DataCutoffDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "invalid data_cutoff_date format, expected RFC3339"}},
			})
			return
		}
		dataCutoff = &t
	}

	submission, err := a.service.CreateSubmission(
		currentUser.ID,
		attrs.ResourceType,
		attrs.Status,
		attrs.ResourcePayload,
		attrs.Attestations,
		attrs.SuggestedPrivacy,
		attrs.PrivacyJustification,
		attrs.PrimaryContact,
		attrs.SecondaryContact,
		attrs.SLAExpectation,
		dataCutoff,
		attrs.DocumentationURL,
		attrs.Notes,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": serializeSubmission(submission)})
}

// listMySubmissions handles GET /common/submissions
func (a *API) listMySubmissions(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	currentUser := user.(*models.User)

	status := c.Query("status")
	pageSize := 20
	pageNumber := 1
	if ps := c.Query("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 {
			pageSize = v
		}
	}
	if pn := c.Query("page_number"); pn != "" {
		if v, err := strconv.Atoi(pn); err == nil && v > 0 {
			pageNumber = v
		}
	}

	submissions, totalCount, totalPages, err := a.service.GetSubmissionsBySubmitter(currentUser.ID, status, pageSize, pageNumber)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	serialized := make([]gin.H, len(submissions))
	for i, s := range submissions {
		serialized[i] = serializeSubmission(&s)
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        serialized,
		"total_count": totalCount,
		"total_pages": totalPages,
		"page_number": pageNumber,
		"page_size":   pageSize,
	})
}

// getMySubmission handles GET /common/submissions/:id
func (a *API) getMySubmission(c *gin.Context) {
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

	// Only allow viewing own submissions
	if submission.SubmitterID != currentUser.ID {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Forbidden", Detail: "not authorized to view this submission"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeSubmission(submission)})
}

// updateMySubmission handles PATCH /common/submissions/:id
func (a *API) updateMySubmission(c *gin.Context) {
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

	var input SubmissionInput
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
	var dataCutoff *time.Time
	if attrs.DataCutoffDate != nil && *attrs.DataCutoffDate != "" {
		t, err := time.Parse(time.RFC3339, *attrs.DataCutoffDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "invalid data_cutoff_date format, expected RFC3339"}},
			})
			return
		}
		dataCutoff = &t
	}

	submission, err := a.service.UpdateSubmission(
		uint(id),
		currentUser.ID,
		attrs.ResourcePayload,
		attrs.Attestations,
		attrs.SuggestedPrivacy,
		attrs.PrivacyJustification,
		attrs.PrimaryContact,
		attrs.SecondaryContact,
		attrs.SLAExpectation,
		dataCutoff,
		attrs.DocumentationURL,
		attrs.Notes,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeSubmission(submission)})
}

// deleteMySubmission handles DELETE /common/submissions/:id
func (a *API) deleteMySubmission(c *gin.Context) {
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

	if err := a.service.DeleteSubmission(uint(id), currentUser.ID); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "submission deleted"})
}

// submitSubmission handles POST /common/submissions/:id/submit
func (a *API) submitSubmission(c *gin.Context) {
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

	submission, err := a.service.SubmitSubmission(uint(id), currentUser.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeSubmission(submission)})
}

// getAttestationTemplatesForSubmission handles GET /common/submissions/attestation-templates
func (a *API) getAttestationTemplatesForSubmission(c *gin.Context) {
	resourceType := c.Query("resource_type")

	var templates models.AttestationTemplates
	var err error
	if resourceType != "" {
		templates, err = a.service.GetAttestationTemplatesByType(resourceType, true)
	} else {
		templates, err = a.service.GetAllAttestationTemplates(true)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": templates})
}

// createUpdateSubmission handles POST /common/submissions/update
// Creates a submission that proposes changes to an existing published resource
func (a *API) createUpdateSubmission(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	currentUser := user.(*models.User)

	var input UpdateSubmissionInput
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
	if attrs.TargetResourceID == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "target_resource_id is required"}},
		})
		return
	}

	var dataCutoff *time.Time
	if attrs.DataCutoffDate != nil && *attrs.DataCutoffDate != "" {
		t, err := time.Parse(time.RFC3339, *attrs.DataCutoffDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "invalid data_cutoff_date format"}},
			})
			return
		}
		dataCutoff = &t
	}

	submission, err := a.service.CreateUpdateSubmission(
		currentUser.ID,
		attrs.ResourceType,
		attrs.TargetResourceID,
		attrs.ResourcePayload,
		attrs.Attestations,
		attrs.SuggestedPrivacy,
		attrs.PrivacyJustification,
		attrs.PrimaryContact,
		attrs.SecondaryContact,
		attrs.SLAExpectation,
		dataCutoff,
		attrs.DocumentationURL,
		attrs.Notes,
		attrs.Status,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": serializeSubmission(submission)})
}

// adminListVersions handles GET /api/v1/submissions/:id/versions
func (a *API) adminListVersions(c *gin.Context) {
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

	if submission.ResourceID == nil {
		c.JSON(http.StatusOK, gin.H{"data": []interface{}{}})
		return
	}

	versions, err := a.service.GetResourceVersions(submission.ResourceType, *submission.ResourceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": versions})
}

// adminRollbackVersion handles POST /api/v1/submissions/:id/rollback/:version_id
func (a *API) adminRollbackVersion(c *gin.Context) {
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

	versionID, err := strconv.ParseUint(c.Param("version_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "invalid version ID"}},
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

	if submission.ResourceID == nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "submission has no associated resource"}},
		})
		return
	}

	if err := a.service.RollbackResource(submission.ResourceType, *submission.ResourceID, uint(versionID), currentUser.ID); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "resource rolled back successfully"})
}

// --- Duplicate detection ---

// checkDuplicates handles POST /common/submissions/check-duplicates
func (a *API) checkDuplicates(c *gin.Context) {
	var input struct {
		ResourceType    string         `json:"resource_type"`
		ResourcePayload models.JSONMap `json:"resource_payload"`
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

	candidates, err := a.service.CheckForDuplicates(input.ResourceType, input.ResourcePayload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": candidates})
}

// --- Nominate from existing ---

// nominateDatasource handles POST /common/submissions/nominate/datasource/:id
func (a *API) nominateDatasource(c *gin.Context) {
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
			}{{Title: "Bad Request", Detail: "invalid datasource ID"}},
		})
		return
	}

	submission, err := a.service.NominateExistingDatasource(currentUser.ID, uint(id))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": serializeSubmission(submission)})
}

// nominateTool handles POST /common/submissions/nominate/tool/:id
func (a *API) nominateTool(c *gin.Context) {
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
			}{{Title: "Bad Request", Detail: "invalid tool ID"}},
		})
		return
	}

	submission, err := a.service.NominateExistingTool(currentUser.ID, uint(id))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": serializeSubmission(submission)})
}

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

	// Get status counts for dashboard
	counts, _ := a.service.GetSubmissionStatusCounts()

	serialized := make([]gin.H, len(submissions))
	for i, s := range submissions {
		serialized[i] = serializeSubmission(&s)
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

	c.JSON(http.StatusOK, gin.H{"data": serializeSubmission(submission)})
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

	c.JSON(http.StatusOK, gin.H{"data": serializeSubmission(submission)})
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

	c.JSON(http.StatusOK, gin.H{"data": serializeSubmission(submission)})
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

	c.JSON(http.StatusOK, gin.H{"data": serializeSubmission(submission)})
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

	c.JSON(http.StatusOK, gin.H{"data": serializeSubmission(submission)})
}

// --- Serializer ---

func serializeSubmission(s *models.Submission) gin.H {
	result := gin.H{
		"id":                    s.ID,
		"resource_type":         s.ResourceType,
		"resource_id":           s.ResourceID,
		"status":                s.Status,
		"is_update":             s.IsUpdate,
		"target_resource_id":    s.TargetResourceID,
		"submitter_id":          s.SubmitterID,
		"reviewer_id":           s.ReviewerID,
		"resource_payload":      s.ResourcePayload,
		"attestations":          s.Attestations,
		"suggested_privacy":     s.SuggestedPrivacy,
		"privacy_justification": s.PrivacyJustification,
		"primary_contact":       s.PrimaryContact,
		"secondary_contact":     s.SecondaryContact,
		"sla_expectation":       s.SLAExpectation,
		"data_cutoff_date":      s.DataCutoffDate,
		"documentation_url":     s.DocumentationURL,
		"notes":                 s.Notes,
		"review_notes":          s.ReviewNotes,
		"submitter_feedback":    s.SubmitterFeedback,
		"assigned_catalogues":   s.AssignedCatalogues,
		"final_privacy_score":   s.FinalPrivacyScore,
		"submitted_at":          s.SubmittedAt,
		"review_started_at":     s.ReviewStartedAt,
		"review_completed_at":   s.ReviewCompletedAt,
		"created_at":            s.CreatedAt,
		"updated_at":            s.UpdatedAt,
	}

	if s.Submitter != nil {
		result["submitter"] = gin.H{
			"id":    s.Submitter.ID,
			"name":  s.Submitter.Name,
			"email": s.Submitter.Email,
		}
	}
	if s.Reviewer != nil {
		result["reviewer"] = gin.H{
			"id":    s.Reviewer.ID,
			"name":  s.Reviewer.Name,
			"email": s.Reviewer.Email,
		}
	}

	return result
}
