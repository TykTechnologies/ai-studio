package services

import (
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupEmbedderService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	return NewService(db)
}

func standalone(name string, privacy int) *models.Embedder {
	return &models.Embedder{Name: name, Vendor: models.OPENAI, Endpoint: "https://api.openai.com/v1", APIKey: "sk-1", ModelName: "text-embedding-3-small", PrivacyScore: privacy}
}

// newDatasourceOn creates a datasource that embeds with the embedder.
func newDatasourceOn(t *testing.T, s *Service, name string, privacy int, embedderID uint) *models.Datasource {
	t.Helper()
	ds, err := s.CreateDatasource(name, "", "", "", "", privacy, 1, nil, "", "pgvector", "", "db",
		EmbedderInput{EmbedderID: &embedderID}, true)
	require.NoError(t, err)
	return ds
}

func TestCreateEmbedder_Validation(t *testing.T) {
	s := setupEmbedderService(t)

	_, err := s.CreateEmbedder(standalone("ok", 50), 1)
	require.NoError(t, err)

	_, err = s.CreateEmbedder(standalone("ok", 50), 1)
	assert.ErrorIs(t, err, ErrEmbedderInvalid, "names are unique")

	bad := standalone("anthropic", 50)
	bad.Vendor = models.ANTHROPIC
	_, err = s.CreateEmbedder(bad, 1)
	assert.ErrorIs(t, err, ErrEmbedderInvalid, "a vendor whose driver cannot embed is refused")

	hf := standalone("hf", 50)
	hf.Vendor = models.HUGGINGFACE
	_, err = s.CreateEmbedder(hf, 1)
	assert.NoError(t, err, "Hugging Face embeds")

	claude := &models.LLM{Name: "Claude", Vendor: models.ANTHROPIC, Active: true}
	require.NoError(t, s.DB.Create(claude).Error)
	_, err = s.CreateEmbedder(&models.Embedder{Name: "linked-claude", LLMID: &claude.ID, ModelName: "m"}, 1)
	assert.ErrorIs(t, err, ErrEmbedderInvalid, "a linked embedder needs an LLM whose vendor embeds")

	missing := uint(999)
	_, err = s.CreateEmbedder(&models.Embedder{Name: "linked-missing", LLMID: &missing, ModelName: "m"}, 1)
	assert.ErrorIs(t, err, ErrEmbedderInvalid)
}

func TestUpdateEmbedder_ModelLockedWhileDatasourcesUseIt(t *testing.T) {
	s := setupEmbedderService(t)
	e, err := s.CreateEmbedder(standalone("shared", 60), 1)
	require.NoError(t, err)

	change := func(mut func(*models.Embedder)) error {
		cur, err := s.GetEmbedder(e.ID)
		require.NoError(t, err)
		next := *cur
		next.APIKey = RedactedEmbedderKey
		mut(&next)
		_, err = s.UpdateEmbedder(e.ID, &next, 1)
		return err
	}

	// Unused: anything goes.
	require.NoError(t, change(func(n *models.Embedder) { n.ModelName = "text-embedding-3-large" }))

	newDatasourceOn(t, s, "Docs", 50, e.ID)

	var locked *EmbedderLockedError
	err = change(func(n *models.Embedder) { n.ModelName = "text-embedding-3-small" })
	require.True(t, errors.As(err, &locked), "model change: %v", err)
	assert.Equal(t, "Docs", locked.Datasources[0].Name)

	err = change(func(n *models.Embedder) { n.Vendor = models.OLLAMA })
	assert.True(t, errors.As(err, &locked), "compatibility change: %v", err)

	llm := &models.LLM{Name: "OpenAI", Vendor: models.OPENAI, PrivacyScore: 90, Active: true}
	require.NoError(t, s.DB.Create(llm).Error)
	err = change(func(n *models.Embedder) { n.LLMID = &llm.ID; n.Vendor, n.Endpoint = "", "" })
	assert.True(t, errors.As(err, &locked), "switching to a linked embedder: %v", err)

	// Connection details and presentation stay editable.
	require.NoError(t, change(func(n *models.Embedder) {
		n.Endpoint = "https://proxy.internal/v1"
		n.APIKey = "sk-rotated"
		n.Name = "shared (renamed)"
		n.Description = "rotated"
	}))
	got, err := s.GetEmbedder(e.ID)
	require.NoError(t, err)
	assert.Equal(t, "sk-rotated", got.APIKey)
	assert.Equal(t, "text-embedding-3-large", got.ModelName)

	// A redacted key is kept.
	require.NoError(t, change(func(n *models.Embedder) { n.Description = "again" }))
	got, _ = s.GetEmbedder(e.ID)
	assert.Equal(t, "sk-rotated", got.APIKey)
}

