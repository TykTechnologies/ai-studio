//go:build enterprise
// +build enterprise

package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/governed_metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rejectingHooks stands in for a plugin object hook that refuses every change.
type rejectingHooks struct{ reason string }

func (r *rejectingHooks) RunMetadataHook(ctx context.Context, hookType string, rec *models.ObjectMetadata, userID uint) (*governed_metadata.HookOutcome, error) {
	if hookType == governed_metadata.HookBeforeUpdate || hookType == governed_metadata.HookBeforeDelete {
		return &governed_metadata.HookOutcome{Allowed: false, RejectionReason: r.reason, Executed: []string{"guard"}}, nil
	}
	return &governed_metadata.HookOutcome{Allowed: true}, nil
}

func TestGovernedMetadataHandlers_BadInputs(t *testing.T) {
	r, _, _ := setupGovernedMetadataRouter(t)

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   interface{}
		want   int
	}{
		{"schema id not numeric", "GET", "/api/v1/metadata/schemas/abc", nil, http.StatusBadRequest},
		{"schema id zero", "GET", "/api/v1/metadata/schemas/0", nil, http.StatusBadRequest},
		{"schema not found", "GET", "/api/v1/metadata/schemas/999", nil, http.StatusNotFound},
		{"schema patch not found", "PATCH", "/api/v1/metadata/schemas/999", jsonAPI(map[string]interface{}{"name": "x"}), http.StatusNotFound},
		{"schema delete not found", "DELETE", "/api/v1/metadata/schemas/999", nil, http.StatusNotFound},
		{"schema create malformed json", "POST", "/api/v1/metadata/schemas", "not json", http.StatusBadRequest},
		{"schema create empty name", "POST", "/api/v1/metadata/schemas", jsonAPI(map[string]interface{}{"applies_to": []string{"llm"}}), http.StatusBadRequest},
		{"schema create unknown object type", "POST", "/api/v1/metadata/schemas", jsonAPI(map[string]interface{}{"name": "x", "applies_to": []string{"rocket"}}), http.StatusBadRequest},
		{"schema create bad enforcement", "POST", "/api/v1/metadata/schemas", jsonAPI(map[string]interface{}{"name": "x", "applies_to": []string{"llm"}, "enforcement": "maybe"}), http.StatusBadRequest},
		{"vocabulary not found", "GET", "/api/v1/metadata/vocabularies/999", nil, http.StatusNotFound},
		{"vocabulary patch not found", "PATCH", "/api/v1/metadata/vocabularies/999", jsonAPI(map[string]interface{}{"name": "x"}), http.StatusNotFound},
		{"vocabulary delete not found", "DELETE", "/api/v1/metadata/vocabularies/999", nil, http.StatusNotFound},
		{"vocabulary without terms", "POST", "/api/v1/metadata/vocabularies", jsonAPI(map[string]interface{}{"name": "Empty"}), http.StatusBadRequest},
		{"vocabulary malformed json", "POST", "/api/v1/metadata/vocabularies", "{", http.StatusBadRequest},
		{"resolve without object_type", "GET", "/api/v1/metadata/schemas/resolve", nil, http.StatusBadRequest},
		{"validate without object_type", "POST", "/api/v1/metadata/validate", map[string]interface{}{"values": map[string]interface{}{}}, http.StatusBadRequest},
		{"validate malformed json", "POST", "/api/v1/metadata/validate", "[", http.StatusBadRequest},
		{"object metadata not found", "GET", "/api/v1/metadata/objects/llm/12345", nil, http.StatusNotFound},
		{"set metadata unknown type", "PUT", "/api/v1/metadata/objects/rocket/1", map[string]interface{}{"values": map[string]interface{}{}}, http.StatusBadRequest},
		{"set metadata malformed json", "PUT", "/api/v1/metadata/objects/llm/1", "{{", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := apitest.PerformRequest(r, tc.method, tc.path, tc.body)
			assert.Equal(t, tc.want, w.Code, w.Body.String())
			if tc.want >= 400 {
				assert.Contains(t, w.Body.String(), `"errors"`, "error envelope present")
			}
		})
	}

	// Deleting metadata that does not exist is idempotent.
	w := apitest.PerformRequest(r, "DELETE", "/api/v1/metadata/objects/llm/12345", nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
	// Audit for an unknown object is an empty list, not an error.
	w = apitest.PerformRequest(r, "GET", "/api/v1/metadata/objects/llm/12345/audit", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"data":[]`)
}

func TestGovernedMetadataHandlers_FiltersLimitsAndPluginSchemas(t *testing.T) {
	r, api, db := setupGovernedMetadataRouter(t)
	svc := api.governedMetadata()
	require.NoError(t, svc.CreateSchema(&models.MetadataSchema{Name: "Core", Slug: "core", AppliesTo: []string{"llm"}, Active: true,
		Fields: []models.MetadataFieldDef{{Key: "owner", Label: "Owner", Type: "string", Required: true}}}))
	require.NoError(t, db.Create(&models.LLM{Name: "with", Vendor: "openai"}).Error)
	require.NoError(t, db.Create(&models.LLM{Name: "without", Vendor: "openai"}).Error)

	// Three writes → audit honours limit and ordering.
	for i, v := range []string{"a", "b", "c"} {
		w := apitest.PerformRequest(r, "PUT", "/api/v1/metadata/objects/llm/1", map[string]interface{}{"values": map[string]interface{}{"owner": v}})
		require.Equal(t, http.StatusOK, w.Code, "write %d: %s", i, w.Body.String())
	}
	w := apitest.PerformRequest(r, "GET", "/api/v1/metadata/objects/llm/1/audit?limit=2", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var audit struct {
		Data []models.ObjectMetadataAudit `json:"data"`
	}
	decode(t, w.Body.Bytes(), &audit)
	require.Len(t, audit.Data, 2)
	assert.Equal(t, "c", audit.Data[0].After["owner"])

	// Compliance filters by status and by object type.
	w = apitest.PerformRequest(r, "GET", "/api/v1/metadata/compliance?status=missing", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var report struct {
		Entries []struct {
			ObjectName string `json:"object_name"`
			Status     string `json:"status"`
		} `json:"entries"`
		Counts map[string]int `json:"counts"`
	}
	decode(t, w.Body.Bytes(), &report)
	require.Len(t, report.Entries, 1)
	assert.Equal(t, "without", report.Entries[0].ObjectName)
	assert.Equal(t, 1, report.Counts["valid"], "counts cover every object regardless of the status filter")
	w = apitest.PerformRequest(r, "GET", "/api/v1/metadata/compliance?object_type=tool", nil)
	decode(t, w.Body.Bytes(), &report)
	assert.Empty(t, report.Entries, "no schema applies to tools")

	// Plugin-sourced schemas: structure is read-only (409), deletion refused (409), toggles allowed.
	require.NoError(t, svc.UpsertPluginSchemas(9, &models.ManifestMetadata{Schemas: []models.ManifestSchema{{
		Slug: "plug", Name: "Plug", AppliesTo: []string{"tool"}, Fields: []models.MetadataFieldDef{{Key: "kind", Type: "string"}}}}}))
	schemas, _ := svc.ListSchemas()
	var plug models.MetadataSchema
	for _, s := range schemas {
		if s.Slug == "plug" {
			plug = s
		}
	}
	require.NotZero(t, plug.ID)
	w = apitest.PerformRequest(r, "PATCH", fmt.Sprintf("/api/v1/metadata/schemas/%d", plug.ID), jsonAPI(map[string]interface{}{"name": "Renamed"}))
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	w = apitest.PerformRequest(r, "DELETE", fmt.Sprintf("/api/v1/metadata/schemas/%d", plug.ID), nil)
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	w = apitest.PerformRequest(r, "PATCH", fmt.Sprintf("/api/v1/metadata/schemas/%d", plug.ID), jsonAPI(map[string]interface{}{"active": true, "enforcement": "enforce"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"enforcement":"enforce"`)

	// Object types now include nothing extra (no opted-in plugin resource types) but stay well-formed.
	w = apitest.PerformRequest(r, "GET", "/api/v1/metadata/object-types", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"slug":"datasource"`)
}

func TestGovernedMetadataHandlers_HookRejection(t *testing.T) {
	r, api, db := setupGovernedMetadataRouter(t)
	v1 := r.Group("/api/v1")
	v1.POST("/llms", api.createLLM)
	// Swap in a service whose hooks reject every metadata write.
	api.service.GovernedMetadataService = governed_metadata.NewService(db, governed_metadata.Deps{Hooks: &rejectingHooks{reason: "policy says no"}})
	svc := api.governedMetadata()
	require.NoError(t, svc.CreateSchema(&models.MetadataSchema{Name: "Core", Slug: "core", AppliesTo: []string{"llm"}, Active: true,
		Fields: []models.MetadataFieldDef{{Key: "owner", Type: "string"}}}))
	require.NoError(t, db.Create(&models.LLM{Name: "x", Vendor: "openai"}).Error)

	// Direct write → 422 with hook_rejected.
	w := apitest.PerformRequest(r, "PUT", "/api/v1/metadata/objects/llm/1", map[string]interface{}{"values": map[string]interface{}{"owner": "me"}})
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	var verr MetadataValidationErrorResponse
	decode(t, w.Body.Bytes(), &verr)
	require.Len(t, verr.Errors, 1)
	assert.Equal(t, "hook_rejected", verr.Errors[0].Code)
	assert.Equal(t, "policy says no", verr.Errors[0].Detail)

	// Object create: the LLM is created, the metadata failure is reported in meta.
	t.Setenv("TYK_AI_SECRET_KEY", "test-key")
	w = apitest.PerformRequest(r, "POST", "/api/v1/llms", map[string]interface{}{"data": map[string]interface{}{"type": "LLM", "attributes": map[string]interface{}{
		"name": "gpt", "api_key": "k", "api_endpoint": "https://api.openai.com/v1", "vendor": "openai", "active": false, "default_model": "gpt-4", "privacy_score": 10,
		"governed_metadata": map[string]interface{}{"owner": "me"}}}})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	resp := parseObjectResponse(t, w.Body.Bytes())
	require.NotNil(t, resp.Meta)
	gmErr, ok := resp.Meta["governed_metadata_error"].(map[string]interface{})
	require.True(t, ok, "%v", resp.Meta)
	assert.Equal(t, "hook_rejected", gmErr["code"])
	assert.Nil(t, resp.Data.GovernedMetadata, "nothing stored")
	var count int64
	db.Model(&models.LLM{}).Count(&count)
	assert.Equal(t, int64(2), count, "the object itself was created")

	// Delete of an existing row is also blocked by the hook.
	api.service.GovernedMetadataService = governed_metadata.NewService(db, governed_metadata.Deps{})
	w = apitest.PerformRequest(r, "PUT", "/api/v1/metadata/objects/llm/1", map[string]interface{}{"values": map[string]interface{}{"owner": "me"}})
	require.Equal(t, http.StatusOK, w.Code)
	api.service.GovernedMetadataService = governed_metadata.NewService(db, governed_metadata.Deps{Hooks: &rejectingHooks{reason: "keep"}})
	w = apitest.PerformRequest(r, "DELETE", "/api/v1/metadata/objects/llm/1", nil)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
}

func TestGovernedMetadataEnforcement_ToolUpdateAndDatasourceDelete(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "test-key")
	r, api, _ := setupGovernedMetadataRouter(t)
	v1 := r.Group("/api/v1")
	v1.POST("/tools", api.createTool)
	v1.PATCH("/tools/:id", api.updateTool)
	v1.GET("/tools", api.getAllTools)
	v1.POST("/datasources", api.createDatasource)
	v1.DELETE("/datasources/:id", api.deleteDatasource)
	v1.GET("/datasources", api.listDatasources)

	svc := api.governedMetadata()
	require.NoError(t, svc.CreateSchema(&models.MetadataSchema{Name: "Owners", Slug: "owners", AppliesTo: []string{"tool", "datasource"}, Active: true,
		Enforcement: models.MetadataEnforcementEnforce,
		Fields: []models.MetadataFieldDef{
			{Key: "team", Label: "Team", Type: "string", Required: true, PortalVisible: true},
			{Key: "contact", Label: "Contact", Type: "email"},
		}}))

	toolBody := func(extra map[string]interface{}) map[string]interface{} {
		attrs := map[string]interface{}{"name": "weather", "tool_type": "REST", "description": "d", "privacy_score": 10, "oas_spec": testOASSpec(`{"openapi": "3.0.0"}`)}
		for k, v := range extra {
			attrs[k] = v
		}
		return map[string]interface{}{"data": map[string]interface{}{"type": "tool", "attributes": attrs}}
	}
	w := apitest.PerformRequest(r, "POST", "/api/v1/tools", toolBody(map[string]interface{}{"governed_metadata": map[string]interface{}{"team": "platform"}}))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	toolID := parseObjectResponse(t, w.Body.Bytes()).Data.ID

	// Tool update with an invalid email under enforcement → 422; without the attribute → untouched.
	w = apitest.PerformRequest(r, "PATCH", "/api/v1/tools/"+toolID, toolBody(map[string]interface{}{"governed_metadata": map[string]interface{}{"team": "platform", "contact": "nope"}}))
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	w = apitest.PerformRequest(r, "PATCH", "/api/v1/tools/"+toolID, toolBody(nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "platform", parseObjectResponse(t, w.Body.Bytes()).Data.GovernedMetadata["team"])

	// Tool list carries admin values; portal helper carries display fields.
	w = apitest.PerformRequest(r, "GET", "/api/v1/tools", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"governed_metadata_status":"valid"`)
	portal := api.withToolGovernedMetadata([]ToolResponse{{ID: toolID}}, true)
	display, ok := portal[0].GovernedMetadata.([]governed_metadata.DisplayField)
	require.True(t, ok)
	require.Len(t, display, 1)
	assert.Equal(t, "Team", display[0].Label)
	assert.Equal(t, "platform", display[0].Value)

	// Datasource: create with metadata, list shows it, delete removes the row, portal helper handles missing rows.
	dsBody := map[string]interface{}{"data": map[string]interface{}{"type": "datasource", "attributes": map[string]interface{}{
		"name": "docs", "short_description": "d", "db_source_type": "qdrant", "embed_vendor": "openai", "embed_model": "m", "active": true,
		"governed_metadata": map[string]interface{}{"team": "data"}}}}
	w = apitest.PerformRequest(r, "POST", "/api/v1/datasources", dsBody)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	dsID := parseObjectResponse(t, w.Body.Bytes()).Data.ID
	w = apitest.PerformRequest(r, "GET", "/api/v1/datasources", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"team":"data"`)
	portalDS := api.withDatasourceGovernedMetadata([]DatasourceResponse{{ID: dsID}, {ID: "999"}}, true)
	assert.NotNil(t, portalDS[0].GovernedMetadata)
	assert.Nil(t, portalDS[1].GovernedMetadata, "objects without metadata carry nothing")

	w = apitest.PerformRequest(r, "DELETE", "/api/v1/datasources/"+dsID, nil)
	require.Equal(t, http.StatusNoContent, w.Code)
	_, err := svc.GetObjectMetadata("datasource", dsID)
	assert.ErrorIs(t, err, governed_metadata.ErrNotFound)
}
