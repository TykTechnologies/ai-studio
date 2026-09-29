//go:build !cgo

// Chroma needs cgo (see chroma.go). Without it these stand-ins report that
// Chroma is unavailable, and AVAILABLE_VECTOR_STORES leaves it out.

package data_session

import (
	"context"
	"errors"

	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/embeddings"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/schema"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/vectorstores"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// ChromaSupported reports whether this build can use Chroma, which needs cgo.
const ChromaSupported = false

// ErrChromaUnavailable is returned for a Chroma datasource by a build
// without cgo, which cannot link the Chroma client.
var ErrChromaUnavailable = errors.New("chroma vector store is not available: this build has no cgo support")

func newChromaStore(*models.Datasource, *embeddings.EmbedderImpl) (vectorstores.VectorStore, error) {
	return nil, ErrChromaUnavailable
}

func chromaMetadataValue(any) (any, bool) { return nil, false }

func (ds *DataSession) storeToChroma(context.Context, vectorstores.VectorStore, *models.Datasource, []string, [][]float32, []map[string]any) error {
	return ErrChromaUnavailable
}

func (ds *DataSession) searchChromaByVector(context.Context, vectorstores.VectorStore, *models.Datasource, []float32, int) ([]schema.Document, error) {
	return nil, ErrChromaUnavailable
}

func (ds *DataSession) deleteChromaByMetadata(context.Context, *models.Datasource, map[string]string, string, bool) (int, error) {
	return 0, ErrChromaUnavailable
}

func (ds *DataSession) queryChromaByMetadata(context.Context, *models.Datasource, map[string]string, string, int, int) ([]schema.Document, int, error) {
	return nil, 0, ErrChromaUnavailable
}

func (ds *DataSession) listChromaCollections(context.Context, *models.Datasource) ([]NamespaceInfo, error) {
	return nil, ErrChromaUnavailable
}

func (ds *DataSession) deleteChromaCollection(context.Context, *models.Datasource, string) error {
	return ErrChromaUnavailable
}
