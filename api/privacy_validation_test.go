package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// privacy_score is 0-100 on create and update of LLMs, tools and data
// sources. Bodies are raw JSON so the non-integer case ("abc") can be sent.
func TestPrivacyScoreValidation(t *testing.T) {
	api, db := setupTestAPI(t)

	llm := &models.LLM{Name: "existing-llm", Vendor: models.MOCK_VENDOR, PrivacyScore: 50}
	require.NoError(t, db.Create(llm).Error)
	tool := &models.Tool{Name: "existing-tool", ToolType: models.ToolTypeREST, OASSpec: testOASSpec(`{"openapi": "3.0.0"}`), PrivacyScore: 50}
	require.NoError(t, db.Create(tool).Error)
	ds := &models.Datasource{Name: "existing-ds", PrivacyScore: 50}
	require.NoError(t, db.Create(ds).Error)

	type route struct {
		name   string
		method string
		path   string
		body   func(score string) string // score is raw JSON
	}
	routes := []route{
		{"create llm", "POST", "/api/v1/llms", func(s string) string {
			return fmt.Sprintf(`{"data":{"type":"llms","attributes":{"name":"new-llm-%s","vendor":"mock","privacy_score":%s}}}`, s, s)
		}},
		{"update llm", "PATCH", "/api/v1/llms/" + idStr(llm.ID), func(s string) string {
			return fmt.Sprintf(`{"data":{"type":"llms","attributes":{"privacy_score":%s}}}`, s)
		}},
		{"create tool", "POST", "/api/v1/tools", func(s string) string {
			return fmt.Sprintf(`{"data":{"type":"tools","attributes":{"name":"new-tool-%s","tool_type":"REST","oas_spec":%q,"privacy_score":%s}}}`, s, testOASSpec(`{"openapi": "3.0.0"}`), s)
		}},
		{"update tool", "PATCH", "/api/v1/tools/" + idStr(tool.ID), func(s string) string {
			return fmt.Sprintf(`{"data":{"type":"tools","attributes":{"name":"existing-tool","tool_type":"REST","oas_spec":%q,"privacy_score":%s}}}`, testOASSpec(`{"openapi": "3.0.0"}`), s)
		}},
		{"create datasource", "POST", "/api/v1/datasources", func(s string) string {
			return fmt.Sprintf(`{"data":{"type":"datasources","attributes":{"name":"new-ds-%s","privacy_score":%s}}}`, s, s)
		}},
		{"update datasource", "PATCH", "/api/v1/datasources/" + idStr(ds.ID), func(s string) string {
			return fmt.Sprintf(`{"data":{"type":"datasources","attributes":{"name":"existing-ds","privacy_score":%s}}}`, s)
		}},
	}

	for _, r := range routes {
		t.Run(r.name, func(t *testing.T) {
			for _, ok := range []string{"0", "100", "37"} {
				w := performRequest(api.router, r.method, r.path, json.RawMessage(r.body(ok)))
				assert.Contains(t, []int{http.StatusOK, http.StatusCreated}, w.Code, "%s with privacy_score %s: %s", r.name, ok, w.Body.String())
			}
			for _, bad := range []string{"-1", "101", "250", `"abc"`} {
				w := performRequest(api.router, r.method, r.path, json.RawMessage(r.body(bad)))
				assert.Equal(t, http.StatusBadRequest, w.Code, "%s with privacy_score %s: %s", r.name, bad, w.Body.String())
				var resp ErrorResponse
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
				require.NotEmpty(t, resp.Errors)
				assert.Equal(t, "Bad Request", resp.Errors[0].Title)
				if bad != `"abc"` {
					assert.Equal(t, "privacy_score must be between 0 and 100", resp.Errors[0].Detail)
				}
			}
		})
	}

	// Nothing invalid was stored.
	var stored models.LLM
	require.NoError(t, db.First(&stored, llm.ID).Error)
	assert.Equal(t, 37, stored.PrivacyScore, "the last valid update stuck")
	var count int64
	require.NoError(t, db.Model(&models.LLM{}).Where("privacy_score < 0 OR privacy_score > 100").Count(&count).Error)
	assert.Equal(t, int64(0), count)
	require.NoError(t, db.Model(&models.Tool{}).Where("privacy_score < 0 OR privacy_score > 100").Count(&count).Error)
	assert.Equal(t, int64(0), count)
	require.NoError(t, db.Model(&models.Datasource{}).Where("privacy_score < 0 OR privacy_score > 100").Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

// A PATCH that does not mention privacy_score keeps the stored value rather
// than being rejected as 0-or-invalid.
func TestPrivacyScoreValidation_OmittedOnPatchKeepsValue(t *testing.T) {
	api, db := setupTestAPI(t)
	llm := &models.LLM{Name: "keep", Vendor: models.MOCK_VENDOR, PrivacyScore: 66}
	require.NoError(t, db.Create(llm).Error)

	w := performRequest(api.router, "PATCH", "/api/v1/llms/"+idStr(llm.ID), json.RawMessage(`{"data":{"type":"llms","attributes":{"short_description":"renamed"}}}`))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var stored models.LLM
	require.NoError(t, db.First(&stored, llm.ID).Error)
	assert.Equal(t, 66, stored.PrivacyScore)
}
