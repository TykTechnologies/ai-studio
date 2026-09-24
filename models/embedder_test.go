package models

import (
	"encoding/json"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupEmbedderTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, InitModels(db))
	return db
}

func uintPtr(v uint) *uint { return &v }

func TestEmbedder_Validate(t *testing.T) {
	cases := []struct {
		name    string
		e       Embedder
		wantErr string
	}{
		{"standalone ok", Embedder{Name: "a", Vendor: OPENAI, ModelName: "m", PrivacyScore: 50}, ""},
		{"linked ok", Embedder{Name: "a", LLMID: uintPtr(1), ModelName: "m"}, ""},
		{"name required", Embedder{Vendor: OPENAI, ModelName: "m"}, "name is required"},
		{"model required", Embedder{Name: "a", Vendor: OPENAI}, "model is required"},
		{"standalone needs vendor", Embedder{Name: "a", ModelName: "m"}, "vendor"},
		{"linked carries no key", Embedder{Name: "a", LLMID: uintPtr(1), ModelName: "m", APIKey: "k"}, "linked embedder"},
		{"linked carries no vendor", Embedder{Name: "a", LLMID: uintPtr(1), ModelName: "m", Vendor: OPENAI}, "linked embedder"},
		{"privacy range", Embedder{Name: "a", Vendor: OPENAI, ModelName: "m", PrivacyScore: 101}, "privacy score"},
		{"LLMID 0 is standalone", Embedder{Name: "a", LLMID: uintPtr(0), ModelName: "m"}, "vendor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.e.Validate()
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrEmbedderInvalid)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestEmbedder_Spec(t *testing.T) {
	db := setupEmbedderTestDB(t)
	secrets.SetDBRef(db)
	t.Setenv("TYK_AI_SECRET_KEY", "test-key")
	require.NoError(t, secrets.CreateSecret(db, &secrets.Secret{VarName: "EMB_KEY", Value: "sk-real"}))
	t.Setenv("EMB_URL", "https://env.example/v1")

	t.Run("standalone resolves secrets only when asked", func(t *testing.T) {
		e := &Embedder{Name: "s", Vendor: OPENAI, Endpoint: "$ENV/EMB_URL", APIKey: "$SECRET/EMB_KEY", ModelName: "m", PrivacyScore: 40}
		require.NoError(t, e.Create(db))

		raw, err := e.Spec(false)
		require.NoError(t, err)
		assert.Equal(t, "$SECRET/EMB_KEY", raw.APIKey)
		assert.Equal(t, "$ENV/EMB_URL", raw.Endpoint)
		assert.Equal(t, 40, raw.PrivacyScore)
		assert.Equal(t, e.ID, raw.EmbedderID)

		resolved, err := GetEmbedderResolved(db, e.ID)
		require.NoError(t, err)
		assert.Equal(t, "sk-real", resolved.APIKey)
		assert.Equal(t, "https://env.example/v1", resolved.Endpoint)
		assert.Equal(t, OPENAI, resolved.Vendor)
		assert.Equal(t, "m", resolved.Model)
	})

	t.Run("linked inherits the LLM's connection and privacy live", func(t *testing.T) {
		llm := &LLM{Name: "Prod OpenAI", Vendor: OPENAI, APIEndpoint: "https://api.openai.com/v1", APIKey: "$SECRET/EMB_KEY", PrivacyScore: 70, Active: true}
		require.NoError(t, db.Create(llm).Error)
		e := &Embedder{Name: "linked", LLMID: &llm.ID, ModelName: "text-embedding-3-small", PrivacyScore: 5}
		require.NoError(t, e.Create(db))

		spec, err := GetEmbedderResolved(db, e.ID)
		require.NoError(t, err)
		assert.Equal(t, OPENAI, spec.Vendor)
		assert.Equal(t, "https://api.openai.com/v1", spec.Endpoint)
		assert.Equal(t, "sk-real", spec.APIKey)
		assert.Equal(t, 70, spec.PrivacyScore, "a linked embedder ignores its own privacy score")

		// Rotating the LLM's endpoint is picked up without touching the embedder.
		require.NoError(t, db.Model(llm).Update("api_endpoint", "https://proxy.internal/v1").Error)
		spec, err = GetEmbedderResolved(db, e.ID)
		require.NoError(t, err)
		assert.Equal(t, "https://proxy.internal/v1", spec.Endpoint)
	})

	t.Run("linked to a deleted LLM does not resolve", func(t *testing.T) {
		llm := &LLM{Name: "Gone", Vendor: OPENAI, Active: true}
		require.NoError(t, db.Create(llm).Error)
		e := &Embedder{Name: "orphan", LLMID: &llm.ID, ModelName: "m"}
		require.NoError(t, e.Create(db))
		require.NoError(t, db.Delete(llm).Error) // soft delete

		_, err := GetEmbedderResolved(db, e.ID)
		assert.ErrorIs(t, err, ErrEmbedderLLMMissing)
	})
}

// The name is unique and the delete is hard, so a name can be reused after
// its embedder is deleted (a soft-deleted row would keep it in the index).
func TestEmbedder_DeleteFreesName(t *testing.T) {
	db := setupEmbedderTestDB(t)
	e := &Embedder{Name: "reused", Vendor: OPENAI, ModelName: "m"}
	require.NoError(t, e.Create(db))
	require.Error(t, (&Embedder{Name: "reused", Vendor: OPENAI, ModelName: "m"}).Create(db), "names are unique")

	require.NoError(t, e.Delete(db))
	assert.NoError(t, (&Embedder{Name: "reused", Vendor: OPENAI, ModelName: "m"}).Create(db))
}

// Datasource JSON keeps the embed_* keys it had before Embedders, derived
// from the embedder, and a decoded document keeps them aside.
func TestDatasource_JSONKeepsLegacyEmbedFields(t *testing.T) {
	ds := Datasource{Name: "Docs", Embedder: &Embedder{Vendor: OPENAI, Endpoint: "https://e", APIKey: "sk-1", ModelName: "m1"}}
	ds.EmbedderID = uintPtr(9)

	raw, err := ds.MarshalJSON()
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &m))
	assert.Equal(t, "openai", m["embed_vendor"])
	assert.Equal(t, "https://e", m["embed_url"])
	assert.Equal(t, "sk-1", m["embed_api_key"])
	assert.Equal(t, "m1", m["embed_model"])
	assert.EqualValues(t, 9, m["embedder_id"])
	assert.Equal(t, "Docs", m["name"])

	t.Run("redacted copy leaves the original alone", func(t *testing.T) {
		red := ds
		red.RedactEmbedKey()
		raw, err := red.MarshalJSON()
		require.NoError(t, err)
		assert.Contains(t, string(raw), `"embed_api_key":"[REDACTED]"`)

		raw, err = ds.MarshalJSON()
		require.NoError(t, err)
		assert.Contains(t, string(raw), `"embed_api_key":"sk-1"`)
	})

	t.Run("decode keeps the legacy fields aside", func(t *testing.T) {
		var back Datasource
		require.NoError(t, json.Unmarshal([]byte(`{"name":"Docs","embedder_id":9,"embed_vendor":"ollama","embed_model":"nomic"}`), &back))
		le, ok := back.LegacyEmbedInput()
		require.True(t, ok)
		assert.Equal(t, "ollama", le.Vendor)
		assert.Equal(t, "nomic", le.Model)
		assert.Equal(t, uint(9), *back.EmbedderID)
		assert.Nil(t, back.Embedder)

		var none Datasource
		require.NoError(t, json.Unmarshal([]byte(`{"name":"Docs"}`), &none))
		_, ok = none.LegacyEmbedInput()
		assert.False(t, ok)
	})

	t.Run("no embedder flattens to empty fields", func(t *testing.T) {
		raw, err := (Datasource{Name: "Bare"}).MarshalJSON()
		require.NoError(t, err)
		assert.Contains(t, string(raw), `"embed_vendor":""`)
	})
}
