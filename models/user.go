package models

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	SuperAdminID   uint = 1
	RoleSuperAdmin      = "Super Admin"
	RoleAdmin           = "Admin"
	RoleDeveloper       = "Developer"
	RoleChatUser        = "Chat user"
)

// AuthSource records how a user account came to exist. It is set once, at
// creation, and never changes on later logins: a self-registered user who
// now signs in through the identity provider is still "local" in origin.
const (
	AuthSourceLocal = "local" // self-registration
	AuthSourceAdmin = "admin" // created by an administrator through the console or API
	AuthSourceSSO   = "sso"   // provisioned on first login through an identity provider
)

// LoginMethod values recorded in LastLoginMethod.
const (
	LoginMethodPassword = "password"
	LoginMethodSSO      = "sso"
)

// AuthMethodContextKey is the gin context key under which the auth
// middleware records how a request was authenticated, so consumers that
// cannot import the auth package (the audit trail) can still read it.
const (
	AuthMethodContextKey = "auth_method"
	AuthMethodSession    = "session" // browser session cookie
	AuthMethodAPIKey     = "api_key" // user API key (header or ?token=)
)

type User struct {
	gorm.Model
	ID                   uint   `json:"id" gorm:"primaryKey"`
	Email                string `json:"email"`
	Name                 string
	Password             string `json:"password"`
	SessionToken         string
	ResetToken           string
	ResetTokenExpiry     time.Time
	EmailVerified        bool
	VerificationToken    string
	IsAdmin              bool
	ShowPortal           bool
	ShowChat             bool
	AccessToSSOConfig    bool
	SkipQuickStart       bool
	APIKey               string
	NotificationsEnabled bool `json:"notifications_enabled"` // Permission to receive notifications about new users, app requests etc.
	// EmailNotificationsEnabled is the user's own delivery preference: when
	// false, notifications are still recorded for the bell but no email is
	// sent. Defaults on. GORM's default:true turns an explicit false into
	// true on insert, so it is only ever switched off through a column
	// update (SetEmailNotificationsEnabled).
	EmailNotificationsEnabled bool    `json:"email_notifications_enabled" gorm:"default:true"`
	Groups                    []Group `json:"groups" gorm:"many2many:user_groups;"`

	// Provenance and activity. AuthSource is one of the AuthSource*
	// constants; SSOProfileID is the identity provider profile that
	// provisioned the user (or last signed them in, when the origin was
	// not SSO). The NOT NULL defaults matter: AutoMigrate adds these
	// columns to existing rows and a NULL would otherwise fall through
	// every equality filter.
	AuthSource       string     `json:"auth_source" gorm:"size:16;not null;default:'';index"`
	SSOProfileID     string     `json:"sso_profile_id" gorm:"size:64"`
	LastLoginAt      *time.Time `json:"last_login_at"`
	LastLoginMethod  string     `json:"last_login_method" gorm:"size:16"`
	APIKeyLastUsedAt *time.Time `json:"api_key_last_used_at"`

	// Disabled accounts cannot authenticate by any means (session, API
	// key, password, SSO, OAuth) until an administrator re-enables them.
	Disabled   bool       `json:"disabled" gorm:"not null;default:false;index"`
	DisabledAt *time.Time `json:"disabled_at"`

	// Plugin-stored metadata
	Metadata JSONMap `json:"metadata" gorm:"type:json"`
}

type Users []User

// NewUser is the self-registration constructor: it issues an API key and
// stamps the local origin. Admin and SSO creation build the struct directly
// and deliberately issue no key.
func NewUser() *User {
	u := &User{
		ShowPortal: true,
		ShowChat:   true,
		AuthSource: AuthSourceLocal,
	}

	u.GenerateAPIKey()
	return u
}

// StampLogin records a completed interactive login on the struct; the
// caller persists it (SetUserSession's Save, or the SSO transaction).
func (u *User) StampLogin(method string) {
	now := time.Now()
	u.LastLoginAt = &now
	u.LastLoginMethod = method
}

// IsSSOOrigin reports whether the account was provisioned by an identity
// provider.
func (u *User) IsSSOOrigin() bool {
	return u.AuthSource == AuthSourceSSO
}

// TouchAPIKeyUse records that the user's API key authenticated a request.
// It is a column update so it never races a whole-struct Save elsewhere.
func TouchAPIKeyUse(db *gorm.DB, userID uint) error {
	return db.Model(&User{}).Where("id = ?", userID).Update("api_key_last_used_at", time.Now()).Error
}

// RevokeAPIKey clears the user's API key. Column update: see TouchAPIKeyUse.
func RevokeAPIKey(db *gorm.DB, userID uint) error {
	return db.Model(&User{}).Where("id = ?", userID).Update("api_key", "").Error
}

