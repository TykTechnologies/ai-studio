package models

import (
	"encoding/json"
	"fmt"
	"testing"

	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func routerWithExamples(slug string, llmID uint) *SemanticRouter {
	return &SemanticRouter{Name: slug, Slug: slug, Active: true, Settings: sr.Settings{DefaultRoute: "a"}, Routes: []sr.Route{
		{Name: "a", Utterances: []string{"hello"}, Target: sr.Target{Type: sr.TargetLLM, LLMID: llmID, Model: "m"}},
	}}
}

func routerFixtures(t *testing.T, db *gorm.DB) (route *LLM, embedLLM *LLM) {
	t.Helper()
	route = &LLM{Name: "Route LLM", Vendor: OPENAI, PrivacyScore: 80, Active: true}
	require.NoError(t, db.Create(route).Error)
	embedLLM = &LLM{Name: "Embed LLM", Vendor: OPENAI, APIEndpoint: "https://api.openai.com/v1", APIKey: "sk-llm", PrivacyScore: 70, Active: true}
	require.NoError(t, db.Create(embedLLM).Error)
	return route, embedLLM
}

func TestSemanticRouter_CompiledConfig(t *testing.T) {
	db := setupEmbedderTestDB(t)
	route, embedLLM := routerFixtures(t, db)

	t.Run("linked embedder becomes an LLM reference", func(t *testing.T) {
		e := &Embedder{Name: "linked", LLMID: &embedLLM.ID, ModelName: "text-embedding-3-small"}
		require.NoError(t, e.Create(db))
		r := routerWithExamples("linked", route.ID)
		r.EmbedderID = &e.ID
		r.Settings.Embedding = &sr.ModelRef{TimeoutMs: 900}
		require.NoError(t, r.Create(db))

		cfg, err := r.CompiledConfig(db, nil)
		require.NoError(t, err)
		assert.Equal(t, &sr.ModelRef{LLMID: embedLLM.ID, Model: "text-embedding-3-small", TimeoutMs: 900}, cfg.Settings.Embedding)
		require.NoError(t, sr.Validate(cfg))
		assert.Equal(t, &sr.ModelRef{TimeoutMs: 900}, r.Settings.Embedding, "the stored settings are not modified")

		var targets []SemanticRouterTarget
		require.NoError(t, db.Where("router_id = ? AND role = ?", r.ID, SemanticTargetClassifier).Find(&targets).Error)
		require.Len(t, targets, 1)
		assert.Equal(t, embedLLM.ID, *targets[0].LLMID, "the linked LLM sees the text, so it is a classifier target")
	})

	t.Run("standalone embedder travels inline, encrypted for edges", func(t *testing.T) {
		e := &Embedder{Name: "standalone", Vendor: OLLAMA, Endpoint: "http://ollama:11434", APIKey: "ollama-key", ModelName: "nomic", PrivacyScore: 20}
		require.NoError(t, e.Create(db))
		r := routerWithExamples("standalone", route.ID)
		r.EmbedderID = &e.ID
		require.NoError(t, r.Create(db))

		hub, err := r.CompiledConfig(db, nil)
		require.NoError(t, err)
		assert.Equal(t, "ollama-key", hub.Settings.Embedding.APIKey)
		assert.True(t, hub.Settings.Embedding.Inline())
		require.NoError(t, sr.Validate(hub))

		js, err := r.EdgeConfigJSON(db, func(s string) (string, error) { return "enc:" + s, nil })
		require.NoError(t, err)
		var edge sr.Config
		require.NoError(t, json.Unmarshal([]byte(js), &edge))
		assert.Equal(t, "enc:ollama-key", edge.Settings.Embedding.APIKeyEncrypted)
		assert.Empty(t, edge.Settings.Embedding.APIKey, "the plain key never leaves the hub")
		assert.Equal(t, "ollama", edge.Settings.Embedding.Vendor)
		assert.Equal(t, e.ID, edge.Settings.Embedding.EmbedderID)
		assert.NotContains(t, js, `"ollama-key"`)

		_, err = r.EdgeConfigJSON(db, func(string) (string, error) { return "", fmt.Errorf("no key") })
		assert.Error(t, err, "a key that cannot be encrypted keeps the router off the edge")

		scores, err := SemanticRouterPrivacyScores(db, []uint{r.ID})
		require.NoError(t, err)
		assert.Equal(t, 20, scores[r.ID], "a standalone embedder's privacy counts")
	})

	t.Run("no embedder keeps settings.embedding as it is", func(t *testing.T) {
		r := routerWithExamples("legacy", route.ID)
		r.Settings.Embedding = &sr.ModelRef{LLMID: embedLLM.ID, Model: "m"}
		cfg, err := r.CompiledConfig(db, nil)
		require.NoError(t, err)
		assert.Equal(t, r.Settings.Embedding, cfg.Settings.Embedding)
	})

	t.Run("an embedder whose LLM is gone does not compile", func(t *testing.T) {
		gone := &LLM{Name: "Gone", Vendor: OPENAI, Active: true}
		require.NoError(t, db.Create(gone).Error)
		e := &Embedder{Name: "orphan", LLMID: &gone.ID, ModelName: "m"}
		require.NoError(t, e.Create(db))
		r := routerWithExamples("orphan", route.ID)
		r.EmbedderID = &e.ID
		require.NoError(t, r.Create(db))
		require.NoError(t, db.Delete(gone).Error)
		r.Embedder = nil
		_, err := r.CompiledConfig(db, nil)
		assert.ErrorIs(t, err, ErrEmbedderLLMMissing)
	})
}

// Routers saved before Embedders named their embedding LLM in settings; the
// migration links them to the matching embedder and keeps the timeout.
func TestMigrateEmbedders_SemanticRouters(t *testing.T) {
	db := setupEmbedderTestDB(t)
	route, embedLLM := routerFixtures(t, db)

	a := routerWithExamples("a", route.ID)
	a.Settings.Embedding = &sr.ModelRef{LLMID: embedLLM.ID, Model: "text-embedding-3-small", TimeoutMs: 1200}
	b := routerWithExamples("b", route.ID)
	b.Settings.Embedding = &sr.ModelRef{LLMID: embedLLM.ID, Model: "text-embedding-3-small"}
	none := &SemanticRouter{Name: "none", Slug: "none", Settings: sr.Settings{DefaultRoute: "x"}, Routes: []sr.Route{{Name: "x"}}}
	for _, r := range []*SemanticRouter{a, b, none} {
		require.NoError(t, r.Create(db))
	}

	require.NoError(t, MigrateEmbedders(db))

	var got []SemanticRouter
	require.NoError(t, db.Preload("Embedder").Order("id").Find(&got).Error)
	require.NotNil(t, got[0].EmbedderID)
	assert.Equal(t, *got[0].EmbedderID, *got[1].EmbedderID, "same LLM and model share one linked embedder")
	assert.True(t, got[0].Embedder.IsLinked())
	assert.Equal(t, embedLLM.ID, *got[0].Embedder.LLMID)
	assert.Equal(t, "Embed LLM · text-embedding-3-small", got[0].Embedder.Name)
	assert.Equal(t, &sr.ModelRef{TimeoutMs: 1200}, got[0].Settings.Embedding, "only the timeout stays in settings")
	assert.Nil(t, got[1].Settings.Embedding)
	assert.Nil(t, got[2].EmbedderID)

	cfg, err := got[0].CompiledConfig(db, nil)
	require.NoError(t, err)
	assert.Equal(t, &sr.ModelRef{LLMID: embedLLM.ID, Model: "text-embedding-3-small", TimeoutMs: 1200}, cfg.Settings.Embedding,
		"the edge sees the same reference as before the migration")

	var count int64
	require.NoError(t, db.Model(&Embedder{}).Count(&count).Error)
	require.NoError(t, MigrateEmbedders(db))
	var again int64
	require.NoError(t, db.Model(&Embedder{}).Count(&again).Error)
	assert.Equal(t, count, again, "a second run is a no-op")
}
