package api

import (
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
)

var serviceTierQueryPattern = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// @Summary Get usage by service tier
// @Description Spend and tokens for a period broken down by vendor service tier (batch, priority, flex...), per tier and per vendor/model. Records without a tier (list-price traffic, and rows recorded before tiers were stored) have service_tier "". Costs are already at each tier's price. Edge traffic is included.
// @Tags Analytics
// @Accept json
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD)"
// @Param llm_id query int false "Only this LLM"
// @Param app_id query int false "Only this App"
// @Param service_tier query string false "Only this tier; none selects records without a tier"
// @Success 200 {object} analytics.ServiceTierUsage
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /analytics/service-tier-usage [get]
// @Security BearerAuth
func (a *API) getServiceTierUsage(c *gin.Context) {
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
	var tier *string
	if v, present := c.GetQuery("service_tier"); present {
		if !serviceTierQueryPattern.MatchString(v) {
			analyticsBadRequest(c, "Invalid service_tier")
			return
		}
		tier = &v
	}

	usage, err := analytics.GetServiceTierUsage(a.service.DB, startDate, endDate, llmID, appID, tier)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Failed to get service tier usage"}},
		})
		return
	}
	c.JSON(http.StatusOK, usage)
}
