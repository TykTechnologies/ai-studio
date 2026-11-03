// Package llmclient reaches LLMs for the Semantic Router engine (embeddings
// for the example and request vectors, and a completion for its judge) and
// for the guardrail second opinion (Generate). It
// builds on the same vendor drivers the gateway uses, so any LLM configured
// in AI Studio whose vendor offers embeddings (OpenAI, Ollama, Google AI,
// Vertex, Hugging Face) can embed, and any LLM can judge.
//
// These calls go straight to the vendor. They are the router's (or the
// guardrail's) own traffic, not the App's request, so they do not pass
// through the App's gateway filters or budget, and cannot re-enter them.
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
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/embeddings"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

// LLMLookup returns the LLM with the given id. On the hub it reads the store,
// on the edge the gateway's loaded configuration.
type LLMLookup func(id uint) (*models.LLM, error)

// ErrNoEmbeddings is returned for an LLM whose vendor offers no embeddings.
var ErrNoEmbeddings = errors.New("this LLM's vendor does not provide embeddings")

// ErrNoChat is returned for an LLM whose vendor serves no chat completions
// (a decision-only vendor).
var ErrNoChat = errors.New("this LLM's vendor does not provide chat completions")

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
	drivers   map[string]llms.Model
}

// Option configures Clients.
type Option func(*Clients)

// WithDecrypter sets how inline embedder keys are decrypted.
func WithDecrypter(d Decrypter) Option { return func(c *Clients) { c.decrypt = d } }

// New returns clients that find LLMs with lookup.
func New(lookup LLMLookup, opts ...Option) *Clients {
	c := &Clients{lookup: lookup, embedders: map[string]*embeddings.EmbedderImpl{}, drivers: map[string]llms.Model{}}
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

// maxCachedDrivers bounds the chat driver cache, keyed the same way.
const maxCachedDrivers = 64

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
	key := fmt.Sprintf("%d|%d|%s|%s|%s|%t", llm.ID, llm.UpdatedAt.UnixNano(), llm.APIEndpoint, model, llm.APIKey, llm.AllowInternalHost)
	return c.cached(key, &models.EmbedderSpec{
		Vendor:            llm.Vendor,
		Endpoint:          llm.APIEndpoint,
		APIKey:            llm.APIKey,
		Model:             model,
		AllowInternalHost: llm.AllowInternalHost,
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
	key := fmt.Sprintf("e|%d|%s|%s|%s|%s|%t", ref.EmbedderID, ref.Vendor, ref.Endpoint, ref.Model, apiKey, ref.AllowInternalHost)
	return c.cached(key, &models.EmbedderSpec{
		EmbedderID: ref.EmbedderID,
		Vendor:     models.Vendor(ref.Vendor),
		Endpoint:   ref.Endpoint,
		APIKey:     apiKey,
		Model:      ref.Model,

		AllowInternalHost: ref.AllowInternalHost,
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

// Complete asks the LLM for a short answer. No temperature is sent: current
// Anthropic models (Claude Sonnet 5 and later) reject any sampling
// parameter with a 400, so a judge on one of them failed every judgement.
func (c *Clients) Complete(ctx context.Context, ref sr.ModelRef, system, user string) (string, error) {
	out, err := c.Generate(ctx, ref, system, user, judgeMaxTokens)
	if err != nil {
		return "", err
	}
	return out.Text, nil
}

// Completion is an answer from Generate with what it cost.
type Completion struct {
	Text string
	// Model is the model that was asked: the reference's, or the LLM's
	// default model when the reference names none.
	Model            string
	PromptTokens     int
	CompletionTokens int
}

// Generate asks the LLM for an answer of at most maxTokens tokens (0 leaves
// it to the vendor), with a system and a user message. Like Complete it sends
// no sampling parameters. An empty ref.Model asks the LLM's default model.
// The vendor's driver is reused across calls while the LLM is unchanged.
func (c *Clients) Generate(ctx context.Context, ref sr.ModelRef, system, user string, maxTokens int) (Completion, error) {
	llm, err := c.resolved(ref.LLMID)
	if err != nil {
		return Completion{}, err
	}
	return c.generate(ctx, llm, ref.Model, system, user, maxTokens)
}

// GenerateFor is Generate for an LLM the caller has already looked up (the
// guardrail judge, which finds it through the runner's service: the store on
// the hub, the loaded configuration on an edge). Its secret references are
// resolved here, as Generate does.
func (c *Clients) GenerateFor(ctx context.Context, llm *models.LLM, model, system, user string, maxTokens int) (Completion, error) {
	if llm == nil {
		return Completion{}, errors.New("no LLM configured")
	}
	cp := *llm
	cp.APIKey = secrets.GetValue(cp.APIKey, false)
	cp.APIEndpoint = secrets.GetValue(cp.APIEndpoint, false)
	return c.generate(ctx, &cp, model, system, user, maxTokens)
}

func (c *Clients) generate(ctx context.Context, llm *models.LLM, model, system, user string, maxTokens int) (Completion, error) {
	driver, err := c.driver(llm)
	if err != nil {
		return Completion{}, err
	}
	if model == "" {
		model = llm.DefaultModel
	}
	opts := []llms.CallOption{llms.WithModel(model)}
	if maxTokens > 0 {
		opts = append(opts, llms.WithMaxTokens(maxTokens))
	}
	resp, err := driver.GenerateContent(ctx, []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, system),
		llms.TextParts(llms.ChatMessageTypeHuman, user),
	}, opts...)
	if err != nil {
		return Completion{}, err
	}
	if resp == nil || len(resp.Choices) == 0 || resp.Choices[0] == nil {
		return Completion{}, errors.New("the LLM returned no answer")
	}
	_, prompt, completion := switches.GetTokenCounts(resp.Choices[0], llm.Vendor)
	return Completion{
		Text:             strings.TrimSpace(resp.Choices[0].Content),
		Model:            model,
		PromptTokens:     prompt,
		CompletionTokens: completion,
	}, nil
}

// driver returns the chat driver for llm, building it on a miss. The key
// holds everything the driver is built from, so an edited LLM (or a rotated
// key) gets a new one.
func (c *Clients) driver(llm *models.LLM) (llms.Model, error) {
	if !switches.SupportsChat(llm.Vendor) {
		return nil, ErrNoChat
	}
	key := fmt.Sprintf("%d|%d|%s|%s|%s|%t", llm.ID, llm.UpdatedAt.UnixNano(), llm.Vendor, llm.APIEndpoint, llm.APIKey, llm.AllowInternalHost)
	c.mu.Lock()
	if d, ok := c.drivers[key]; ok {
		c.mu.Unlock()
		return d, nil
	}
	c.mu.Unlock()

	d, err := switches.FetchDriver(llm, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if len(c.drivers) >= maxCachedDrivers {
		c.drivers = map[string]llms.Model{}
	}
	c.drivers[key] = d
	c.mu.Unlock()
	return d, nil
}
