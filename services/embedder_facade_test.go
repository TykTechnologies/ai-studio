package services

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func legacy(vendor, url, key, model string) EmbedderInput {
	return EmbedderInput{Vendor: vendor, URL: url, APIKey: key, Model: model}
}

func createLegacyDS(t *testing.T, s *Service, name string, privacy int, in EmbedderInput) *models.Datasource {
	t.Helper()
	ds, err := s.CreateDatasource(name, "", "", "", "", privacy, 1, nil, "", "pgvector", "", "db", in, true)
	require.NoError(t, err)
	return ds
}

func updateLegacyDS(t *testing.T, s *Service, ds *models.Datasource, privacy int, in EmbedderInput) (*models.Datasource, error) {
	t.Helper()
	return s.UpdateDatasource(ds.ID, ds.Name, "", "", "", "", privacy, "", "", "[redacted]", "db", in, true, nil, 1)
}

func embedderCount(t *testing.T, s *Service) int64 {
	t.Helper()
	var n int64
	require.NoError(t, s.DB.Model(&models.Embedder{}).Count(&n).Error)
	return n
}

// The datasource API's embed_* fields keep working: they are resolved to an
// embedder, identical settings share one, and a change never edits a shared
// embedder (copy-on-write).
func TestLegacyEmbedFields_CopyOnWrite(t *testing.T) {
	s := setupEmbedderService(t)
	openai := legacy("openai", "https://api.openai.com/v1", "sk-1", "text-embedding-3-small")

	a := createLegacyDS(t, s, "A", 40, openai)
	b := createLegacyDS(t, s, "B", 40, openai)
	require.NotNil(t, a.EmbedderID)
	assert.Equal(t, *a.EmbedderID, *b.EmbedderID, "identical settings dedupe to one embedder")
	assert.EqualValues(t, 1, embedderCount(t, s))
	assert.Equal(t, "openai · text-embedding-3-small", a.Embedder.Name)
	assert.Equal(t, 40, a.Embedder.PrivacyScore)

	t.Run("echoing the flattened fields back changes nothing", func(t *testing.T) {
		got, err := updateLegacyDS(t, s, a, 40, legacy("openai", "https://api.openai.com/v1", RedactedEmbedderKey, "text-embedding-3-small"))
		require.NoError(t, err)
		assert.Equal(t, *b.EmbedderID, *got.EmbedderID)
		assert.EqualValues(t, 1, embedderCount(t, s))
	})

	t.Run("empty vendor, url and model keep the current values", func(t *testing.T) {
		got, err := updateLegacyDS(t, s, a, 40, legacy("", "", RedactedEmbedderKey, ""))
		require.NoError(t, err)
		assert.Equal(t, *b.EmbedderID, *got.EmbedderID)
	})

	t.Run("changing the model moves only this datasource", func(t *testing.T) {
		got, err := updateLegacyDS(t, s, a, 40, legacy("", "", RedactedEmbedderKey, "text-embedding-3-large"))
		require.NoError(t, err)
		assert.NotEqual(t, *b.EmbedderID, *got.EmbedderID)
		assert.Equal(t, "text-embedding-3-large", got.FlattenedEmbed().Model)
		assert.Equal(t, "sk-1", got.FlattenedEmbed().APIKey, "the redacted key carried over to the new embedder")

		bNow, err := s.GetDatasourceByID(b.ID)
		require.NoError(t, err)
		assert.Equal(t, "text-embedding-3-small", bNow.FlattenedEmbed().Model, "the shared embedder is untouched")
	})

	t.Run("an empty key clears it (on a new embedder)", func(t *testing.T) {
		got, err := updateLegacyDS(t, s, b, 40, legacy("", "", "", ""))
		require.NoError(t, err)
		assert.Equal(t, "", got.FlattenedEmbed().APIKey)
		assert.Equal(t, "text-embedding-3-small", got.FlattenedEmbed().Model)
	})

	t.Run("an explicit embedder_id wins over the legacy fields", func(t *testing.T) {
		e, err := s.CreateEmbedder(&models.Embedder{Name: "explicit", Vendor: models.OLLAMA, Endpoint: "http://ollama:11434", ModelName: "nomic", PrivacyScore: 100}, 1)
		require.NoError(t, err)
		in := legacy("openai", "https://x", "k", "m")
		in.EmbedderID = &e.ID
		got, err := updateLegacyDS(t, s, a, 40, in)
		require.NoError(t, err)
		assert.Equal(t, e.ID, *got.EmbedderID)
	})

	t.Run("raising a datasource's privacy above its embedder forks a covering one", func(t *testing.T) {
		c := createLegacyDS(t, s, "C", 10, legacy("openai", "https://c", "k", "m"))
		got, err := updateLegacyDS(t, s, c, 90, legacy("", "", RedactedEmbedderKey, ""))
		require.NoError(t, err)
		assert.NotEqual(t, *c.EmbedderID, *got.EmbedderID)
		assert.Equal(t, 90, got.Embedder.PrivacyScore)
	})
}

