package models

import (
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLLMSettings_NewLLMSettings(t *testing.T) {
	settings := NewLLMSettings()
	assert.NotNil(t, settings)
}

func TestLLMSettings_CRUD(t *testing.T) {
	db := setupTestDB(t)

	// Create
	settings := &LLMSettings{
		MaxLength:         100,
		MaxTokens:         50,
		Metadata:          map[string]interface{}{"key": "value"},
		MinLength:         10,
		ModelName:         "TestModel",
		RepetitionPenalty: 1.2,
		Seed:              42,
		StopWords:         []string{"stop1", "stop2"},
		Temperature:       0.7,
		TopK:              40,
		TopP:              0.9,
	}
	err := settings.Create(db)
	assert.NoError(t, err)
	assert.NotZero(t, settings.ID)

	// Get
	fetchedSettings := NewLLMSettings()
	err = fetchedSettings.Get(db, settings.ID)
	assert.NoError(t, err)
	assert.Equal(t, settings.ModelName, fetchedSettings.ModelName)
	assert.Equal(t, settings.Temperature, fetchedSettings.Temperature)
	assert.ElementsMatch(t, settings.StopWords, fetchedSettings.StopWords)

	// Update
	settings.ModelName = "UpdatedTestModel"
	settings.Temperature = 0.8
	err = settings.Update(db)
	assert.NoError(t, err)

	err = fetchedSettings.Get(db, settings.ID)
	assert.NoError(t, err)
	assert.Equal(t, "UpdatedTestModel", fetchedSettings.ModelName)
	assert.Equal(t, 0.8, fetchedSettings.Temperature)

	// Delete
	err = settings.Delete(db)
	assert.NoError(t, err)

	err = fetchedSettings.Get(db, settings.ID)
	assert.Error(t, err) // Should return an error as the settings are deleted
}

func TestLLMSettings_GetAll(t *testing.T) {
	db := setupTestDB(t)

	// Create some test LLMSettings
	settings := []LLMSettings{
		{ModelName: "Model1", Temperature: 0.7},
		{ModelName: "Model2", Temperature: 0.8},
		{ModelName: "Model3", Temperature: 0.9},
	}
	for _, s := range settings {
		err := db.Create(&s).Error
		assert.NoError(t, err)
	}

	// Test GetAll
	var fetchedSettings LLMSettingsSlice
	_, _, err := fetchedSettings.GetAll(db, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, fetchedSettings, 3)
	assert.Equal(t, "Model1", fetchedSettings[0].ModelName)
	assert.Equal(t, "Model2", fetchedSettings[1].ModelName)
	assert.Equal(t, "Model3", fetchedSettings[2].ModelName)
}

func TestLLMSettings_GetByModel(t *testing.T) {
	db := setupTestDB(t)

	// Create a test LLMSettings
	settings := &LLMSettings{
		ModelName:   "UniqueModel",
		Temperature: 0.75,
	}
	err := db.Create(settings).Error
	assert.NoError(t, err)

	// Test GetByModel
	fetchedSettings := NewLLMSettings()
	err = fetchedSettings.GetByModel(db, "UniqueModel")
	assert.NoError(t, err)
	assert.Equal(t, settings.ID, fetchedSettings.ID)
	assert.Equal(t, settings.ModelName, fetchedSettings.ModelName)
	assert.Equal(t, settings.Temperature, fetchedSettings.Temperature)

	// Test with non-existent model
	err = fetchedSettings.GetByModel(db, "NonExistentModel")
	assert.Error(t, err)
}

func TestLLMSettings_SearchByModelStub(t *testing.T) {
	db := setupTestDB(t)

	// Create some test LLMSettings
	settings := []LLMSettings{
		{ModelName: "GPT-3", Temperature: 0.7},
		{ModelName: "GPT-4", Temperature: 0.8},
		{ModelName: "BERT", Temperature: 0.9},
	}
	for _, s := range settings {
		err := db.Create(&s).Error
		assert.NoError(t, err)
	}

	// Test SearchByModelStub
	var fetchedSettings LLMSettingsSlice
	err := fetchedSettings.SearchByModelStub(db, "GPT")
	assert.NoError(t, err)
	assert.Len(t, fetchedSettings, 2)
	assert.Equal(t, "GPT-3", fetchedSettings[0].ModelName)
	assert.Equal(t, "GPT-4", fetchedSettings[1].ModelName)

	// Test with a different stub
	fetchedSettings = LLMSettingsSlice{}
	err = fetchedSettings.SearchByModelStub(db, "BERT")
	assert.NoError(t, err)
	assert.Len(t, fetchedSettings, 1)
	assert.Equal(t, "BERT", fetchedSettings[0].ModelName)

	// Test with a stub that doesn't match any settings
	fetchedSettings = LLMSettingsSlice{}
	err = fetchedSettings.SearchByModelStub(db, "XYZ")
	assert.NoError(t, err)
	assert.Len(t, fetchedSettings, 0)
}

// The seeded providers used to point at gpt-4o and claude-sonnet-4-20250514,
// both end-of-life. A new user who filled in their API key -- already the
// second stumble, after the empty bootstrap secret -- then hit model-not-found
// on their first call. Nothing caught the drift.
//
// This is the anti-rot mechanism: a default that is not in this build's model
// catalogue fails here rather than on a customer's first request.
func TestDefaultBootstrapModelsAreInCatalogue(t *testing.T) {
	catalogue := make(map[string]bool)
	for _, s := range DefaultLLMSettings() {
		catalogue[s.ModelName] = true
	}

	assert.NotEmpty(t, DefaultBootstrapModels, "every seeded provider needs a default model")

	for vendor, model := range DefaultBootstrapModels {
		assert.True(t, catalogue[model],
			"default model %q for vendor %q is not in DefaultLLMSettings; it will fail with model-not-found on first use",
			model, vendor)
	}
}

// The seeded providers must actually use the centralised defaults, or the two
// can drift apart again.
func TestDefaultLLMsUseBootstrapModels(t *testing.T) {
	db := setupTestDB(t)

	err := GetOrCreateDefaultLLMs(db, nil)
	assert.NoError(t, err)

	for vendor, want := range DefaultBootstrapModels {
		var llm LLM
		err := db.Where("vendor = ?", vendor).First(&llm).Error
		assert.NoError(t, err, "expected a seeded provider for vendor %q", vendor)
		assert.Equal(t, want, llm.DefaultModel)
	}
}

// A default an administrator deleted is not seeded again on the next start,
// so it does not quietly take back a licensed LLM slot.
func TestDefaultLLMsStayDeleted(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, GetOrCreateDefaultLLMs(db, nil))

	var openai LLM
	require.NoError(t, db.Where("name = ?", "OpenAI").First(&openai).Error)
	require.NoError(t, openai.Delete(db))

	require.NoError(t, GetOrCreateDefaultLLMs(db, nil))
	var n int64
	require.NoError(t, db.Model(&LLM{}).Where("name = ?", "OpenAI").Count(&n).Error)
	assert.Equal(t, int64(0), n)
	require.NoError(t, db.Model(&LLM{}).Where("name = ?", "Anthropic").Count(&n).Error)
	assert.Equal(t, int64(1), n)
}

