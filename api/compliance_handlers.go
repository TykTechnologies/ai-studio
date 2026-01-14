package api

import (
	"fmt"
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
		// Set end date to end of day
		endDate = time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 23, 59, 59, 0, parsedDate.Location())
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
		endDate.Format("2006-01-02"))

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
