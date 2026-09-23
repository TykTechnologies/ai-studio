package services

import (
	"errors"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/team_budget"
	"gorm.io/gorm"
)

// AppOption adjusts an App being created or updated.
type AppOption func(*appOptions)

type appOptions struct {
	teamID *uint
}

// WithAppTeam attributes the App to the given team instead of the owner's
// resolved budget team (administrators only).
func WithAppTeam(teamID uint) AppOption {
	return func(o *appOptions) { o.teamID = &teamID }
}

func collectAppOptions(opts []AppOption) appOptions {
	var o appOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}

// validateAppTeam checks that an explicitly chosen team exists.
func (s *Service) validateAppTeam(teamID uint) error {
	var n int64
	if err := s.DB.Model(&models.Group{}).Where("id = ?", teamID).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return helpers.NewBadRequestError(fmt.Sprintf("team %d does not exist", teamID))
	}
	return nil
}

// attributeNewApp stamps a new App with its team and, when that team's budget
// is managed, draws the App's budget from the team pool. requested is the
// budget the caller asked for (nil when none). It reports whether the budget
// came from the team, in which case no other default applies.
func (s *Service) attributeNewApp(app *models.App, o appOptions) (bool, error) {
	if o.teamID != nil {
		if err := s.validateAppTeam(*o.teamID); err != nil {
			return false, err
		}
		app.TeamID = o.teamID
	} else {
		teamID, err := models.ResolveBudgetTeam(s.DB, app.UserID)
		if err != nil {
			return false, fmt.Errorf("resolving the app's team: %w", err)
		}
		app.TeamID = teamID
	}
	if app.TeamID == nil || s.TeamBudget == nil {
		return false, nil
	}
	fromTeam, err := s.TeamBudget.AllocateForNewApp(app)
	if err != nil {
		return false, teamBudgetError(err)
	}
	return fromTeam, nil
}

// teamBudgetError maps team budget validation failures onto 400s.
func teamBudgetError(err error) error {
	if errors.Is(err, team_budget.ErrAllocationExceedsPool) || errors.Is(err, team_budget.ErrValidation) {
		return helpers.NewBadRequestError(err.Error())
	}
	return err
}

// releaseTeamAllocations returns the allocations of Apps that live on without
// an owner to their team pools. Best effort: a failure leaves the allocation
// in place, which only overstates what the team has handed out.
func (s *Service) releaseTeamAllocations(apps []models.App) {
	if s.TeamBudget == nil {
		return
	}
	for i := range apps {
		if err := s.TeamBudget.ReleaseAllocation(&apps[i]); err != nil {
			fmt.Printf("Error releasing team allocation of app %d: %v\n", apps[i].ID, err)
		}
	}
}

// IsUserInGroup reports whether the user is a member of the team.
func (s *Service) IsUserInGroup(userID, groupID uint) (bool, error) {
	var n int64
	err := s.DB.Table("user_groups").Where("user_id = ? AND group_id = ?", userID, groupID).Count(&n).Error
	return n > 0, err
}

// SetUserBudgetTeam sets (or clears, with nil) the team a user's new Apps are
// attributed to. The user must be a member of the team.
func (s *Service) SetUserBudgetTeam(userID uint, teamID *uint) error {
	if teamID != nil {
		member, err := s.IsUserInGroup(userID, *teamID)
		if err != nil {
			return err
		}
		if !member {
			return helpers.NewBadRequestError("the user is not a member of that team")
		}
	}
	res := s.DB.Model(&models.User{}).Where("id = ?", userID).Update("budget_team_id", teamID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
