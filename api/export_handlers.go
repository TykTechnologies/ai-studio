package api

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/log_export"
	"github.com/gin-gonic/gin"
)

// ExportRequest represents the request body for starting an export
type ExportRequest struct {
	SourceType string `json:"source_type" binding:"required,oneof=app llm user"`
	SourceID   uint   `json:"source_id" binding:"required"`
	StartDate  string `json:"start_date" binding:"required"`
	EndDate    string `json:"end_date" binding:"required"`
	Search     string `json:"search"`
}

// startExport godoc
// @Summary Start a proxy log export (Enterprise)
// @Description Initiates a background job to export proxy logs for an app or LLM
// @Tags Exports
// @Accept json
// @Produce json
// @Param request body ExportRequest true "Export request parameters"
// @Success 202 {object} models.ProxyLogExportResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 402 {object} models.ErrorResponse "Enterprise feature required"
// @Failure 403 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Security BearerAuth
// @Router /exports [post]
func (a *API) startExport(c *gin.Context) {
	// Check if enterprise feature is available
	if !log_export.IsEnterpriseAvailable() {
		c.JSON(http.StatusPaymentRequired, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{
				Title:  "Enterprise Feature Required",
				Detail: "Proxy log export is an Enterprise Edition feature. Visit https://tyk.io/ai-studio/pricing for more information.",
			}},
		})
		return
	}

	// Get current user
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "User not authenticated"}},
		})
		return
	}

	currentUser := user.(*models.User)

	// Only admins can export logs
	if !currentUser.IsAdmin {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Forbidden", Detail: "Only administrators can export proxy logs"}},
		})
		return
	}

	var req ExportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	// Parse dates
	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid start_date format. Use YYYY-MM-DD"}},
		})
		return
	}

	endDate, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid end_date format. Use YYYY-MM-DD"}},
		})
		return
	}

	// End date should include the full day
	endDate = endDate.Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	// Convert source type
	var sourceType models.ExportSourceType
	switch req.SourceType {
	case "app":
		sourceType = models.ExportSourceApp
	case "llm":
		sourceType = models.ExportSourceLLM
	case "user":
		sourceType = models.ExportSourceUser
	}

	// Create export request
	exportReq := &log_export.ExportRequest{
		SourceType:   sourceType,
		SourceID:     req.SourceID,
		StartDate:    startDate,
		EndDate:      endDate,
		SearchFilter: req.Search,
		RequestedBy:  currentUser.ID,
	}

	// Start the export
	export, err := a.service.LogExportService.StartExport(c.Request.Context(), exportReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to start export: " + err.Error()}},
		})
		return
	}

	c.JSON(http.StatusAccepted, export.ToResponse())
}

// getExport godoc
// @Summary Get export status (Enterprise)
// @Description Retrieves the status of a proxy log export job
// @Tags Exports
// @Accept json
// @Produce json
// @Param id path string true "Export ID"
// @Success 200 {object} models.ProxyLogExportResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 402 {object} models.ErrorResponse "Enterprise feature required"
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Security BearerAuth
// @Router /exports/{id} [get]
func (a *API) getExport(c *gin.Context) {
	// Check if enterprise feature is available
	if !log_export.IsEnterpriseAvailable() {
		c.JSON(http.StatusPaymentRequired, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{
				Title:  "Enterprise Feature Required",
				Detail: "Proxy log export is an Enterprise Edition feature. Visit https://tyk.io/ai-studio/pricing for more information.",
			}},
		})
		return
	}

	// Get current user
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "User not authenticated"}},
		})
		return
	}

	currentUser := user.(*models.User)

	// Only admins can view exports
	if !currentUser.IsAdmin {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Forbidden", Detail: "Only administrators can view export status"}},
		})
		return
	}

	exportID := c.Param("id")
	if exportID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Export ID is required"}},
		})
		return
	}

	export, err := a.service.LogExportService.GetExport(c.Request.Context(), exportID)
	if err != nil {
		if errors.Is(err, log_export.ErrEnterpriseFeature) {
			c.JSON(http.StatusPaymentRequired, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Enterprise Feature Required", Detail: err.Error()}},
			})
			return
		}

		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Export not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, export.ToResponse())
}

// downloadExport godoc
// @Summary Download export file (Enterprise)
// @Description Downloads the exported proxy logs JSON file
// @Tags Exports
// @Accept json
// @Produce application/json
// @Param id path string true "Export ID"
// @Success 200 {file} file "JSON file containing proxy logs"
// @Failure 401 {object} models.ErrorResponse
// @Failure 402 {object} models.ErrorResponse "Enterprise feature required"
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 410 {object} models.ErrorResponse "Export expired"
// @Failure 500 {object} models.ErrorResponse
// @Security BearerAuth
// @Router /exports/{id}/download [get]
func (a *API) downloadExport(c *gin.Context) {
	// Check if enterprise feature is available
	if !log_export.IsEnterpriseAvailable() {
		c.JSON(http.StatusPaymentRequired, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{
				Title:  "Enterprise Feature Required",
				Detail: "Proxy log export is an Enterprise Edition feature. Visit https://tyk.io/ai-studio/pricing for more information.",
			}},
		})
		return
	}

	// Get current user
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "User not authenticated"}},
		})
		return
	}

	currentUser := user.(*models.User)

	// Only admins can download exports
	if !currentUser.IsAdmin {
		c.JSON(http.StatusForbidden, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Forbidden", Detail: "Only administrators can download exports"}},
		})
		return
	}

	exportID := c.Param("id")
	if exportID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Export ID is required"}},
		})
		return
	}

	filePath, err := a.service.LogExportService.GetDownloadPath(c.Request.Context(), exportID, currentUser.ID)
	if err != nil {
		switch {
		case errors.Is(err, log_export.ErrEnterpriseFeature):
			c.JSON(http.StatusPaymentRequired, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Enterprise Feature Required", Detail: err.Error()}},
			})
		case err.Error() == "export not found":
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "Export not found"}},
			})
		case err.Error() == "export is not ready for download":
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Ready", Detail: "Export is not ready for download. Please wait for completion."}},
			})
		case err.Error() == "export has expired":
			c.JSON(http.StatusGone, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Expired", Detail: "This export has expired and is no longer available for download."}},
			})
		case err.Error() == "unauthorized access to export":
			c.JSON(http.StatusForbidden, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Forbidden", Detail: "You do not have permission to download this export"}},
			})
		default:
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Internal Server Error", Detail: "Failed to retrieve export: " + err.Error()}},
			})
		}
		return
	}

	// Get filename for Content-Disposition header
	filename := fmt.Sprintf("proxy-logs-%s.json", exportID)

	// Set headers for file download
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/json")
	c.Header("Content-Transfer-Encoding", "binary")

	// Serve the file
	c.File(filepath.Clean(filePath))
}
