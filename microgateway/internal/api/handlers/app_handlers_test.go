// internal/api/handlers/app_handlers_test.go
package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListApps_Handler(t *testing.T) {
	container, router := setupTestHandlers(t)
	
	// Create test apps
	app1 := &database.App{
		Name:       "Test App 1",
		OwnerEmail: "test1@example.com",
		IsActive:   true,
	}
	app2 := &database.App{
		Name:       "Test App 2", 
		OwnerEmail: "test2@example.com",
		IsActive:   true, // Create as active first
	}
	
	container.Repository.CreateApp(app1)
	container.Repository.CreateApp(app2)
	
	// Now explicitly set app2 to inactive
	app2.IsActive = false
	container.Repository.UpdateApp(app2)

	// Setup route
	router.GET("/apps", ListApps(container))

	t.Run("ListActiveApps", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/apps?active=true", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		data := response["data"].([]interface{})
		assert.Len(t, data, 1)
		
		pagination := response["pagination"].(map[string]interface{})
		assert.Equal(t, float64(1), pagination["total"])
	})

	t.Run("ListInactiveApps", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/apps?active=false", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("PaginationLimits", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/apps?page=0&limit=200", nil) // Invalid values
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		pagination := response["pagination"].(map[string]interface{})
		assert.Equal(t, float64(1), pagination["page"])  // Corrected to 1
		assert.Equal(t, float64(20), pagination["limit"]) // Corrected to 20
	})
}

func TestCreateApp_Handler(t *testing.T) {
	container, router := setupTestHandlers(t)
	router.POST("/apps", CreateApp(container))

	t.Run("ValidCreate", func(t *testing.T) {
		reqData := services.CreateAppRequest{
			Name:          "Test Application",
			Description:   "A test app",
			OwnerEmail:    "owner@example.com",
			MonthlyBudget: 100.0,
			RateLimitRPM:  1000,
		}

		jsonData, _ := json.Marshal(reqData)
		req := httptest.NewRequest("POST", "/apps", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		data := response["data"].(map[string]interface{})
		assert.Equal(t, "Test Application", data["name"])
		assert.Equal(t, "owner@example.com", data["owner_email"])
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/apps", bytes.NewBufferString("invalid json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestGetApp_Handler(t *testing.T) {
	container, router := setupTestHandlers(t)
	router.GET("/apps/:id", GetApp(container))

	// Create test app
	app := &database.App{
		Name:       "Test App",
		OwnerEmail: "test@example.com",
		IsActive:   true,
	}
	container.Repository.CreateApp(app)

	t.Run("ValidGet", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/apps/"+strconv.Itoa(int(app.ID)), nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		data := response["data"].(map[string]interface{})
		assert.Equal(t, "Test App", data["name"])
	})

	t.Run("InvalidID", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/apps/invalid", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("NotFound", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/apps/999", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestListCredentials_Handler(t *testing.T) {
	container, router := setupTestHandlers(t)
	router.GET("/apps/:id/credentials", ListCredentials(container))

	// Create test app
	app := &database.App{
		Name:       "Test App",
		OwnerEmail: "test@example.com", 
		IsActive:   true,
	}
	container.Repository.CreateApp(app)

	// Create test credentials
	cred := &database.Credential{
		AppID:      app.ID,
		KeyID:      "test-key",
		SecretHash: "hashed-secret",
		IsActive:   true,
	}
	container.Repository.CreateCredential(cred)

	t.Run("ValidList", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/apps/"+strconv.Itoa(int(app.ID))+"/credentials", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		data := response["data"].([]interface{})
		assert.Len(t, data, 1)
	})
}

func TestCreateCredential_Handler(t *testing.T) {
	container, router := setupTestHandlers(t)
	router.POST("/apps/:id/credentials", CreateCredential(container))

	// Create test app
	app := &database.App{
		Name:       "Test App",
		OwnerEmail: "test@example.com",
		IsActive:   true,
	}
	container.Repository.CreateApp(app)

	t.Run("ValidCreate", func(t *testing.T) {
		reqData := services.CreateCredentialRequest{
			Name: "Test Credential",
		}

		jsonData, _ := json.Marshal(reqData)
		req := httptest.NewRequest("POST", "/apps/"+strconv.Itoa(int(app.ID))+"/credentials", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		
		assert.Contains(t, response, "data")
		assert.Contains(t, response, "warning")
	})

	t.Run("InvalidAppID", func(t *testing.T) {
		reqData := services.CreateCredentialRequest{
			Name: "Test Credential",
		}

		jsonData, _ := json.Marshal(reqData)
		req := httptest.NewRequest("POST", "/apps/invalid/credentials", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}