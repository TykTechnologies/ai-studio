package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// MCP analytics: calls Tyk Gateways proxied to MCP servers Studio brokers,
// ingested from Tyk Pump (see api/tyk_ingest_handlers.go).

// MCPPageMeta is the paging block of an MCP analytics list.
type MCPPageMeta struct {
	TotalCount int64 `json:"total_count"`
	TotalPages int   `json:"total_pages"`
	PageSize   int   `json:"page_size"`
	PageNumber int   `json:"page_number"`
}

// MCPToolStatsPage is one page of a server's tool statistics.
type MCPToolStatsPage struct {
	Data []analytics.MCPToolRow `json:"data"`
	Meta MCPPageMeta            `json:"meta"`
}

// MCPAppStatsPage is one page of the Apps calling a server.
type MCPAppStatsPage struct {
	Data []analytics.MCPAppRow `json:"data"`
	Meta MCPPageMeta           `json:"meta"`
}

// MCPAppUsage is an App's MCP traffic, per server.
type MCPAppUsage struct {
	Servers []analytics.MCPServerRow `json:"servers"`
}

// mcpStatsFilter reads start_date and end_date (YYYY-MM-DD, end inclusive),
// answering 400 itself when they are missing or malformed.
func mcpStatsFilter(c *gin.Context) (analytics.MCPStatsFilter, bool) {
	start, end, err := getDateRange(c)
	if err != nil {
		webhookBadRequest(c, err.Error())
		return analytics.MCPStatsFilter{}, false
	}
	return analytics.MCPStatsFilter{Start: start, End: end}, true
}

// mcpStatsServerFilter adds the :id server to the date range.
func mcpStatsServerFilter(c *gin.Context) (analytics.MCPStatsFilter, bool) {
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return analytics.MCPStatsFilter{}, false
	}
	f, ok := mcpStatsFilter(c)
	f.ServerID = &id
	return f, ok
}

func mcpPaging(c *gin.Context) (int, int) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	return page, pageSize
}

func mcpPageMeta(total int64, page, pageSize int) MCPPageMeta {
	return MCPPageMeta{TotalCount: total, TotalPages: int((total + int64(pageSize) - 1) / int64(pageSize)), PageSize: pageSize, PageNumber: page}
}

func mcpStatsFailed(c *gin.Context, what string, err error) {
	logger.Warnf("MCP analytics: %s failed: %v", what, err)
	webhookError(c, http.StatusInternalServerError, "Internal Server Error", "Failed to get "+what)
}

