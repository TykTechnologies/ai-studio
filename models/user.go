package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	ID                uint   `json:"id" gorm:"primaryKey"`
	Email             string `json:"email"`
	Name              string
	Password          string `json:"password"`
	SessionToken      string
	ResetToken        string
	ResetTokenExpiry  time.Time
	EmailVerified     bool
	VerificationToken string
	IsAdmin           bool
}

type Users []User

func NewUser() *User {
	return &User{}
}

func (u *User) Get(db *gorm.DB, id uint) error {
	return db.First(u, id).Error
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

func (u *Users) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&User{})

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

	err := query.Find(u).Error
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
		Where("user_groups.user_id = ?", u.ID).
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
		Where("user_groups.user_id = ?", u.ID).
		Distinct().
		Find(&llms).Error
	return llms, err
}
