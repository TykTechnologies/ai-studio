package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/audit"
	"github.com/gin-gonic/gin"
)

// auditErrorResponse maps service errors onto the API error envelope.
func auditErrorResponse(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, audit.ErrEnterpriseFeature):
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Enterprise Feature", Detail: err.Error()}},
		})
	case errors.Is(err, audit.ErrDisabled):
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Audit Trail Disabled", Detail: err.Error()}},
		})
	case errors.Is(err, audit.ErrNotFound):
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: err.Error()}},
		})
	default:
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: fallback}},
		})
	}
}

func auditBadRequest(c *gin.Context, detail string) {
	c.JSON(http.StatusBadRequest, models.ErrorResponse{
		Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: detail}},
	})
}

// parseAuditTime accepts YYYY-MM-DD or RFC3339. Date-only end values are
// pushed to the end of that day.
func parseAuditTime(value string, endOfDay bool) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("expected YYYY-MM-DD or RFC3339, got %q", value)
	}
	if endOfDay {
		t = t.Add(24*time.Hour - time.Nanosecond)
	}
	return t, nil
}

// parseAuditQuery reads the shared filter parameters.
func parseAuditQuery(c *gin.Context) (audit.Query, error) {
	var q audit.Query

	if v := c.Query("start_date"); v != "" {
		t, err := parseAuditTime(v, false)
		if err != nil {
			return q, fmt.Errorf("invalid start_date: %w", err)
		}
		q.Start = t
	}
	if v := c.Query("end_date"); v != "" {
		t, err := parseAuditTime(v, true)
		if err != nil {
			return q, fmt.Errorf("invalid end_date: %w", err)
		}
		q.End = t
	}
	if !q.Start.IsZero() && !q.End.IsZero() && q.End.Before(q.Start) {
		return q, errors.New("end_date must not be before start_date")
	}

	if v := c.Query("user_id"); v != "" {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return q, errors.New("invalid user_id")
		}
		q.UserID = uint(n)
	}
	q.UserEmail = strings.TrimSpace(c.Query("user"))
	q.Action = strings.TrimSpace(c.Query("action"))
	q.ResourceType = strings.TrimSpace(c.Query("resource_type"))
	q.ResourceID = strings.TrimSpace(c.Query("resource_id"))
	q.Method = strings.ToUpper(strings.TrimSpace(c.Query("method")))
	q.IP = strings.TrimSpace(c.Query("ip"))
	q.RequestID = strings.TrimSpace(c.Query("req_id"))
	q.Search = strings.TrimSpace(c.Query("search"))

	for _, field := range []struct {
		name string
		val  *string
		max  int
	}{
		{"user", &q.UserEmail, 255}, {"action", &q.Action, 128}, {"resource_type", &q.ResourceType, 64},
		{"resource_id", &q.ResourceID, 255}, {"method", &q.Method, 16}, {"ip", &q.IP, 64},
		{"req_id", &q.RequestID, 64}, {"search", &q.Search, 255},
	} {
		if len(*field.val) > field.max {
			return q, fmt.Errorf("%s is too long", field.name)
		}
	}

	if v := c.Query("status"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 100 || n > 599 {
			return q, errors.New("invalid status")
		}
		q.Status = n
	}
	if v := c.Query("status_class"); v != "" {
		v = strings.TrimSuffix(strings.ToLower(v), "xx")
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 5 {
			return q, errors.New("invalid status_class, expected one of 2xx, 3xx, 4xx, 5xx")
		}
		q.StatusClass = n
	}

	if v := c.Query("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return q, errors.New("invalid page")
		}
		q.Page = n
	}
	if v := c.Query("page_size"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return q, errors.New("invalid page_size")
		}
		q.PageSize = n
	}
	if v := strings.ToLower(c.Query("sort")); v == "asc" {
		q.SortAsc = true
	}

	return q, nil
}

// getAuditStatus godoc
// @Summary Audit trail availability and configuration
// @Description Reports whether the audit trail is available (Enterprise), enabled, and how it is configured
// @Tags Audit
// @Produce json
// @Success 200 {object} audit.Status
// @Router /audit/status [get]
func (a *API) getAuditStatus(c *gin.Context) {
	if a.auditService == nil {
		c.JSON(http.StatusOK, audit.Status{Available: false})
		return
	}
	c.JSON(http.StatusOK, a.auditService.Status())
}

