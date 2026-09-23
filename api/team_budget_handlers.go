package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/team_budget"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// asHelperError unwraps a service error that already carries its HTTP status.
func asHelperError(err error) (helpers.ErrorResponse, bool) {
	var he helpers.ErrorResponse
	if errors.As(err, &he) {
		return he, true
	}
	return he, false
}

// portalBudgetSource labels an App's budget for the portal: "team" when team
// budgets are on and the App's team hands out budgets from a pool.
func (a *API) portalBudgetSource(app *models.App) string {
	if app.TeamID == nil || app.MonthlyBudget == nil || a.service.TeamBudget == nil || !a.service.TeamBudget.Enabled() {
		return ""
	}
	tb, err := a.service.TeamBudget.GetTeamBudget(*app.TeamID)
	if err != nil || !tb.IsManaged() {
		return ""
	}
	return "team"
}

// teamBudgetErrorResponse maps team budget service errors onto HTTP statuses.
func teamBudgetErrorResponse(c *gin.Context, err error) {
	switch {
	case errors.Is(err, team_budget.ErrEnterpriseFeature):
		helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError(err.Error()))
	case errors.Is(err, team_budget.ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		helpers.SendErrorResponse(c, helpers.NewNotFoundError(err.Error()))
	case errors.Is(err, team_budget.ErrValidation), errors.Is(err, team_budget.ErrAllocationExceedsPool):
		helpers.SendErrorResponse(c, helpers.NewBadRequestError(err.Error()))
	default:
		if he, ok := asHelperError(err); ok {
			helpers.SendErrorResponse(c, he)
			return
		}
		helpers.SendErrorResponse(c, helpers.NewInternalServerError(err.Error()))
	}
}

// TeamBudgetSettingsBody is the global team budget switch.
type TeamBudgetSettingsBody struct {
	Enabled bool `json:"enabled"`
}

// @Summary Get the team budget switch
// @Description Whether team budgets allocate and enforce (Enterprise)
// @Tags team-budgets
// @Produce json
// @Success 200 {object} TeamBudgetSettingsBody
// @Router /team-budgets/settings [get]
// @Security BearerAuth
func (a *API) getTeamBudgetSettings(c *gin.Context) {
	c.JSON(http.StatusOK, TeamBudgetSettingsBody{Enabled: a.service.TeamBudget.Enabled()})
}

// @Summary Set the team budget switch
// @Description Switching on gives the Default team an empty pool (budget 0) unless it has a budget (Enterprise)
// @Tags team-budgets
// @Accept json
// @Produce json
// @Param body body TeamBudgetSettingsBody true "Switch"
// @Success 200 {object} TeamBudgetSettingsBody
// @Failure 402 {object} ErrorResponse
// @Router /team-budgets/settings [put]
// @Security BearerAuth
func (a *API) setTeamBudgetSettings(c *gin.Context) {
	var body TeamBudgetSettingsBody
	if err := c.ShouldBindJSON(&body); err != nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError("malformed request body: "+err.Error()))
		return
	}
	if err := a.service.TeamBudget.SetEnabled(body.Enabled); err != nil {
		teamBudgetErrorResponse(c, err)
		return
	}
	a.service.Budget.ClearCache()
	c.JSON(http.StatusOK, TeamBudgetSettingsBody{Enabled: a.service.TeamBudget.Enabled()})
}

// @Summary Get a team's budget report
// @Description Budget, spend, allocations and flags for the team's current period (Enterprise)
// @Tags team-budgets
// @Produce json
// @Param id path int true "Team ID"
// @Success 200 {object} team_budget.Report
// @Failure 402 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /groups/{id}/budget [get]
// @Security BearerAuth
func (a *API) getTeamBudget(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	report, err := a.service.TeamBudget.GetReport(id)
	if err != nil {
		teamBudgetErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, report)
}

// @Summary Set a team's budget
// @Description A null monthly_budget leaves the team unmanaged; 0 is an empty pool (Enterprise)
// @Tags team-budgets
// @Accept json
// @Produce json
// @Param id path int true "Team ID"
// @Param body body team_budget.BudgetInput true "Budget"
// @Success 200 {object} team_budget.Report
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse
// @Router /groups/{id}/budget [put]
// @Security BearerAuth
func (a *API) setTeamBudget(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var in team_budget.BudgetInput
	if err := c.ShouldBindJSON(&in); err != nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError("malformed request body: "+err.Error()))
		return
	}
	if _, err := a.service.TeamBudget.SetTeamBudget(id, in); err != nil {
		teamBudgetErrorResponse(c, err)
		return
	}
	a.getTeamBudget(c)
}

// @Summary Remove a team's budget
// @Description The team becomes unmanaged; its Apps keep their budgets (Enterprise)
// @Tags team-budgets
// @Param id path int true "Team ID"
// @Success 204
// @Router /groups/{id}/budget [delete]
// @Security BearerAuth
func (a *API) deleteTeamBudget(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if !team_budget.IsEnterpriseAvailable() {
		teamBudgetErrorResponse(c, team_budget.ErrEnterpriseFeature)
		return
	}
	if err := a.service.TeamBudget.DeleteTeamBudget(id); err != nil {
		teamBudgetErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// @Summary Reset a team's budget period
// @Description Starts a new budget period now (Enterprise)
// @Tags team-budgets
// @Param id path int true "Team ID"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Router /groups/{id}/budget/reset [post]
// @Security BearerAuth
func (a *API) resetTeamBudget(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := a.service.TeamBudget.ResetTeamBudget(id); err != nil {
		teamBudgetErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// @Summary Spend per team
// @Description Cost, tokens and requests per team over a window; defaults to the last 30 days (Enterprise)
// @Tags analytics
// @Produce json
// @Param start_date query string false "Start date (YYYY-MM-DD)"
// @Param end_date query string false "End date (YYYY-MM-DD, inclusive)"
// @Success 200 {object} team_budget.TeamCosts
// @Failure 400 {object} ErrorResponse
// @Router /analytics/team-costs [get]
// @Security BearerAuth
func (a *API) getTeamCosts(c *gin.Context) {
	now := time.Now()
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	start := end.AddDate(0, 0, -30)
	if s := c.Query("start_date"); s != "" {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			helpers.SendErrorResponse(c, helpers.NewBadRequestError("start_date must be YYYY-MM-DD"))
			return
		}
		start = t
	}
	if s := c.Query("end_date"); s != "" {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			helpers.SendErrorResponse(c, helpers.NewBadRequestError("end_date must be YYYY-MM-DD"))
			return
		}
		end = t.AddDate(0, 0, 1)
	}
	costs, err := a.service.TeamBudget.GetTeamCosts(start, end)
	if err != nil {
		teamBudgetErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, costs)
}

// UserBudgetTeamBody sets the team a user's new Apps are attributed to.
type UserBudgetTeamBody struct {
	TeamID *uint `json:"team_id"`
}

// @Summary Set a user's budget team
// @Description The team (one the user belongs to) their new Apps and chat spend are attributed to; null clears it
// @Tags users
// @Accept json
// @Param id path int true "User ID"
// @Param body body UserBudgetTeamBody true "Team"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Router /users/{id}/budget-team [put]
// @Security BearerAuth
func (a *API) setUserBudgetTeam(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var body UserBudgetTeamBody
	if err := c.ShouldBindJSON(&body); err != nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError("malformed request body: "+err.Error()))
		return
	}
	if err := a.service.SetUserBudgetTeam(id, body.TeamID); err != nil {
		teamBudgetErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