// SetEmailNotificationsEnabled stores the user's email delivery preference.
// Column update: a Save of a struct carrying false would work, but an
// insert would not (default:true), so every writer goes through here.
func SetEmailNotificationsEnabled(db *gorm.DB, userID uint, enabled bool) error {
	return db.Model(&User{}).Where("id = ?", userID).Update("email_notifications_enabled", enabled).Error
}

// SetNotificationsEnabled stores the in-app (admin fan-out) notification
// flag. Column update: see TouchAPIKeyUse.
func SetNotificationsEnabled(db *gorm.DB, userID uint, enabled bool) error {
	return db.Model(&User{}).Where("id = ?", userID).Update("notifications_enabled", enabled).Error
}

// SetDisabled flips the account switch. Disabling also drops the live
// session and any pending password reset so the lock-out is immediate.
func SetDisabled(db *gorm.DB, userID uint, disabled bool) error {
	updates := map[string]interface{}{
		"disabled":    disabled,
		"disabled_at": gorm.Expr("NULL"),
	}
	if disabled {
		updates["disabled_at"] = time.Now()
		updates["session_token"] = ""
		updates["reset_token"] = ""
	}
	return db.Model(&User{}).Where("id = ?", userID).Updates(updates).Error
}

// BackfillAuthSource classifies rows created before AuthSource existed.
// Only rows with an empty (or NULL, on Postgres) auth_source are touched,
// so it is idempotent and safe to run on every start. The order matters:
// SSO-provisioned users are the only ones with no password hash; admin-
// created users have a password but were never issued a key; everyone
// else registered. An admin-created user whose key was later rolled, or
// an SSO user who set a password through the reset flow, is misread as
// "local"; administrators can correct auth_source through the API.
func BackfillAuthSource(db *gorm.DB) error {
	unset := "COALESCE(auth_source, '') = ''"
	steps := []struct {
		where  string
		source string
	}{
		{"COALESCE(password, '') = ''", AuthSourceSSO},
		{"COALESCE(api_key, '') = ''", AuthSourceAdmin},
		{"1 = 1", AuthSourceLocal},
	}
	for _, step := range steps {
		if err := db.Model(&User{}).Where(unset).Where(step.where).
			Update("auth_source", step.source).Error; err != nil {
			return fmt.Errorf("backfill auth_source=%s: %w", step.source, err)
		}
	}
	return nil
}

// AdminFlag exposes IsAdmin to packages that cannot import models (see
// pkg/authz.AdminFlagged).
func (u *User) AdminFlag() bool {
	return u != nil && u.IsAdmin
}

func (u *User) GenerateAPIKey() error {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		return err
	}
	u.APIKey = base64.URLEncoding.EncodeToString(key)
	return nil
}

func (u *User) Get(db *gorm.DB, id uint, preloads ...string) error {
	query := db.Model(u)

	for _, preload := range preloads {
		query = query.Preload(preload)
	}

	return query.First(u, id).Error
}

// GetByAPIKey looks a user up by API key. An empty key never matches: users
// created by an administrator or through SSO have no key, and a blank
// comparison would otherwise select the first of them.
func (u *User) GetByAPIKey(db *gorm.DB, apiKey string) error {
	if apiKey == "" {
		return gorm.ErrRecordNotFound
	}
	return db.Where("api_key = ?", apiKey).First(u).Error
}

func (u *User) Create(db *gorm.DB) error {
	return db.Create(u).Error
}

func (u *User) Update(db *gorm.DB) error {
	return db.Save(u).Error
}

func (u *User) Delete(db *gorm.DB) error {
	return db.Delete(u).Error
}

func (u *User) GetByEmail(db *gorm.DB, email string) error {
	return db.Where("email = ?", email).First(u).Error
}

func (u *User) DoesPasswordMatch(password string) bool {
	// hash the password using bcrypt and compare it with the hashed password in the database
	return IsPasswordValid(password, u.Password)
}

func (u *User) SetPassword(password string) error {
	// hash the password using bcrypt
	hashed, err := HashPassword(password)
	if err != nil {
		return err
	}

	u.Password = hashed
	return nil
}

func (u *Users) GetByGroupID(db *gorm.DB, groupID uint) error {
	return db.Joins("JOIN user_groups ON user_groups.user_id = users.id").Where("user_groups.group_id = ?", groupID).Find(u).Error
}

func (u *Users) SearchByEmailStub(db *gorm.DB, emailStub string) error {
	return db.Where("email LIKE ?", emailStub+"%").Find(u).Error
}

