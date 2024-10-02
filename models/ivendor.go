package models

import (
	"context"
	"net/http"

	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/schema"
)

type LLMVendorProvider interface {
	GetTokenCounts(choice *llms.ContentChoice) (int, int, int)
	GetDriver(LLMConfig *LLM, settings *LLMSettings, mem schema.Memory, streamingFunc func(ctx context.Context, chunk []byte) error) (llms.Model, error)
	GetEmbedder(d *Datasource) (*embeddings.EmbedderImpl, error)
	AnalyzeResponse(llm *LLM, app *App, statusCode int, body []byte, r *http.Request) (*LLM, *App, ITokenResponse, error)
	AnalyzeStreamingResponse(llm *LLM, app *App, statusCode int, resps []byte, r *http.Request, chunks [][]byte) (*LLM, *App, ITokenResponse, error)
	ProxySetAuthHeader(r *http.Request, llm *LLM) error
	ProxyScreenRequest(llm *LLM, r *http.Request, isStreamingChannel bool) error

	ProvidesEmbedder() bool
}
