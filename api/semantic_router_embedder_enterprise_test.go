//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	entsr "github.com/TykTechnologies/midsommar/v2/enterprise/features/semantic_router"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/semantic_router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Importing the Enterprise router registers its factory for the whole test
// binary; undo that so the Community Edition tests still see the stub, and
// give this file's API the Enterprise service explicitly.
func init() { semantic_router.RegisterEnterpriseFactory(nil) }

func routerBody(attrs map[string]interface{}) map[string]interface{} {
	base := map[string]interface{}{
		"name": "Smart", "slug": "smart",
		"settings": map[string]interface{}{"default_route": "a"},
		"routes": []map[string]interface{}{
			{"name": "a", "utterances": []string{"hello"}, "target": map[string]interface{}{"type": "llm", "llm_id": 1, "model": "m"}},
		},
	}
	for k, v := range attrs {
		base[k] = v
	}
	return map[string]interface{}{"data": map[string]interface{}{"type": "semantic-routers", "attributes": base}}
}

type routerDoc struct {
	Data struct {
		ID         string                 `json:"id"`
		Attributes map[string]interface{} `json:"attributes"`
	} `json:"data"`
}

// Clients that still send settings.embedding as {llm_id, model} get the
// embedder linked to that LLM, and read both shapes back; embedder_id works
// directly, and an embedder in use by a router cannot be deleted.
func TestSemanticRouterAPI_Embedders(t *testing.T) {
	a, service, db := setupEmbedderAPI(t)
	service.SemanticRouterService = entsr.NewEnterpriseService(db)
	r := a.Router()

	target := &models.LLM{Name: "Target", Vendor: models.OPENAI, PrivacyScore: 80, Active: true}
	require.NoError(t, db.Create(target).Error)
	embedLLM := &models.LLM{Name: "Embed", Vendor: models.OPENAI, PrivacyScore: 70, Active: true}
	require.NoError(t, db.Create(embedLLM).Error)
	claude := &models.LLM{Name: "Claude", Vendor: models.ANTHROPIC, Active: true}
	require.NoError(t, db.Create(claude).Error)
	require.EqualValues(t, 1, target.ID)

	w := apitest.PerformRequest(r, "POST", "/api/v1/semantic-routers", routerBody(map[string]interface{}{
		"settings": map[string]interface{}{"default_route": "a",
			"embedding": map[string]interface{}{"llm_id": embedLLM.ID, "model": "text-embedding-3-small", "timeout_ms": 900}},
	}))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var doc routerDoc
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &doc))
	require.NotNil(t, doc.Data.Attributes["embedder_id"])
	assert.Equal(t, "Embed · text-embedding-3-small", doc.Data.Attributes["embedder_name"])
	emb := doc.Data.Attributes["settings"].(map[string]interface{})["embedding"].(map[string]interface{})
	assert.EqualValues(t, embedLLM.ID, emb["llm_id"], "the old shape is still readable")
	assert.Equal(t, "text-embedding-3-small", emb["model"])
	assert.EqualValues(t, 900, emb["timeout_ms"])
	embedderID := uint(doc.Data.Attributes["embedder_id"].(float64))

	var stored models.SemanticRouter
	require.NoError(t, db.First(&stored, doc.Data.ID).Error)
	assert.Equal(t, uint(0), stored.Settings.Embedding.LLMID, "only the timeout is stored in settings")

	// A second router naming the same LLM and model reuses the embedder.
	w = apitest.PerformRequest(r, "POST", "/api/v1/semantic-routers", routerBody(map[string]interface{}{
		"slug": "smart-2", "name": "Smart 2",
		"settings": map[string]interface{}{"default_route": "a",
			"embedding": map[string]interface{}{"llm_id": embedLLM.ID, "model": "text-embedding-3-small"}},
	}))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var embedders int64
	require.NoError(t, db.Model(&models.Embedder{}).Count(&embedders).Error)
	assert.EqualValues(t, 1, embedders)

	// A router that fails validation creates no embedder.
	w = apitest.PerformRequest(r, "POST", "/api/v1/semantic-routers", routerBody(map[string]interface{}{
		"slug": "broken", "name": "Broken",
		"settings": map[string]interface{}{"default_route": "a",
			"embedding": map[string]interface{}{"llm_id": claude.ID, "model": "m"}},
	}))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.NoError(t, db.Model(&models.Embedder{}).Count(&embedders).Error)
	assert.EqualValues(t, 1, embedders, "no embedder for a refused router")

	// embedder_id directly, with a standalone embedder.
	standalone, err := service.CreateEmbedder(&models.Embedder{Name: "Local", Vendor: models.OLLAMA, Endpoint: "http://ollama:11434", ModelName: "nomic", PrivacyScore: 40}, 1)
	require.NoError(t, err)
	w = apitest.PerformRequest(r, "POST", "/api/v1/semantic-routers", routerBody(map[string]interface{}{
		"slug": "local", "name": "Local", "embedder_id": standalone.ID,
	}))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &doc))
	emb = doc.Data.Attributes["settings"].(map[string]interface{})["embedding"].(map[string]interface{})
	assert.Equal(t, "nomic", emb["model"])
	assert.EqualValues(t, 0, emb["llm_id"])

	// The embedders routers use cannot be deleted.
	for _, id := range []uint{embedderID, standalone.ID} {
		w = apitest.PerformRequest(r, "DELETE", fmt.Sprintf("/api/v1/embedders/%d", id), nil)
		assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "semantic router")
	}
}

// A router can only use an embedder whose LLM serves its namespace: edges in
// other namespaces never receive that LLM (or its key).
func TestSemanticRouterAPI_EmbedderNamespace(t *testing.T) {
	a, service, db := setupEmbedderAPI(t)
	service.SemanticRouterService = entsr.NewEnterpriseService(db)
	r := a.Router()

	target := &models.LLM{Name: "Target", Vendor: models.OPENAI, PrivacyScore: 80, Active: true}
	require.NoError(t, db.Create(target).Error)
	euLLM := &models.LLM{Name: "EU Embed", Vendor: models.OPENAI, PrivacyScore: 80, Active: true, Namespace: "eu"}
	require.NoError(t, db.Create(euLLM).Error)
	e, err := service.CreateEmbedder(&models.Embedder{Name: "eu-linked", LLMID: &euLLM.ID, ModelName: "m"}, 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, target.ID)

	w := apitest.PerformRequest(r, "POST", "/api/v1/semantic-routers", routerBody(map[string]interface{}{"embedder_id": e.ID}))
	assert.Equal(t, http.StatusBadRequest, w.Code, "a global router cannot use an EU-scoped LLM: %s", w.Body.String())
	assert.Contains(t, w.Body.String(), "namespace")

	w = apitest.PerformRequest(r, "POST", "/api/v1/semantic-routers", routerBody(map[string]interface{}{
		"embedder_id": e.ID, "namespace": "eu", "slug": "smart-eu", "name": "Smart EU",
	}))
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}