func (u *User) GetAccessibleCatalogues(db *gorm.DB) ([]Catalogue, error) {
	var catalogues []Catalogue
	err := db.Table("catalogues").
		Joins("JOIN group_catalogues ON group_catalogues.catalogue_id = catalogues.id").
		Joins("JOIN user_groups ON user_groups.group_id = group_catalogues.group_id").
		Where("user_groups.user_id = ?", u.ID).
		Distinct().
		Find(&catalogues).Error
	return catalogues, err
}

func (u *User) GetAccessibleDataCatalogues(db *gorm.DB) ([]DataCatalogue, error) {
	var dataCatalogues []DataCatalogue
	err := db.Table("data_catalogues").
		Joins("JOIN group_datacatalogues ON group_datacatalogues.data_catalogue_id = data_catalogues.id").
		Joins("JOIN user_groups ON user_groups.group_id = group_datacatalogues.group_id").
		Where("user_groups.user_id = ?", u.ID).
		Distinct().
		Find(&dataCatalogues).Error
	return dataCatalogues, err
}

func (u *User) GetAccessibleToolCatalogues(db *gorm.DB) ([]ToolCatalogue, error) {
	var toolCatalogues []ToolCatalogue
	err := db.Table("tool_catalogues").
		Joins("JOIN group_toolcatalogues ON group_toolcatalogues.tool_catalogue_id = tool_catalogues.id").
		Joins("JOIN user_groups ON user_groups.group_id = group_toolcatalogues.group_id").
		Where("user_groups.user_id = ?", u.ID).
		Distinct().
		Find(&toolCatalogues).Error
	return toolCatalogues, err
}

func (u *User) GetAccessibleDataSources(db *gorm.DB) ([]Datasource, error) {
	var dataSources []Datasource
	err := db.Joins("JOIN data_catalogue_data_sources ON data_catalogue_data_sources.datasource_id = datasources.id").
		Joins("JOIN data_catalogues ON data_catalogues.id = data_catalogue_data_sources.data_catalogue_id").
		Joins("JOIN group_datacatalogues ON group_datacatalogues.data_catalogue_id = data_catalogues.id").
		Joins("JOIN user_groups ON user_groups.group_id = group_datacatalogues.group_id").
		Where("user_groups.user_id = ? AND datasources.active = ?", u.ID, true).
		Group("datasources.id").
		Find(&dataSources).Error
	return dataSources, err
}

func (u *User) GetAccessibleLLMs(db *gorm.DB) ([]LLM, error) {
	var llms []LLM
	err := db.Joins("JOIN catalogue_llms ON catalogue_llms.llm_id = llms.id").
		Joins("JOIN catalogues ON catalogues.id = catalogue_llms.catalogue_id").
		Joins("JOIN group_catalogues ON group_catalogues.catalogue_id = catalogues.id").
		Joins("JOIN user_groups ON user_groups.group_id = group_catalogues.group_id").
		Where("user_groups.user_id = ? AND llms.active = ?", u.ID, true).
		Group("llms.id").
		Find(&llms).Error
	return llms, err
}

func (u *User) GetAccessibleTools(db *gorm.DB) ([]Tool, error) {
	var tools []Tool
	err := db.Table("tools").
		Joins("JOIN tool_catalogue_tools ON tool_catalogue_tools.tool_id = tools.id").
		Joins("JOIN tool_catalogues ON tool_catalogues.id = tool_catalogue_tools.tool_catalogue_id").
		Joins("JOIN group_toolcatalogues ON group_toolcatalogues.tool_catalogue_id = tool_catalogues.id").
		Joins("JOIN user_groups ON user_groups.group_id = group_toolcatalogues.group_id").
		Where("user_groups.user_id = ?", u.ID).
		Group("tools.id").
		Find(&tools).Error
	return tools, err
}

func (u *Users) CountActive(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&User{}).Where("deleted_at IS NULL").Count(&count).Error
	return count, err
}

func (u *User) UpdateGroupMemberships(db *gorm.DB, groupIDs ...string) error {
	var groupUintIDs []uint
	for _, idStr := range groupIDs {
		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid group ID: %s", idStr)
		}
		groupUintIDs = append(groupUintIDs, uint(id))
	}

	var groups []Group
	if err := db.Where("id IN ?", groupUintIDs).Find(&groups).Error; err != nil {
		return fmt.Errorf("failed to find groups: %w", err)
	}

	if err := db.Model(u).Association("Groups").Replace(groups); err != nil {
		return fmt.Errorf("failed to update user group memberships: %w", err)
	}

	return nil
}

func (u *User) ParseGroupAssociations(groupIDs []uint) {
	u.Groups = make([]Group, 0, len(groupIDs))

	for _, groupID := range groupIDs {
		u.Groups = append(u.Groups, Group{ID: groupID})
	}
}