func TestExplicitEmbedder_PrivacyChecked(t *testing.T) {
	s := setupEmbedderService(t)
	e, err := s.CreateEmbedder(standalone("low", 20), 1)
	require.NoError(t, err)

	_, err = s.CreateDatasource("Secret", "", "", "", "", 50, 1, nil, "", "", "", "db", EmbedderInput{EmbedderID: &e.ID}, true)
	var privacy *EmbedderPrivacyError
	require.True(t, errors.As(err, &privacy), "%v", err)

	missing := uint(404)
	_, err = s.CreateDatasource("Missing", "", "", "", "", 0, 1, nil, "", "", "", "db", EmbedderInput{EmbedderID: &missing}, true)
	assert.ErrorIs(t, err, ErrEmbedderInvalid)
}

// A linked embedder flattens to its LLM's connection; changing only the model
// through the legacy fields links the same LLM with the new model rather than
// copying the LLM's key into a standalone embedder.
func TestLegacyEmbedFields_LinkedEmbedder(t *testing.T) {
	s := setupEmbedderService(t)
	llm := &models.LLM{Name: "OpenAI", Vendor: models.OPENAI, APIEndpoint: "https://api.openai.com/v1", APIKey: "$SECRET/OPENAI", PrivacyScore: 90, Active: true}
	require.NoError(t, s.DB.Create(llm).Error)
	e, err := s.FindOrCreateLinkedEmbedder(llm.ID, "text-embedding-3-small", 1)
	require.NoError(t, err)
	again, err := s.FindOrCreateLinkedEmbedder(llm.ID, "text-embedding-3-small", 1)
	require.NoError(t, err)
	assert.Equal(t, e.ID, again.ID)

	ds := newDatasourceOn(t, s, "Docs", 50, e.ID)
	flat := ds.FlattenedEmbed()
	assert.Equal(t, "openai", flat.Vendor)
	assert.Equal(t, "https://api.openai.com/v1", flat.URL)
	assert.Equal(t, "$SECRET/OPENAI", flat.APIKey, "API reads keep the reference")

	got, err := updateLegacyDS(t, s, ds, 50, legacy(flat.Vendor, flat.URL, RedactedEmbedderKey, flat.Model))
	require.NoError(t, err)
	assert.Equal(t, e.ID, *got.EmbedderID, "an echo is a no-op")

	got, err = updateLegacyDS(t, s, ds, 50, legacy("", "", RedactedEmbedderKey, "text-embedding-3-large"))
	require.NoError(t, err)
	require.NotNil(t, got.Embedder)
	assert.True(t, got.Embedder.IsLinked())
	assert.Equal(t, llm.ID, *got.Embedder.LLMID)
	assert.Equal(t, "text-embedding-3-large", got.Embedder.ModelName)
}

// A plugin hook sees the legacy embed_* keys; editing them moves the
// datasource to a matching embedder, as a legacy API write would.
func TestApplyHookEmbedEdits(t *testing.T) {
	s := setupEmbedderService(t)
	ds := createLegacyDS(t, s, "Docs", 10, legacy("openai", "https://e", "sk", "m1"))
	original := ds.Embedder

	unchanged := decodeDatasourceJSON(t, ds)
	require.NoError(t, s.applyHookEmbedEdits(s.DB, original, unchanged, 1))
	assert.Equal(t, original.ID, *unchanged.EmbedderID)

	redacted := *ds
	redacted.RedactEmbedKey()
	viaRedacted := decodeDatasourceJSON(t, &redacted)
	require.NoError(t, s.applyHookEmbedEdits(s.DB, original, viaRedacted, 1))
	assert.Equal(t, original.ID, *viaRedacted.EmbedderID, "a redacted key round-trips as 'keep'")

	edited := decodeFromMap(t, ds, map[string]interface{}{"embed_model": "m2"})
	require.NoError(t, s.applyHookEmbedEdits(s.DB, original, edited, 1))
	assert.NotEqual(t, original.ID, *edited.EmbedderID)
	assert.Equal(t, "m2", edited.Embedder.ModelName)
}

