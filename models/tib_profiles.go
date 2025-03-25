package models

import (
	"github.com/TykTechnologies/tyk-identity-broker/tap"
	"gorm.io/gorm"
)

// Profile represents an sso profile in the store
type Profile struct {
	gorm.Model                `json:"-"`
	ProfileID                 string `gorm:"index" json:"ID"`
	Name                      string
	OrgID                     string
	ActionType                string
	MatchedPolicyID           string
	Type                      string
	ProviderName              string
	CustomEmailField          string
	CustomUserIDField         string
	ProviderConfig            JSONMap `gorm:"type:json"`
	IdentityHandlerConfig     JSONMap `gorm:"type:json"`
	ProviderConstraintsDomain string
	ProviderConstraintsGroup  string
	ReturnURL                 string
	DefaultUserGroupID        string
	CustomUserGroupField      string
	UserGroupMapping          StringMap `gorm:"type:json"`
	UserGroupSeparator        string
	SSOOnlyForRegisteredUsers bool
	SelectedProviderType      string `json:"-"`
	UserID                    uint   `json:"-"`
	User                      User   `json:"-"`
}

type Profiles []Profile

func NewProfile() *Profile {
	return &Profile{}
}

func (p *Profile) Create(db *gorm.DB) error {
	return db.Create(p).Error
}

func (p *Profile) Get(db *gorm.DB, profileID string) error {
	return db.Where("profile_id = ?", profileID).First(p).Error
}

func (p *Profile) Update(db *gorm.DB) error {
	return db.Save(p).Error
}

func (p *Profile) Delete(db *gorm.DB) error {
	return db.Delete(p).Error
}

func (p *Profiles) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&Profile{})

	if err := query.Count(&totalCount).Error; err != nil {
		return 0, 0, err
	}

	totalPages := int(totalCount) / pageSize
	if int(totalCount)%pageSize != 0 {
		totalPages++
	}

	if !all {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	err := query.Preload("User").Find(p).Error
	return totalCount, totalPages, err
}

func (p *Profile) GetByName(db *gorm.DB, name string) error {
	return db.Where("name = ?", name).First(p).Error
}

// MapToTapProfile fills a tap.Profile with data from the local Profile
func (p *Profile) MapToTapProfile(tapProfile *tap.Profile) {
	tapProfile.ID = p.ProfileID
	tapProfile.Name = p.Name
	tapProfile.OrgID = p.OrgID
	tapProfile.ActionType = tap.Action(p.ActionType)
	tapProfile.MatchedPolicyID = p.MatchedPolicyID
	tapProfile.Type = tap.ProviderType(p.Type)
	tapProfile.ProviderName = p.ProviderName
	tapProfile.CustomEmailField = p.CustomEmailField
	tapProfile.CustomUserIDField = p.CustomUserIDField
	tapProfile.ProviderConfig = p.ProviderConfig
	tapProfile.IdentityHandlerConfig = p.IdentityHandlerConfig
	tapProfile.ProviderConstraints = tap.ProfileConstraint{
		Domain: p.ProviderConstraintsDomain,
		Group:  p.ProviderConstraintsGroup,
	}
	tapProfile.ReturnURL = p.ReturnURL
	tapProfile.DefaultUserGroupID = p.DefaultUserGroupID
	tapProfile.CustomUserGroupField = p.CustomUserGroupField
	tapProfile.UserGroupMapping = p.UserGroupMapping
	tapProfile.UserGroupSeparator = p.UserGroupSeparator
	tapProfile.SSOOnlyForRegisteredUsers = p.SSOOnlyForRegisteredUsers
}
