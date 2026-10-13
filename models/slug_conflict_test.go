package models

import (
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requireSlugConflict(t *testing.T, err error, existingID uint) {
	t.Helper()
	require.Error(t, err)
	var conflict *SlugConflictError
	require.True(t, errors.As(err, &conflict), "want a *SlugConflictError, got %v", err)
	assert.True(t, errors.Is(err, ErrSlugConflict))
	assert.Equal(t, existingID, conflict.ExistingID)
}

// Tool names that fold to one slug are refused, whatever their case, spacing
// or punctuation, and in any namespace.
func TestToolSlugIsUnique(t *testing.T) {
	db := setupTestDB(t)
	first := &Tool{Name: "Page fetcher", ToolType: ToolTypeREST}
	require.NoError(t, first.Create(db))

	for _, name := range []string{"Page fetcher", "PAGE FETCHER", "page-fetcher", " Page  fetcher! "} {
		err := (&Tool{Name: name, ToolType: ToolTypeREST}).Create(db)
		requireSlugConflict(t, err, first.ID)
	}
	err := (&Tool{Name: "Page fetcher", ToolType: ToolTypeREST, Namespace: "eu"}).Create(db)
	requireSlugConflict(t, err, first.ID)

	// Renaming another tool onto the slug is refused too.
	other := &Tool{Name: "Other", ToolType: ToolTypeREST}
	require.NoError(t, other.Create(db))
	other.Name = "page fetcher"
	requireSlugConflict(t, other.Update(db), first.ID)

	var count int64
	require.NoError(t, db.Model(&Tool{}).Where("slug = ?", "page-fetcher").Count(&count).Error)
	assert.Equal(t, int64(1), count)

	// Saving a tool under its own name is not a conflict.
	first.Description = "edited"
	require.NoError(t, first.Update(db))

	// A deleted tool frees its slug.
	require.NoError(t, first.Delete(db))
	require.NoError(t, (&Tool{Name: "Page fetcher", ToolType: ToolTypeREST}).Create(db))
}

// A duplicate stored before the check stays editable; it can be renamed
// away from the conflict, and is refused only if it would take a new one.
func TestToolSlugExistingDuplicateStaysEditable(t *testing.T) {
	db := setupTestDB(t)
	first := &Tool{Name: "Dup", ToolType: ToolTypeREST}
	require.NoError(t, first.Create(db))
	second := &Tool{Name: "Dup", ToolType: ToolTypeREST, Slug: "dup"}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(second).Error)

	second.Description = "still a duplicate"
	require.NoError(t, second.Update(db), "an unchanged slug is not re-checked")

	second.Name = "Dup two"
	require.NoError(t, second.Update(db))
	assert.Equal(t, "dup-two", second.Slug)
}

// The same for an LLM stored with a duplicate name before the check.
func TestLLMSlugExistingDuplicateStaysEditable(t *testing.T) {
	db := setupTestDB(t)
	first := &LLM{Name: "Dup LLM", Vendor: OPENAI}
	require.NoError(t, first.Create(db))
	second := &LLM{Name: "dup llm", Vendor: OPENAI}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(second).Error)

	second.ShortDescription = "still a duplicate"
	require.NoError(t, second.Update(db), "an unchanged slug is not re-checked")
	second.Name = "Dup LLM two"
	require.NoError(t, second.Update(db))
}

// Column updates (Updates with a map run no slug hook) go through
// PrepareToolUpdates: a new name brings its slug, a taken one is refused.
func TestPrepareToolUpdates(t *testing.T) {
	db := setupTestDB(t)
	a := &Tool{Name: "Alpha", ToolType: ToolTypeREST}
	b := &Tool{Name: "Beta", ToolType: ToolTypeREST}
	require.NoError(t, a.Create(db))
	require.NoError(t, b.Create(db))

	updates := map[string]interface{}{"name": "ALPHA"}
	requireSlugConflict(t, PrepareToolUpdates(db, b.ID, updates), a.ID)

	updates = map[string]interface{}{"name": "Gamma ray", "description": "x"}
	require.NoError(t, PrepareToolUpdates(db, b.ID, updates))
	assert.Equal(t, "gamma-ray", updates["slug"])
	require.NoError(t, db.Model(&Tool{}).Where("id = ?", b.ID).Updates(updates).Error)
	var stored Tool
	require.NoError(t, db.First(&stored, b.ID).Error)
	assert.Equal(t, "gamma-ray", stored.Slug)

	// No name, nothing to check or add.
	updates = map[string]interface{}{"description": "y"}
	require.NoError(t, PrepareToolUpdates(db, a.ID, updates))
	assert.NotContains(t, updates, "slug")
}

// LLM names that fold to one slug are refused the same way; the error also
// matches ErrRouteSlugTaken, as an LLM's slug is its route.
func TestLLMSlugIsUnique(t *testing.T) {
	db := setupTestDB(t)
	first := &LLM{Name: "GPT 4", Vendor: OPENAI}
	require.NoError(t, first.Create(db))

	for _, name := range []string{"gpt-4", "GPT  4", "gpt 4!"} {
		err := (&LLM{Name: name, Vendor: OPENAI}).Create(db)
		requireSlugConflict(t, err, first.ID)
		assert.True(t, errors.Is(err, ErrRouteSlugTaken))
	}
	err := (&LLM{Name: "GPT 4", Vendor: OPENAI, Namespace: "eu"}).Create(db)
	requireSlugConflict(t, err, first.ID)

	other := &LLM{Name: "Claude", Vendor: ANTHROPIC}
	require.NoError(t, other.Create(db))
	other.Name = "GPT-4"
	requireSlugConflict(t, other.Update(db), first.ID)

	first.ShortDescription = "edited"
	require.NoError(t, first.Update(db), "saving an LLM under its own name")

	// Column updates of other fields are not affected.
	require.NoError(t, db.Model(&LLM{}).Where("id = ?", other.ID).Update("active", false).Error)
}
