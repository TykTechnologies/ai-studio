package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
)

// newTestProxyWithDatasource creates a minimal Proxy with one datasource pre-loaded.
func newTestProxyWithDatasource(t *testing.T, ds *models.Datasource) *Proxy {
	t.Helper()
	db, cancel := setupTest(t)
	t.Cleanup(func() { tearDownTest(db, cancel) })

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := budget.NewService(db, notificationSvc)
	p := NewProxy(service, &Config{Port: 9999}, budgetService)
	require.NotNil(t, p)

	p.mu.Lock()
	p.datasources = map[string]*models.Datasource{"test-ds": ds}
	p.mu.Unlock()

	return p
}

// callHandler invokes a Proxy handler directly with mux vars and a JSON body.
func callHandler(handler http.HandlerFunc, vars map[string]string, body interface{}) *httptest.ResponseRecorder {
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/datasource/test-ds", bytes.NewReader(bodyBytes))
	req = mux.SetURLVars(req, vars)
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

// callHandlerWithApp invokes a handler with an app set in the request context.
func callHandlerWithApp(handler http.HandlerFunc, vars map[string]string, body interface{}, app *models.App) *httptest.ResponseRecorder {
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/datasource/test-ds", bytes.NewReader(bodyBytes))
	req = mux.SetURLVars(req, vars)
	if app != nil {
		ctx := context.WithValue(req.Context(), "app", app)
		req = req.WithContext(ctx)
	}
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

func TestHandleDatasourceVectorSearch(t *testing.T) {
	ds := &models.Datasource{Name: "Test DS", Active: true, DBSourceType: "pinecone"}
	ds.ID = 1
	p := newTestProxyWithDatasource(t, ds)
	vars := map[string]string{"dsSlug": "test-ds"}

	t.Run("missing embedding returns 400", func(t *testing.T) {
		w := callHandler(p.handleDatasourceVectorSearch, vars, VectorSearchQuery{})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "embedding vector is required")
	})

	t.Run("empty embedding array returns 400", func(t *testing.T) {
		w := callHandler(p.handleDatasourceVectorSearch, vars, VectorSearchQuery{Embedding: []float32{}})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "embedding vector is required")
	})

	t.Run("datasource not found returns 404", func(t *testing.T) {
		notFoundVars := map[string]string{"dsSlug": "nonexistent"}
		w := callHandler(p.handleDatasourceVectorSearch, notFoundVars, VectorSearchQuery{Embedding: []float32{0.1}})
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/datasource/test-ds", bytes.NewReader([]byte("not json")))
		req = mux.SetURLVars(req, vars)
		w := httptest.NewRecorder()
		p.handleDatasourceVectorSearch(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandleDatasourceMetadataQuery(t *testing.T) {
	ds := &models.Datasource{Name: "Test DS", Active: true, DBSourceType: "pinecone"}
	ds.ID = 1
	p := newTestProxyWithDatasource(t, ds)
	vars := map[string]string{"dsSlug": "test-ds"}

	t.Run("missing filter returns 400", func(t *testing.T) {
		w := callHandler(p.handleDatasourceMetadataQuery, vars, MetadataQuery{})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "filter is required")
	})

	t.Run("invalid filter_mode returns 400", func(t *testing.T) {
		w := callHandler(p.handleDatasourceMetadataQuery, vars, MetadataQuery{
			Filter:     map[string]string{"key": "val"},
			FilterMode: "INVALID",
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "filter_mode must be")
	})

	t.Run("limit exceeding 100 returns 400", func(t *testing.T) {
		w := callHandler(p.handleDatasourceMetadataQuery, vars, MetadataQuery{
			Filter: map[string]string{"key": "val"},
			Limit:  200,
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "limit must not exceed 100")
	})

	t.Run("valid filter_mode AND is accepted", func(t *testing.T) {
		w := callHandler(p.handleDatasourceMetadataQuery, vars, MetadataQuery{
			Filter:     map[string]string{"key": "val"},
			FilterMode: "AND",
		})
		// Will fail at the data session layer (no real vector store), but should NOT be 400
		assert.NotEqual(t, http.StatusBadRequest, w.Code)
	})

	t.Run("valid filter_mode OR is accepted", func(t *testing.T) {
		w := callHandler(p.handleDatasourceMetadataQuery, vars, MetadataQuery{
			Filter:     map[string]string{"key": "val"},
			FilterMode: "OR",
		})
		assert.NotEqual(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty filter_mode defaults without error", func(t *testing.T) {
		w := callHandler(p.handleDatasourceMetadataQuery, vars, MetadataQuery{
			Filter: map[string]string{"key": "val"},
		})
		// Empty string is allowed (data session layer defaults to "AND")
		assert.NotEqual(t, http.StatusBadRequest, w.Code)
	})

	t.Run("datasource not found returns 404", func(t *testing.T) {
		notFoundVars := map[string]string{"dsSlug": "nonexistent"}
		w := callHandler(p.handleDatasourceMetadataQuery, notFoundVars, MetadataQuery{
			Filter: map[string]string{"key": "val"},
		})
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("filter key too long returns 400", func(t *testing.T) {
		longKey := strings.Repeat("k", 257)
		w := callHandler(p.handleDatasourceMetadataQuery, vars, MetadataQuery{
			Filter: map[string]string{longKey: "val"},
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "filter keys must not exceed")
	})

	t.Run("filter value too long returns 400", func(t *testing.T) {
		longVal := strings.Repeat("v", 1025)
		w := callHandler(p.handleDatasourceMetadataQuery, vars, MetadataQuery{
			Filter: map[string]string{"key": longVal},
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "values must not exceed")
	})
}

func TestHandleDatasourceGenerateEmbedding(t *testing.T) {
	dsWithEmbedder := &models.Datasource{
		Name:        "Test DS",
		Active:      true,
		EmbedVendor: "openai",
		EmbedModel:  "text-embedding-3-small",
	}
	dsWithEmbedder.ID = 1

	dsWithoutEmbedder := &models.Datasource{
		Name:   "No Embedder DS",
		Active: true,
	}
	dsWithoutEmbedder.ID = 2

	t.Run("missing embedder returns 400", func(t *testing.T) {
		p := newTestProxyWithDatasource(t, dsWithoutEmbedder)
		vars := map[string]string{"dsSlug": "test-ds"}
		w := callHandler(p.handleDatasourceGenerateEmbedding, vars, EmbeddingRequest{Texts: []string{"hello"}})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "embedder configured")
	})

	t.Run("empty texts returns 400", func(t *testing.T) {
		p := newTestProxyWithDatasource(t, dsWithEmbedder)
		vars := map[string]string{"dsSlug": "test-ds"}
		w := callHandler(p.handleDatasourceGenerateEmbedding, vars, EmbeddingRequest{Texts: []string{}})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "texts array is required")
	})

	t.Run("exceeding 100 texts returns 400", func(t *testing.T) {
		p := newTestProxyWithDatasource(t, dsWithEmbedder)
		vars := map[string]string{"dsSlug": "test-ds"}
		texts := make([]string, 101)
		for i := range texts {
			texts[i] = "text"
		}
		w := callHandler(p.handleDatasourceGenerateEmbedding, vars, EmbeddingRequest{Texts: texts})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "must not exceed 100")
	})

	t.Run("datasource not found returns 404", func(t *testing.T) {
		p := newTestProxyWithDatasource(t, dsWithEmbedder)
		notFoundVars := map[string]string{"dsSlug": "nonexistent"}
		w := callHandler(p.handleDatasourceGenerateEmbedding, notFoundVars, EmbeddingRequest{Texts: []string{"hello"}})
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestDatasourceAccessControl(t *testing.T) {
	ds := &models.Datasource{Name: "Test DS", Active: true, DBSourceType: "pinecone"}
	ds.ID = 1

	t.Run("app without datasource access returns 403", func(t *testing.T) {
		p := newTestProxyWithDatasource(t, ds)
		vars := map[string]string{"dsSlug": "test-ds"}
		app := &models.App{
			Model:       gorm.Model{ID: 1},
			Name:        "Test App",
			Datasources: []models.Datasource{}, // no datasources
		}
		w := callHandlerWithApp(p.handleDatasourceVectorSearch, vars,
			VectorSearchQuery{Embedding: []float32{0.1}}, app)
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), "does not have access")
	})

	t.Run("app with matching datasource is allowed", func(t *testing.T) {
		p := newTestProxyWithDatasource(t, ds)
		vars := map[string]string{"dsSlug": "test-ds"}
		matchingDS := models.Datasource{}
		matchingDS.ID = 1 // matches ds.ID
		app := &models.App{
			Model: gorm.Model{ID: 1},
			Name:  "Test App",
			Datasources: []models.Datasource{matchingDS},
		}
		w := callHandlerWithApp(p.handleDatasourceVectorSearch, vars,
			VectorSearchQuery{Embedding: []float32{0.1}}, app)
		// Should pass access check (will fail at vector store layer, but not 403)
		assert.NotEqual(t, http.StatusForbidden, w.Code)
	})

	t.Run("no app in context allows access", func(t *testing.T) {
		p := newTestProxyWithDatasource(t, ds)
		vars := map[string]string{"dsSlug": "test-ds"}
		// callHandler does NOT set app in context
		w := callHandler(p.handleDatasourceVectorSearch, vars,
			VectorSearchQuery{Embedding: []float32{0.1}})
		// Should pass access check (no app = OAuth flow, allowed)
		assert.NotEqual(t, http.StatusForbidden, w.Code)
	})

	t.Run("access check applies to metadata endpoint", func(t *testing.T) {
		p := newTestProxyWithDatasource(t, ds)
		vars := map[string]string{"dsSlug": "test-ds"}
		app := &models.App{
			Model:       gorm.Model{ID: 1},
			Datasources: []models.Datasource{},
		}
		w := callHandlerWithApp(p.handleDatasourceMetadataQuery, vars,
			MetadataQuery{Filter: map[string]string{"k": "v"}}, app)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("access check applies to embeddings endpoint", func(t *testing.T) {
		dsEmbed := &models.Datasource{Name: "Test DS", Active: true, EmbedVendor: "openai", EmbedModel: "m"}
		dsEmbed.ID = 1
		p := newTestProxyWithDatasource(t, dsEmbed)
		vars := map[string]string{"dsSlug": "test-ds"}
		app := &models.App{
			Model:       gorm.Model{ID: 1},
			Datasources: []models.Datasource{},
		}
		w := callHandlerWithApp(p.handleDatasourceGenerateEmbedding, vars,
			EmbeddingRequest{Texts: []string{"hello"}}, app)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestDatasourceResponseFormat(t *testing.T) {
	t.Run("search results use backward-compatible field names", func(t *testing.T) {
		doc := DatasourceDocument{
			PageContent: "test content",
			Metadata:    map[string]any{"source": "file.pdf"},
			Score:       0.95,
		}
		result := SearchResults{Documents: []DatasourceDocument{doc}}
		data, err := json.Marshal(result)
		require.NoError(t, err)

		// Verify field names match original schema.Document marshaling
		var raw map[string]interface{}
		err = json.Unmarshal(data, &raw)
		require.NoError(t, err)

		docs := raw["documents"].([]interface{})
		require.Len(t, docs, 1)
		firstDoc := docs[0].(map[string]interface{})

		assert.Contains(t, firstDoc, "PageContent")
		assert.Contains(t, firstDoc, "Metadata")
		assert.Contains(t, firstDoc, "Score")
		assert.NotContains(t, firstDoc, "content")
		assert.NotContains(t, firstDoc, "similarity_score")

		assert.Equal(t, "test content", firstDoc["PageContent"])
		assert.InDelta(t, 0.95, firstDoc["Score"], 0.01)
	})

	t.Run("metadata results include total_count", func(t *testing.T) {
		result := MetadataResults{
			Documents:  []DatasourceDocument{{PageContent: "test"}},
			TotalCount: 42,
		}
		data, err := json.Marshal(result)
		require.NoError(t, err)

		var raw map[string]interface{}
		err = json.Unmarshal(data, &raw)
		require.NoError(t, err)

		assert.Equal(t, float64(42), raw["total_count"])
		assert.Len(t, raw["documents"], 1)
	})
}
