package models

import (
	"fmt"

	"github.com/TykTechnologies/tyk-identity-broker/tap"
	"gorm.io/gorm"
)

// GormAuthRegisterBackend implements AuthRegisterBackend using GORM
type GormAuthRegisterBackend struct {
	DB *gorm.DB
}

// NewGormAuthRegisterBackend creates a new instance of
// GormAuthRegisterBackend and initializes it with the given database connection
func NewGormAuthRegisterBackend(db *gorm.DB) tap.AuthRegisterBackend {
	store := &GormAuthRegisterBackend{}

	err := store.Init(db)
	if err != nil {
		return nil
	}

	return store
}

// Init initializes the GormAuthRegisterBackend with the given configuration
func (g *GormAuthRegisterBackend) Init(config interface{}) error {
	db, ok := config.(*gorm.DB)
	if !ok {
		return fmt.Errorf("invalid config")
	}

	g.DB = db
	g.DB.AutoMigrate(&Profile{})

	return nil
}

// SetKey stores the given value in the database with the specified key and orgId
func (g *GormAuthRegisterBackend) SetKey(_, _ string, _ interface{}) error {
	// Func is not used, we are just satisfying TIB interface.
	return nil
}

// GetKey retrieves the value from the database for the specified key and orgId
func (g *GormAuthRegisterBackend) GetKey(key, _ string, val interface{}) error {
	profile := &Profile{}

	if err := g.DB.Where("profile_id = ?", key).First(profile).Error; err != nil {
		return err
	}

	tapProfile, ok := val.(*tap.Profile)
	if !ok {
		return fmt.Errorf("invalid value")
	}

	profile.MapToTapProfile(tapProfile)

	return nil
}

// GetAll retrieves all values from the database for the specified orgId
func (g *GormAuthRegisterBackend) GetAll(_ string) []interface{} {
	// Func is not used, we are just satisfying TIB interface.
	return nil
}

// DeleteKey deletes the value from the database for the specified key and orgId
func (g *GormAuthRegisterBackend) DeleteKey(_, _ string) error {
	// Func is not used, we are just satisfying TIB interface.
	return nil
}