// getMCPAnalyticsOverview godoc
// @Summary MCP traffic overview
// @Description Calls, errors, latency and the busiest tools across every MCP server, from Tyk Gateway analytics. An error is an HTTP status >= 400 or a JSON-RPC error.
// @Tags Analytics
// @Produce json
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD, inclusive)"
// @Success 200 {object} analytics.MCPOverview
// @Failure 400 {object} models.ErrorResponse
// @Router /analytics/mcp/overview [get]
func (a *API) getMCPAnalyticsOverview(c *gin.Context) {
	f, ok := mcpStatsFilter(c)
	if !ok {
		return
	}
	out, err := analytics.GetMCPOverview(a.service.DB, f)
	if err != nil {
		mcpStatsFailed(c, "the MCP overview", err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// getMCPServerSummary godoc
// @Summary MCP server usage summary
// @Tags Analytics
// @Produce json
// @Param id path int true "MCP server ID"
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD, inclusive)"
// @Success 200 {object} analytics.MCPSummary
// @Failure 400 {object} models.ErrorResponse
// @Router /analytics/mcp/servers/{id}/summary [get]
func (a *API) getMCPServerSummary(c *gin.Context) {
	f, ok := mcpStatsServerFilter(c)
	if !ok {
		return
	}
	out, err := analytics.GetMCPSummary(a.service.DB, f)
	if err != nil {
		mcpStatsFailed(c, "the MCP server summary", err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// getMCPServerTimeseries godoc
// @Summary MCP server calls, errors and latency over time
// @Description Buckets with no calls are omitted.
// @Tags Analytics
// @Produce json
// @Param id path int true "MCP server ID"
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD, inclusive)"
// @Param interval query string false "hour or day (default day)"
// @Success 200 {object} analytics.MCPTimeseries
// @Failure 400 {object} models.ErrorResponse
// @Router /analytics/mcp/servers/{id}/timeseries [get]
func (a *API) getMCPServerTimeseries(c *gin.Context) {
	f, ok := mcpStatsServerFilter(c)
	if !ok {
		return
	}
	interval := c.DefaultQuery("interval", "day")
	if !analytics.MCPIntervals[interval] {
		webhookBadRequest(c, "interval must be hour or day")
		return
	}
	out, err := analytics.GetMCPTimeseries(a.service.DB, f, interval)
	if err != nil {
		mcpStatsFailed(c, "the MCP server time series", err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// getMCPServerTools godoc
// @Summary MCP server per-tool usage
// @Description One page of the server's tools (and resources, prompts, discovery calls), busiest first.
// @Tags Analytics
// @Produce json
// @Param id path int true "MCP server ID"
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD, inclusive)"
// @Param page query int false "Page (default 1)"
// @Param page_size query int false "Page size (default 10, max 100)"
// @Param search query string false "Filter by tool name"
// @Success 200 {object} MCPToolStatsPage
// @Failure 400 {object} models.ErrorResponse
// @Router /analytics/mcp/servers/{id}/tools [get]
func (a *API) getMCPServerTools(c *gin.Context) {
	f, ok := mcpStatsServerFilter(c)
	if !ok {
		return
	}
	page, size := mcpPaging(c)
	rows, total, err := analytics.GetMCPToolStats(a.service.DB, f, page, size, c.Query("search"))
	if err != nil {
		mcpStatsFailed(c, "the MCP server tools", err)
		return
	}
	c.JSON(http.StatusOK, MCPToolStatsPage{Data: rows, Meta: mcpPageMeta(total, page, size)})
}

// getMCPServerApps godoc
// @Summary Apps calling an MCP server
// @Description One page of the Apps calling the server, busiest first. A row with a null app_id is the traffic no App could be attributed to.
// @Tags Analytics
// @Produce json
// @Param id path int true "MCP server ID"
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD, inclusive)"
// @Param page query int false "Page (default 1)"
// @Param page_size query int false "Page size (default 10, max 100)"
// @Param search query string false "Filter by App name"
// @Success 200 {object} MCPAppStatsPage
// @Failure 400 {object} models.ErrorResponse
// @Router /analytics/mcp/servers/{id}/apps [get]
func (a *API) getMCPServerApps(c *gin.Context) {
	f, ok := mcpStatsServerFilter(c)
	if !ok {
		return
	}
	page, size := mcpPaging(c)
	rows, total, err := analytics.GetMCPAppStats(a.service.DB, f, page, size, c.Query("search"))
	if err != nil {
		mcpStatsFailed(c, "the MCP server apps", err)
		return
	}
	c.JSON(http.StatusOK, MCPAppStatsPage{Data: rows, Meta: mcpPageMeta(total, page, size)})
}

// getMCPAppUsage godoc
// @Summary An App's MCP usage
// @Description The App's calls to each MCP server, busiest first.
// @Tags Analytics
// @Produce json
// @Param id path int true "App ID"
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD, inclusive)"
// @Success 200 {object} MCPAppUsage
// @Failure 400 {object} models.ErrorResponse
// @Router /analytics/mcp/apps/{id} [get]
func (a *API) getMCPAppUsage(c *gin.Context) {
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	a.writeMCPAppUsage(c, id)
}

func (a *API) writeMCPAppUsage(c *gin.Context, appID uint) {
	f, ok := mcpStatsFilter(c)
	if !ok {
		return
	}
	f.AppID = &appID
	rows, err := analytics.GetMCPServerStats(a.service.DB, f)
	if err != nil {
		mcpStatsFailed(c, "the App's MCP usage", err)
		return
	}
	c.JSON(http.StatusOK, MCPAppUsage{Servers: rows})
}

// getUserAppMCPUsage godoc
// @Summary MCP usage of the caller's App
// @Description The App's calls to each MCP server through Tyk Gateways. Only the App's owner may read it.
// @Tags portal-analytics
// @Produce json
// @Param id path int true "App ID"
// @Param start_date query string true "Start date (YYYY-MM-DD)"
// @Param end_date query string true "End date (YYYY-MM-DD, inclusive)"
// @Success 200 {object} MCPAppUsage
// @Failure 400 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /common/apps/{id}/analytics/mcp [get]
func (a *API) getUserAppMCPUsage(c *gin.Context) {
	u, exists := c.Get("user")
	user, _ := u.(*models.User)
	if !exists || user == nil {
		webhookError(c, http.StatusUnauthorized, "Unauthorized", "User not found in context")
		return
	}
	id, ok := tykMCPIDParam(c, "id")
	if !ok {
		return
	}
	app, err := a.service.GetAppByID(id)
	if err != nil {
		webhookError(c, http.StatusNotFound, "Not Found", "App not found")
		return
	}
	// The same rule as the App's other portal analytics: its owner only.
	if app.UserID != user.ID {
		webhookError(c, http.StatusForbidden, "Forbidden", "You don't have permission to access this app's analytics")
		return
	}
	a.writeMCPAppUsage(c, app.ID)
}
