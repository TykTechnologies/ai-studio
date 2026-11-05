package entitlements

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// communityService limits nothing: every change runs as it would without
// the hook.
type communityService struct{}

// NewCommunityService returns the stub that allows everything. Tests of
// core call sites use it to assert unchanged behaviour.
func NewCommunityService() Service {
	return communityService{}
}

func (communityService) GuardAppCreate(tx *gorm.DB, _ uint, _ bool, fn func(*gorm.DB) error) error {
	return fn(tx)
}

func (communityService) GuardUserChange(tx *gorm.DB, fn func(*gorm.DB) error) error {
	return fn(tx)
}

func (communityService) AdmitNamespace(*gorm.DB, string) (bool, error) { return true, nil }

func (communityService) IsNamespaceLicensed(*gorm.DB, string) bool { return true }

func (communityService) LicensedNamespaceScope(q *gorm.DB) *gorm.DB { return q }

func (communityService) ReleaseNamespace(*gorm.DB, string) error { return ErrEnterpriseFeature }

func (communityService) MoveNamespace(*gorm.DB, string, string, string) (bool, error) {
	return true, nil
}

func (communityService) Sweep(*gorm.DB) error { return nil }

func (communityService) OnNamespacesAdmitted(func([]string)) {}

func (communityService) CanGrant(*gorm.DB, Seat) bool { return true }

func (communityService) Report(*gorm.DB) (*Report, error) { return nil, ErrEnterpriseFeature }

func (communityService) Notices(*gorm.DB, *models.User) ([]Notice, error) {
	return nil, ErrEnterpriseFeature
}

func (communityService) DismissNotice(*gorm.DB, *models.User, string) error {
	return ErrEnterpriseFeature
}

func (communityService) GetTeamAppQuota(*gorm.DB, uint) (*TeamAppQuota, error) {
	return nil, ErrEnterpriseFeature
}

func (communityService) SetTeamAppQuota(*gorm.DB, uint, TeamAppQuotaInput) error {
	return ErrEnterpriseFeature
}

func (communityService) DeleteTeamAppQuota(*gorm.DB, uint) error { return ErrEnterpriseFeature }

func (communityService) UserAppQuota(*gorm.DB, uint) (int64, int64, bool, error) {
	return 0, 0, false, ErrEnterpriseFeature
}
