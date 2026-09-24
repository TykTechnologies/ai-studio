//go:build enterprise
// +build enterprise

package services

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/semantic_router/engine"
	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// embeddingStub answers /v1/embeddings like OpenAI: texts about maths point
// one way, everything else the other. It records the Authorization headers.
func embeddingStub(t *testing.T) (*httptest.Server, func() []string) {
	var mu sync.Mutex
	var auth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auth = append(auth, r.Header.Get("Authorization"))
		mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.Unmarshal(body, &req)
		data := []map[string]interface{}{}
		for i, in := range req.Input {
			vec := []float32{0, 1}
			if strings.Contains(in, "formula") || strings.Contains(in, "integral") {
				vec = []float32{1, 0}
			}
			data = append(data, map[string]interface{}{"object": "embedding", "index": i, "embedding": vec})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"object": "list", "data": data, "model": "e",
			"usage": map[string]int{"prompt_tokens": 1, "total_tokens": 1}})
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), auth...) }
}

// inlineRouterConfig is what the hub sends for a router whose embedder is
// standalone: the embedder inline, its key encrypted.
func inlineRouterConfig(endpoint string) string {
	cfg := sr.Config{Settings: sr.Settings{DefaultRoute: "simple", Embedding: &sr.ModelRef{
		Model: "e", EmbedderID: 7, Vendor: "openai", Endpoint: endpoint, APIKeyEncrypted: "enc:sk-edge",
	}}, Routes: []sr.Route{
		{Name: "simple", Utterances: []string{"hello there"}, Target: sr.Target{Type: sr.TargetLLM, LLMID: 1, Model: "small"}},
		{Name: "maths", Utterances: []string{"derive the formula"}, Target: sr.Target{Type: sr.TargetLLM, LLMID: 1, Model: "big"}},
	}}
	b, _ := json.Marshal(cfg)
	return string(b)
}

func loadInlineRouter(t *testing.T, endpoint string, decrypt func(string) (string, error)) *SemanticRouterService {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&database.SemanticRouter{}))
	require.NoError(t, db.Create(&database.SemanticRouter{ID: 1, Name: "Smart", Slug: "smart", IsActive: true,
		ConfigJSON: inlineRouterConfig(endpoint)}).Error)
	svc := NewSemanticRouterService(db)
	if decrypt != nil {
		svc.SetDecrypter(decrypt)
	}
	require.NoError(t, svc.LoadRouters(""))
	return svc
}

func classify(svc *SemanticRouterService, text string) sr.Decision {
	r, _ := svc.GetRouter("smart")
	return svc.Classify(context.Background(), r, sr.Request{Model: sr.ReservedAutoModel,
		Messages: []sr.Message{{Role: "user", Content: text}}})
}

// The edge decrypts a standalone embedder's key and classifies by meaning.
func TestSemanticRouter_InlineEmbedderOnTheEdge(t *testing.T) {
	srv, auth := embeddingStub(t)
	svc := loadInlineRouter(t, srv.URL+"/v1", func(c string) (string, error) { return strings.TrimPrefix(c, "enc:"), nil })
	require.Equal(t, 1, svc.GetRouterCount())

	// Examples embed in the background; until then the stage is skipped.
	require.Eventually(t, func() bool {
		return classify(svc, "solve this integral").Reason == sr.ReasonEmbedding
	}, 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, "maths", classify(svc, "solve this integral").Route)
	assert.Contains(t, auth(), "Bearer sk-edge", "the decrypted key reaches the embedder")
}

// Without the gateway's crypto the key cannot be used: the router still
// answers, with its default route.
func TestSemanticRouter_InlineEmbedderWithoutDecrypter(t *testing.T) {
	srv, auth := embeddingStub(t)
	svc := loadInlineRouter(t, srv.URL+"/v1", nil)
	d := classify(svc, "solve this integral")
	assert.Equal(t, "simple", d.Route)
	assert.NotEqual(t, sr.ReasonEmbedding, d.Reason)
	for _, a := range auth() {
		assert.NotContains(t, a, "enc:", "the ciphertext is never sent as a key")
	}
}
