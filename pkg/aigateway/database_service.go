package aigateway

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// DatabaseService implements GatewayServiceInterface using the existing services layer.
// This provides a bridge between the new interface-based architecture and the current
// database-backed implementation.
type DatabaseService struct {
	service *services.Service
}

// NewDatabaseService creates a new DatabaseService that wraps the existing services layer.
func NewDatabaseService(service *services.Service) GatewayServiceInterface {
	return &DatabaseService{
		service: service,
	}
}

// GetActiveLLMs returns all active LLM configurations from the database.
func (d *DatabaseService) GetActiveLLMs() ([]models.LLM, error) {
	return d.service.GetActiveLLMs()
}

// GetActiveDatasources returns all active datasource configurations from the database.
func (d *DatabaseService) GetActiveDatasources() ([]models.Datasource, error) {
	return d.service.GetActiveDatasources()
}

// GetToolBySlug returns a tool by its slug from the database.
func (d *DatabaseService) GetToolBySlug(slug string) (*models.Tool, error) {
	return d.service.GetToolBySlug(slug)
}

// GetCredentialBySecret returns a credential by its secret token from the database.
func (d *DatabaseService) GetCredentialBySecret(secret string) (*models.Credential, error) {
	return d.service.GetCredentialBySecret(secret)
}

// GetAppByCredentialID returns an app by its credential ID from the database.
func (d *DatabaseService) GetAppByCredentialID(credID uint) (*models.App, error) {
	return d.service.GetAppByCredentialID(credID)
}

// GetModelPriceByModelNameAndVendor returns pricing information for a model and vendor from the database.
func (d *DatabaseService) GetModelPriceByModelNameAndVendor(modelName, vendor string) (*models.ModelPrice, error) {
	return d.service.GetModelPriceByModelNameAndVendor(modelName, vendor)
}

// CallToolOperation executes a tool operation through the service layer.
func (d *DatabaseService) CallToolOperation(toolID uint, operationID string, params map[string][]string, payload map[string]interface{}, headers map[string][]string) (interface{}, error) {
	return d.service.CallToolOperation(toolID, operationID, params, payload, headers)
}

// GetDB returns the database interface for OAuth token services.
func (d *DatabaseService) GetDB() interface{} {
	return d.service.DB
}

// GetUserByID returns a user by their ID from the database.
func (d *DatabaseService) GetUserByID(id uint) (*models.User, error) {
	return d.service.GetUserByID(id)
}
