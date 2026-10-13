package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func slugToolBody(name string) map[string]interface{} {
	return toolBody(name, models.ToolTypeREST, testToolSpec)
}

// A tool name whose slug another tool has is refused with 409, on create and
// on rename, and nothing is stored.
func TestToolSlugConflictIs409(t *testing.T) {
	api, db := setupTestAPI(t)

	w := performRequest(api.router, "POST", "/api/v1/tools", slugToolBody("Page fetcher"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	w = performRequest(api.router, "POST", "/api/v1/tools", slugToolBody("PAGE fetcher!"))
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	detail := errorDetail(t, w.Body.Bytes())
	assert.Contains(t, detail, `"page-fetcher"`)
	// The admin UI matches this shape to show it under the Name field.
	assert.True(t, strings.HasPrefix(detail, "tool name "), detail)
	assert.Contains(t, detail, "is too close to")

	w = performRequest(api.router, "POST", "/api/v1/tools", slugToolBody("Other tool"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/tools/%s", created.Data.ID), slugToolBody("page-fetcher"))
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.Tool{}).Where("slug = ?", "page-fetcher").Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

// The same for LLMs, answered like the other LLM route-name conflicts (400).
func TestLLMSlugConflictIsRefused(t *testing.T) {
	api, db := setupTestAPI(t)
	llmBody := func(name string) map[string]interface{} {
		return map[string]interface{}{"data": map[string]interface{}{
			"type": "LLM",
			"attributes": map[string]interface{}{
				"name": name, "vendor": string(models.OPENAI), "api_key": "k",
				"api_endpoint": "https://api.openai.com/v1", "privacy_score": 1, "active": true,
			},
		}}
	}

	w := performRequest(api.router, "POST", "/api/v1/llms", llmBody("GPT 4"))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	w = performRequest(api.router, "POST", "/api/v1/llms", llmBody("gpt-4"))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	detail := errorDetail(t, w.Body.Bytes())
	assert.Contains(t, detail, `"gpt-4"`)
	// The admin UI matches this shape to show it under the Name field.
	assert.True(t, strings.HasPrefix(detail, "LLM name "), detail)
	assert.Contains(t, detail, "is too close to")

	var count int64
	require.NoError(t, db.Model(&models.LLM{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}
