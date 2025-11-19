package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/stretchr/testify/assert"
)

// Additional tests for proxy core functions

func TestProxy_New(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetSvc := budget.NewService(db, notificationSvc)

	proxyCfg := &Config{Port: 8080}

	t.Run("Create new proxy with interface", func(t *testing.T) {
		proxy := New(service, budgetSvc, proxyCfg)

		assert.NotNil(t, proxy)
		assert.Equal(t, 8080, proxy.config.Port)
		assert.NotNil(t, proxy.credValidator)
		assert.NotNil(t, proxy.modelValidator)
	})
}

func TestProxy_NewProxy(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetSvc := budget.NewService(db, notificationSvc)

	proxyCfg := &Config{Port: 9090}

	t.Run("Create proxy with legacy constructor", func(t *testing.T) {
		proxy := NewProxy(service, proxyCfg, budgetSvc)

		assert.NotNil(t, proxy)
		assert.Equal(t, 9090, proxy.config.Port)
	})
}

func TestProxy_Reload(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetSvc := budget.NewService(db, notificationSvc)

	proxyCfg := &Config{Port: 8080}
	proxy := New(service, budgetSvc, proxyCfg)

	t.Run("Reload resources successfully", func(t *testing.T) {
		// Create test LLM
		user, err := service.CreateUser(services.UserDTO{
			Email:    "reload@example.com",
			Password: "password",
			Name:     "Reload User",
		})
		assert.NoError(t, err)

		_, err = service.CreateLLM("Reload LLM", "Test LLM", "gpt-4", 5, "", "", "", models.OPENAI, true, nil, "", nil, nil, nil)
		assert.NoError(t, err)

		// Reload proxy
		err = proxy.Reload()
		assert.NoError(t, err)

		// Verify LLM was loaded
		llm, exists := proxy.GetLLM("reload-llm")
		assert.True(t, exists)
		assert.Equal(t, "Reload LLM", llm.Name)

		_ = user
	})
}

func TestProxy_AddFilter(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetSvc := budget.NewService(db, notificationSvc)

	proxyCfg := &Config{Port: 8080}
	proxy := New(service, budgetSvc, proxyCfg)

	t.Run("Add filter successfully", func(t *testing.T) {
		filter := &models.Filter{Name: "Test Filter"}
		proxy.AddFilter(filter)
		assert.NotNil(t, proxy)
	})
}

func TestProxy_ResponseHooks(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetSvc := budget.NewService(db, notificationSvc)

	proxyCfg := &Config{Port: 8080}
	proxy := New(service, budgetSvc, proxyCfg)

	t.Run("Has response hooks", func(t *testing.T) {
		assert.False(t, proxy.hasResponseHooks())

		hook := NewExampleResponseHook("test")
		proxy.AddResponseHook(hook)

		assert.True(t, proxy.hasResponseHooks())
	})

	t.Run("Get response hook manager", func(t *testing.T) {
		manager := proxy.GetResponseHookManager()
		assert.NotNil(t, manager)
	})
}

func TestProxy_AuthHooks(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetSvc := budget.NewService(db, notificationSvc)

	proxyCfg := &Config{Port: 8080}
	proxy := New(service, budgetSvc, proxyCfg)

	t.Run("Set auth hooks", func(t *testing.T) {
		hooks := &AuthHooks{}
		proxy.SetAuthHooks(hooks)
		assert.NotNil(t, proxy.credValidator)
	})

	t.Run("Set post auth callback", func(t *testing.T) {
		callback := func(w http.ResponseWriter, r *http.Request, appID uint) bool {
			return true
		}
		proxy.SetPostAuthCallback(callback)
		assert.NotNil(t, proxy.credValidator)
	})
}

func TestProxy_GetDatasource(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetSvc := budget.NewService(db, notificationSvc)

	proxyCfg := &Config{Port: 8080}
	proxy := New(service, budgetSvc, proxyCfg)

	t.Run("Get existing datasource", func(t *testing.T) {
		ds := &models.Datasource{Name: "Test DS"}
		proxy.mu.Lock()
		proxy.datasources["test-ds"] = ds
		proxy.mu.Unlock()

		result, exists := proxy.GetDatasource("test-ds")
		assert.True(t, exists)
		assert.Equal(t, "Test DS", result.Name)
	})

	t.Run("Get non-existent datasource", func(t *testing.T) {
		result, exists := proxy.GetDatasource("missing")
		assert.False(t, exists)
		assert.Nil(t, result)
	})
}

func TestProxy_GetLLM(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetSvc := budget.NewService(db, notificationSvc)

	proxyCfg := &Config{Port: 8080}
	proxy := New(service, budgetSvc, proxyCfg)

	t.Run("Get existing LLM", func(t *testing.T) {
		llm := &models.LLM{Name: "Test LLM", Vendor: models.OPENAI}
		proxy.mu.Lock()
		proxy.llms["test-llm"] = llm
		proxy.mu.Unlock()

		result, exists := proxy.GetLLM("test-llm")
		assert.True(t, exists)
		assert.Equal(t, "Test LLM", result.Name)
	})

	t.Run("Get non-existent LLM", func(t *testing.T) {
		result, exists := proxy.GetLLM("missing")
		assert.False(t, exists)
		assert.Nil(t, result)
	})
}

func TestProxy_CloudflareHeadersMiddleware(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetSvc := budget.NewService(db, notificationSvc)

	proxyCfg := &Config{Port: 8080}
	proxy := New(service, budgetSvc, proxyCfg)

	t.Run("Set keepalive headers", func(t *testing.T) {
		testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		middleware := proxy.cloudflareHeadersMiddleware(testHandler)
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		assert.Equal(t, "keep-alive", w.Header().Get("Connection"))
		assert.Equal(t, "timeout=300", w.Header().Get("Keep-Alive"))
		assert.Equal(t, "no", w.Header().Get("X-Accel-Buffering"))
	})
}

func TestProxy_Handler(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetSvc := budget.NewService(db, notificationSvc)

	proxyCfg := &Config{Port: 8080}
	proxy := New(service, budgetSvc, proxyCfg)

	t.Run("Get handler returns router", func(t *testing.T) {
		handler := proxy.Handler()
		assert.NotNil(t, handler)
	})
}
