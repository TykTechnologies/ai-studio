package models

import (
	"errors"

	"gorm.io/gorm"
)

type App struct {
	gorm.Model
	ID           uint   `json:"id" gorm:"primary_key"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	UserID       uint   `json:"user_id" gorm:"foreignKey:ID"`
	CredentialID uint   `json:"credential_id"`
	Credential   Credential
	Datasources  []Datasource `json:"datasources" gorm:"many2many:app_datasources;"`
	LLMs         []LLM        `json:"llms" gorm:"many2many:app_llms;"`
}

type Apps []App

func NewApp() *App {
	return &App{}
}

// Create a new app
func (a *App) Create(db *gorm.DB) error {
	if a.CredentialID == 0 {
		credential, err := NewCredential()
		if err != nil {
			return err
		}
		if err := credential.Create(db); err != nil {
			return err
		}
		a.CredentialID = credential.ID
	}
	return db.Create(a).Error
}

// Get an app by ID
func (a *App) Get(db *gorm.DB, id uint) error {
	return db.Preload("Credential").Preload("Datasources").Preload("LLMs").First(a, id).Error
}

// Update an existing app
func (a *App) Update(db *gorm.DB) error {
	return db.Save(a).Error
}

// Delete an app
func (a *App) Delete(db *gorm.DB) error {
	return db.Delete(a).Error
}

// GetByUserID gets all apps for a specific user
func (a *App) GetByUserID(db *gorm.DB, userID uint) ([]App, error) {
	var apps []App
	err := db.Where("user_id = ?", userID).Preload("Credential").Find(&apps).Error
	return apps, err
}

// GetByName gets an app by its name
func (a *App) GetByName(db *gorm.DB, name string) error {
	return db.Where("name = ?", name).Preload("Credential").First(a).Error
}

// GetByCredentialID gets an app by its credential ID
func (a *App) GetByCredentialID(db *gorm.DB, credentialID uint) error {
	return db.Where("credential_id = ?", credentialID).Preload("Credential").First(a).Error
}

// ActivateCredential activates the credential associated with the app
func (a *App) ActivateCredential(db *gorm.DB) error {
	if a.CredentialID == 0 {
		return errors.New("no credential associated with this app")
	}
	credential := &Credential{ID: a.CredentialID}
	if err := credential.Get(db, a.CredentialID); err != nil {
		return err
	}
	return credential.Activate(db)
}

// DeactivateCredential deactivates the credential associated with the app
func (a *App) DeactivateCredential(db *gorm.DB) error {
	if a.CredentialID == 0 {
		return errors.New("no credential associated with this app")
	}
	var credential Credential
	if err := db.First(&credential, a.CredentialID).Error; err != nil {
		return err
	}
	return credential.Deactivate(db)
}

func (a *App) AddDatasource(db *gorm.DB, datasource *Datasource) error {
	return db.Model(a).Association("Datasources").Append(datasource)
}

func (a *App) RemoveDatasource(db *gorm.DB, datasource *Datasource) error {
	return db.Model(a).Association("Datasources").Delete(datasource)
}

func (a *App) AddLLM(db *gorm.DB, llm *LLM) error {
	return db.Model(a).Association("LLMs").Append(llm)
}

func (a *App) RemoveLLM(db *gorm.DB, llm *LLM) error {
	return db.Model(a).Association("LLMs").Delete(llm)
}

func (a *App) GetDatasources(db *gorm.DB) error {
	return db.Model(a).Association("Datasources").Find(&a.Datasources)
}

func (a *App) GetLLMs(db *gorm.DB, pageSize, pageNumber int, all bool) ([]LLM, int64, int, error) {
	var llms []LLM
	var totalCount int64
	var totalPages int

	// Count total number of LLMs associated with this app
	totalCount = db.Model(a).Association("LLMs").Count()

	// Calculate total pages
	totalPages = int(totalCount) / pageSize
	if int(totalCount)%pageSize != 0 {
		totalPages++
	}

	if all {
		// Fetch all LLMs associated with this app
		if err := db.Model(a).Association("LLMs").Find(&llms); err != nil {
			return nil, 0, 0, err
		}
	} else {
		// Apply pagination
		offset := (pageNumber - 1) * pageSize
		if err := db.Preload("LLMs", func(db *gorm.DB) *gorm.DB {
			return db.Offset(offset).Limit(pageSize)
		}).First(a).Error; err != nil {
			return nil, 0, 0, err
		}
		llms = a.LLMs
	}

	return llms, totalCount, totalPages, nil
}

func (a *App) List(db *gorm.DB) (Apps, error) {
	var apps Apps
	err := db.Preload("Credential").Find(&apps).Error
	return apps, err
}

// ListWithPagination returns a paginated list of apps
func (a *Apps) ListWithPagination(db *gorm.DB, pageSize int, pageNumber int, all bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&App{})

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

	err := query.Preload("Credential").Preload("Datasources").Preload("LLMs").Find(a).Error
	return totalCount, totalPages, err
}

// ListByUserID returns all apps for a specific user with pagination
func (a *Apps) ListByUserID(db *gorm.DB, userID uint, pageSize int, pageNumber int, all bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&App{}).Where("user_id = ?", userID)

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

	err := query.Preload("Credential").Preload("Datasources").Preload("LLMs").Find(a).Error
	return totalCount, totalPages, err
}

// Search returns apps matching the given search term with pagination
func (a *Apps) Search(db *gorm.DB, searchTerm string, pageSize int, pageNumber int, all bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&App{}).Where("name LIKE ? OR description LIKE ?", "%"+searchTerm+"%", "%"+searchTerm+"%")

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

	err := query.Preload("Credential").Preload("Datasources").Preload("LLMs").Find(a).Error
	return totalCount, totalPages, err
}

// Count returns the total number of apps
func (a *App) Count(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&App{}).Count(&count).Error
	return count, err
}

// CountByUserID returns the total number of apps for a specific user
func (a *App) CountByUserID(db *gorm.DB, userID uint) (int64, error) {
	var count int64
	err := db.Model(&App{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}
