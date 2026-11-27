package api

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/compliance"
	"github.com/gin-gonic/gin"
)

// complianceService holds the compliance service instance
var complianceService compliance.Service

// InitComplianceService initializes the compliance service
func (a *API) InitComplianceService() {
	complianceService = compliance.NewService(a.service.DB)
}

// getComplianceDateRange parses start_date and end_date from query params
// Defaults to last 7 days if not provided
func getComplianceDateRange(c *gin.Context) (time.Time, time.Time, error) {
	now := time.Now()
	defaultStart := now.AddDate(0, 0, -7)
	defaultEnd := now

	startDate := defaultStart
	endDate := defaultEnd

	if startDateStr := c.Query("start_date"); startDateStr != "" {
		parsedDate, err := time.Parse("2006-01-02", startDateStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid start_date format, expected YYYY-MM-DD")
		}
		startDate = parsedDate
	}

	if endDateStr := c.Query("end_date"); endDateStr != "" {
		parsedDate, err := time.Parse("2006-01-02", endDateStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid end_date format, expected YYYY-MM-DD")
		}
		// end_date is inclusive: the range ends (exclusively) at the next
		// midnight, as getDateRange's, so the last sub-second counts.
		endDate = nextMidnight(parsedDate)
	}

	return startDate, endDate, nil
}

// getComplianceSummary godoc
// @Summary Get compliance summary
// @Description Get high-level compliance metrics for the dashboard
// @Tags Compliance
// @Accept json
// @Produce json
// @Param start_date query string false "Start date (YYYY-MM-DD), defaults to 7 days ago"
// @Param end_date query string false "End date (YYYY-MM-DD), defaults to today"
// @Success 200 {object} compliance.ComplianceSummary
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /compliance/summary [get]
func (a *API) getComplianceSummary(c *gin.Context) {
	startDate, endDate, err := getComplianceDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	summary, err := complianceService.GetSummary(startDate, endDate)
	if err != nil {
		if err == compliance.ErrEnterpriseFeature {
			c.JSON(http.StatusForbidden, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Enterprise Feature", Detail: err.Error()}},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get compliance summary"}},
		})
		return
	}

	c.JSON(http.StatusOK, summary)
}

// getHighRiskApps godoc
// @Summary Get high risk apps
// @Description Get apps ranked by compliance risk
// @Tags Compliance
// @Accept json
// @Produce json
// @Param start_date query string false "Start date (YYYY-MM-DD), defaults to 7 days ago"
// @Param end_date query string false "End date (YYYY-MM-DD), defaults to today"
// @Param limit query int false "Maximum number of apps to return, defaults to 10"
// @Success 200 {array} compliance.HighRiskApp
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /compliance/high-risk-apps [get]
func (a *API) getHighRiskApps(c *gin.Context) {
	startDate, endDate, err := getComplianceDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	limit := 10
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	apps, err := complianceService.GetHighRiskApps(startDate, endDate, limit)
	if err != nil {
		if err == compliance.ErrEnterpriseFeature {
			c.JSON(http.StatusForbidden, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Enterprise Feature", Detail: err.Error()}},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get high risk apps"}},
		})
		return
	}

	c.JSON(http.StatusOK, apps)
}

// getAccessIssues godoc
// @Summary Get access issues
// @Description Get authentication and authorization failures
// @Tags Compliance
// @Accept json
// @Produce json
// @Param start_date query string false "Start date (YYYY-MM-DD), defaults to 7 days ago"
// @Param end_date query string false "End date (YYYY-MM-DD), defaults to today"
// @Param app_id query int false "Filter by app ID"
// @Success 200 {object} compliance.AccessIssuesData
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /compliance/access-issues [get]
func (a *API) getAccessIssues(c *gin.Context) {
	startDate, endDate, err := getComplianceDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	var appID *uint
	if appIDStr := c.Query("app_id"); appIDStr != "" {
		if parsed, err := strconv.ParseUint(appIDStr, 10, 32); err == nil {
			id := uint(parsed)
			appID = &id
		}
	}

	data, err := complianceService.GetAccessIssues(startDate, endDate, appID)
	if err != nil {
		if err == compliance.ErrEnterpriseFeature {
			c.JSON(http.StatusForbidden, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Enterprise Feature", Detail: err.Error()}},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get access issues"}},
		})
		return
	}

	c.JSON(http.StatusOK, data)
}