func decodeDatasourceJSON(t *testing.T, ds *models.Datasource) *models.Datasource {
	return decodeFromMap(t, ds, nil)
}

func decodeFromMap(t *testing.T, ds *models.Datasource, overrides map[string]interface{}) *models.Datasource {
	t.Helper()
	raw, err := ds.MarshalJSON()
	require.NoError(t, err)
	m := map[string]interface{}{}
	require.NoError(t, json.Unmarshal(raw, &m))
	for k, v := range overrides {
		m[k] = v
	}
	raw, err = json.Marshal(m)
	require.NoError(t, err)
	var out models.Datasource
	require.NoError(t, json.Unmarshal(raw, &out))
	return &out
}

// Version snapshots keep the embed_* keys (derived from the embedder), and
// rolling back to one written before Embedders existed resolves those keys
// to an embedder instead of writing the retired columns.
func TestSubmissionVersions_EmbedderRoundTrip(t *testing.T) {
	s := setupEmbedderService(t)
	ds := createLegacyDS(t, s, "Docs", 20, legacy("openai", "https://api.openai.com/v1", "sk-1", "m1"))
	firstEmbedder := *ds.EmbedderID

	snap, err := s.snapshotDatasource(ds.ID)
	require.NoError(t, err)
	assert.Equal(t, "openai", snap["embed_vendor"])
	assert.Equal(t, "m1", snap["embed_model"])
	assert.Equal(t, "sk-1", snap["embed_api_key"])
	assert.EqualValues(t, firstEmbedder, snap["embedder_id"])

	// Move the datasource to another model.
	_, err = updateLegacyDS(t, s, ds, 20, legacy("", "", RedactedEmbedderKey, "m2"))
	require.NoError(t, err)

	// A version stored before Embedders: embed_* keys, no embedder_id.
	old := &models.SubmissionVersion{ResourceID: ds.ID, ResourceType: models.SubmissionResourceTypeDatasource, VersionNumber: 1,
		Payload: models.JSONMap{
			"name": "Docs", "privacy_score": float64(20),
			"embed_vendor": "openai", "embed_url": "https://api.openai.com/v1", "embed_api_key": "sk-1", "embed_model": "m1",
		}}
	require.NoError(t, old.Create(s.DB))

	require.NoError(t, s.RollbackResource(models.SubmissionResourceTypeDatasource, ds.ID, old.ID, 1))

	back, err := s.GetDatasourceByID(ds.ID)
	require.NoError(t, err)
	assert.Equal(t, "m1", back.FlattenedEmbed().Model)
	assert.Equal(t, firstEmbedder, *back.EmbedderID, "the rollback found the matching embedder rather than creating one")
}

// The gateway loads datasources with GetActiveDatasources and embeds with
// their resolved embedder: a $SECRET/ key reaches the vendor as its value.
// (Before Embedders the gateway passed the reference string through.)
func TestActiveDatasources_ResolveEmbedderSecrets(t *testing.T) {
	s := setupEmbedderService(t)
	t.Setenv("TYK_AI_SECRET_KEY", "test-key")
	secrets.SetDBRef(s.DB)
	require.NoError(t, secrets.CreateSecret(s.DB, &secrets.Secret{VarName: "EMB", Value: "sk-real"}))
	createLegacyDS(t, s, "Docs", 10, legacy("openai", "https://api.openai.com/v1", "$SECRET/EMB", "m"))

	active, err := s.GetActiveDatasources()
	require.NoError(t, err)
	require.Len(t, active, 1)
	spec, err := DatasourceEmbedderSpec(&active[0])
	require.NoError(t, err)
	assert.Equal(t, "sk-real", spec.APIKey)
	assert.Equal(t, "$SECRET/EMB", active[0].FlattenedEmbed().APIKey, "API reads keep the reference")
}
