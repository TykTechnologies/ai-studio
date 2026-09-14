package api

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The secrets list says whether each secret holds a value (without exposing
// it) and which objects read it, so an unfilled placeholder such as the
// bootstrap OPENAI_KEY is visible on the Secrets page and not only as a
// warning on the LLM list.
func TestListSecrets_HasValueAndReferencedBy(t *testing.T) {
	originalKey := os.Getenv("TYK_AI_SECRET_KEY")
	defer os.Setenv("TYK_AI_SECRET_KEY", originalKey)
	require.NoError(t, os.Setenv("TYK_AI_SECRET_KEY", "test-encryption-key-for-secrets-list"))

	api, db := setupTestAPI(t)

	empty := &secrets.Secret{VarName: "OPENAI_KEY", Value: ""}
	require.NoError(t, secrets.CreateSecret(db, empty))
	filled := &secrets.Secret{VarName: "ANTHROPIC_KEY", Value: "sk-ant-real"}
	require.NoError(t, secrets.CreateSecret(db, filled))
	unused := &secrets.Secret{VarName: "UNUSED", Value: "x"}
	require.NoError(t, secrets.CreateSecret(db, unused))

	openai := &models.LLM{Name: "OpenAI Prod", APIKey: "$SECRET/OPENAI_KEY", Active: true}
	require.NoError(t, db.Create(openai).Error)
	tool := &models.Tool{Name: "CRM", AuthKey: "$SECRET/ANTHROPIC_KEY", Active: true}
	require.NoError(t, db.Create(tool).Error)

	w := performRequest(api.router, "GET", "/api/v1/secrets?all=true", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var response SecretListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Data, 3)

	byName := map[string]SecretResponse{}
	for _, item := range response.Data {
		byName[item.Attributes.VarName] = item
	}

	assert.False(t, byName["OPENAI_KEY"].Attributes.HasValue, "an encrypted empty value is still no value")
	assert.Equal(t, []SecretReferenceResponse{{Type: "llm", ID: openai.ID, Name: "OpenAI Prod"}},
		byName["OPENAI_KEY"].Attributes.ReferencedBy)

	assert.True(t, byName["ANTHROPIC_KEY"].Attributes.HasValue)
	assert.Equal(t, []SecretReferenceResponse{{Type: "tool", ID: tool.ID, Name: "CRM"}},
		byName["ANTHROPIC_KEY"].Attributes.ReferencedBy)

	assert.True(t, byName["UNUSED"].Attributes.HasValue)
	assert.NotNil(t, byName["UNUSED"].Attributes.ReferencedBy, "referenced_by is always an array")
	assert.Empty(t, byName["UNUSED"].Attributes.ReferencedBy)

	// The raw JSON carries the keys the frontend reads, and never the value
	// in a form that tells the secret apart from its emptiness flag.
	assert.Contains(t, w.Body.String(), `"has_value":false`)
	assert.Contains(t, w.Body.String(), `"referenced_by":[]`)

	t.Run("single secret carries the same state after decryption", func(t *testing.T) {
		w := performRequest(api.router, "GET", "/api/v1/secrets/"+idStr(empty.ID), nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var single struct {
			Data SecretResponse `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &single))
		assert.False(t, single.Data.Attributes.HasValue)
		assert.Len(t, single.Data.Attributes.ReferencedBy, 1)
	})

	t.Run("secret dependents endpoint", func(t *testing.T) {
		w := performRequest(api.router, "GET", "/api/v1/secrets/"+idStr(empty.ID)+"/dependents", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var deps DependentsResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &deps))
		assert.Equal(t, "dependents", deps.Data.Type)
		assert.Equal(t, idStr(empty.ID), deps.Data.ID)
		assert.Equal(t, 1, deps.Data.Attributes.Total)
		require.Len(t, deps.Data.Attributes.LLMs, 1)
		assert.Equal(t, "OpenAI Prod", deps.Data.Attributes.LLMs[0].Name)
	})
}
