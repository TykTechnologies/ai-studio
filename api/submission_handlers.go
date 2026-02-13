package api

import (
	"fmt"
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

const maxAdminReviewFieldLength = 10000

// validateAdminReviewInput checks length limits on admin review fields.
func validateAdminReviewInput(reviewNotes, feedback string) error {
	if len(reviewNotes) > maxAdminReviewFieldLength {
		return fmt.Errorf("review_notes must not exceed %d characters", maxAdminReviewFieldLength)
	}
	if len(feedback) > maxAdminReviewFieldLength {
		return fmt.Errorf("feedback must not exceed %d characters", maxAdminReviewFieldLength)
	}
	return nil
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

	c.JSON(http.StatusCreated, gin.H{"data": serializeSubmissionForPortal(submission)})
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
		serialized[i] = serializeSubmissionForPortal(&s)
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

	c.JSON(http.StatusOK, gin.H{"data": serializeSubmissionForPortal(submission)})
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

	c.JSON(http.StatusOK, gin.H{"data": serializeSubmissionForPortal(submission)})
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

	c.JSON(http.StatusOK, gin.H{"data": serializeSubmissionForPortal(submission)})
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

	c.JSON(http.StatusCreated, gin.H{"data": serializeSubmissionForPortal(submission)})
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

	// Redact secrets from version snapshot payloads
	for i := range versions {
		versions[i].Payload = redactPayloadSecrets(versions[i].Payload)
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

// --- Serializer ---

// secretPayloadFields are credential fields that must be redacted before returning to any client
var secretPayloadFields = []string{
	"db_conn_api_key", "embed_api_key", "auth_key", "db_conn_string",
}

// redactPayloadSecrets returns a copy of the payload with secret fields replaced by "[redacted]"
func redactPayloadSecrets(payload models.JSONMap) models.JSONMap {
	if payload == nil {
		return nil
	}
	redacted := make(models.JSONMap, len(payload))
	for k, v := range payload {
		redacted[k] = v
	}
	for _, field := range secretPayloadFields {
		if _, ok := redacted[field]; ok {
			redacted[field] = "[redacted]"
		}
	}
	return redacted
}

// serializeSubmissionForAdmin returns full submission data with secrets redacted but review_notes visible
func serializeSubmissionForAdmin(s *models.Submission) gin.H {
	return serializeSubmissionInternal(s, true)
}

// serializeSubmissionForPortal returns submission data with secrets redacted AND review_notes stripped
func serializeSubmissionForPortal(s *models.Submission) gin.H {
	return serializeSubmissionInternal(s, false)
}

func serializeSubmissionInternal(s *models.Submission, includeInternalNotes bool) gin.H {
	result := gin.H{
		"id":                    s.ID,
		"resource_type":         s.ResourceType,
		"resource_id":           s.ResourceID,
		"status":                s.Status,
		"is_update":             s.IsUpdate,
		"target_resource_id":    s.TargetResourceID,
		"submitter_id":          s.SubmitterID,
		"reviewer_id":           s.ReviewerID,
		"resource_payload":      redactPayloadSecrets(s.ResourcePayload),
		"attestations":          s.Attestations,
		"suggested_privacy":     s.SuggestedPrivacy,
		"privacy_justification": s.PrivacyJustification,
		"primary_contact":       s.PrimaryContact,
		"secondary_contact":     s.SecondaryContact,
		"sla_expectation":       s.SLAExpectation,
		"data_cutoff_date":      s.DataCutoffDate,
		"documentation_url":     s.DocumentationURL,
		"notes":                 s.Notes,
		"submitter_feedback":    s.SubmitterFeedback,
		"assigned_catalogues":   s.AssignedCatalogues,
		"final_privacy_score":   s.FinalPrivacyScore,
		"submitted_at":          s.SubmittedAt,
		"review_started_at":     s.ReviewStartedAt,
		"review_completed_at":   s.ReviewCompletedAt,
		"created_at":            s.CreatedAt,
		"updated_at":            s.UpdatedAt,
	}

	if includeInternalNotes {
		result["review_notes"] = s.ReviewNotes
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
