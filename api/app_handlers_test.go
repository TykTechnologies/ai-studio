package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Dummy assignments to satisfy "imported and not used" error for packages
// used by performRequest (defined in api_test.go).
var _ = bytes.NewBufferString("")
var _ = httptest.NewRecorder()

// TestAppInput defines the structure for app creation/update payloads in tests.
type TestAppInput struct {
	Data struct {
		Type       string `json:"type" binding:"required,eq=app"`
		Attributes struct {
			Name            string     `json:"name"`
			Description     string     `json:"description"`
			UserID          uint       `json:"user_id"`
			DatasourceIDs   []uint     `json:"datasource_ids"`
			LLMIDs          []uint     `json:"llm_ids"`
			ToolIDs         []uint     `json:"tool_ids"`
			MonthlyBudget   *float64   `json:"monthly_budget"`
			BudgetStartDate *time.Time `json:"budget_start_date"`
		} `json:"attributes" binding:"required"`
	} `json:"data" binding:"required"`
}

func TestAppEndpointsWithTools(t *testing.T) {
	api, db := setupTestAPI(t)

	user := &models.User{Email: "apphandlertest@example.com", Name: "AppHandler User", IsAdmin: true, EmailVerified: true}
	err := user.Create(db)
	assert.NoError(t, err)

	tool1 := &models.Tool{Name: "Handler Tool 1", Description: "Desc 1", ToolType: "REST"}
	err = tool1.Create(db)
	assert.NoError(t, err)

	tool2 := &models.Tool{Name: "Handler Tool 2", Description: "Desc 2", ToolType: "REST"}
	err = tool2.Create(db)
	assert.NoError(t, err)

	createAppPayload := TestAppInput{
		Data: struct {
			Type       string `json:"type" binding:"required,eq=app"`
			Attributes struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []uint     `json:"datasource_ids"`
				LLMIDs          []uint     `json:"llm_ids"`
				ToolIDs         []uint     `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			} `json:"attributes" binding:"required"`
		}{
			Type: "app",
			Attributes: struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []uint     `json:"datasource_ids"`
				LLMIDs          []uint     `json:"llm_ids"`
				ToolIDs         []uint     `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			}{
				Name:        "App With Tools Handler Test",
				Description: "Test app creation with tools",
				UserID:      user.ID,
				ToolIDs:     []uint{tool1.ID},
			},
		},
	}
	w := performRequest(api.router, "POST", "/api/v1/apps", createAppPayload)
	assert.Equal(t, http.StatusCreated, w.Code)

	var createAppResponse AppResponseWrapper
	err = json.Unmarshal(w.Body.Bytes(), &createAppResponse)
	assert.NoError(t, err)
	createdAppID, _ := strconv.ParseUint(createAppResponse.Data.ID, 10, 32)
	assert.NotZero(t, createdAppID)
	assert.Len(t, createAppResponse.Data.Attributes.ToolIDs, 1)
	if len(createAppResponse.Data.Attributes.ToolIDs) > 0 {
		assert.Equal(t, tool1.ID, createAppResponse.Data.Attributes.ToolIDs[0])
	}

	wGetTools := performRequest(api.router, "GET", fmt.Sprintf("/api/v1/apps/%d/tools", createdAppID), nil)
	assert.Equal(t, http.StatusOK, wGetTools.Code)
	// Use a more complex structure to match the API response
	var getToolsResponse struct {
		Data []struct {
			Type       string `json:"type"`
			ID         uint   `json:"id"`
			Attributes struct {
				Name        string    `json:"name"`
				Description string    `json:"description"`
				ToolType    string    `json:"tool_type"`
				CreatedAt   time.Time `json:"created_at"`
				UpdatedAt   time.Time `json:"updated_at"`
			} `json:"attributes"`
		} `json:"data"`
	}
	err = json.Unmarshal(wGetTools.Body.Bytes(), &getToolsResponse)
	assert.NoError(t, err)
	assert.Len(t, getToolsResponse.Data, 1)
	assert.Equal(t, tool1.ID, getToolsResponse.Data[0].ID)

	wAddTool := performRequest(api.router, "POST", fmt.Sprintf("/api/v1/apps/%d/tools/%d", createdAppID, tool2.ID), nil)
	assert.Equal(t, http.StatusOK, wAddTool.Code)
	var addToolResponse AppResponseWrapper
	err = json.Unmarshal(wAddTool.Body.Bytes(), &addToolResponse)
	assert.NoError(t, err)
	assert.Len(t, addToolResponse.Data.Attributes.ToolIDs, 2)

	wGetToolsAfterAdd := performRequest(api.router, "GET", fmt.Sprintf("/api/v1/apps/%d/tools", createdAppID), nil)
	assert.Equal(t, http.StatusOK, wGetToolsAfterAdd.Code)
	err = json.Unmarshal(wGetToolsAfterAdd.Body.Bytes(), &getToolsResponse)
	assert.NoError(t, err)
	assert.Len(t, getToolsResponse.Data, 2)

	// Extract tool IDs from response for easier assertion
	toolIDs := make([]uint, len(getToolsResponse.Data))
	for i, toolData := range getToolsResponse.Data {
		toolIDs[i] = toolData.ID
	}

	// Check that both tool IDs are in the response
	assert.Contains(t, toolIDs, tool1.ID)
	assert.Contains(t, toolIDs, tool2.ID)

	updateAppPayload := TestAppInput{
		Data: struct {
			Type       string `json:"type" binding:"required,eq=app"`
			Attributes struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []uint     `json:"datasource_ids"`
				LLMIDs          []uint     `json:"llm_ids"`
				ToolIDs         []uint     `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			} `json:"attributes" binding:"required"`
		}{
			Type: "app",
			Attributes: struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []uint     `json:"datasource_ids"`
				LLMIDs          []uint     `json:"llm_ids"`
				ToolIDs         []uint     `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			}{
				Name:        "App With Tools Handler Test Updated",
				Description: "Test app update with tools",
				UserID:      user.ID,
				ToolIDs:     []uint{tool2.ID},
			},
		},
	}
	wUpdate := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/apps/%d", createdAppID), updateAppPayload)
	assert.Equal(t, http.StatusOK, wUpdate.Code)
	var updateAppResponse AppResponseWrapper
	err = json.Unmarshal(wUpdate.Body.Bytes(), &updateAppResponse)
	assert.NoError(t, err)
	assert.Len(t, updateAppResponse.Data.Attributes.ToolIDs, 1)
	if len(updateAppResponse.Data.Attributes.ToolIDs) > 0 {
		assert.Equal(t, tool2.ID, updateAppResponse.Data.Attributes.ToolIDs[0])
	}

	wRemoveTool := performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/apps/%d/tools/%d", createdAppID, tool2.ID), nil)
	assert.Equal(t, http.StatusNoContent, wRemoveTool.Code)

	wGetToolsAfterRemove := performRequest(api.router, "GET", fmt.Sprintf("/api/v1/apps/%d/tools", createdAppID), nil)
	assert.Equal(t, http.StatusOK, wGetToolsAfterRemove.Code)
	err = json.Unmarshal(wGetToolsAfterRemove.Body.Bytes(), &getToolsResponse)
	assert.NoError(t, err)
	assert.Len(t, getToolsResponse.Data, 0)

	wGetToolsNonExistentApp := performRequest(api.router, "GET", "/api/v1/apps/99999/tools", nil)
	assert.Equal(t, http.StatusNotFound, wGetToolsNonExistentApp.Code)

	wAddToolNonExistentApp := performRequest(api.router, "POST", fmt.Sprintf("/api/v1/apps/99999/tools/%d", tool1.ID), nil)
	assert.Equal(t, http.StatusNotFound, wAddToolNonExistentApp.Code)

	wAddNonExistentTool := performRequest(api.router, "POST", fmt.Sprintf("/api/v1/apps/%d/tools/88888", createdAppID), nil)
	assert.Equal(t, http.StatusNotFound, wAddNonExistentTool.Code)
}

