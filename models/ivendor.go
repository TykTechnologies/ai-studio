package models

import (
	"context"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/embeddings"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/schema"
)

type LLMVendorProvider interface {
	GetTokenCounts(choice *llms.ContentChoice) (int, int, int)
	GetDriver(LLMConfig *LLM, settings *LLMSettings, mem schema.Memory, streamingFunc func(ctx context.Context, chunk []byte) error) (llms.Model, error)
	GetEmbedder(spec *EmbedderSpec) (*embeddings.EmbedderImpl, error)
	AnalyzeResponse(llm *LLM, app *App, statusCode int, body []byte, r *http.Request) (*LLM, *App, ITokenResponse, error)
	AnalyzeStreamingResponse(llm *LLM, app *App, statusCode int, resps []byte, r *http.Request, chunks [][]byte) (*LLM, *App, ITokenResponse, error)
	ProxySetAuthHeader(r *http.Request, llm *LLM) error
	ProxyScreenRequest(llm *LLM, r *http.Request, isStreamingChannel bool) error

	ProvidesEmbedder() bool
	// ProvidesChat reports whether the vendor serves generative chat (a
	// langchaingo driver and the chat-shaped /ai/ and /v1 bridges).
	// Decision-only vendors return false.
	ProvidesChat() bool
	// DecisionEndpoint is the upstream path of the vendor's decision-model API
	// (e.g. /v1/decisions), or "" when the vendor serves no decision models.
	DecisionEndpoint() string
}