// listAuditRecords godoc
// @Summary List audit records
// @Description Page through the audit trail with filters. Newest first unless sort=asc.
// @Tags Audit
// @Produce json
// @Param start_date query string false "Start (YYYY-MM-DD or RFC3339)"
// @Param end_date query string false "End (YYYY-MM-DD or RFC3339)"
// @Param user query string false "User email substring"
// @Param user_id query int false "User ID"
// @Param action query string false "Exact action name, e.g. Update LLM"
// @Param resource_type query string false "Resource type, e.g. llm"
// @Param resource_id query string false "Resource ID"
// @Param method query string false "HTTP method"
// @Param status query int false "Exact HTTP status"
// @Param status_class query string false "2xx, 3xx, 4xx or 5xx"
// @Param ip query string false "Client IP"
// @Param req_id query string false "Request ID"
// @Param search query string false "Free text over action, URL, user, resource name, IP, request ID"
// @Param page query int false "Page (1-based)"
// @Param page_size query int false "Page size (max 500)"
// @Param sort query string false "asc or desc"
// @Success 200 {object} audit.Page
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Router /audit/records [get]
func (a *API) listAuditRecords(c *gin.Context) {
	q, err := parseAuditQuery(c)
	if err != nil {
		auditBadRequest(c, err.Error())
		return
	}
	page, err := a.auditService.List(c.Request.Context(), q)
	if err != nil {
		auditErrorResponse(c, err, "Failed to list audit records")
		return
	}
	c.JSON(http.StatusOK, page)
}

// getAuditRecord godoc
// @Summary Get one audit record
// @Description Returns a single record including request/response dumps when detailed recording is on
// @Tags Audit
// @Produce json
// @Param id path int true "Record ID"
// @Success 200 {object} models.AuditRecord
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /audit/records/{id} [get]
func (a *API) getAuditRecord(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		auditBadRequest(c, "invalid record id")
		return
	}
	rec, err := a.auditService.Get(c.Request.Context(), uint(id))
	if err != nil {
		auditErrorResponse(c, err, "Failed to get audit record")
		return
	}
	c.JSON(http.StatusOK, rec)
}

// getAuditSummary godoc
// @Summary Summarise audit records
// @Description Counts by action, user, resource type and status class plus a daily timeline for the filter
// @Tags Audit
// @Produce json
// @Success 200 {object} audit.Summary
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Router /audit/summary [get]
func (a *API) getAuditSummary(c *gin.Context) {
	q, err := parseAuditQuery(c)
	if err != nil {
		auditBadRequest(c, err.Error())
		return
	}
	summary, err := a.auditService.Summary(c.Request.Context(), q)
	if err != nil {
		auditErrorResponse(c, err, "Failed to summarise audit records")
		return
	}
	c.JSON(http.StatusOK, summary)
}

// exportAuditRecords godoc
// @Summary Export audit records
// @Description Downloads matching records (max 50,000) as CSV or JSON
// @Tags Audit
// @Produce text/csv,application/json
// @Param format query string false "csv (default) or json"
// @Success 200 {string} string "file"
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Router /audit/export [get]
func (a *API) exportAuditRecords(c *gin.Context) {
	q, err := parseAuditQuery(c)
	if err != nil {
		auditBadRequest(c, err.Error())
		return
	}
	format := strings.ToLower(c.DefaultQuery("format", audit.FormatCSV))
	if format != audit.FormatCSV && format != audit.FormatJSON {
		auditBadRequest(c, "format must be csv or json")
		return
	}
	data, contentType, err := a.auditService.Export(c.Request.Context(), q, format)
	if err != nil {
		auditErrorResponse(c, err, "Failed to export audit records")
		return
	}
	filename := fmt.Sprintf("audit_trail_%s.%s", time.Now().UTC().Format("20060102_150405"), format)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, contentType, data)
}

// getAuditResourceHistory godoc
// @Summary History of one object
// @Description Every recorded action on a single resource, newest first
// @Tags Audit
// @Produce json
// @Param type path string true "Resource type, e.g. llm"
// @Param id path string true "Resource ID"
// @Success 200 {object} audit.Page
// @Failure 403 {object} models.ErrorResponse
// @Router /audit/resources/{type}/{id} [get]
func (a *API) getAuditResourceHistory(c *gin.Context) {
	q, err := parseAuditQuery(c)
	if err != nil {
		auditBadRequest(c, err.Error())
		return
	}
	q.ResourceType = c.Param("type")
	q.ResourceID = c.Param("id")
	if q.ResourceType == "" || q.ResourceID == "" {
		auditBadRequest(c, "resource type and id are required")
		return
	}
	page, err := a.auditService.List(c.Request.Context(), q)
	if err != nil {
		auditErrorResponse(c, err, "Failed to load resource history")
		return
	}
	c.JSON(http.StatusOK, page)
}
