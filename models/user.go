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
	NotificationsEnabled bool    `json:"notifications_enabled"` // Permission to receive notifications about new users, app requests etc.
	Groups               []Group `json:"groups" gorm:"many2many:user_groups;"`
}

type Users []User

func NewUser() *User {
	u := &User{
		ShowPortal: true,
		ShowChat:   true,
	}

	u.GenerateAPIKey()
	return u
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

func (u *User) Get(db *gorm.DB, id uint) error {
	return db.First(u, id).Error
}

func (u *User) GetByAPIKey(db *gorm.DB, apiKey string) error {
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

func (u *Users) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool, sort string) (int64, int, error) {
	query := db.Model(&User{})

	query, totalCount, totalPages, err := PaginateAndSort(query, pageSize, pageNumber, all, sort)
	if err != nil {
		return 0, 0, err
	}

	err = query.Find(u).Error
	return totalCount, totalPages, err
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
		Distinct().
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
		Distinct().
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
		Distinct().
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
	if u.ID == 1 {
		return "Super Admin"
	} else if u.IsAdmin {
		return "Admin"
	} else if u.ShowPortal {
		return "Developer"
	} else {
		return "Chat user"
	}
}

func (u *Users) SearchByTerm(db *gorm.DB, term string, pageSize int, pageNumber int, all bool, sort string) (int64, int, error) {
	query := db.Model(&User{})

	if term != "" {
		searchTerm := "%" + term + "%"
		query = query.Where("email LIKE ? OR name LIKE ?", searchTerm, searchTerm)
	}

	query, totalCount, totalPages, err := PaginateAndSort(query, pageSize, pageNumber, all, sort)
	if err != nil {
		return 0, 0, err
	}

	err = query.Find(u).Error
	return totalCount, totalPages, err
}
