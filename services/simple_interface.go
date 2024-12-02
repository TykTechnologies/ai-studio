package services

import "github.com/TykTechnologies/midsommar/v2/models"

// Used by proxy and some tests
type ServiceInterface interface {
	// LLM related methods
	GetActiveLLMs() (models.LLMs, error)
	GetLLMByID(id uint) (*models.LLM, error)

	GetLLMSettingsByID(id uint) (*models.LLMSettings, error)

	// Datasource related methods
	GetActiveDatasources() (models.Datasources, error)
	GetDatasourceByID(id uint) (*models.Datasource, error)

	// Credential related methods
	GetCredentialBySecret(secret string) (*models.Credential, error)

	// App related methods
	GetAppByCredentialID(credID uint) (*models.App, error)

	// Analytics related methods
	GetModelPriceByModelNameAndVendor(modelName, vendor string) (*models.ModelPrice, error)
}
