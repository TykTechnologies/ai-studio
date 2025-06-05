package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockService for testing
type MockAnalyticsService struct {
	mock.Mock
}

func (m *MockAnalyticsService) GetApp(appID uint) (*App, error) {
	args := m.Called(appID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*App), args.Error(1)
}

func (m *MockAnalyticsService) GetTokenUsageAndCostForApp(startDate, endDate string, appID uint) (interface{}, error) {
	args := m.Called(startDate, endDate, appID)
	return args.Get(0), args.Error(1)
}

func (m *MockAnalyticsService) GetBudgetUsageForApp(appID uint) (interface{}, error) {
	args := m.Called(appID)
	return args.Get(0), args.Error(1)
}

func (m *MockAnalyticsService) GetAppInteractionsOverTime(appID uint, startDate, endDate string) (interface{}, error) {
	args := m.Called(appID, startDate, endDate)
	return args.Get(0), args.Error(1)
}

// Mock App struct for testing
type App struct {
	ID     uint `json:"id"`
	UserID uint `json:"user_id"`
	Name   string `json:"name"`
}

func setupTestAPI() (*API, *MockAnalyticsService) {
	mockService := &MockAnalyticsService{}
	api := &API{
		service: mockService,
	}
	return api, mockService
}

func setupAuthenticatedContext(userID uint) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := performRequest(nil, "GET", "/test", nil)
	c, _ := gin.CreateTestContext(w)
	c.Set("userID", userID)
	return c
}

func setupUnauthenticatedContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	w := performRequest(nil, "GET", "/test", nil)
	c, _ := gin.CreateTestContext(w)
	return c
}

func TestGetUserAppTokenUsageAndCost(t *testing.T) {
	t.Run("should reject unauthenticated requests", func(t *testing.T) {
		api, _ := setupTestAPI()
		c := setupUnauthenticatedContext()
		
		api.getUserAppTokenUsageAndCost(c)
		
		assert.Equal(t, http.StatusUnauthorized, c.Writer.Status())
	})

	t.Run("should reject requests without app_id parameter", func(t *testing.T) {
		api, _ := setupTestAPI()
		c := setupAuthenticatedContext(1)
		
		api.getUserAppTokenUsageAndCost(c)
		
		assert.Equal(t, http.StatusBadRequest, c.Writer.Status())
	})

	t.Run("should reject requests with invalid app_id parameter", func(t *testing.T) {
		api, _ := setupTestAPI()
		c := setupAuthenticatedContext(1)
		c.Request.URL.RawQuery = "app_id=invalid"
		
		api.getUserAppTokenUsageAndCost(c)
		
		assert.Equal(t, http.StatusBadRequest, c.Writer.Status())
	})

	t.Run("should reject requests for non-existent app", func(t *testing.T) {
		api, mockService := setupTestAPI()
		c := setupAuthenticatedContext(1)
		c.Request.URL.RawQuery = "app_id=999"
		
		mockService.On("GetApp", uint(999)).Return(nil, assert.AnError)
		
		api.getUserAppTokenUsageAndCost(c)
		
		assert.Equal(t, http.StatusNotFound, c.Writer.Status())
		mockService.AssertExpectations(t)
	})

	t.Run("should reject requests for apps not owned by user", func(t *testing.T) {
		api, mockService := setupTestAPI()
		c := setupAuthenticatedContext(1)
		c.Request.URL.RawQuery = "app_id=123"
		
		app := &App{ID: 123, UserID: 2, Name: "Test App"}
		mockService.On("GetApp", uint(123)).Return(app, nil)
		
		api.getUserAppTokenUsageAndCost(c)
		
		assert.Equal(t, http.StatusForbidden, c.Writer.Status())
		mockService.AssertExpectations(t)
	})

	t.Run("should handle service errors gracefully", func(t *testing.T) {
		api, mockService := setupTestAPI()
		c := setupAuthenticatedContext(1)
		c.Request.URL.RawQuery = "app_id=123&start_date=2023-01-01&end_date=2023-12-31"
		
		app := &App{ID: 123, UserID: 1, Name: "Test App"}
		mockService.On("GetApp", uint(123)).Return(app, nil)
		mockService.On("GetTokenUsageAndCostForApp", "2023-01-01", "2023-12-31", uint(123)).Return(nil, assert.AnError)
		
		api.getUserAppTokenUsageAndCost(c)
		
		assert.Equal(t, http.StatusInternalServerError, c.Writer.Status())
		mockService.AssertExpectations(t)
	})

	t.Run("should return data for valid requests", func(t *testing.T) {
		api, mockService := setupTestAPI()
		c := setupAuthenticatedContext(1)
		c.Request.URL.RawQuery = "app_id=123&start_date=2023-01-01&end_date=2023-12-31"
		
		app := &App{ID: 123, UserID: 1, Name: "Test App"}
		expectedData := map[string]interface{}{"tokens": 1000, "cost": 0.05}
		
		mockService.On("GetApp", uint(123)).Return(app, nil)
		mockService.On("GetTokenUsageAndCostForApp", "2023-01-01", "2023-12-31", uint(123)).Return(expectedData, nil)
		
		api.getUserAppTokenUsageAndCost(c)
		
		assert.Equal(t, http.StatusOK, c.Writer.Status())
		mockService.AssertExpectations(t)
	})
}

