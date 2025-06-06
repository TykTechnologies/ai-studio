package services

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

type ServiceInterface interface {
	// LLM
	GetActiveLLMs() (models.LLMs, error)
	GetLLMByID(id uint) (*models.LLM, error)
	GetLLMSettingsByID(id uint) (*models.LLMSettings, error)
	// DS
	GetActiveDatasources() (models.Datasources, error)
	GetDatasourceByID(id uint) (*models.Datasource, error)
	// Cred
	GetCredentialBySecret(secret string) (*models.Credential, error)
	// App
	GetAppByCredentialID(credID uint) (*models.App, error)
	// Analytics
	GetModelPriceByModelNameAndVendor(modelName, vendor string) (*models.ModelPrice, error)
	// DB
	GetDB() *gorm.DB
	// Auth
	AuthenticateUser(email, password string) (*models.User, error)
	GetUserByAPIKey(apiKey string) (*models.User, error)
	GetUserByEmail(email string) (*models.User, error)
	AddUserToGroup(userID, groupID uint) error
	// Tool
	GetToolByID(id uint) (*models.Tool, error)
	GetToolBySlug(slug string) (*models.Tool, error)
}
