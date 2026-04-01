package services

import (
	"os"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForDatasources(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return db
}

func TestCreateDatasource(t *testing.T) {
	db := setupTestDBForDatasources(t)
	service := NewService(db)

	user, _ := service.CreateUser(UserDTO{Email: "test@example.com", Name: "Test User", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})

	datasource, err := service.CreateDatasource("Test Datasource", "Short Desc", "Long Desc", "icon.png", "https://example.com", 75, user.ID, []string{"AI", "ML"}, "conn_string", "source_type", "api_key", "db1", "embed_vendor", "embed_url", "embed_api_key", "embed_model", true)
	assert.NoError(t, err)
	assert.NotNil(t, datasource)
	assert.NotZero(t, datasource.ID)
	assert.Equal(t, "Test Datasource", datasource.Name)
	assert.Len(t, datasource.Tags, 2)
}

func TestGetDatasourceByID(t *testing.T) {
	db := setupTestDBForDatasources(t)
	service := NewService(db)

	user, _ := service.CreateUser(UserDTO{Email: "test@example.com", Name: "Test User", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})
	datasource, _ := service.CreateDatasource("Test Datasource", "Short Desc", "Long Desc", "icon.png", "https://example.com", 75, user.ID, []string{"AI", "ML"}, "conn_string", "source_type", "api_key", "db1", "embed_vendor", "embed_url", "embed_api_key", "embed_model", true)

	fetchedDatasource, err := service.GetDatasourceByID(datasource.ID)
	assert.NoError(t, err)
	assert.Equal(t, datasource.ID, fetchedDatasource.ID)
	assert.Equal(t, datasource.Name, fetchedDatasource.Name)
}

func TestUpdateDatasource(t *testing.T) {
	db := setupTestDBForDatasources(t)
	service := NewService(db)

	user, _ := service.CreateUser(UserDTO{Email: "test@example.com", Name: "Test User", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})
	datasource, _ := service.CreateDatasource("Test Datasource", "Short Desc", "Long Desc", "icon.png", "https://example.com", 75, user.ID, []string{"AI", "ML"}, "conn_string", "source_type", "api_key", "db1", "embed_vendor", "embed_url", "embed_api_key", "embed_model", true)

	updatedDatasource, err := service.UpdateDatasource(datasource.ID, "Updated Datasource", "Updated Short", "Updated Long", "updated-icon.png", "https://updated-example.com", 80, "updated_conn_string", "updated_source_type", "updated_api_key", "updated_db_name", "updated_embed_vendor", "updated_embed_url", "updated_embed_api_key", "updated_embed_model", true, []string{"AI", "ML"}, 0)
	assert.NoError(t, err)
	assert.Equal(t, datasource.ID, updatedDatasource.ID)
	assert.Equal(t, "Updated Datasource", updatedDatasource.Name)
	assert.Equal(t, 80, updatedDatasource.PrivacyScore)
}

func TestGetAllDatasources(t *testing.T) {
	db := setupTestDBForDatasources(t)
	service := NewService(db)

	user, _ := service.CreateUser(UserDTO{Email: "test@example.com", Name: "Test User", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})
	datasource, _ := service.CreateDatasource("Test Datasource", "Short Desc", "Long Desc", "icon.png", "https://example.com", 75, user.ID, []string{"AI", "ML"}, "conn_string", "source_type", "api_key", "db1", "embed_vendor", "embed_url", "embed_api_key", "embed_model", true)

	allDatasources, _, _, err := service.GetAllDatasources(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, allDatasources, 1)
	assert.Equal(t, datasource.ID, allDatasources[0].ID)
}

func TestSearchDatasources(t *testing.T) {
	db := setupTestDBForDatasources(t)
	service := NewService(db)

	user, _ := service.CreateUser(UserDTO{Email: "test@example.com", Name: "Test User", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})
	datasource, _ := service.CreateDatasource("Test Datasource", "Short Desc", "Long Desc", "icon.png", "https://example.com", 75, user.ID, []string{"AI", "ML"}, "conn_string", "source_type", "api_key", "db1", "embed_vendor", "embed_url", "embed_api_key", "embed_model", true)

	searchedDatasources, err := service.SearchDatasources("Test")
	assert.NoError(t, err)
	assert.Len(t, searchedDatasources, 1)
	assert.Equal(t, datasource.ID, searchedDatasources[0].ID)
}

func TestGetDatasourcesByTag(t *testing.T) {
	db := setupTestDBForDatasources(t)
	service := NewService(db)

	user, _ := service.CreateUser(UserDTO{Email: "test@example.com", Name: "Test User", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})
	datasource, _ := service.CreateDatasource("Test Datasource", "Short Desc", "Long Desc", "icon.png", "https://example.com", 75, user.ID, []string{"AI", "ML"}, "conn_string", "source_type", "api_key", "db1", "embed_vendor", "embed_url", "embed_api_key", "embed_model", true)

	datasourcesByTag, err := service.GetDatasourcesByTag("AI")
	assert.NoError(t, err)
	assert.Len(t, datasourcesByTag, 1)
	assert.Equal(t, datasource.ID, datasourcesByTag[0].ID)
}

func TestAddTagsToDatasource(t *testing.T) {
	db := setupTestDBForDatasources(t)
	service := NewService(db)

	user, _ := service.CreateUser(UserDTO{Email: "test@example.com", Name: "Test User", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})
	datasource, _ := service.CreateDatasource("Test Datasource", "Short Desc", "Long Desc", "icon.png", "https://example.com", 75, user.ID, []string{"AI", "ML"}, "conn_string", "source_type", "api_key", "db1", "embed_vendor", "embed_url", "embed_api_key", "embed_model", true)

	err := service.AddTagsToDatasource(datasource.ID, []string{"NLP"})
	assert.NoError(t, err)
	updatedDatasource, _ := service.GetDatasourceByID(datasource.ID)
	assert.Len(t, updatedDatasource.Tags, 3)
}

func TestGetDatasourcesByPrivacyScoreRange(t *testing.T) {
	db := setupTestDBForDatasources(t)
	service := NewService(db)

	user, _ := service.CreateUser(UserDTO{Email: "test@example.com", Name: "Test User", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})
	datasource, _ := service.CreateDatasource("Test Datasource", "Short Desc", "Long Desc", "icon.png", "https://example.com", 75, user.ID, []string{"AI", "ML"}, "conn_string", "source_type", "api_key", "db1", "embed_vendor", "embed_url", "embed_api_key", "embed_model", true)

	datasourcesByScore, err := service.GetDatasourcesByPrivacyScoreRange(70, 80)
	assert.NoError(t, err)
	assert.Len(t, datasourcesByScore, 1)
	assert.Equal(t, datasource.ID, datasourcesByScore[0].ID)
}

func TestGetDatasourcesByUserID(t *testing.T) {
	db := setupTestDBForDatasources(t)
	service := NewService(db)

	user, _ := service.CreateUser(UserDTO{Email: "test@example.com", Name: "Test User", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})
	datasource, _ := service.CreateDatasource("Test Datasource", "Short Desc", "Long Desc", "icon.png", "https://example.com", 75, user.ID, []string{"AI", "ML"}, "conn_string", "source_type", "api_key", "db1", "embed_vendor", "embed_url", "embed_api_key", "embed_model", true)

	datasourcesByUser, err := service.GetDatasourcesByUserID(user.ID)
	assert.NoError(t, err)
	assert.Len(t, datasourcesByUser, 1)
	assert.Equal(t, datasource.ID, datasourcesByUser[0].ID)
}

func TestDeleteDatasource(t *testing.T) {
	db := setupTestDBForDatasources(t)
	service := NewService(db)

	user, _ := service.CreateUser(UserDTO{Email: "test@example.com", Name: "Test User", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})
	datasource, _ := service.CreateDatasource("Test Datasource", "Short Desc", "Long Desc", "icon.png", "https://example.com", 75, user.ID, []string{"AI", "ML"}, "conn_string", "source_type", "api_key", "db1", "embed_vendor", "embed_url", "embed_api_key", "embed_model", true)

	err := service.DeleteDatasource(datasource.ID)
	assert.NoError(t, err)

	_, err = service.GetDatasourceByID(datasource.ID)
	assert.Error(t, err)
}

func TestDatasourceService_MultipleDatasourcesScenario(t *testing.T) {
	db := setupTestDBForDatasources(t)
	service := NewService(db)

	// Create a user for testing
	user, _ := service.CreateUser(UserDTO{Email: "test@example.com", Name: "Test User", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})

	// Create multiple datasources
	ds1, _ := service.CreateDatasource("Datasource 1", "Short 1", "Long 1", "icon1.png", "https://ds1.com", 60, user.ID, []string{"AI", "ML"}, "conn_string1", "source_type1", "api_key1", "db1", "embed_vendor1", "embed_url1", "embed_api_key1", "embed_model1", true)
	ds2, _ := service.CreateDatasource("Datasource 2", "Short 2", "Long 2", "icon2.png", "https://ds2.com", 75, user.ID, []string{"NLP", "ML"}, "conn_string2", "source_type2", "api_key2", "db2", "embed_vendor2", "embed_url2", "embed_api_key2", "embed_model2", true)
	ds3, _ := service.CreateDatasource("Datasource 3", "Short 3", "Long 3", "icon3.png", "https://ds3.com", 90, user.ID, []string{"AI", "NLP"}, "conn_string3", "source_type3", "api_key3", "db3", "embed_vendor3", "embed_url3", "embed_api_key3", "embed_model3", true)

	// Test GetAllDatasources
	allDatasources, _, _, err := service.GetAllDatasources(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, allDatasources, 3)

	// Test SearchDatasources
	searchedDatasources, err := service.SearchDatasources("Datasource")
	assert.NoError(t, err)
	assert.Len(t, searchedDatasources, 3)

	// Test GetDatasourcesByTag
	mlDatasources, err := service.GetDatasourcesByTag("ML")
	assert.NoError(t, err)
	assert.Len(t, mlDatasources, 2)

	// Test GetDatasourcesByMinPrivacyScore
	highPrivacyDatasources, err := service.GetDatasourcesByMinPrivacyScore(75)
	assert.NoError(t, err)
	assert.Len(t, highPrivacyDatasources, 2)
	assert.Contains(t, []uint{ds2.ID, ds3.ID}, highPrivacyDatasources[0].ID)
	assert.Contains(t, []uint{ds2.ID, ds3.ID}, highPrivacyDatasources[1].ID)

	// Test GetDatasourcesByMaxPrivacyScore
	lowPrivacyDatasources, err := service.GetDatasourcesByMaxPrivacyScore(75)
	assert.NoError(t, err)
	assert.Len(t, lowPrivacyDatasources, 2)
	assert.Contains(t, []uint{ds1.ID, ds2.ID}, lowPrivacyDatasources[0].ID)
	assert.Contains(t, []uint{ds1.ID, ds2.ID}, lowPrivacyDatasources[1].ID)

	// Test GetDatasourcesByPrivacyScoreRange
	midPrivacyDatasources, err := service.GetDatasourcesByPrivacyScoreRange(70, 80)
	assert.NoError(t, err)
	assert.Len(t, midPrivacyDatasources, 1)
	assert.Equal(t, ds2.ID, midPrivacyDatasources[0].ID)

	// Test GetDatasourcesByUserID
	userDatasources, err := service.GetDatasourcesByUserID(user.ID)
	assert.NoError(t, err)
	assert.Len(t, userDatasources, 3)
}

func TestGetDatasourceByIDResolved_ResolvesSecrets(t *testing.T) {
	os.Setenv("TYK_AI_SECRET_KEY", "test-encryption-key-for-secrets")
	defer os.Unsetenv("TYK_AI_SECRET_KEY")

	db := setupTestDBForDatasources(t)
	service := NewService(db)
	secrets.SetDBRef(db)

	// Create secrets
	embedSecret := &secrets.Secret{VarName: "EMBED_KEY", Value: "sk-embed-real-key"}
	err := secrets.CreateSecret(db, embedSecret)
	assert.NoError(t, err)

	dbConnSecret := &secrets.Secret{VarName: "DBCONN_KEY", Value: "dbconn-real-key"}
	err = secrets.CreateSecret(db, dbConnSecret)
	assert.NoError(t, err)

	user, _ := service.CreateUser(UserDTO{Email: "test@example.com", Name: "Test User", Password: "password123", IsAdmin: true, ShowChat: true, ShowPortal: true, EmailVerified: true, NotificationsEnabled: true, AccessToSSOConfig: true, Groups: []uint{}})

	// Create datasource with secret references
	datasource, err := service.CreateDatasource(
		"Secret DS", "Short", "Long", "icon.png", "https://example.com", 75, user.ID,
		[]string{}, "conn_string", "source_type",
		"$SECRET/DBCONN_KEY", "db1", "openai", "https://api.openai.com",
		"$SECRET/EMBED_KEY", "text-embedding-3-small", true,
	)
	assert.NoError(t, err)

	// GetDatasourceByID should preserve references (for API responses)
	preserved, err := service.GetDatasourceByID(datasource.ID)
	assert.NoError(t, err)
	assert.Equal(t, "$SECRET/EMBED_KEY", preserved.EmbedAPIKey,
		"GetDatasourceByID should preserve secret references")
	assert.Equal(t, "$SECRET/DBCONN_KEY", preserved.DBConnAPIKey,
		"GetDatasourceByID should preserve secret references")

	// GetDatasourceByIDResolved should resolve to actual values
	resolved, err := service.GetDatasourceByIDResolved(datasource.ID)
	assert.NoError(t, err)
	assert.Equal(t, "sk-embed-real-key", resolved.EmbedAPIKey,
		"GetDatasourceByIDResolved should resolve EmbedAPIKey to actual secret value")
	assert.Equal(t, "dbconn-real-key", resolved.DBConnAPIKey,
		"GetDatasourceByIDResolved should resolve DBConnAPIKey to actual secret value")
}