func TestGetUserAppBudgetUsage(t *testing.T) {
	t.Run("should reject unauthenticated requests", func(t *testing.T) {
		api, _ := setupTestAPI()
		c := setupUnauthenticatedContext()
		
		api.getUserAppBudgetUsage(c)
		
		assert.Equal(t, http.StatusUnauthorized, c.Writer.Status())
	})

	t.Run("should reject requests without app_id parameter", func(t *testing.T) {
		api, _ := setupTestAPI()
		c := setupAuthenticatedContext(1)
		
		api.getUserAppBudgetUsage(c)
		
		assert.Equal(t, http.StatusBadRequest, c.Writer.Status())
	})

	t.Run("should reject requests with invalid app_id parameter", func(t *testing.T) {
		api, _ := setupTestAPI()
		c := setupAuthenticatedContext(1)
		c.Request.URL.RawQuery = "app_id=invalid"
		
		api.getUserAppBudgetUsage(c)
		
		assert.Equal(t, http.StatusBadRequest, c.Writer.Status())
	})

	t.Run("should reject requests for non-existent app", func(t *testing.T) {
		api, mockService := setupTestAPI()
		c := setupAuthenticatedContext(1)
		c.Request.URL.RawQuery = "app_id=999"
		
		mockService.On("GetApp", uint(999)).Return(nil, assert.AnError)
		
		api.getUserAppBudgetUsage(c)
		
		assert.Equal(t, http.StatusNotFound, c.Writer.Status())
		mockService.AssertExpectations(t)
	})

	t.Run("should reject requests for apps not owned by user", func(t *testing.T) {
		api, mockService := setupTestAPI()
		c := setupAuthenticatedContext(1)
		c.Request.URL.RawQuery = "app_id=123"
		
		app := &App{ID: 123, UserID: 2, Name: "Test App"}
		mockService.On("GetApp", uint(123)).Return(app, nil)
		
		api.getUserAppBudgetUsage(c)
		
		assert.Equal(t, http.StatusForbidden, c.Writer.Status())
		mockService.AssertExpectations(t)
	})

	t.Run("should handle service errors gracefully", func(t *testing.T) {
		api, mockService := setupTestAPI()
		c := setupAuthenticatedContext(1)
		c.Request.URL.RawQuery = "app_id=123"
		
		app := &App{ID: 123, UserID: 1, Name: "Test App"}
		mockService.On("GetApp", uint(123)).Return(app, nil)
		mockService.On("GetBudgetUsageForApp", uint(123)).Return(nil, assert.AnError)
		
		api.getUserAppBudgetUsage(c)
		
		assert.Equal(t, http.StatusInternalServerError, c.Writer.Status())
		mockService.AssertExpectations(t)
	})

	t.Run("should return data for valid requests", func(t *testing.T) {
		api, mockService := setupTestAPI()
		c := setupAuthenticatedContext(1)
		c.Request.URL.RawQuery = "app_id=123"
		
		app := &App{ID: 123, UserID: 1, Name: "Test App"}
		expectedData := map[string]interface{}{"budget": 100.0, "used": 25.0, "remaining": 75.0}
		
		mockService.On("GetApp", uint(123)).Return(app, nil)
		mockService.On("GetBudgetUsageForApp", uint(123)).Return(expectedData, nil)
		
		api.getUserAppBudgetUsage(c)
		
		assert.Equal(t, http.StatusOK, c.Writer.Status())
		mockService.AssertExpectations(t)
	})
}

