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
	// NewUserShowPortal and NewUserShowChat are the provisioning defaults
	// applied to users this profile creates on their first login. They are
	// only consulted at creation time; existing users keep whatever an
	// administrator set. Team membership comes from the claim mapping above.
	NewUserShowPortal    bool
	NewUserShowChat      bool
	SelectedProviderType string `json:"-"`
	UserID               uint   `json:"-"`
	User                 User   `json:"-"`
	UseInLoginPage       bool   `json:"-"`
}

type Profiles []Profile

func NewProfile() *Profile {
	return &Profile{
		NewUserShowPortal: true,
		NewUserShowChat:   true,
	}
}

// MigrateProfiles creates or updates the profiles table. Profiles that
// predate the provisioning defaults are backfilled to "show both surfaces",
// which is what admin-created users get, so upgrading never hides the
// Portal or Chat from newly provisioned SSO users.
func MigrateProfiles(db *gorm.DB) error {
	backfill := db.Migrator().HasTable(&Profile{}) && !db.Migrator().HasColumn(&Profile{}, "NewUserShowPortal")
	if err := db.AutoMigrate(&Profile{}); err != nil {
		return err
	}
	if !backfill {
		return nil
	}
	return db.Model(&Profile{}).Where("1 = 1").Updates(map[string]interface{}{
		"new_user_show_portal": true,
		"new_user_show_chat":   true,
	}).Error
}

// ProvisioningDefaults returns the surface flags a user created through
// this profile starts with. A nil profile (login via a profile that no
// longer exists, or an external broker that does not identify one) falls
// back to the same defaults as NewUser.
func (p *Profile) ProvisioningDefaults() (showPortal, showChat bool) {
	if p == nil {
		return true, true
	}
	return p.NewUserShowPortal, p.NewUserShowChat
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

func (p *Profiles) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool, sort string) (int64, int, error) {
	var totalCount int64
	query := db.Model(&Profile{})

	// Handle sorting
	if sort != "" {
		if sort[0] == '-' {
			query = query.Order(sort[1:] + " DESC")
		} else {
			query = query.Order(sort + " ASC")
		}
	} else {
		query = query.Order("id ASC") // Default sort by ID ascending
	}

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

func ResetUseInLoginPageForAll(db *gorm.DB) error {
	return db.Model(&Profile{}).Where("use_in_login_page = ?", true).Update("use_in_login_page", false).Error
}

func (p *Profile) UpdateUseInLoginPage(db *gorm.DB, value bool) error {
	return db.Model(p).Update("use_in_login_page", value).Error
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

func (p *Profile) GetLoginPageProfile(db *gorm.DB) error {
	return db.Where("use_in_login_page = ?", true).First(p).Error
}
