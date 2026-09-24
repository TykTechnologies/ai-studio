// Package llmclient reaches LLMs for the Semantic Router engine: embeddings
// for the example and request vectors, and a completion for the judge. It
// builds on the same vendor drivers the gateway uses, so any LLM configured
// in AI Studio whose vendor offers embeddings (OpenAI, Ollama, Google AI,
// Vertex, Hugging Face) can embed, and any LLM can judge.
//
// These calls go straight to the vendor. They are the router's own traffic,
// not the App's request, so they do not pass through the App's gateway
// filters or budget.
package llmclient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/models"
	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
)

// LLMLookup returns the LLM with the given id. On the hub it reads the store,
// on the edge the gateway's loaded configuration.
type LLMLookup func(id uint) (*models.LLM, error)

// ErrNoEmbeddings is returned for an LLM whose vendor offers no embeddings.
var ErrNoEmbeddings = errors.New("this LLM's vendor does not provide embeddings")

// judgeMaxTokens bounds the judge's answer: it only names a route.
const judgeMaxTokens = 64

// Decrypter turns an inline embedder's APIKeyEncrypted back into the key.
// Edges set it (the hub encrypts the key for transit); on the hub the key
// arrives resolved in ModelRef.APIKey instead.
type Decrypter func(ciphertext string) (string, error)

// Clients implements semanticrouting.Embedder and Completer.
type Clients struct {
	lookup  LLMLookup
	decrypt Decrypter

	mu        sync.Mutex
	embedders map[string]*embeddings.EmbedderImpl
}

// Option configures Clients.
type Option func(*Clients)

// WithDecrypter sets how inline embedder keys are decrypted.
func WithDecrypter(d Decrypter) Option { return func(c *Clients) { c.decrypt = d } }

// New returns clients that find LLMs with lookup.
func New(lookup LLMLookup, opts ...Option) *Clients {
	c := &Clients{lookup: lookup, embedders: map[string]*embeddings.EmbedderImpl{}}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Deps is these clients as the engine's dependencies.
func (c *Clients) Deps() sr.Deps { return sr.Deps{Embedder: c, Completer: c} }

// maxCachedEmbedders bounds the embedder cache; an entry is keyed by the
// LLM's configuration, so an edited LLM gets a new one.
const maxCachedEmbedders = 64

// resolved is the LLM with its secret references resolved.
func (c *Clients) resolved(id uint) (*models.LLM, error) {
	if id == 0 {
		return nil, errors.New("no LLM configured")
	}
	llm, err := c.lookup(id)
	if err != nil {
		return nil, fmt.Errorf("LLM %d: %w", id, err)
	}
	if llm == nil {
		return nil, fmt.Errorf("LLM %d not found", id)
	}
	cp := *llm
	cp.APIKey = secrets.GetValue(cp.APIKey, false)
	cp.APIEndpoint = secrets.GetValue(cp.APIEndpoint, false)
	return &cp, nil
}

// SupportsEmbeddings reports whether the vendor offers embeddings.
func SupportsEmbeddings(vendor models.Vendor) bool {
	return switches.SupportsEmbeddings(vendor)
}

func (c *Clients) embedder(llm *models.LLM, model string) (*embeddings.EmbedderImpl, error) {
	if !SupportsEmbeddings(llm.Vendor) {
		return nil, ErrNoEmbeddings
	}
	key := fmt.Sprintf("%d|%d|%s|%s|%s", llm.ID, llm.UpdatedAt.UnixNano(), llm.APIEndpoint, model, llm.APIKey)
	return c.cached(key, &models.EmbedderSpec{
		Vendor:   llm.Vendor,
		Endpoint: llm.APIEndpoint,
		APIKey:   llm.APIKey,
		Model:    model,
	})
}

// inlineEmbedder is the client for a standalone embedder carried in the
// reference, keyed by its whole connection so an edit gets a new client.
func (c *Clients) inlineEmbedder(ref sr.ModelRef) (*embeddings.EmbedderImpl, error) {
	if !SupportsEmbeddings(models.Vendor(ref.Vendor)) {
		return nil, ErrNoEmbeddings
	}
	apiKey := ref.APIKey
	if apiKey == "" && ref.APIKeyEncrypted != "" {
		if c.decrypt == nil {
			return nil, errors.New("embedder key is encrypted and no decrypter is configured")
		}
		k, err := c.decrypt(ref.APIKeyEncrypted)
		if err != nil {
			return nil, fmt.Errorf("decrypt embedder key: %w", err)
		}
		apiKey = k
	}
	key := fmt.Sprintf("e|%d|%s|%s|%s|%s", ref.EmbedderID, ref.Vendor, ref.Endpoint, ref.Model, apiKey)
	return c.cached(key, &models.EmbedderSpec{
		EmbedderID: ref.EmbedderID,
		Vendor:     models.Vendor(ref.Vendor),
		Endpoint:   ref.Endpoint,
		APIKey:     apiKey,
		Model:      ref.Model,
	})
}

// cached returns the client for key, building it from spec on a miss.
func (c *Clients) cached(key string, spec *models.EmbedderSpec) (*embeddings.EmbedderImpl, error) {
	c.mu.Lock()
	if e, ok := c.embedders[key]; ok {
		c.mu.Unlock()
		return e, nil
	}
	c.mu.Unlock()

	e, err := switches.GetEmbedder(spec)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if len(c.embedders) >= maxCachedEmbedders {
		c.embedders = map[string]*embeddings.EmbedderImpl{}
	}
	c.embedders[key] = e
	c.mu.Unlock()
	return e, nil
}

// Embed embeds texts with the LLM's embedding model, or with the inline
// embedder the reference carries.
func (c *Clients) Embed(ctx context.Context, ref sr.ModelRef, texts []string) ([][]float32, error) {
	if ref.Inline() {
		e, err := c.inlineEmbedder(ref)
		if err != nil {
			return nil, err
		}
		return e.EmbedDocuments(ctx, texts)
	}
	llm, err := c.resolved(ref.LLMID)
	if err != nil {
		return nil, err
	}
	e, err := c.embedder(llm, ref.Model)
	if err != nil {
		return nil, err
	}
	return e.EmbedDocuments(ctx, texts)
}

// Complete asks the LLM for a short, deterministic answer.
func (c *Clients) Complete(ctx context.Context, ref sr.ModelRef, system, user string) (string, error) {
	llm, err := c.resolved(ref.LLMID)
	if err != nil {
		return "", err
	}
	driver, err := switches.FetchDriver(llm, nil, nil, nil)
	if err != nil {
		return "", err
	}
	resp, err := driver.GenerateContent(ctx, []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, system),
		llms.TextParts(llms.ChatMessageTypeHuman, user),
	}, llms.WithModel(ref.Model), llms.WithTemperature(0), llms.WithMaxTokens(judgeMaxTokens))
	if err != nil {
		return "", err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", errors.New("the judge returned no answer")
	}
	return strings.TrimSpace(resp.Choices[0].Content), nil
}
