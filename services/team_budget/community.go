package team_budget

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// communityService allocates and enforces nothing. Apps are still attributed
// to a team (that happens in the core create path), so upgrading to
// Enterprise starts with history already in place.
type communityService struct{}

func newCommunityService() Service {
	return &communityService{}
}

func (s *communityService) Enabled() bool { return false }

func (s *communityService) SetEnabled(bool) error { return ErrEnterpriseFeature }

func (s *communityService) GetTeamBudget(uint) (*models.TeamBudget, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) SetTeamBudget(uint, BudgetInput) (*models.TeamBudget, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) DeleteTeamBudget(uint) error { return nil }

func (s *communityService) ResetTeamBudget(uint) error { return ErrEnterpriseFeature }

func (s *communityService) AllocateForNewApp(*models.App) (bool, error) { return false, nil }

func (s *communityService) ValidateAllocation(*models.App, *float64, *uint) error { return nil }

func (s *communityService) ReleaseAllocation(*models.App) error { return nil }

func (s *communityService) CheckApp(*models.App) error { return nil }

func (s *communityService) AnalyzeTeamUsage(*models.App) {}

func (s *communityService) EdgeBlocks() (map[uint]string, error) { return nil, nil }

func (s *communityService) GetReport(uint) (*Report, error) { return nil, ErrEnterpriseFeature }

func (s *communityService) GetTeamCosts(time.Time, time.Time) (*TeamCosts, error) {
	return nil, ErrEnterpriseFeature
}

func (s *communityService) ClearCache() {}
