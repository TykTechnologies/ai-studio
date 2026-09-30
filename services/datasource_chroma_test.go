package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setChromaSupported makes the test behave like a build with cgo (true) or a
// CGO_ENABLED=0 build, which has no Chroma client (false).
func setChromaSupported(t *testing.T, v bool) {
	t.Helper()
	prev := chromaSupported
	chromaSupported = v
	t.Cleanup(func() { chromaSupported = prev })
}

// A build without cgo leaves Chroma out of the vector store list, so a new
// Chroma datasource, or a switch to Chroma, is refused with a clear error
// instead of being saved and failing at query time.
func TestDatasource_ChromaRefusedWithoutCgo(t *testing.T) {
	setChromaSupported(t, true)
	db := setupTestDBForDatasources(t)
	service := NewService(db)
	embed := EmbedderInput{Vendor: "openai", URL: "http://embed", APIKey: "k", Model: "m"}

	existing, err := service.CreateDatasource("Old Chroma", "", "", "", "", 50, 1, nil, "http://chroma:8000", "chroma", "", "c1", embed, true)
	require.NoError(t, err, "with cgo, Chroma is accepted")
	other, err := service.CreateDatasource("Qdrant", "", "", "", "", 50, 1, nil, "http://qdrant:6333", "qdrant", "", "q1", embed, true)
	require.NoError(t, err)

	setChromaSupported(t, false)

	_, err = service.CreateDatasource("New Chroma", "", "", "", "", 50, 1, nil, "http://chroma:8000", "chroma", "", "c2", embed, true)
	require.ErrorIs(t, err, ErrVectorStoreUnavailable)
	assert.Contains(t, err.Error(), "chroma")

	_, err = service.UpdateDatasource(other.ID, "Qdrant", "", "", "", "", 50, "", "chroma", "", "q1", embed, true, nil, 1)
	require.ErrorIs(t, err, ErrVectorStoreUnavailable, "switching to Chroma is refused")

	// An existing Chroma datasource can still be edited (for instance
	// deactivated or moved to another store).
	_, err = service.UpdateDatasource(existing.ID, "Old Chroma", "", "", "", "", 50, "", "chroma", "", "c1", embed, false, nil, 1)
	require.NoError(t, err)
	_, err = service.UpdateDatasource(existing.ID, "Old Chroma", "", "", "", "", 50, "", "", "", "c1", embed, false, nil, 1)
	require.NoError(t, err, "an omitted type keeps the current one")

	unavailable, err := service.UnavailableVectorStoreDatasources()
	require.NoError(t, err)
	require.Len(t, unavailable, 1)
	assert.Equal(t, existing.ID, unavailable[0].ID)

	_, err = service.UpdateDatasource(existing.ID, "Old Chroma", "", "", "", "", 50, "", "qdrant", "", "c1", embed, false, nil, 1)
	require.NoError(t, err)
	unavailable, err = service.UnavailableVectorStoreDatasources()
	require.NoError(t, err)
	assert.Empty(t, unavailable)
}

// With cgo there is nothing to warn about.
func TestUnavailableVectorStoreDatasources_WithChroma(t *testing.T) {
	setChromaSupported(t, true)
	db := setupTestDBForDatasources(t)
	service := NewService(db)
	_, err := service.CreateDatasource("Chroma", "", "", "", "", 50, 1, nil, "http://chroma:8000", "chroma", "", "c1", EmbedderInput{Vendor: "openai", URL: "http://embed", APIKey: "k", Model: "m"}, true)
	require.NoError(t, err)

	unavailable, err := service.UnavailableVectorStoreDatasources()
	require.NoError(t, err)
	assert.Empty(t, unavailable)
}
