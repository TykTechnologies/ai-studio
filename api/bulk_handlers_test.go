package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/services/model_router"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bulkBody(action string, ids ...uint) map[string]interface{} {
	return map[string]interface{}{"action": action, "ids": ids}
}

func decodeBulk(t *testing.T, w *httptest.ResponseRecorder) BulkActionResponse {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp BulkActionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

// A bulk request reports every id: the ones that went through and the ones
// the service refused, with the same reason the single route would give.
func TestBulkLLMs_MixedSuccessAndFailure(t *testing.T) {
	api, db := setupTestAPI(t)

	target := &models.LLM{Name: "fallback", Vendor: models.MOCK_VENDOR, Active: true}
	require.NoError(t, db.Create(target).Error)
	primary := &models.LLM{Name: "primary", Vendor: models.MOCK_VENDOR, Active: true,
		Failover: models.LLMFailover{Targets: []models.LLMFailoverTarget{{LLMID: target.ID}}}}
	require.NoError(t, db.Create(primary).Error)
	spare := &models.LLM{Name: "spare", Vendor: models.MOCK_VENDOR, Active: true}
	require.NoError(t, db.Create(spare).Error)

	// deactivate: all three flip, and the order of results follows the ids.
	resp := decodeBulk(t, performRequest(api.router, "POST", "/api/v1/llms/bulk", bulkBody("deactivate", spare.ID, primary.ID, target.ID)))
	assert.Equal(t, "deactivate", resp.Data.Action)
	assert.Equal(t, 3, resp.Data.Succeeded)
	assert.Equal(t, 0, resp.Data.Failed)
	require.Len(t, resp.Data.Results, 3)
	assert.Equal(t, spare.ID, resp.Data.Results[0].ID)
	assert.True(t, resp.Data.Results[0].OK)
	assert.Empty(t, resp.Data.Results[0].Error)
	var stored models.LLM
	require.NoError(t, db.First(&stored, spare.ID).Error)
	assert.False(t, stored.Active)

	// activate one, with an unknown id alongside.
	resp = decodeBulk(t, performRequest(api.router, "POST", "/api/v1/llms/bulk", bulkBody("activate", spare.ID, 9999)))
	assert.Equal(t, 1, resp.Data.Succeeded)
	assert.Equal(t, 1, resp.Data.Failed)
	assert.True(t, resp.Data.Results[0].OK)
	assert.False(t, resp.Data.Results[1].OK)
	assert.Equal(t, "LLM not found", resp.Data.Results[1].Error)
	require.NoError(t, db.First(&stored, spare.ID).Error)
	assert.True(t, stored.Active)

	// delete: the failover target is refused with the service's own message,
	// the others are soft-deleted.
	resp = decodeBulk(t, performRequest(api.router, "POST", "/api/v1/llms/bulk", bulkBody("delete", target.ID, spare.ID)))
	assert.Equal(t, "delete", resp.Data.Action)
	assert.Equal(t, 1, resp.Data.Succeeded)
	assert.Equal(t, 1, resp.Data.Failed)
	assert.Equal(t, target.ID, resp.Data.Results[0].ID)
	assert.False(t, resp.Data.Results[0].OK)
	assert.Contains(t, resp.Data.Results[0].Error, `LLM "fallback" is a failover target of: [primary]; remove it from those waterfalls first`)
	assert.True(t, resp.Data.Results[1].OK)

	var count int64
	require.NoError(t, db.Model(&models.LLM{}).Count(&count).Error)
	assert.Equal(t, int64(2), count, "spare is gone, fallback and primary remain")
	require.NoError(t, db.Unscoped().Model(&models.LLM{}).Where("id = ?", spare.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count, "soft delete, as on the single route")
}

func TestBulk_Validation(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "test-key")
	api, db := setupTestAPI(t)
	require.NoError(t, db.Create(&models.Filter{Name: "PII"}).Error)

	bad := func(t *testing.T, path string, body interface{}, detail string) {
		t.Helper()
		w := performRequest(api.router, "POST", path, body)
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		var resp ErrorResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Len(t, resp.Errors, 1)
		assert.Equal(t, "Bad Request", resp.Errors[0].Title)
		assert.Contains(t, resp.Errors[0].Detail, detail)
	}

	bad(t, "/api/v1/llms/bulk", bulkBody("delete"), "at least one id")
	bad(t, "/api/v1/llms/bulk", map[string]interface{}{"action": "delete"}, "at least one id")
	bad(t, "/api/v1/llms/bulk", bulkBody("archive", 1), "unsupported action: archive")
	bad(t, "/api/v1/llms/bulk", json.RawMessage(`{"action": "delete", "ids": "1,2"}`), "")

	ids := make([]uint, maxBulkIDs+1)
	for i := range ids {
		ids[i] = uint(i + 1)
	}
	bad(t, "/api/v1/llms/bulk", bulkBody("deactivate", ids...), "at most 100 ids")

	// Exactly 100 is fine (they all fail as unknown, but the request is accepted).
	resp := decodeBulk(t, performRequest(api.router, "POST", "/api/v1/llms/bulk", bulkBody("deactivate", ids[:maxBulkIDs]...)))
	assert.Len(t, resp.Data.Results, maxBulkIDs)
	assert.Equal(t, maxBulkIDs, resp.Data.Failed)

	// Types without a live flag only delete.
	bad(t, "/api/v1/filters/bulk", bulkBody("activate", 1), "no active flag")
	bad(t, "/api/v1/secrets/bulk", bulkBody("deactivate", 1), "")
	resp = decodeBulk(t, performRequest(api.router, "POST", "/api/v1/filters/bulk", bulkBody("delete", 1, 2)))
	assert.Equal(t, 1, resp.Data.Succeeded)
	assert.Equal(t, "filter not found", resp.Data.Results[1].Error)
}

func TestBulk_OtherTypes(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "test-key")
	api, db := setupTestAPI(t)

	tool := &models.Tool{Name: "Weather", Active: true}
	require.NoError(t, db.Create(tool).Error)
	ds := &models.Datasource{Name: "Docs", Active: true}
	require.NoError(t, db.Create(ds).Error)
	app := &models.App{Name: "Copilot", UserID: 1, IsActive: true}
	require.NoError(t, db.Create(app).Error)
	secret := &secrets.Secret{VarName: "API_KEY", Value: "v"}
	require.NoError(t, secrets.CreateSecret(db, secret))

	resp := decodeBulk(t, performRequest(api.router, "POST", "/api/v1/tools/bulk", bulkBody("deactivate", tool.ID)))
	assert.Equal(t, 1, resp.Data.Succeeded)
	var storedTool models.Tool
	require.NoError(t, db.First(&storedTool, tool.ID).Error)
	assert.False(t, storedTool.Active)

	resp = decodeBulk(t, performRequest(api.router, "POST", "/api/v1/datasources/bulk", bulkBody("deactivate", ds.ID, 404)))
	assert.Equal(t, 1, resp.Data.Succeeded)
	assert.Equal(t, "datasource not found", resp.Data.Results[1].Error)

	resp = decodeBulk(t, performRequest(api.router, "POST", "/api/v1/apps/bulk", bulkBody("deactivate", app.ID)))
	assert.Equal(t, 1, resp.Data.Succeeded)
	var storedApp models.App
	require.NoError(t, db.First(&storedApp, app.ID).Error)
	assert.False(t, storedApp.IsActive)
	resp = decodeBulk(t, performRequest(api.router, "POST", "/api/v1/apps/bulk", bulkBody("delete", app.ID, app.ID)))
	assert.Equal(t, 1, resp.Data.Succeeded, "the second delete of the same id finds nothing")
	assert.Equal(t, "app not found", resp.Data.Results[1].Error)

	resp = decodeBulk(t, performRequest(api.router, "POST", "/api/v1/secrets/bulk", bulkBody("delete", secret.ID, 777)))
	assert.Equal(t, 1, resp.Data.Succeeded)
	assert.Equal(t, "secret not found", resp.Data.Results[1].Error)
	var secretCount int64
	require.NoError(t, db.Model(&secrets.Secret{}).Count(&secretCount).Error)
	assert.Equal(t, int64(0), secretCount)

	// Model routers are gated on edition; Community answers 402 for the
	// request as a whole, exactly like the single routes. The test service
	// leaves the router service unset; wire it as services.NewService does.
	api.service.ModelRouterService = model_router.NewService(db)
	w := performRequest(api.router, "POST", "/api/v1/model-routers/bulk", bulkBody("delete", 1))
	if w.Code == http.StatusPaymentRequired {
		assert.Contains(t, w.Body.String(), "Enterprise")
	} else {
		resp = decodeBulk(t, w)
		assert.Equal(t, 1, resp.Data.Failed)
	}
}