// getPolicyViolations godoc
// @Summary Get policy violations
// @Description Get filter blocks and model access violations
// @Tags Compliance
// @Accept json
// @Produce json
// @Param start_date query string false "Start date (YYYY-MM-DD), defaults to 7 days ago"
// @Param end_date query string false "End date (YYYY-MM-DD), defaults to today"
// @Param app_id query int false "Filter by app ID"
// @Success 200 {object} compliance.PolicyViolationsData
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /compliance/policy-violations [get]
func (a *API) getPolicyViolations(c *gin.Context) {
	startDate, endDate, err := getComplianceDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	var appID *uint
	if appIDStr := c.Query("app_id"); appIDStr != "" {
		if parsed, err := strconv.ParseUint(appIDStr, 10, 32); err == nil {
			id := uint(parsed)
			appID = &id
		}
	}

	data, err := complianceService.GetPolicyViolations(startDate, endDate, appID)
	if err != nil {
		if err == compliance.ErrEnterpriseFeature {
			c.JSON(http.StatusForbidden, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Enterprise Feature", Detail: err.Error()}},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get policy violations"}},
		})
		return
	}

	c.JSON(http.StatusOK, data)
}

// getBudgetAlerts godoc
// @Summary Get budget alerts
// @Description Get apps/LLMs approaching or exceeding budget limits
// @Tags Compliance
// @Accept json
// @Produce json
// @Param start_date query string false "Start date (YYYY-MM-DD), defaults to 7 days ago"
// @Param end_date query string false "End date (YYYY-MM-DD), defaults to today"
// @Success 200 {object} compliance.BudgetAlertsData
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /compliance/budget-alerts [get]
func (a *API) getBudgetAlerts(c *gin.Context) {
	startDate, endDate, err := getComplianceDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	data, err := complianceService.GetBudgetAlerts(startDate, endDate)
	if err != nil {
		if err == compliance.ErrEnterpriseFeature {
			c.JSON(http.StatusForbidden, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Enterprise Feature", Detail: err.Error()}},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get budget alerts"}},
		})
		return
	}

	c.JSON(http.StatusOK, data)
}

// getComplianceErrors godoc
// @Summary Get compliance errors
// @Description Get error metrics by vendor
// @Tags Compliance
// @Accept json
// @Produce json
// @Param start_date query string false "Start date (YYYY-MM-DD), defaults to 7 days ago"
// @Param end_date query string false "End date (YYYY-MM-DD), defaults to today"
// @Param vendor query string false "Filter by vendor"
// @Success 200 {object} compliance.ErrorData
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /compliance/errors [get]
func (a *API) getComplianceErrors(c *gin.Context) {
	startDate, endDate, err := getComplianceDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	var vendor *string
	if vendorStr := c.Query("vendor"); vendorStr != "" {
		vendor = &vendorStr
	}

	data, err := complianceService.GetErrors(startDate, endDate, vendor)
	if err != nil {
		if err == compliance.ErrEnterpriseFeature {
			c.JSON(http.StatusForbidden, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Enterprise Feature", Detail: err.Error()}},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get compliance errors"}},
		})
		return
	}

	c.JSON(http.StatusOK, data)
}

