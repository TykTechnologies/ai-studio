//go:build enterprise

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func scriptFilterInput(name, description, script string) FilterInput {
	var in FilterInput
	in.Data.Type = "filters"
	in.Data.Attributes.Name = name
	in.Data.Attributes.Description = description
	in.Data.Attributes.Script = []byte(script)
	return in
}

func guardrailFilterInput(name string, response bool, config map[string]interface{}) FilterInput {
	var in FilterInput
	in.Data.Type = "filters"
	in.Data.Attributes.Name = name
	in.Data.Attributes.Description = "guardrail under test"
	in.Data.Attributes.Kind = models.FilterKindGuardrail
	in.Data.Attributes.ResponseFilter = response
	in.Data.Attributes.Config = config
	return in
}

func builtinConfig(action string, detectors ...string) map[string]interface{} {
	ds := make([]interface{}, 0, len(detectors))
	for _, d := range detectors {
		ds = append(ds, map[string]interface{}{"name": d})
	}
	return map[string]interface{}{"provider": "builtin", "detectors": ds, "on_detect": action}
}

func TestFilterEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create Filter
	createFilterInput := scriptFilterInput("Test Filter", "A test filter", "function testFilter() { return true; }")

	w := performRequest(api.router, "POST", "/api/v1/filters", createFilterInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response FilterResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Filter", response.Attributes.Name)
	assert.Equal(t, models.FilterKindScript, response.Attributes.Kind, "a filter created without a kind is a script filter")

	filterID := response.ID

	// Test Get Filter
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/filters/%s", filterID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update Filter
	updateFilterInput := scriptFilterInput("Updated Filter", "An updated test filter", "function updatedTestFilter() { return false; }")

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/filters/%s", filterID), updateFilterInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List Filters
	w = performRequest(api.router, "GET", "/api/v1/filters", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse []FilterResponse
	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.Len(t, listResponse, 1)
	assert.Equal(t, "Updated Filter", listResponse[0].Attributes.Name)

	// Test Delete Filter
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/filters/%s", filterID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify filter is deleted
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/filters/%s", filterID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestFilterEndpointsErrors(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Get non-existent filter
	w := performRequest(api.router, "GET", "/api/v1/filters/999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Update non-existent filter
	updateFilterInput := scriptFilterInput("Updated Filter", "An updated test filter", "function updatedTestFilter() { return false; }")
	w = performRequest(api.router, "PATCH", "/api/v1/filters/999", updateFilterInput)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Delete non-existent filter
	w = performRequest(api.router, "DELETE", "/api/v1/filters/999", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Test Create filter with invalid input
	invalidCreateFilterInput := scriptFilterInput("", "", "")
	w = performRequest(api.router, "POST", "/api/v1/filters", invalidCreateFilterInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// A guardrail filter is stored with its config normalised: the defaults an
// edge must not have to guess (fail mode, scope, timeout, block message) are
// written out.
func TestGuardrailFilterEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	w := performRequest(api.router, "POST", "/api/v1/filters", guardrailFilterInput("Secrets", false, builtinConfig("block", "secrets")))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var created FilterResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, models.FilterKindGuardrail, created.Attributes.Kind)
	assert.Equal(t, "builtin", created.Attributes.Config["provider"])
	assert.Equal(t, guardrails.FailClosed, created.Attributes.Config["fail_mode"], "request-side guardrails fail closed by default")
	assert.Equal(t, guardrails.ScopeAllUser, created.Attributes.Config["scope"])
	assert.Equal(t, guardrails.DefaultRequestBlockMessage, created.Attributes.Config["block_message"])
	assert.EqualValues(t, guardrails.DefaultTimeoutMs, created.Attributes.Config["timeout_ms"])

	// Response-side default fail mode is open.
	w = performRequest(api.router, "POST", "/api/v1/filters", guardrailFilterInput("Leaks", true, builtinConfig("block", "secrets", "leak")))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp FilterResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, guardrails.FailOpen, resp.Attributes.Config["fail_mode"])
	assert.Equal(t, guardrails.DefaultResponseBlockMsg, resp.Attributes.Config["block_message"])

	// Update can switch the action; the stored config is re-normalised.
	w = performRequest(api.router, "PATCH", "/api/v1/filters/"+created.ID, guardrailFilterInput("Secrets", false, builtinConfig("redact", "secrets", "pii")))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var updated FilterResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	assert.Equal(t, guardrails.ActionRedact, updated.Attributes.Config["on_detect"])
	assert.Equal(t, guardrails.RedactionPlaceholder, updated.Attributes.Config["redaction"].(map[string]interface{})["style"])

	// Rejections are the caller's fault: 400, with the reason.
	cases := map[string]FilterInput{
		"unknown provider":   guardrailFilterInput("Bad", false, map[string]interface{}{"provider": "nope", "detectors": []interface{}{map[string]interface{}{"name": "x"}}, "on_detect": "block"}),
		"no detectors":       guardrailFilterInput("Bad", false, map[string]interface{}{"provider": "builtin", "on_detect": "block"}),
		"unknown detector":   guardrailFilterInput("Bad", false, builtinConfig("block", "no_such_category")),
		"bad action":         guardrailFilterInput("Bad", false, builtinConfig("explode", "secrets")),
		"redact unsupported": guardrailFilterInput("Bad", false, map[string]interface{}{"provider": "azure_content_safety", "detectors": []interface{}{map[string]interface{}{"name": "Hate"}}, "on_detect": "redact", "connection": map[string]interface{}{"endpoint": "https://x", "api_key": "$SECRET/K"}}),
		"missing connection": guardrailFilterInput("Bad", false, map[string]interface{}{"provider": "lakera", "detectors": []interface{}{map[string]interface{}{"name": "prompt_attack"}}, "on_detect": "block"}),
		"missing config":     guardrailFilterInput("Bad", false, nil),
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			w := performRequest(api.router, "POST", "/api/v1/filters", in)
			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		})
	}
	// Lakera needs no key when self-hosted, but does need at least one of endpoint or key.
	w = performRequest(api.router, "POST", "/api/v1/filters", guardrailFilterInput("Lakera", false, map[string]interface{}{
		"provider": "lakera", "detectors": []interface{}{map[string]interface{}{"name": "prompt_attack"}}, "on_detect": "block",
		"connection": map[string]interface{}{"endpoint": "http://lakera.internal/v2/guard"},
	}))
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}