func TestGetUserAppInteractionsOverTime(t *testing.T) {
	t.Run("should reject unauthenticated requests", func(t *testing.T) {
		api, _ := setupTestAPI()
		c := setupUnauthenticatedContext()
		
		api.getUserAppInteractionsOverTime(c)
		
		assert.Equal(t, http.StatusUnauthorized, c.Writer.Status())
	})

	t.Run("should reject requests without app_id parameter", func(t *testing.T) {
		api, _ := setupTestAPI()
		c := setupAuthenticatedContext(1)
		
		api.getUserAppInteractionsOverTime(c)
		
		assert.Equal(t, http.StatusBadRequest, c.Writer.Status())
	})

	t.Run("should reject requests with invalid app_id parameter", func(t *testing.T) {
		api, _ := setupTestAPI()
		c := setupAuthenticatedContext(1)
		c.Request.URL.RawQuery = "app_id=invalid"
		
		api.getUserAppInteractionsOverTime(c)
		
		assert.Equal(t, http.StatusBadRequest, c.Writer.Status())
	})

	t.Run("should reject requests for non-existent app", func(t *testing.T) {
		api, mockService := setupTestAPI()
		c := setupAuthenticatedContext(1)
		c.Request.URL.RawQuery = "app_id=999"
		
		mockService.On("GetApp", uint(999)).Return(nil, assert.AnError)
		
		api.getUserAppInteractionsOverTime(c)
		
		assert.Equal(t, http.StatusNotFound, c.Writer.Status())
		mockService.AssertExpectations(t)
	})

	t.Run("should reject requests for apps not owned by user", func(t *testing.T) {
		api, mockService := setupTestAPI()
		c := setupAuthenticatedContext(1)
		c.Request.URL.RawQuery = "app_id=123"
		
		app := &App{ID: 123, UserID: 2, Name: "Test App"}
		mockService.On("GetApp", uint(123)).Return(app, nil)
		
		api.getUserAppInteractionsOverTime(c)
		
		assert.Equal(t, http.StatusForbidden, c.Writer.Status())
		mockService.AssertExpectations(t)
	})

	t.Run("should handle service errors gracefully", func(t *testing.T) {
		api, mockService := setupTestAPI()
		c := setupAuthenticatedContext(1)
		c.Request.URL.RawQuery = "app_id=123&start_date=2023-01-01&end_date=2023-12-31"
		
		app := &App{ID: 123, UserID: 1, Name: "Test App"}
		mockService.On("GetApp", uint(123)).Return(app, nil)
		mockService.On("GetAppInteractionsOverTime", uint(123), "2023-01-01", "2023-12-31").Return(nil, assert.AnError)
		
		api.getUserAppInteractionsOverTime(c)
		
		assert.Equal(t, http.StatusInternalServerError, c.Writer.Status())
		mockService.AssertExpectations(t)
	})

	t.Run("should return data for valid requests", func(t *testing.T) {
		api, mockService := setupTestAPI()
		c := setupAuthenticatedContext(1)
		c.Request.URL.RawQuery = "app_id=123&start_date=2023-01-01&end_date=2023-12-31"
		
		app := &App{ID: 123, UserID: 1, Name: "Test App"}
		expectedData := []map[string]interface{}{
			{"date": "2023-01-01", "interactions": 10},
			{"date": "2023-01-02", "interactions": 15},
		}
		
		mockService.On("GetApp", uint(123)).Return(app, nil)
		mockService.On("GetAppInteractionsOverTime", uint(123), "2023-01-01", "2023-12-31").Return(expectedData, nil)
		
		api.getUserAppInteractionsOverTime(c)
		
		assert.Equal(t, http.StatusOK, c.Writer.Status())
		mockService.AssertExpectations(t)
	})
}

