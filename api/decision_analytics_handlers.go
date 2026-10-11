package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

func analyticsBadRequest(c *gin.Context, detail string) {
	c.JSON(http.StatusBadRequest, models.ErrorResponse{
		Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: detail}},
	})
}

// optionalUintQuery parses an optional numeric query parameter.
func optionalUintQuery(c *gin.Context, name string) (*uint, bool) {
	raw := c.Query(name)
	if raw == "" {
		return nil, true
	}
	id, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		analyticsBadRequest(c, "Invalid "+name)
		return nil, false
	}
	v := uint(id)
	return &v, true
}

// getDecisionUsage godoc
// @Summary Get decision-model usage
// @Description Decision-model traffic (/v1/decisions, /v1/systemone) for a period: totals, a per-day series and a per-model breakdown with cost per thousand decisions. Decision spend is also included in the general cost endpoints.
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
// @Param llm_id query int false "Only this LLM"
// @Param app_id query int false "Only this App"
// @Success 200 {object} analytics.DecisionUsage
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/decisions [get]
// @Security BearerAuth
func (a *API) getDecisionUsage(c *gin.Context) {
	startDate, endDate, err := getDateRange(c)
	if err != nil {
		analyticsBadRequest(c, err.Error())
		return
	}
	llmID, ok := optionalUintQuery(c, "llm_id")
	if !ok {
		return
	}
	appID, ok := optionalUintQuery(c, "app_id")
	if !ok {
		return
	}

	usage, err := analytics.GetDecisionUsage(a.service.DB, startDate, endDate, llmID, appID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get decision usage"}},
		})
		return
	}
	c.JSON(http.StatusOK, usage)
}