func TestUpdateEmbedder_PrivacyCannotDropBelowDatasources(t *testing.T) {
	s := setupEmbedderService(t)
	e, err := s.CreateEmbedder(standalone("priv", 80), 1)
	require.NoError(t, err)
	newDatasourceOn(t, s, "Sensitive", 70, e.ID)

	next := *e
	next.APIKey = RedactedEmbedderKey
	next.PrivacyScore = 60
	_, err = s.UpdateEmbedder(e.ID, &next, 1)
	var privacy *EmbedderPrivacyError
	require.True(t, errors.As(err, &privacy), "%v", err)
	assert.Equal(t, 70, privacy.Required)

	next.PrivacyScore = 70
	_, err = s.UpdateEmbedder(e.ID, &next, 1)
	assert.NoError(t, err)
}

func TestDeleteEmbedder_RefusedWhileUsed(t *testing.T) {
	s := setupEmbedderService(t)
	e, err := s.CreateEmbedder(standalone("used", 80), 1)
	require.NoError(t, err)
	ds := newDatasourceOn(t, s, "Docs", 10, e.ID)

	err = s.DeleteEmbedder(e.ID, 1)
	var inUse *EmbedderInUseError
	require.True(t, errors.As(err, &inUse), "%v", err)
	assert.Equal(t, []DependentRef{{ID: ds.ID, Name: "Docs"}}, inUse.Dependents.Datasources)
	assert.Contains(t, err.Error(), "Docs")

	zero := uint(0)
	_, err = s.UpdateDatasource(ds.ID, ds.Name, "", "", "", "", ds.PrivacyScore, "", "", "[redacted]", "db",
		EmbedderInput{EmbedderID: &zero}, true, nil, 1)
	require.NoError(t, err)
	require.NoError(t, s.DeleteEmbedder(e.ID, 1))
	_, err = s.GetEmbedder(e.ID)
	assert.ErrorIs(t, err, ErrEmbedderNotFound)
}

func TestLLMGuards_LinkedEmbedders(t *testing.T) {
	s := setupEmbedderService(t)
	llm := &models.LLM{Name: "OpenAI", Vendor: models.OPENAI, PrivacyScore: 80, Active: true}
	require.NoError(t, s.DB.Create(llm).Error)
	e, err := s.CreateEmbedder(&models.Embedder{Name: "via-llm", LLMID: &llm.ID, ModelName: "text-embedding-3-small"}, 1)
	require.NoError(t, err)

	deps, err := s.GetLLMDependents(llm.ID)
	require.NoError(t, err)
	assert.Equal(t, []DependentRef{{ID: e.ID, Name: "via-llm"}}, deps.Embedders)

	var conflict *LLMEmbedderConflictError
	err = s.DeleteLLM(llm.ID)
	require.True(t, errors.As(err, &conflict), "delete: %v", err)

	// With no datasource on it yet, the LLM can still change vendor to one
	// that embeds, but not to one that cannot.
	assert.NoError(t, s.CheckLLMUpdateForEmbedders(llm, models.OLLAMA, 10, ""))
	assert.True(t, errors.As(s.CheckLLMUpdateForEmbedders(llm, models.ANTHROPIC, 80, ""), &conflict))

	newDatasourceOn(t, s, "Docs", 70, e.ID)
	assert.True(t, errors.As(s.CheckLLMUpdateForEmbedders(llm, models.OLLAMA, 80, ""), &conflict),
		"a vendor change moves the vectors to another space")
	var privacy *EmbedderPrivacyError
	assert.True(t, errors.As(s.CheckLLMUpdateForEmbedders(llm, models.OPENAI, 50, ""), &privacy))
	assert.NoError(t, s.CheckLLMUpdateForEmbedders(llm, models.OPENAI, 70, ""))
}

func TestEmbeddingVendors_ComeFromDrivers(t *testing.T) {
	s := setupEmbedderService(t)
	vendors := s.EmbeddingVendors()
	assert.Contains(t, vendors, models.OPENAI)
	assert.Contains(t, vendors, models.HUGGINGFACE)
	assert.Contains(t, vendors, models.VERTEX)
	assert.NotContains(t, vendors, models.ANTHROPIC)
	assert.NotContains(t, vendors, models.BEDROCK)

	list, err := s.GetAvailableEmbedders()
	require.NoError(t, err)
	names := map[string]bool{}
	for _, v := range list {
		names[v.Vendor] = true
	}
	assert.True(t, names["huggingface"], "the /vendors/embedders list now includes Hugging Face")
	assert.Len(t, list, len(vendors))
}
