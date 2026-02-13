package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
)

func TestDatasourceWithSecretReference(t *testing.T) {
	// Set required environment variable
	t.Setenv("TYK_AI_SECRET_KEY", "test-key")

	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	a := NewAPI(service, true, authService, config, nil, apitest.EmptyFile, nil)

	// Initialize secrets package with DB reference
	secrets.SetDBRef(db)

	// Create secrets first
	secretInput1 := SecretInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Value   string `json:"value"`
				VarName string `json:"var_name"`
			} `json:"attributes"`
		}{
			Type: "secrets",
			Attributes: struct {
				Value   string `json:"value"`
				VarName string `json:"var_name"`
			}{
				Value:   "db-key-123",
				VarName: "DB_KEY",
			},
		},
	}

	secretInput2 := SecretInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Value   string `json:"value"`
				VarName string `json:"var_name"`
			} `json:"attributes"`
		}{
			Type: "secrets",
			Attributes: struct {
				Value   string `json:"value"`
				VarName string `json:"var_name"`
			}{
				Value:   "embed-key-123",
				VarName: "EMBED_KEY",
			},
		},
	}

	w := apitest.PerformRequest(a.Router(), "POST", "/api/v1/secrets", secretInput1)
	assert.Equal(t, http.StatusCreated, w.Code)

	w = apitest.PerformRequest(a.Router(), "POST", "/api/v1/secrets", secretInput2)
	assert.Equal(t, http.StatusCreated, w.Code)

	// Create a datasource that references these secrets
	createDatasourceInput := DatasourceInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name             string   `json:"name"`
				ShortDescription string   `json:"short_description"`
				LongDescription  string   `json:"long_description"`
				Icon             string   `json:"icon"`
				Url              string   `json:"url"`
				PrivacyScore     int      `json:"privacy_score"`
				UserID           uint     `json:"user_id"`
				Tags             []string `json:"tags"`
				DBConnString     string   `json:"db_conn_string"`
				DBSourceType     string   `json:"db_source_type"`
				DBConnAPIKey     string   `json:"db_conn_api_key"`
				DBName           string   `json:"db_name"`
				EmbedVendor      string   `json:"embed_vendor"`
				EmbedUrl         string   `json:"embed_url"`
				EmbedAPIKey      string   `json:"embed_api_key"`
				EmbedModel       string   `json:"embed_model"`
				Active           bool     `json:"active"`
				Namespace        string   `json:"namespace"`
			} `json:"attributes"`
		}{
			Type: "datasources",
			Attributes: struct {
				Name             string   `json:"name"`
				ShortDescription string   `json:"short_description"`
				LongDescription  string   `json:"long_description"`
				Icon             string   `json:"icon"`
				Url              string   `json:"url"`
				PrivacyScore     int      `json:"privacy_score"`
				UserID           uint     `json:"user_id"`
				Tags             []string `json:"tags"`
				DBConnString     string   `json:"db_conn_string"`
				DBSourceType     string   `json:"db_source_type"`
				DBConnAPIKey     string   `json:"db_conn_api_key"`
				DBName           string   `json:"db_name"`
				EmbedVendor      string   `json:"embed_vendor"`
				EmbedUrl         string   `json:"embed_url"`
				EmbedAPIKey      string   `json:"embed_api_key"`
				EmbedModel       string   `json:"embed_model"`
				Active           bool     `json:"active"`
				Namespace        string   `json:"namespace"`
			}{
				Name:             "Test Datasource",
				ShortDescription: "Test datasource with secret refs",
				LongDescription:  "A test datasource using secret references",
				Icon:             "test-icon.png",
				Url:              "https://test.com",
				PrivacyScore:     75,
				UserID:           1,
				Tags:             []string{"test"},
				DBConnString:     "postgres://localhost:5432/test",
				DBSourceType:     "postgres",
				DBConnAPIKey:     "$SECRET/DB_KEY",
				DBName:           "test",
				EmbedVendor:      string(models.OPENAI),
				EmbedUrl:         "https://api.openai.com/v1",
				EmbedAPIKey:      "$SECRET/EMBED_KEY",
				EmbedModel:       "text-embedding-ada-002",
				Active:           true,
			},
		},
	}

	w = apitest.PerformRequest(a.Router(), "POST", "/api/v1/datasources", createDatasourceInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]DatasourceResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Datasource", response["data"].Attributes.Name)
	assert.Equal(t, "[redacted]", response["data"].Attributes.DBConnAPIKey)
	assert.Equal(t, "[redacted]", response["data"].Attributes.EmbedAPIKey)
	assert.True(t, response["data"].Attributes.HasDBConnAPIKey)
	assert.True(t, response["data"].Attributes.HasEmbedAPIKey)

	dsID := response["data"].ID

	// Test Get Datasource - verify secret references are redacted in API response
	w = apitest.PerformRequest(a.Router(), "GET", fmt.Sprintf("/api/v1/datasources/%s", dsID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var getDSResponse map[string]DatasourceResponse
	err = json.Unmarshal(w.Body.Bytes(), &getDSResponse)
	assert.NoError(t, err)
	assert.Equal(t, "[redacted]", getDSResponse["data"].Attributes.DBConnAPIKey)
	assert.Equal(t, "[redacted]", getDSResponse["data"].Attributes.EmbedAPIKey)
	assert.True(t, getDSResponse["data"].Attributes.HasDBConnAPIKey)
	assert.True(t, getDSResponse["data"].Attributes.HasEmbedAPIKey)

	// Test Update Datasource - verify secret references are redacted in API response
	updateDSInput := createDatasourceInput
	updateDSInput.Data.Attributes.Name = "Updated Test Datasource"

	w = apitest.PerformRequest(a.Router(), "PATCH", fmt.Sprintf("/api/v1/datasources/%s", dsID), updateDSInput)
	assert.Equal(t, http.StatusOK, w.Code)

	var updateResponse map[string]DatasourceResponse
	err = json.Unmarshal(w.Body.Bytes(), &updateResponse)
	assert.NoError(t, err)
	assert.Equal(t, "[redacted]", updateResponse["data"].Attributes.DBConnAPIKey)
	assert.Equal(t, "[redacted]", updateResponse["data"].Attributes.EmbedAPIKey)
	assert.True(t, updateResponse["data"].Attributes.HasDBConnAPIKey)
	assert.True(t, updateResponse["data"].Attributes.HasEmbedAPIKey)

	// Test Search Datasources - verify secret references are redacted in API response
	w = apitest.PerformRequest(a.Router(), "GET", "/api/v1/datasources/search?query=Test", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var searchResponse map[string][]DatasourceResponse
	err = json.Unmarshal(w.Body.Bytes(), &searchResponse)
	assert.NoError(t, err)
	assert.Greater(t, len(searchResponse["data"]), 0)
	assert.Equal(t, "[redacted]", searchResponse["data"][0].Attributes.DBConnAPIKey)
	assert.Equal(t, "[redacted]", searchResponse["data"][0].Attributes.EmbedAPIKey)

	// Verify the actual secret values are used when needed
	ds, err := service.GetDatasourceByID(uint(1))
	assert.NoError(t, err)
	assert.Equal(t, "$SECRET/DB_KEY", ds.DBConnAPIKey)
	assert.Equal(t, "$SECRET/EMBED_KEY", ds.EmbedAPIKey)
}

func TestSerializeDatasourceRedactsAPIKeys(t *testing.T) {
	// Set required environment variable
	t.Setenv("TYK_AI_SECRET_KEY", "test-key")

	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	a := NewAPI(service, true, authService, config, nil, apitest.EmptyFile, nil)

	// Initialize secrets package with DB reference
	secrets.SetDBRef(db)

	// Create a secret
	secret := &secrets.Secret{
		VarName: "TEST_EMBED_KEY",
		Value:   "embed-key-789",
	}
	err := secrets.CreateSecret(db, secret)
	assert.NoError(t, err)

	// Test 1: Datasource with direct API keys
	datasourceWithDirectKeys := &models.Datasource{
		Name:             "Direct Keys Datasource",
		ShortDescription: "Test datasource with direct API keys",
		DBConnAPIKey:     "direct-db-key-123",
		EmbedAPIKey:      "direct-embed-key-456",
		PrivacyScore:     75,
		Active:           true,
	}

	// Test 2: Datasource with secret reference
	datasourceWithSecretRef := &models.Datasource{
		Name:             "Secret Ref Datasource",
		ShortDescription: "Test datasource with secret reference",
		DBConnAPIKey:     "direct-db-key-789",
		EmbedAPIKey:      "$SECRET/TEST_EMBED_KEY",
		PrivacyScore:     80,
		Active:           true,
	}

	// Test 3: Datasource with empty API keys
	datasourceWithEmptyKeys := &models.Datasource{
		Name:             "Empty Keys Datasource",
		ShortDescription: "Test datasource with no API keys",
		DBConnAPIKey:     "",
		EmbedAPIKey:      "",
		PrivacyScore:     85,
		Active:           true,
	}

	// Create test user first
	user := &models.User{
		Email:         "test@test.com",
		Name:          "Test User",
		IsAdmin:       true,
		EmailVerified: true,
		ShowPortal:    true,
		ShowChat:      true,
	}
	err = db.Create(user).Error
	assert.NoError(t, err)

	// Set UserID for datasources
	datasourceWithDirectKeys.UserID = user.ID
	datasourceWithSecretRef.UserID = user.ID
	datasourceWithEmptyKeys.UserID = user.ID

	// Store datasources in database to test via actual API endpoints
	err = db.Create(datasourceWithDirectKeys).Error
	assert.NoError(t, err)
	err = db.Create(datasourceWithSecretRef).Error
	assert.NoError(t, err)
	err = db.Create(datasourceWithEmptyKeys).Error
	assert.NoError(t, err)

	// Test serialization through actual API endpoints
	t.Run("DirectAPIKeysAreRedacted", func(t *testing.T) {
		w := apitest.PerformRequest(a.Router(), "GET", "/api/v1/datasources/1", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response struct {
			Data DatasourceResponse `json:"data"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "[redacted]", response.Data.Attributes.DBConnAPIKey)
		assert.Equal(t, "[redacted]", response.Data.Attributes.EmbedAPIKey)
		assert.True(t, response.Data.Attributes.HasDBConnAPIKey)
		assert.True(t, response.Data.Attributes.HasEmbedAPIKey)
	})

	t.Run("SecretReferenceAPIKeysAreRedacted", func(t *testing.T) {
		w := apitest.PerformRequest(a.Router(), "GET", "/api/v1/datasources/2", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response struct {
			Data DatasourceResponse `json:"data"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "[redacted]", response.Data.Attributes.DBConnAPIKey)
		assert.Equal(t, "[redacted]", response.Data.Attributes.EmbedAPIKey)
		assert.True(t, response.Data.Attributes.HasDBConnAPIKey)
		assert.True(t, response.Data.Attributes.HasEmbedAPIKey)
	})

	t.Run("EmptyAPIKeysShowCorrectly", func(t *testing.T) {
		w := apitest.PerformRequest(a.Router(), "GET", "/api/v1/datasources/3", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response struct {
			Data DatasourceResponse `json:"data"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "[redacted]", response.Data.Attributes.DBConnAPIKey)
		assert.Equal(t, "[redacted]", response.Data.Attributes.EmbedAPIKey)
		assert.False(t, response.Data.Attributes.HasDBConnAPIKey)
		assert.False(t, response.Data.Attributes.HasEmbedAPIKey)
	})
}
