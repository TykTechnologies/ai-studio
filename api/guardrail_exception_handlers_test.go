package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	mgmt "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	mgwmgmt "github.com/TykTechnologies/midsommar/v2/proto/microgateway_management"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestGuardrailExceptionHandlers(t *testing.T) {
	api, db := setupTestAPI(t)
	app := &models.App{Name: "Visor", UserID: 1, GuardrailCapability: "read_only"}
	require.NoError(t, app.Create(db))
	filter := &models.Filter{Name: "Wolf Defender", Kind: models.FilterKindGuardrail, Config: models.JSONMap{"provider": "http", "on_detect": "block"}}
	require.NoError(t, db.Create(filter).Error)
	base := fmt.Sprintf("/api/v1/apps/%d/guardrail-exceptions", app.ID)

	decode := func(body []byte) GuardrailExceptionResponse {
		var resp struct {
			Data GuardrailExceptionResponse `json:"data"`
		}
		require.NoError(t, json.Unmarshal(body, &resp), string(body))
		return resp.Data
	}

	in := map[string]any{
		"filter_id":     filter.ID,
		"detector":      "prompt_injection",
		"channel":       "text_user",
		"action":        "log",
		"justification": "PR diffs carry injection test vectors",
	}
	// The test API runs without a user, so nobody is a full administrator:
	// the exception is proposed, then approved with the reviewed hash. A
	// full administrator's own exception is approved at once (services
	// tests cover it; guardrailExceptionActor passes authz.FullAdmin).
	w := performRequest(api.router, http.MethodPost, base, in)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decode(w.Body.Bytes())
	assert.Equal(t, "proposed", created.Status)
	require.NotEmpty(t, created.ContentHash)
	w = performRequest(api.router, http.MethodPost, fmt.Sprintf("%s/%d/approve", base, created.ID), map[string]string{"content_hash": created.ContentHash})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "approved", decode(w.Body.Bytes()).Status)

	w = performRequest(api.router, http.MethodPost, base, in)
	assert.Equal(t, http.StatusConflict, w.Code, "same scope twice")

	below := map[string]any{"filter_id": filter.ID, "channel": "tool_call", "action": "quarantine", "justification": "x"}
	w = performRequest(api.router, http.MethodPost, base, below)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	w = performRequest(api.router, http.MethodGet, base, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var list struct {
		Data []GuardrailExceptionResponse `json:"data"`
		Meta GuardrailExceptionMeta       `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Data, 1)
	assert.Equal(t, 20, list.Meta.MaxApproved)
	assert.Equal(t, []string{"log"}, list.Meta.Actions["tool_call"])
	assert.Equal(t, "", list.Meta.Floors["acting"]["tool_result"])

	item := fmt.Sprintf("%s/%d", base, created.ID)
	w = performRequest(api.router, http.MethodPatch, item, map[string]any{"action": "quarantine"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "quarantine", decode(w.Body.Bytes()).Action)

	w = performRequest(api.router, http.MethodPost, item+"/retire", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "retired", decode(w.Body.Bytes()).Status)

	w = performRequest(api.router, http.MethodGet, fmt.Sprintf("/api/v1/filters/%d/exceptions", filter.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"app_id":`+fmt.Sprint(app.ID))

	w = performRequest(api.router, http.MethodDelete, item, nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
	w = performRequest(api.router, http.MethodGet, item, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// An approval must carry the hash of what was reviewed; a proposal that
// changed since is refused with 409.
func TestGuardrailExceptionHandlers_ApproveContentHash(t *testing.T) {
	api, db := setupTestAPI(t)
	app := &models.App{Name: "Visor", UserID: 1, GuardrailCapability: "read_only"}
	require.NoError(t, app.Create(db))
	filter := &models.Filter{Name: "Wolf", Kind: models.FilterKindGuardrail}
	require.NoError(t, db.Create(filter).Error)
	e := &models.GuardrailException{AppID: app.ID, FilterID: filter.ID, Channel: "text_user", Action: "log",
		Justification: "reviewed", Status: models.GuardrailExceptionProposed, ExpiresAfterDays: 30}
	require.NoError(t, db.Create(e).Error)
	reviewed := e.ContentHash()
	item := fmt.Sprintf("/api/v1/apps/%d/guardrail-exceptions/%d", app.ID, e.ID)

	w := performRequest(api.router, http.MethodPost, item+"/approve", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "content_hash")

	require.NoError(t, db.Model(e).Update("action", "quarantine").Error)
	w = performRequest(api.router, http.MethodPost, item+"/approve", map[string]string{"content_hash": reviewed})
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var errBody ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errBody))
	require.NotEmpty(t, errBody.Errors)
	assert.Equal(t, guardrailExceptionChangedTitle, errBody.Errors[0].Title)

	require.NoError(t, db.First(e, e.ID).Error)
	w = performRequest(api.router, http.MethodPost, item+"/approve", map[string]string{"content_hash": e.ContentHash()})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// Approving loosens enforcement, so it is for full administrators only;
// proposing, editing and retiring are app writes.
func TestGuardrailExceptionRoutes_Permissions(t *testing.T) {
	api, _ := setupTestAPI(t)
	get := func(method, path string) authz.Permission {
		e, ok := api.routePerms[routeKey(method, path)]
		require.True(t, ok, "%s %s", method, path)
		return e.perm
	}
	assert.Equal(t, authz.FullAdmin, get("POST", "/api/v1/apps/:id/guardrail-exceptions/:eid/approve"))
	assert.Equal(t, authz.Write("apps"), get("POST", "/api/v1/apps/:id/guardrail-exceptions"))
	assert.Equal(t, authz.Write("apps"), get("PATCH", "/api/v1/apps/:id/guardrail-exceptions/:eid"))
	assert.Equal(t, authz.Write("apps"), get("POST", "/api/v1/apps/:id/guardrail-exceptions/:eid/retire"))
	assert.Equal(t, authz.Delete("apps"), get("DELETE", "/api/v1/apps/:id/guardrail-exceptions/:eid"))
	assert.Equal(t, authz.Read("apps"), get("GET", "/api/v1/apps/:id/guardrail-exceptions"))
	assert.Equal(t, authz.Read("apps"), get("GET", "/api/v1/apps/:id/guardrail-exceptions/:eid"))
	assert.Equal(t, authz.Read("filters"), get("GET", "/api/v1/filters/:id/exceptions"))
}

func TestGuardrailExceptionRoutes_NonAdminDenied(t *testing.T) {
	api, _, member := setupAuthzAPI(t)
	w := apitest.PerformAuthRequest(api.router, http.MethodPost, "/api/v1/apps/1/guardrail-exceptions/1/approve", map[string]string{"content_hash": "x"}, member.APIKey)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// Exceptions are managed through the admin REST API only: no gRPC
// management service plugins or edges reach has a method for them.
func TestGuardrailExceptions_NotOnManagementSurfaces(t *testing.T) {
	for _, desc := range []grpc.ServiceDesc{mgmt.AIStudioManagementService_ServiceDesc, mgwmgmt.MicrogatewayManagementService_ServiceDesc} {
		for _, m := range desc.Methods {
			assert.NotContains(t, strings.ToLower(m.MethodName), "exception", "%s.%s", desc.ServiceName, m.MethodName)
		}
		for _, s := range desc.Streams {
			assert.NotContains(t, strings.ToLower(s.StreamName), "exception", "%s.%s", desc.ServiceName, s.StreamName)
		}
	}
}

// A full administrator's own exception is approved as it is created.
func TestGuardrailExceptionHandlers_FullAdminCreateIsApproved(t *testing.T) {
	api, admin, _ := setupAuthzAPI(t)
	db := api.service.DB
	app := &models.App{Name: "Visor", UserID: admin.ID, GuardrailCapability: "read_only"}
	require.NoError(t, app.Create(db))
	filter := &models.Filter{Name: "Wolf", Kind: models.FilterKindGuardrail}
	require.NoError(t, db.Create(filter).Error)

	w := apitest.PerformAuthRequest(api.router, http.MethodPost, fmt.Sprintf("/api/v1/apps/%d/guardrail-exceptions", app.ID), map[string]any{
		"filter_id": filter.ID, "channel": "text_user", "action": "log", "justification": "false positives on PR diffs",
	}, admin.APIKey)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp struct {
		Data GuardrailExceptionResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "approved", resp.Data.Status)
	require.NotNil(t, resp.Data.ApprovedBy)
	assert.Equal(t, admin.ID, *resp.Data.ApprovedBy)
	assert.Equal(t, admin.ID, resp.Data.ProposedBy)
}