// getAppRiskProfile godoc
// @Summary Get app risk profile
// @Description Get detailed compliance profile for a single app
// @Tags Compliance
// @Accept json
// @Produce json
// @Param id path int true "App ID"
// @Param start_date query string false "Start date (YYYY-MM-DD), defaults to 7 days ago"
// @Param end_date query string false "End date (YYYY-MM-DD), defaults to today"
// @Success 200 {object} compliance.AppRiskProfile
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /compliance/app/{id}/risk-profile [get]
func (a *API) getAppRiskProfile(c *gin.Context) {
	appIDStr := c.Param("id")
	appID, err := strconv.ParseUint(appIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid app ID"}},
		})
		return
	}

	startDate, endDate, err := getComplianceDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	profile, err := complianceService.GetAppRiskProfile(uint(appID), startDate, endDate)
	if err != nil {
		if err == compliance.ErrEnterpriseFeature {
			c.JSON(http.StatusForbidden, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Enterprise Feature", Detail: err.Error()}},
			})
			return
		}
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "App not found"}},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get app risk profile"}},
		})
		return
	}

	c.JSON(http.StatusOK, profile)
}

// exportComplianceData godoc
// @Summary Export compliance data
// @Description Export compliance data in CSV format
// @Tags Compliance
// @Accept json
// @Produce text/csv
// @Param start_date query string false "Start date (YYYY-MM-DD), defaults to 7 days ago"
// @Param end_date query string false "End date (YYYY-MM-DD), defaults to today"
// @Param view query string true "View to export (summary, access, policy, budget, errors)"
// @Success 200 {file} file
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /compliance/export [get]
func (a *API) exportComplianceData(c *gin.Context) {
	startDate, endDate, err := getComplianceDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	view := c.Query("view")
	if view == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "view parameter is required"}},
		})
		return
	}

	data, err := complianceService.ExportData(startDate, endDate, view)
	if err != nil {
		if err == compliance.ErrEnterpriseFeature {
			c.JSON(http.StatusForbidden, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Enterprise Feature", Detail: err.Error()}},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: fmt.Sprintf("Failed to export data: %v", err)}},
		})
		return
	}

	filename := fmt.Sprintf("compliance_%s_%s_%s.csv",
		view,
		startDate.Format("2006-01-02"),
		endDate.Add(-time.Nanosecond).Format("2006-01-02")) // the last day: endDate is exclusive

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Data(http.StatusOK, "text/csv", data)
}

// isComplianceAvailable godoc
// @Summary Check if compliance features are available
// @Description Returns whether compliance monitoring is available (Enterprise Edition)
// @Tags Compliance
// @Accept json
// @Produce json
// @Success 200 {object} map[string]bool
// @Router /compliance/available [get]
func (a *API) isComplianceAvailable(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"available": compliance.IsEnterpriseAvailable(),
	})
}

// getViolationRecords godoc
// @Summary Get individual violation records
// @Description Get individual violation records with full details for drill-down view
// @Tags Compliance
// @Accept json
// @Produce json
// @Param start_date query string false "Start date (YYYY-MM-DD), defaults to 7 days ago"
// @Param end_date query string false "End date (YYYY-MM-DD), defaults to today"
// @Param app_id query int false "Filter by app ID"
// @Param limit query int false "Maximum number of records to return, defaults to 100"
// @Success 200 {array} compliance.ViolationRecord
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /compliance/violations [get]
func (a *API) getViolationRecords(c *gin.Context) {
	startDate, endDate, err := getComplianceDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	var appID *uint
	if appIDStr := c.Query("app_id"); appIDStr != "" {
		if parsed, err := strconv.ParseUint(appIDStr, 10, 32); err == nil {
			id := uint(parsed)
			appID = &id
		}
	}

	limit := 100
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	records, err := complianceService.GetViolationRecords(startDate, endDate, appID, limit)
	if err != nil {
		if err == compliance.ErrEnterpriseFeature {
			c.JSON(http.StatusForbidden, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Enterprise Feature", Detail: err.Error()}},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get violation records"}},
		})
		return
	}

	c.JSON(http.StatusOK, records)
}