type AppResponseWrapper struct {
	Data AppResponse `json:"data"`
}

func TestAppEndpoints(t *testing.T) {
	api, db := setupTestAPI(t)

	user := &models.User{Email: "testendpoints@example.com", Name: "Test User", IsAdmin: true, EmailVerified: true}
	err := user.Create(db)
	assert.NoError(t, err)

	createAppInput := TestAppInput{
		Data: struct {
			Type       string `json:"type" binding:"required,eq=app"`
			Attributes struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []uint     `json:"datasource_ids"`
				LLMIDs          []uint     `json:"llm_ids"`
				ToolIDs         []uint     `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			} `json:"attributes" binding:"required"`
		}{
			Type: "app",
			Attributes: struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []uint     `json:"datasource_ids"`
				LLMIDs          []uint     `json:"llm_ids"`
				ToolIDs         []uint     `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			}{
				Name:            "Test App Old",
				Description:     "Test Description Old",
				UserID:          user.ID,
				DatasourceIDs:   []uint{},
				LLMIDs:          []uint{},
				ToolIDs:         []uint{},
				MonthlyBudget:   nil,
				BudgetStartDate: nil,
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/apps", createAppInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response AppResponseWrapper
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "app", response.Data.Type)
	assert.Equal(t, "Test App Old", response.Data.Attributes.Name)
	assert.Empty(t, response.Data.Attributes.ToolIDs)
}

func TestAppPagination(t *testing.T) {
	api, db := setupTestAPI(t)
	user := &models.User{Email: "testpagination@example.com", Name: "Pagination User", IsAdmin: true, EmailVerified: true}
	err := user.Create(db)
	assert.NoError(t, err)

	for i := 0; i < 10; i++ {
		createAppInput := TestAppInput{
			Data: struct {
				Type       string `json:"type" binding:"required,eq=app"`
				Attributes struct {
					Name            string     `json:"name"`
					Description     string     `json:"description"`
					UserID          uint       `json:"user_id"`
					DatasourceIDs   []uint     `json:"datasource_ids"`
					LLMIDs          []uint     `json:"llm_ids"`
					ToolIDs         []uint     `json:"tool_ids"`
					MonthlyBudget   *float64   `json:"monthly_budget"`
					BudgetStartDate *time.Time `json:"budget_start_date"`
				} `json:"attributes" binding:"required"`
			}{
				Type: "app",
				Attributes: struct {
					Name            string     `json:"name"`
					Description     string     `json:"description"`
					UserID          uint       `json:"user_id"`
					DatasourceIDs   []uint     `json:"datasource_ids"`
					LLMIDs          []uint     `json:"llm_ids"`
					ToolIDs         []uint     `json:"tool_ids"`
					MonthlyBudget   *float64   `json:"monthly_budget"`
					BudgetStartDate *time.Time `json:"budget_start_date"`
				}{
					Name:    fmt.Sprintf("Test App %d", i),
					UserID:  user.ID,
					ToolIDs: []uint{},
				},
			},
		}
		w := performRequest(api.router, "POST", "/api/v1/apps", createAppInput)
		assert.Equal(t, http.StatusCreated, w.Code)
	}

	w := performRequest(api.router, "GET", "/api/v1/apps?page=2&page_size=5", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	var response AppListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	assert.Len(t, response.Data, 5)
}

func TestCreateAppPrivacyScoreMismatch(t *testing.T) {
	api, db := setupTestAPI(t)
	user := &models.User{Email: "testprivacy@example.com", Name: "Privacy User", IsAdmin: true, EmailVerified: true}
	err := user.Create(db)
	assert.NoError(t, err)

	llm := &models.LLM{Name: "Low Privacy LLM", PrivacyScore: 1, Vendor: "Test", DefaultModel: "test"}
	err = db.Create(llm).Error
	assert.NoError(t, err)

	datasource := &models.Datasource{Name: "High Privacy DS", PrivacyScore: 5, UserID: user.ID}
	err = db.Create(datasource).Error
	assert.NoError(t, err)

	createAppInput := TestAppInput{
		Data: struct {
			Type       string `json:"type" binding:"required,eq=app"`
			Attributes struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []uint     `json:"datasource_ids"`
				LLMIDs          []uint     `json:"llm_ids"`
				ToolIDs         []uint     `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			} `json:"attributes" binding:"required"`
		}{
			Type: "app",
			Attributes: struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []uint     `json:"datasource_ids"`
				LLMIDs          []uint     `json:"llm_ids"`
				ToolIDs         []uint     `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			}{
				Name:          "Privacy Mismatch App",
				UserID:        user.ID,
				DatasourceIDs: []uint{datasource.ID},
				LLMIDs:        []uint{llm.ID},
				ToolIDs:       []uint{},
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/apps", createAppInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var errorResponse ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
	assert.NoError(t, err)
	assert.Len(t, errorResponse.Errors, 1)
	// The refusal must name the offending pair and both numbers: "Please try
	// again" was unactionable because nothing about the request had changed.
	detail := errorResponse.Errors[0].Detail
	assert.Contains(t, detail, "High Privacy DS", "must name the offending resource")
	assert.Contains(t, detail, "Low Privacy LLM", "must name the provider compared against")
	assert.Contains(t, detail, "5", "must state the required privacy level")
}

func TestUpdateAppPrivacyScoreMismatch(t *testing.T) {
	api, db := setupTestAPI(t)
	user := &models.User{Email: "testupdateprivacy@example.com", Name: "Update Privacy User", IsAdmin: true, EmailVerified: true}
	err := user.Create(db)
	assert.NoError(t, err)

	llm := &models.LLM{Name: "Low Privacy LLM Update", PrivacyScore: 1, Vendor: "Test", DefaultModel: "test-update"}
	err = db.Create(llm).Error
	assert.NoError(t, err)

	datasource := &models.Datasource{Name: "High Privacy DS Update", PrivacyScore: 5, UserID: user.ID}
	err = db.Create(datasource).Error
	assert.NoError(t, err)

	app := &models.App{Name: "App to Update Privacy", UserID: user.ID}
	err = db.Create(app).Error
	assert.NoError(t, err)
	err = db.Model(app).Association("LLMs").Append(llm)
	assert.NoError(t, err)

	updateAppInput := TestAppInput{
		Data: struct {
			Type       string `json:"type" binding:"required,eq=app"`
			Attributes struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []uint     `json:"datasource_ids"`
				LLMIDs          []uint     `json:"llm_ids"`
				ToolIDs         []uint     `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			} `json:"attributes" binding:"required"`
		}{
			Type: "app",
			Attributes: struct {
				Name            string     `json:"name"`
				Description     string     `json:"description"`
				UserID          uint       `json:"user_id"`
				DatasourceIDs   []uint     `json:"datasource_ids"`
				LLMIDs          []uint     `json:"llm_ids"`
				ToolIDs         []uint     `json:"tool_ids"`
				MonthlyBudget   *float64   `json:"monthly_budget"`
				BudgetStartDate *time.Time `json:"budget_start_date"`
			}{
				Name:          "Updated App Privacy",
				UserID:        user.ID,
				DatasourceIDs: []uint{datasource.ID},
				LLMIDs:        []uint{llm.ID},
				ToolIDs:       []uint{},
			},
		},
	}

	w := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/apps/%d", app.ID), updateAppInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	var errorResponse ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &errorResponse)
	assert.NoError(t, err)
	assert.Len(t, errorResponse.Errors, 1)
	// The refusal must name the offending pair and both numbers: "Please try
	// again" was unactionable because nothing about the request had changed.
	detail := errorResponse.Errors[0].Detail
	assert.Contains(t, detail, "High Privacy DS", "must name the offending resource")
	assert.Contains(t, detail, "Low Privacy LLM", "must name the provider compared against")
	assert.Contains(t, detail, "5", "must state the required privacy level")
}

// performRequest is removed as it's defined in api_test.go
// setupTestAPI is a helper from existing tests.
// Ensure it initializes the DB and API correctly.
// It's assumed that setupTestAPI internally calls models.InitModels(db)
// which now includes AppTool migration.
// For this example, I'll stub a simplified version if not provided.
// func setupTestAPI(t *testing.T) (*API, *gorm.DB) {
// 	gin.SetMode(gin.TestMode)
// 	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
// 	assert.NoError(t, err)
// 	models.InitModels(db) // Ensure this migrates AppTool
// 	service := services.NewService(db)
// 	// Simplified auth service for testing
// 	authService := auth.NewAuthService(&auth.Config{DB: db, TestMode: true}, nil, service, nil)
// 	api := NewAPI(service, true, authService, &auth.Config{DB: db, TestMode: true}, nil, embed.FS{}, nil)
// 	return api, db
// }

// ErrorResponse struct, assuming it's defined globally or in models
// type ErrorResponse struct {
// 	Errors []struct {
// 		Title  string `json:"title"`
// 		Detail string `json:"detail"`
// 	} `json:"errors"`
// }
// performRequest is defined in api_test.go

// A PATCH that leaves monthly_budget out keeps the App's budget. With null
// meaning "no limit", reading an omitted key as null silently lifted the
// App's spending limit. An explicit null still clears it.
func TestUpdateApp_OmittedBudgetIsKept(t *testing.T) {
	api, db := setupTestAPI(t)
	user := &models.User{Email: "budgetkeep@example.com", Name: "Budget Keep", IsAdmin: true, EmailVerified: true}
	require.NoError(t, user.Create(db))

	budget := 50.0
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	app := &models.App{Name: "Budgeted", UserID: user.ID, MonthlyBudget: &budget, BudgetStartDate: &start}
	require.NoError(t, db.Create(app).Error)

	patch := func(attrs map[string]interface{}) {
		t.Helper()
		body := map[string]interface{}{"data": map[string]interface{}{"type": "app", "attributes": attrs}}
		w := performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/apps/%d", app.ID), body)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	reload := func() *models.App {
		t.Helper()
		var got models.App
		require.NoError(t, db.First(&got, app.ID).Error)
		return &got
	}

	patch(map[string]interface{}{"name": "Renamed", "user_id": user.ID})
	got := reload()
	assert.Equal(t, "Renamed", got.Name)
	require.NotNil(t, got.MonthlyBudget, "an omitted monthly_budget must not become no limit")
	assert.Equal(t, 50.0, *got.MonthlyBudget)
	require.NotNil(t, got.BudgetStartDate, "an omitted budget must keep its start date")
	assert.True(t, start.Equal(*got.BudgetStartDate))

	patch(map[string]interface{}{"name": "Renamed", "user_id": user.ID, "monthly_budget": nil})
	assert.Nil(t, reload().MonthlyBudget, "an explicit null clears the budget")
}

// The key probe fails closed: an unreadable body must not read as "every key
// omitted", which would keep values the caller asked to change, nor as
// "nothing to keep".
func TestBoundAttributeKeys_FailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := func(body []byte) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		if body != nil {
			c.Set(gin.BodyBytesKey, body)
		}
		return c
	}

	_, err := boundAttributeKeys(ctx(nil))
	assert.Error(t, err, "no bound body")
	_, err = boundAttributeKeys(ctx([]byte("not json")))
	assert.Error(t, err, "malformed body")
	_, err = boundAttributeKeys(ctx([]byte(`{"data":{"type":"app"}}`)))
	assert.Error(t, err, "no attributes object")

	present, err := boundAttributeKeys(ctx([]byte(`{"data":{"attributes":{"name":"x","monthly_budget":null}}}`)))
	require.NoError(t, err)
	assert.Contains(t, present, "monthly_budget", "an explicit null is present")
	assert.NotContains(t, present, "budget_start_date")
}
