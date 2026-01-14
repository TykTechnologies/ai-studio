package compliance

import (
	"errors"
	"time"
)

var (
	// ErrEnterpriseFeature is returned when attempting to use enterprise-only features in CE
	ErrEnterpriseFeature = errors.New("compliance monitoring is an Enterprise Edition feature - visit https://tyk.io/ai-studio/pricing for more information")
)

// communityService is a stub implementation of the compliance service for Community Edition.
// It returns enterprise feature errors for all methods.
type communityService struct{}

// newCommunityService creates a new community edition compliance service stub.
func newCommunityService() Service {
	return &communityService{}
}

// GetSummary returns an enterprise feature error in Community Edition.
func (s *communityService) GetSummary(startDate, endDate time.Time) (*ComplianceSummary, error) {
	return nil, ErrEnterpriseFeature
}

// GetHighRiskApps returns an enterprise feature error in Community Edition.
func (s *communityService) GetHighRiskApps(startDate, endDate time.Time, limit int) ([]HighRiskApp, error) {
	return nil, ErrEnterpriseFeature
}

// GetAccessIssues returns an enterprise feature error in Community Edition.
func (s *communityService) GetAccessIssues(startDate, endDate time.Time, appID *uint) (*AccessIssuesData, error) {
	return nil, ErrEnterpriseFeature
}

// GetPolicyViolations returns an enterprise feature error in Community Edition.
func (s *communityService) GetPolicyViolations(startDate, endDate time.Time, appID *uint) (*PolicyViolationsData, error) {
	return nil, ErrEnterpriseFeature
}

// GetBudgetAlerts returns an enterprise feature error in Community Edition.
func (s *communityService) GetBudgetAlerts(startDate, endDate time.Time) (*BudgetAlertsData, error) {
	return nil, ErrEnterpriseFeature
}

// GetErrors returns an enterprise feature error in Community Edition.
func (s *communityService) GetErrors(startDate, endDate time.Time, vendor *string) (*ErrorData, error) {
	return nil, ErrEnterpriseFeature
}

// GetAppRiskProfile returns an enterprise feature error in Community Edition.
func (s *communityService) GetAppRiskProfile(appID uint, startDate, endDate time.Time) (*AppRiskProfile, error) {
	return nil, ErrEnterpriseFeature
}

// ExportData returns an enterprise feature error in Community Edition.
func (s *communityService) ExportData(startDate, endDate time.Time, view string) ([]byte, error) {
	return nil, ErrEnterpriseFeature
}

// GetViolationRecords returns an enterprise feature error in Community Edition.
func (s *communityService) GetViolationRecords(startDate, endDate time.Time, appID *uint, limit int) ([]ViolationRecord, error) {
	return nil, ErrEnterpriseFeature
}