// getComplianceEvents godoc
// @Summary Get script-reported compliance events
// @Description Get compliance events reported by filter scripts, with filtering and aggregation
// @Tags Compliance
// @Accept json
// @Produce json
// @Param start_date query string false "Start date (YYYY-MM-DD), defaults to 7 days ago"
// @Param end_date query string false "End date (YYYY-MM-DD), defaults to today"
// @Param app_id query int false "Filter by app ID"
// @Param event_type query string false "Filter by event type"
// @Param severity query string false "Filter by severity (info, warning, critical)"
// @Param page_size query int false "Events per page, defaults to 100, at most 1000"
// @Param page query int false "Page number (1-based), defaults to 1"
// @Param limit query int false "Older form of page_size, used only without page_size and page"
// @Param offset query int false "Older form of page: events to skip, used only without page_size and page"
// @Success 200 {object} compliance.ComplianceEventsData
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /compliance/events [get]
func (a *API) getComplianceEvents(c *gin.Context) {
	startDate, endDate, err := getComplianceDateRange(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	var appID *uint
	if appIDStr := c.Query("app_id"); appIDStr != "" {
		if parsed, err := strconv.ParseUint(appIDStr, 10, 32); err == nil {
			id := uint(parsed)
			appID = &id
		}
	}

	var eventType *string
	if et := c.Query("event_type"); et != "" {
		if len(et) > 100 {
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "event_type must be 100 characters or fewer"}},
			})
			return
		}
		eventType = &et
	}

	var severity *string
	if sev := c.Query("severity"); sev != "" {
		switch sev {
		case "info", "warning", "critical":
			severity = &sev
		default:
			c.JSON(http.StatusBadRequest, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "severity must be one of: info, warning, critical"}},
			})
			return
		}
	}

	limit, offset, page, err := complianceEventsPaging(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	data, err := complianceService.GetComplianceEvents(startDate, endDate, appID, eventType, severity, limit, offset)
	if err != nil {
		if err == compliance.ErrEnterpriseFeature {
			c.JSON(http.StatusForbidden, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Enterprise Feature", Detail: err.Error()}},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get compliance events"}},
		})
		return
	}

	data.Meta = &compliance.PageMeta{
		TotalCount: data.TotalCount,
		TotalPages: int((data.TotalCount + int64(limit) - 1) / int64(limit)),
		PageSize:   limit,
		PageNumber: page,
		Offset:     offset,
	}
	c.JSON(http.StatusOK, data)
}

// Compliance events paging: page/page_size like the other list endpoints,
// or the older limit/offset (the Filter Events tab and existing scripts).
const (
	complianceEventsDefaultPageSize = 100
	complianceEventsMaxPageSize     = 1000
)

// complianceEventsPaging returns the SQL limit and offset for a compliance
// events request, and the 1-based page they make. page_size and page win
// over limit and offset when either is given. A page size over the maximum
// is capped; a malformed or non-positive page or page_size is a 400.
func complianceEventsPaging(c *gin.Context) (limit, offset, page int, err error) {
	positive := func(name string, min int) (int, bool, error) {
		raw, ok := c.GetQuery(name)
		if !ok || raw == "" {
			return 0, false, nil
		}
		v, convErr := strconv.Atoi(raw)
		if convErr != nil || v < min {
			return 0, false, fmt.Errorf("%s must be an integer of at least %d", name, min)
		}
		return v, true, nil
	}
	pageSize, hasSize, err := positive("page_size", 1)
	if err != nil {
		return 0, 0, 0, err
	}
	pageNum, hasPage, err := positive("page", 1)
	if err != nil {
		return 0, 0, 0, err
	}
	if hasSize || hasPage {
		if !hasSize {
			pageSize = complianceEventsDefaultPageSize
		}
		if !hasPage {
			pageNum = 1
		}
		pageSize = min(pageSize, complianceEventsMaxPageSize)
		// The offset must not overflow (a huge page would wrap negative).
		if pageNum-1 > (math.MaxInt32-pageSize)/pageSize {
			return 0, 0, 0, fmt.Errorf("page is too large")
		}
		return pageSize, (pageNum - 1) * pageSize, pageNum, nil
	}

	// limit and offset keep their old leniency: a malformed or out-of-range
	// value falls back to the default.
	limit = complianceEventsDefaultPageSize
	if v, convErr := strconv.Atoi(c.Query("limit")); convErr == nil && v > 0 {
		limit = min(v, complianceEventsMaxPageSize)
	}
	if v, convErr := strconv.Atoi(c.Query("offset")); convErr == nil && v >= 0 {
		offset = v
	}
	return limit, offset, offset/limit + 1, nil
}