// Integration tests using the performRequest helper
func TestPortalAnalyticsHandlersIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	t.Run("token usage endpoint integration", func(t *testing.T) {
		api, mockService := setupTestAPI()
		router := gin.New()
		
		// Add middleware to set userID
		router.Use(func(c *gin.Context) {
			c.Set("userID", uint(1))
			c.Next()
		})
		
		router.GET("/analytics/token-usage", api.getUserAppTokenUsageAndCost)
		
		app := &App{ID: 123, UserID: 1, Name: "Test App"}
		expectedData := map[string]interface{}{"tokens": 1000, "cost": 0.05}
		
		mockService.On("GetApp", uint(123)).Return(app, nil)
		mockService.On("GetTokenUsageAndCostForApp", "2023-01-01", "2023-12-31", uint(123)).Return(expectedData, nil)
		
		w := performRequest(router, "GET", "/analytics/token-usage?app_id=123&start_date=2023-01-01&end_date=2023-12-31", nil)
		
		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, expectedData, response)
		
		mockService.AssertExpectations(t)
	})

	t.Run("budget usage endpoint integration", func(t *testing.T) {
		api, mockService := setupTestAPI()
		router := gin.New()
		
		// Add middleware to set userID
		router.Use(func(c *gin.Context) {
			c.Set("userID", uint(1))
			c.Next()
		})
		
		router.GET("/analytics/budget-usage", api.getUserAppBudgetUsage)
		
		app := &App{ID: 123, UserID: 1, Name: "Test App"}
		expectedData := map[string]interface{}{"budget": 100.0, "used": 25.0}
		
		mockService.On("GetApp", uint(123)).Return(app, nil)
		mockService.On("GetBudgetUsageForApp", uint(123)).Return(expectedData, nil)
		
		w := performRequest(router, "GET", "/analytics/budget-usage?app_id=123", nil)
		
		assert.Equal(t, http.StatusOK, w.Code)
		
		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, expectedData, response)
		
		mockService.AssertExpectations(t)
	})

	t.Run("interactions over time endpoint integration", func(t *testing.T) {
		api, mockService := setupTestAPI()
		router := gin.New()
		
		// Add middleware to set userID
		router.Use(func(c *gin.Context) {
			c.Set("userID", uint(1))
			c.Next()
		})
		
		router.GET("/analytics/interactions", api.getUserAppInteractionsOverTime)
		
		app := &App{ID: 123, UserID: 1, Name: "Test App"}
		expectedData := []map[string]interface{}{
			{"date": "2023-01-01", "interactions": 10},
		}
		
		mockService.On("GetApp", uint(123)).Return(app, nil)
		mockService.On("GetAppInteractionsOverTime", uint(123), "2023-01-01", "2023-12-31").Return(expectedData, nil)
		
		w := performRequest(router, "GET", "/analytics/interactions?app_id=123&start_date=2023-01-01&end_date=2023-12-31", nil)
		
		assert.Equal(t, http.StatusOK, w.Code)
		
		var response []map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, expectedData, response)
		
		mockService.AssertExpectations(t)
	})
}
