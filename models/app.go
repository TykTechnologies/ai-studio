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
}

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
	return db.Preload("Credential").First(a, id).Error
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
