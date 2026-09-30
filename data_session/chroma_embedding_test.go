//go:build cgo

package data_session

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// fakeChroma is a minimal Chroma v2 HTTP server: one tenant and database,
// collections by name, and canned add/query responses. It records every
// request so tests can see what the client asked for.
type fakeChroma struct {
	mu          sync.Mutex
	collections map[string]string // name -> id
	requests    []string          // "METHOD path"
	addBodies   []map[string]any
	queryBodies []map[string]any
}

const fakeChromaCollectionsPath = "/api/v2/tenants/default_tenant/databases/default_database/collections"

func newFakeChroma(t *testing.T, existing ...string) (*fakeChroma, *httptest.Server) {
	t.Helper()
	f := &fakeChroma{collections: map[string]string{}}
	for _, name := range existing {
		f.collections[name] = "id-" + name
	}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeChroma) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	w.Header().Set("Content-Type", "application/json")

	writeCollection := func(name, id string) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": id, "name": name, "tenant": "default_tenant", "database": "default_database",
		})
	}
	decode := func() map[string]any {
		var body map[string]any
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		return body
	}

	path := r.URL.Path
	switch {
	case r.Method == http.MethodGet && path == "/api/v2/pre-flight-checks":
		_, _ = io.WriteString(w, `{"max_batch_size": 100}`)
	case r.Method == http.MethodPost && path == fakeChromaCollectionsPath:
		body := decode()
		name, _ := body["name"].(string)
		id, ok := f.collections[name]
		if !ok {
			id = "id-" + name
			f.collections[name] = id
		}
		writeCollection(name, id)
	case r.Method == http.MethodGet && strings.HasPrefix(path, fakeChromaCollectionsPath+"/"):
		name := strings.TrimPrefix(path, fakeChromaCollectionsPath+"/")
		id, ok := f.collections[name]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"NotFoundError","message":"Collection does not exist."}`)
			return
		}
		writeCollection(name, id)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/add"):
		f.addBodies = append(f.addBodies, decode())
		_, _ = io.WriteString(w, `{}`)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/query"):
		f.queryBodies = append(f.queryBodies, decode())
		_, _ = io.WriteString(w, `{"ids":[["doc-1"]],"documents":[["hello"]],"metadatas":[[{"filename":"a.txt"}]],"distances":[[0.25]]}`)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"unexpected request"}`)
	}
}

func (f *fakeChroma) snapshot() (requests []string, adds, queries []map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...), append([]map[string]any(nil), f.addBodies...), append([]map[string]any(nil), f.queryBodies...)
}

func (f *fakeChroma) created() bool {
	requests, _, _ := f.snapshot()
	for _, r := range requests {
		if r == "POST "+fakeChromaCollectionsPath {
			return true
		}
	}
	return false
}

// Storing and searching with precomputed vectors must not need chroma-go's
// default embedding function: building it downloads an ONNX runtime that the
// linked onnxruntime_go (v1.26, API 24) rejects, so a store or search that
// falls back to it fails (H1 in the post-2.2 sweep). With an existing
// collection the client must open it with a plain GET and never create it.
func TestChromaStoreAndSearchWithPrecomputedVectors_ExistingCollection(t *testing.T) {
	f, srv := newFakeChroma(t, "docs")
	ds := &DataSession{}
	d := &models.Datasource{DBConnString: srv.URL, DBName: "docs"}
	ctx := context.Background()

	err := ds.storeToChroma(ctx, nil, d,
		[]string{"hello", "world"},
		[][]float32{{0.1, 0.2, 0.3}, {0.4, 0.5, 0.6}},
		[]map[string]any{{"filename": "a.txt"}, {"filename": "b.txt"}})
	if err != nil {
		t.Fatalf("storeToChroma: %v", err)
	}

	docs, err := ds.searchChromaByVector(ctx, nil, d, []float32{0.1, 0.2, 0.3}, 3)
	if err != nil {
		t.Fatalf("searchChromaByVector: %v", err)
	}
	if len(docs) != 1 || docs[0].PageContent != "hello" || docs[0].Metadata["filename"] != "a.txt" {
		t.Fatalf("unexpected search result: %+v", docs)
	}

	requests, adds, queries := f.snapshot()
	getPath := "GET " + fakeChromaCollectionsPath + "/docs"
	gets := 0
	for _, r := range requests {
		if r == getPath {
			gets++
		}
	}
	if gets != 2 {
		t.Errorf("want the existing collection opened by GET twice (store, search), got %d; requests: %v", gets, requests)
	}
	if f.created() {
		t.Errorf("an existing collection must not be (re)created; requests: %v", requests)
	}
	if len(adds) != 2 {
		t.Fatalf("want 2 add requests, got %d", len(adds))
	}
	for i, body := range adds {
		if embs, _ := body["embeddings"].([]any); len(embs) != 1 {
			t.Errorf("add %d: want the precomputed embedding sent, got %v", i, body["embeddings"])
		}
	}
	if len(queries) != 1 {
		t.Fatalf("want 1 query request, got %d", len(queries))
	}
	if qe, _ := queries[0]["query_embeddings"].([]any); len(qe) != 1 {
		t.Errorf("want the query vector sent as query_embeddings, got %v", queries[0])
	}
}

// A missing collection is created with get_or_create, again without the
// default embedding function.
func TestChromaStoreWithPrecomputedVectors_CreatesMissingCollection(t *testing.T) {
	f, srv := newFakeChroma(t)
	ds := &DataSession{}
	d := &models.Datasource{DBConnString: srv.URL, DBName: "fresh"}

	if err := ds.storeToChroma(context.Background(), nil, d,
		[]string{"hello"}, [][]float32{{0.1, 0.2}}, nil); err != nil {
		t.Fatalf("storeToChroma: %v", err)
	}
	if !f.created() {
		requests, _, _ := f.snapshot()
		t.Fatalf("want the missing collection created; requests: %v", requests)
	}
	if _, adds, _ := f.snapshot(); len(adds) != 1 {
		t.Fatalf("want 1 add request, got %d", len(adds))
	}
}

// The embedding function Studio passes refuses to embed, so a code path that
// ever sends text without vectors fails loudly instead of storing vectors
// from a different model.
func TestPrecomputedEmbeddingsRefuseToEmbed(t *testing.T) {
	ef := precomputedEmbeddings{}
	if _, err := ef.EmbedDocuments(context.Background(), []string{"x"}); !errors.Is(err, errPrecomputedEmbeddings) {
		t.Fatalf("EmbedDocuments: want errPrecomputedEmbeddings, got %v", err)
	}
	if _, err := ef.EmbedQuery(context.Background(), "x"); !errors.Is(err, errPrecomputedEmbeddings) {
		t.Fatalf("EmbedQuery: want errPrecomputedEmbeddings, got %v", err)
	}
}