func (u *User) ExtractGroupIDs() []uint {
	groupIDs := make([]uint, len(u.Groups))

	for i, group := range u.Groups {
		groupIDs[i] = group.ID
	}

	return groupIDs
}

func (u *User) GetGroupsToUpdate(groupIDs []uint) []Group {
	currentGroupIDs := u.ExtractGroupIDs()

	if SameIDs(currentGroupIDs, groupIDs) {
		return nil
	}

	u.ParseGroupAssociations(groupIDs)

	return u.Groups
}

func (u *User) ReplaceGroupAssociation(db *gorm.DB, groups []Group) error {
	return db.Model(u).Association("Groups").Replace(groups)
}

func (u *User) DeleteGroupAssociation(db *gorm.DB) error {
	return db.Model(u).Association("Groups").Clear()
}

func IsEmailUnique(db *gorm.DB, email string, userID uint) (bool, error) {
	email = strings.ToLower(email)

	var count int64
	query := db.Model(&User{}).Where("LOWER(email) = ?", email)

	if userID != 0 {
		query = query.Where("id != ?", userID)
	}

	if err := query.Count(&count).Error; err != nil {
		return false, err
	}

	return count == 0, nil
}

func SetSkipQuickStartForUser(db *gorm.DB, userID uint) error {
	return db.Model(&User{}).Where("id = ?", userID).Update("skip_quick_start", true).Error
}

type UserCounts struct {
	UserCount      int64
	AdminCount     int64
	DeveloperCount int64
	ChatUserCount  int64
}

func GetUserCounts(db *gorm.DB) (UserCounts, error) {
	var results UserCounts
	err := db.Model(&User{}).
		Select(`
			COUNT(*) as user_count,
			SUM(CASE WHEN is_admin = true THEN 1 ELSE 0 END) as admin_count,
			SUM(CASE WHEN is_admin = false AND show_portal = true THEN 1 ELSE 0 END) as developer_count,
			SUM(CASE WHEN is_admin = false AND show_portal = false AND show_chat = true THEN 1 ELSE 0 END) as chat_user_count
		`).
		Scan(&results).Error

	return results, err
}

func GetUserGroupCount(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&Group{}).Count(&count).Error

	return count, err
}

func (u *User) GetRole() string {
	switch {
	case u.IsAdmin && u.ID == SuperAdminID:
		return RoleSuperAdmin
	case u.IsAdmin:
		return RoleAdmin
	case u.ShowPortal:
		return RoleDeveloper
	default:
		return RoleChatUser
	}
}

func (u *Users) GetGroupUsersPaginated(db *gorm.DB, groupID uint, pageSize, pageNumber int, all bool) (int64, int, error) {
	query := db.Model(&User{}).
		Joins("JOIN user_groups ON user_groups.user_id = users.id").
		Where("user_groups.group_id = ?", groupID)

	query, totalCount, totalPages, err := PaginateAndSort(query, pageSize, pageNumber, all, "id")
	if err != nil {
		return 0, 0, err
	}

	err = query.Find(u).Error
	return totalCount, totalPages, err
}

type UserQueryParams struct {
	Search         string
	ExcludeGroupID uint
	PageSize       int
	PageNumber     int
	All            bool
	Sort           string

	// Optional filters; nil / empty means "any".
	AuthSource string
	HasAPIKey  *bool
	Disabled   *bool
}

func (u *Users) QueryUsers(db *gorm.DB, params UserQueryParams) (int64, int, error) {
	query := db.Model(&User{})

	if params.Search != "" {
		searchTerm := "%" + params.Search + "%"
		query = query.Where("email LIKE ? OR name LIKE ?", searchTerm, searchTerm)
	}

	if params.AuthSource != "" {
		query = query.Where("auth_source = ?", params.AuthSource)
	}

	if params.HasAPIKey != nil {
		if *params.HasAPIKey {
			query = query.Where("COALESCE(api_key, '') <> ''")
		} else {
			query = query.Where("COALESCE(api_key, '') = ''")
		}
	}

	if params.Disabled != nil {
		query = query.Where("disabled = ?", *params.Disabled)
	}

	if params.ExcludeGroupID > 0 {
		query = query.Joins("LEFT JOIN user_groups ON user_groups.user_id = users.id AND user_groups.group_id = ?", params.ExcludeGroupID).
			Where("user_groups.group_id IS NULL")
	}

	query, totalCount, totalPages, err := PaginateAndSort(query, params.PageSize, params.PageNumber, params.All, params.Sort)
	if err != nil {
		return 0, 0, err
	}

	err = query.Find(u).Error
	return totalCount, totalPages, err
}
