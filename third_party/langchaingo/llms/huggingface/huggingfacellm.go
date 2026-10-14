package huggingface

import (
	"context"
	"errors"

	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/callbacks"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms/huggingface/internal/huggingfaceclient"
)

var (
	ErrEmptyResponse            = errors.New("empty response")
	ErrMissingToken             = errors.New("missing the Hugging Face API token")
	ErrUnexpectedResponseLength = errors.New("unexpected length of response")
)

type LLM struct {
	CallbacksHandler callbacks.Handler
	client           *huggingfaceclient.Client
}

var _ llms.Model = (*LLM)(nil)

// Call implements the LLM interface.
func (o *LLM) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	return llms.GenerateFromSinglePrompt(ctx, o, prompt, options...)
}

// GenerateContent implements the Model interface.
func (o *LLM) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) { //nolint: lll, cyclop, whitespace

	if o.CallbacksHandler != nil {
		o.CallbacksHandler.HandleLLMGenerateContentStart(ctx, messages)
	}

	opts := &llms.CallOptions{Model: defaultModel}
	for _, opt := range options {
		opt(opts)
	}

	// Assume we get a single text message
	msg0 := messages[0]
	part := msg0.Parts[0]
	result, err := o.client.RunInference(ctx, &huggingfaceclient.InferenceRequest{
		Model:             o.client.Model,
		Prompt:            part.(llms.TextContent).Text,
		Task:              huggingfaceclient.InferenceTaskTextGeneration,
		Temperature:       opts.Temperature,
		TopP:              opts.TopP,
		TopK:              opts.TopK,
		MinLength:         opts.MinLength,
		MaxLength:         opts.MaxLength,
		RepetitionPenalty: opts.RepetitionPenalty,
		Seed:              opts.Seed,
	})
	if err != nil {
		if o.CallbacksHandler != nil {
			o.CallbacksHandler.HandleLLMError(ctx, err)
		}
		return nil, err
	}

	resp := &llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{
				Content: result.Text,
			},
		},
	}
	return resp, nil
}

func New(opts ...Option) (*LLM, error) {
	options := &options{
		// Tyk: no default token; upstream reads HF_TOKEN,
		// HUGGINGFACEHUB_API_TOKEN and token files under HF_TOKEN_PATH,
		// HF_HOME, XDG_CACHE_HOME and ~/.cache/huggingface. Callers pass it.
		model: defaultModel,
		url:   defaultURL,
	}

	for _, opt := range opts {
		opt(options)
	}

	if len(options.token) == 0 {
		return nil, ErrMissingToken
	}

	// If a provider is specified, use the router URL
	if options.provider != "" {
		options.url = routerURL
	}

	var clientOpts []huggingfaceclient.Option
	if options.httpClient != nil {
		clientOpts = append(clientOpts, huggingfaceclient.WithHTTPClient(options.httpClient))
	}
	if options.provider != "" {
		clientOpts = append(clientOpts, huggingfaceclient.WithProvider(options.provider))
	}

	c, err := huggingfaceclient.New(options.token, options.model, options.url, clientOpts...)
	if err != nil {
		return nil, err
	}

	return &LLM{
		client: c,
	}, nil
}

// CreateEmbedding creates embeddings for the given input texts.
func (o *LLM) CreateEmbedding(
	ctx context.Context,
	inputTexts []string,
	model string,
	task string,
) ([][]float32, error) {
	embeddings, err := o.client.CreateEmbedding(ctx, model, task, &huggingfaceclient.EmbeddingRequest{
		Inputs: inputTexts,
		Options: map[string]any{
			"use_gpu":        false,
			"wait_for_model": true,
		},
	})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, ErrEmptyResponse
	}
	if len(inputTexts) != len(embeddings) {
		return embeddings, ErrUnexpectedResponseLength
	}
	return embeddings, nil
}