func TestBulkSecrets_RequiresSecretKey(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "")
	api, _ := setupTestAPI(t)
	w := performRequest(api.router, "POST", "/api/v1/secrets/bulk", bulkBody("delete", 1))
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// The route's permission comes from the body: publish for the live switch,
// delete for delete, so the annotation matches the single routes exactly.
func TestBulk_PermissionResolver(t *testing.T) {
	api, _ := setupTestAPI(t)

	for _, resource := range []string{"llms", "tools", "datasources", "apps", "filters", "secrets", "model-routers"} {
		e, ok := api.routePerms[routeKey("POST", "/api/v1/"+resource+"/bulk")]
		require.True(t, ok, resource)
		require.True(t, e.dynamic(), resource)

		resolve := func(body string) authz.Permission {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/api/v1/"+resource+"/bulk", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			return e.permission(c)
		}
		assert.Equal(t, authz.Publish(resource), resolve(`{"action":"activate","ids":[1]}`), resource)
		assert.Equal(t, authz.Publish(resource), resolve(`{"action":"deactivate","ids":[1]}`), resource)
		assert.Equal(t, authz.Delete(resource), resolve(`{"action":"delete","ids":[1]}`), resource)
		assert.Equal(t, authz.Delete(resource), resolve(`{"action":"bogus"}`), "unknown actions need the stricter grant")
		assert.Equal(t, authz.Delete(resource), resolve(`not json`), "a malformed body needs the stricter grant")
	}
}