// complianceEventsByRequestResponse is the by-request lookup's answer.
type complianceEventsByRequestResponse struct {
	Events []compliance.ComplianceEventRecord `json:"events"`
}

// getComplianceEventsByRequest godoc
// @Summary Find the compliance events of one gateway request
// @Description The App's compliance events that record request_id (the gateway's X-Tyk-AI-Request-ID) in their metadata, recorded within 10 minutes of at, oldest first, at most 20 (Enterprise). An agent trace's guardrail outcome links to them.
// @Tags Compliance
// @Produce json
// @Param request_id query string true "Gateway request id (tyai_ followed by 32 hex digits)"
// @Param app_id query int true "App ID"
// @Param at query string true "When the request ran (RFC 3339)"
// @Success 200 {object} complianceEventsByRequestResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /compliance/events-by-request [get]
// @Security BearerAuth
func (a *API) getComplianceEventsByRequest(c *gin.Context) {
	fail := func(code int, title, detail string) {
		c.JSON(code, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: title, Detail: detail}},
		})
	}
	appID, err := strconv.ParseUint(c.Query("app_id"), 10, 32)
	if err != nil || appID == 0 {
		fail(http.StatusBadRequest, "Bad Request", "app_id must be a positive integer")
		return
	}
	at, err := time.Parse(time.RFC3339Nano, c.Query("at"))
	if err != nil {
		fail(http.StatusBadRequest, "Bad Request", "at must be an RFC 3339 time")
		return
	}
	events, err := compliance.EventsByRequest(a.service.DB, uint(appID), c.Query("request_id"), at)
	switch {
	case errors.Is(err, compliance.ErrInvalidRequestID):
		fail(http.StatusBadRequest, "Bad Request", err.Error())
	case errors.Is(err, compliance.ErrEnterpriseFeature):
		fail(http.StatusForbidden, "Enterprise Feature", err.Error())
	case err != nil:
		fail(http.StatusInternalServerError, "Internal Server Error", "Failed to find the compliance events")
	default:
		c.JSON(http.StatusOK, complianceEventsByRequestResponse{Events: events})
	}
}

// getComplianceEvent godoc
// @Summary Get one compliance event
// @Description Get a script-reported compliance event by id, as the events list returns it (Enterprise). Guardrail alerts and outcome events link to it.
// @Tags Compliance
// @Produce json
// @Param id path int true "Compliance event ID"
// @Success 200 {object} compliance.ComplianceEventRecord
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /compliance/events/{id} [get]
// @Security BearerAuth
func (a *API) getComplianceEvent(c *gin.Context) {
	fail := func(code int, title, detail string) {
		c.JSON(code, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: title, Detail: detail}},
		})
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		fail(http.StatusBadRequest, "Bad Request", "id must be a positive integer")
		return
	}
	rec, err := compliance.EventRecord(a.service.DB, uint(id))
	switch {
	case errors.Is(err, compliance.ErrEnterpriseFeature):
		fail(http.StatusForbidden, "Enterprise Feature", err.Error())
	case errors.Is(err, compliance.ErrEventNotFound):
		fail(http.StatusNotFound, "Not Found", "Compliance event not found")
	case err != nil:
		fail(http.StatusInternalServerError, "Internal Server Error", "Failed to get the compliance event")
	default:
		c.JSON(http.StatusOK, rec)
	}
}