func TestGuardrailProvidersEndpoint(t *testing.T) {
	api, _ := setupTestAPI(t)

	w := performRequest(api.router, "GET", "/api/v1/filters/guardrail-providers", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var specs []guardrails.Spec
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &specs))
	byName := map[string]guardrails.Spec{}
	for _, s := range specs {
		byName[s.Name] = s
	}
	for _, want := range []string{"builtin", "http", "presidio", "lakera", "azure_content_safety", "azure_pii", "bedrock_guardrails"} {
		assert.Contains(t, byName, want)
	}
	assert.True(t, byName["builtin"].Available, "the built-in provider is implemented in the enterprise build")
	assert.True(t, byName["builtin"].Local)
	assert.NotEmpty(t, byName["lakera"].ConnectionFields)
}

// The test endpoint runs a guardrail filter for real, so an author can see
// what a configuration does before attaching it.
func TestFilterTestEndpoint_Guardrail(t *testing.T) {
	api, _ := setupTestAPI(t)

	body := FilterTestInput{
		Kind:   models.FilterKindGuardrail,
		Config: builtinConfig("block", "secrets"),
		Input: map[string]interface{}{
			"raw_input": `{"messages":[{"role":"user","content":"my key is AKIAIOSFODNN7EXAMPLE"}]}`,
			"messages":  []interface{}{map[string]interface{}{"role": "user", "content": "my key is AKIAIOSFODNN7EXAMPLE"}},
		},
	}
	w := performRequest(api.router, "POST", "/api/v1/filters/test", body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var out FilterTestOutput
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.True(t, out.Success, out.Error)
	assert.Equal(t, true, out.Output["block"])
	events, _ := out.Output["compliance_events"].([]interface{})
	require.Len(t, events, 1)
	assert.Equal(t, "guardrail.secrets.aws_access_key", events[0].(map[string]interface{})["event_type"])

	// A guardrail test without a config is a bad request, not a script error.
	w = performRequest(api.router, "POST", "/api/v1/filters/test", FilterTestInput{Kind: models.FilterKindGuardrail, Input: map[string]interface{}{"raw_input": "x"}})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