// A guard that skips a default leaves it out and seeds the rest; any other
// guard error fails the seed.
func TestDefaultLLMsGuard(t *testing.T) {
	db := setupTestDB(t)
	calls := 0
	skipFirst := func(tx *gorm.DB, fn func(*gorm.DB) error) error {
		calls++
		if calls == 1 {
			return ErrDefaultLLMSkipped
		}
		return tx.Transaction(fn)
	}
	require.NoError(t, GetOrCreateDefaultLLMs(db, skipFirst))
	var n int64
	require.NoError(t, db.Model(&LLM{}).Count(&n).Error)
	assert.Equal(t, int64(1), n)

	boom := errors.New("boom")
	err := GetOrCreateDefaultLLMs(setupTestDB(t), func(*gorm.DB, func(*gorm.DB) error) error { return boom })
	assert.ErrorIs(t, err, boom)
}

// LLM.Create works inside a caller's transaction (the licence guard) and
// rolls back with it.
func TestLLMCreateInsideTransaction(t *testing.T) {
	db := setupTestDB(t)
	refused := errors.New("refused")
	err := db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, (&LLM{Name: "inner", Vendor: OPENAI}).Create(tx))
		return refused
	})
	require.ErrorIs(t, err, refused)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return (&LLM{Name: "kept", Vendor: OPENAI}).Create(tx)
	}))

	var names []string
	require.NoError(t, db.Model(&LLM{}).Pluck("name", &names).Error)
	assert.Equal(t, []string{"kept"}, names)
}
