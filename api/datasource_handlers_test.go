package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/api"
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
	licenser := apitest.SetupTestLicenser()
	a := api.NewAPI(service, true, authService, config, nil, apitest.EmptyFile, licenser)

	// Initialize secrets package with DB reference
	secrets.SetDBRef(db)

	// Create secrets first
	secretInput1 := api.SecretInput{
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

	secretInput2 := api.SecretInput{
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
	createDatasourceInput := api.DatasourceInput{
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

	var response map[string]api.DatasourceResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Datasource", response["data"].Attributes.Name)
	assert.Equal(t, "$SECRET/DB_KEY", response["data"].Attributes.DBConnAPIKey)
	assert.Equal(t, "$SECRET/EMBED_KEY", response["data"].Attributes.EmbedAPIKey)

	dsID := response["data"].ID

	// Test Get Datasource - verify secret references are preserved
	w = apitest.PerformRequest(a.Router(), "GET", fmt.Sprintf("/api/v1/datasources/%s", dsID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var getDSResponse map[string]api.DatasourceResponse
	err = json.Unmarshal(w.Body.Bytes(), &getDSResponse)
	assert.NoError(t, err)
	assert.Equal(t, "$SECRET/DB_KEY", getDSResponse["data"].Attributes.DBConnAPIKey)
	assert.Equal(t, "$SECRET/EMBED_KEY", getDSResponse["data"].Attributes.EmbedAPIKey)

	// Test Update Datasource - verify secret references are preserved
	updateDSInput := createDatasourceInput
	updateDSInput.Data.Attributes.Name = "Updated Test Datasource"

	w = apitest.PerformRequest(a.Router(), "PATCH", fmt.Sprintf("/api/v1/datasources/%s", dsID), updateDSInput)
	assert.Equal(t, http.StatusOK, w.Code)

	var updateResponse map[string]api.DatasourceResponse
	err = json.Unmarshal(w.Body.Bytes(), &updateResponse)
	assert.NoError(t, err)
	assert.Equal(t, "$SECRET/DB_KEY", updateResponse["data"].Attributes.DBConnAPIKey)
	assert.Equal(t, "$SECRET/EMBED_KEY", updateResponse["data"].Attributes.EmbedAPIKey)

	// Test Search Datasources - verify secret references are preserved
	w = apitest.PerformRequest(a.Router(), "GET", "/api/v1/datasources/search?query=Test", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var searchResponse map[string][]api.DatasourceResponse
	err = json.Unmarshal(w.Body.Bytes(), &searchResponse)
	assert.NoError(t, err)
	assert.Greater(t, len(searchResponse["data"]), 0)
	assert.Equal(t, "$SECRET/DB_KEY", searchResponse["data"][0].Attributes.DBConnAPIKey)
	assert.Equal(t, "$SECRET/EMBED_KEY", searchResponse["data"][0].Attributes.EmbedAPIKey)

	// Verify the actual secret values are used when needed
	ds, err := service.GetDatasourceByID(uint(1))
	assert.NoError(t, err)
	assert.Equal(t, "$SECRET/DB_KEY", ds.DBConnAPIKey)
	assert.Equal(t, "$SECRET/EMBED_KEY", ds.EmbedAPIKey)
}
