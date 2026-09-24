package llmclient

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
	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openAIStub serves /embeddings and /chat/completions like OpenAI does.
type openAIStub struct {
	mu       sync.Mutex
	requests []map[string]interface{}
	auth     []string
}

func (s *openAIStub) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req map[string]interface{}
	_ = json.Unmarshal(body, &req)
	s.mu.Lock()
	s.requests = append(s.requests, req)
	s.auth = append(s.auth, r.Header.Get("Authorization"))
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasSuffix(r.URL.Path, "/embeddings"):
		inputs, _ := req["input"].([]interface{})
		data := []map[string]interface{}{}
		for i := range inputs {
			data = append(data, map[string]interface{}{"object": "embedding", "index": i, "embedding": []float32{float32(i + 1), 0.5}})
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"object": "list", "data": data, "model": req["model"],
			"usage": map[string]int{"prompt_tokens": 1, "total_tokens": 1}})
	case strings.HasSuffix(r.URL.Path, "/chat/completions"):
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "c", "object": "chat.completion", "created": 1, "model": req["model"],
			"choices": []map[string]interface{}{{"index": 0, "finish_reason": "stop",
				"message": map[string]string{"role": "assistant", "content": ` {"route": "complex"} `}}},
			"usage": map[string]int{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	default:
		http.NotFound(w, r)
	}
}

func TestClients_EmbedAndComplete(t *testing.T) {
	stub := &openAIStub{}
	srv := httptest.NewServer(http.HandlerFunc(stub.handler))
	defer srv.Close()

	llms := map[uint]*models.LLM{
		1: {ID: 1, Name: "Embedder", Vendor: models.OPENAI, APIEndpoint: srv.URL + "/v1", APIKey: "sk-embed"},
		2: {ID: 2, Name: "Claude", Vendor: models.ANTHROPIC, APIEndpoint: srv.URL, APIKey: "sk-ant"},
	}
	c := New(func(id uint) (*models.LLM, error) {
		if l, ok := llms[id]; ok {
			return l, nil
		}
		return nil, errors.New("not found")
	})

	vecs, err := c.Embed(context.Background(), sr.ModelRef{LLMID: 1, Model: "text-embedding-3-small"}, []string{"a", "b"})
	require.NoError(t, err)
	require.Len(t, vecs, 2)
	assert.Equal(t, []float32{2, 0.5}, vecs[1])
	assert.Equal(t, "text-embedding-3-small", stub.requests[0]["model"])
	assert.Equal(t, "Bearer sk-embed", stub.auth[0])

	answer, err := c.Complete(context.Background(), sr.ModelRef{LLMID: 1, Model: "gpt-4o-mini"}, "sys", "user text")
	require.NoError(t, err)
	assert.Equal(t, `{"route": "complex"}`, answer)
	last := stub.requests[len(stub.requests)-1]
	assert.Equal(t, "gpt-4o-mini", last["model"])
	assert.EqualValues(t, 0, last["temperature"])

	_, err = c.Embed(context.Background(), sr.ModelRef{LLMID: 2, Model: "x"}, []string{"a"})
	assert.ErrorIs(t, err, ErrNoEmbeddings, "Anthropic offers no embeddings")
	_, err = c.Embed(context.Background(), sr.ModelRef{LLMID: 9, Model: "x"}, []string{"a"})
	assert.Error(t, err)
	_, err = c.Complete(context.Background(), sr.ModelRef{LLMID: 0}, "s", "u")
	assert.Error(t, err)
}

func TestSupportsEmbeddings(t *testing.T) {
	assert.True(t, SupportsEmbeddings(models.OPENAI))
	assert.True(t, SupportsEmbeddings(models.OLLAMA))
	assert.False(t, SupportsEmbeddings(models.ANTHROPIC))
	assert.False(t, SupportsEmbeddings(models.BEDROCK))
}

// A standalone embedder travels inline in the reference: on the hub with its
// key resolved, on edges encrypted and decrypted by the injected Decrypter.
func TestClients_InlineEmbedder(t *testing.T) {
	stub := &openAIStub{}
	srv := httptest.NewServer(http.HandlerFunc(stub.handler))
	defer srv.Close()
	noLLMs := func(id uint) (*models.LLM, error) { return nil, errors.New("no LLMs here") }

	hubRef := sr.ModelRef{EmbedderID: 4, Vendor: "openai", Endpoint: srv.URL + "/v1", APIKey: "sk-hub", Model: "m1"}
	require.True(t, hubRef.Inline())
	_, err := New(noLLMs).Embed(context.Background(), hubRef, []string{"a"})
	require.NoError(t, err)
	assert.Equal(t, "Bearer sk-hub", stub.auth[len(stub.auth)-1])

	edgeRef := sr.ModelRef{EmbedderID: 4, Vendor: "openai", Endpoint: srv.URL + "/v1", APIKeyEncrypted: "enc(sk-edge)", Model: "m1"}
	_, err = New(noLLMs).Embed(context.Background(), edgeRef, []string{"a"})
	assert.ErrorContains(t, err, "no decrypter", "an encrypted key needs a decrypter")

	decrypt := func(s string) (string, error) { return strings.TrimSuffix(strings.TrimPrefix(s, "enc("), ")"), nil }
	edge := New(noLLMs, WithDecrypter(decrypt))
	_, err = edge.Embed(context.Background(), edgeRef, []string{"a"})
	require.NoError(t, err)
	assert.Equal(t, "Bearer sk-edge", stub.auth[len(stub.auth)-1])
	assert.Equal(t, "m1", stub.requests[len(stub.requests)-1]["model"])

	// A changed key is a new client, not a stale cached one.
	rotated := edgeRef
	rotated.APIKeyEncrypted = "enc(sk-rotated)"
	_, err = edge.Embed(context.Background(), rotated, []string{"a"})
	require.NoError(t, err)
	assert.Equal(t, "Bearer sk-rotated", stub.auth[len(stub.auth)-1])

	_, err = edge.Embed(context.Background(), sr.ModelRef{Vendor: "anthropic", Model: "x"}, []string{"a"})
	assert.ErrorIs(t, err, ErrNoEmbeddings)

	failing := New(noLLMs, WithDecrypter(func(string) (string, error) { return "", errors.New("bad key") }))
	_, err = failing.Embed(context.Background(), edgeRef, []string{"a"})
	assert.ErrorContains(t, err, "decrypt embedder key")
}
