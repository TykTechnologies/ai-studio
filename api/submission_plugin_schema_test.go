package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// The portal submission endpoint must enforce the plugin's submission_schema
// end to end: a payload that violates it is refused with 400 and nothing is stored.
func TestCreateSubmission_PluginSchemaEnforcedAtAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	service := services.NewService(db)
	service.NotificationService = services.NewTestNotificationService(db)
	api := &API{service: service}

	plugin := &models.Plugin{Name: "asset-catalog", Command: "/usr/bin/asset-catalog", HookType: models.HookTypeResourceProvider, IsActive: true}
	require.NoError(t, db.Create(plugin).Error)
	schema := `{"type":"object","properties":{"name":{"type":"string","minLength":1},"purpose":{"type":"string"}},"required":["name","purpose"],"additionalProperties":false}`
	require.NoError(t, service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{
		{Slug: "agent", Name: "Agent", SupportsSubmissions: true, SubmissionSchema: schema},
	}))
	prt, err := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "agent")
	require.NoError(t, err)

	user := &models.User{Email: "user@test.com", Name: "User", EmailVerified: true}
	require.NoError(t, user.Create(db))

	router := gin.New()
	group := router.Group("/common")
	group.Use(func(c *gin.Context) { c.Set("user", user); c.Next() })
	group.POST("/submissions", api.createSubmission)

	post := func(payload map[string]interface{}) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{
			"resource_type":           models.SubmissionResourceTypePlugin,
			"plugin_resource_type_id": prt.ID,
			"status":                  models.SubmissionStatusDraft,
			"resource_payload":        payload,
			"suggested_privacy":       10,
			"primary_contact":         "contact@test.com",
		}}})
		req, _ := http.NewRequest("POST", "/common/submissions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	count := func() int64 {
		var n int64
		require.NoError(t, db.Model(&models.Submission{}).Count(&n).Error)
		return n
	}

	// Missing required property.
	w := post(map[string]interface{}{"name": "No purpose"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "purpose")
	assert.Equal(t, int64(0), count(), "rejected submissions are not stored")

	// Wrong type and an unexpected property.
	w = post(map[string]interface{}{"name": 42, "purpose": "x", "shell": "rm -rf /"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, int64(0), count())

	// A conforming payload is accepted.
	w = post(map[string]interface{}{"name": "Triage Agent", "purpose": "Route tickets"})
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, int64(1), count())
}