// With the real authorization chain a member without the permission is
// refused before any id is touched, and an admin goes through. The body is
// read by the resolver and again by the handler.
func TestBulk_PermissionGate(t *testing.T) {
	api, admin, member := setupAuthzAPI(t)
	llm := &models.LLM{Name: "gated", Vendor: models.MOCK_VENDOR, Active: true}
	require.NoError(t, api.service.DB.Create(llm).Error)

	w := apitest.PerformAuthRequest(api.router, "POST", "/api/v1/llms/bulk", bulkBody("deactivate", llm.ID), member.APIKey)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Equal(t, authz.CodePermissionDenied, decodeErrorCode(t, w.Body.Bytes()))
	var stored models.LLM
	require.NoError(t, api.service.DB.First(&stored, llm.ID).Error)
	assert.True(t, stored.Active, "nothing changed")

	w = apitest.PerformAuthRequest(api.router, "POST", "/api/v1/llms/bulk", bulkBody("delete", llm.ID), member.APIKey)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	w = apitest.PerformAuthRequest(api.router, "POST", "/api/v1/llms/bulk", bulkBody("deactivate", llm.ID), admin.APIKey)
	resp := decodeBulk(t, w)
	assert.Equal(t, 1, resp.Data.Succeeded)
	require.NoError(t, api.service.DB.First(&stored, llm.ID).Error)
	assert.False(t, stored.Active)
}

// JSON:API rows carry string ids, so a list page may post back
// {"ids":["3","4"]}; numbers and numeric strings both bind, anything else
// is a 400 before any id is touched.
func TestBulk_AcceptsNumericStringIDs(t *testing.T) {
	api, db := setupTestAPI(t)
	a := &models.LLM{Name: "a", Vendor: models.MOCK_VENDOR, Active: true}
	require.NoError(t, db.Create(a).Error)
	b := &models.LLM{Name: "b", Vendor: models.MOCK_VENDOR, Active: true}
	require.NoError(t, db.Create(b).Error)

	body := json.RawMessage(`{"action":"deactivate","ids":["` + idStr(a.ID) + `", ` + idStr(b.ID) + `]}`)
	resp := decodeBulk(t, performRequest(api.router, "POST", "/api/v1/llms/bulk", body))
	assert.Equal(t, 2, resp.Data.Succeeded)
	assert.Equal(t, a.ID, resp.Data.Results[0].ID)
	assert.Equal(t, b.ID, resp.Data.Results[1].ID)
	var stored models.LLM
	require.NoError(t, db.First(&stored, a.ID).Error)
	assert.False(t, stored.Active)

	for _, bad := range []string{`["abc"]`, `["3x"]`, `[""]`, `["-1"]`, `[1.5]`, `[true]`, `[null]`} {
		w := performRequest(api.router, "POST", "/api/v1/llms/bulk", json.RawMessage(`{"action":"activate","ids":`+bad+`}`))
		assert.Equal(t, http.StatusBadRequest, w.Code, "ids %s: %s", bad, w.Body.String())
		assert.Contains(t, w.Body.String(), "Bad Request", bad)
	}
	require.NoError(t, db.First(&stored, a.ID).Error)
	assert.False(t, stored.Active, "a rejected request changes nothing")

	// The permission resolver reads the same body: string ids still resolve
	// to the action's permission.
	e := api.routePerms[routeKey("POST", "/api/v1/llms/bulk")]
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/api/v1/llms/bulk", strings.NewReader(`{"action":"activate","ids":["1"]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	assert.Equal(t, authz.Publish("llms"), e.permission(c))
}
