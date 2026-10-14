package openai

import (
	"errors"

	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/httputil"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms/openai/internal/openaiclient"
)

var (
	ErrEmptyResponse              = errors.New("no response")
	ErrMissingToken               = errors.New("missing the OpenAI API key")
	ErrMissingAzureModel          = errors.New("model needs to be provided when using Azure API")
	ErrMissingAzureEmbeddingModel = errors.New("embeddings model needs to be provided when using Azure API")

	ErrUnexpectedResponseLength = errors.New("unexpected length of response")
)

// newClient creates an instance of the internal client.
//
// Tyk: nothing is read from the process environment (upstream defaults the
// token, model, base URL and organization from OPENAI_* variables). Callers
// pass every value explicitly; an empty key filled from OPENAI_API_KEY sent
// the host's key to the endpoint an LLM's writer chose.
func newClient(opts ...Option) (*options, *openaiclient.Client, error) {
	options := &options{
		apiType:    APIType(openaiclient.APITypeOpenAI),
		httpClient: httputil.DefaultClient,
	}

	for _, opt := range opts {
		opt(options)
	}
	// set of options needed for Azure client
	if openaiclient.IsAzure(openaiclient.APIType(options.apiType)) && options.apiVersion == "" {
		options.apiVersion = DefaultAPIVersion
		if options.model == "" {
			return options, nil, ErrMissingAzureModel
		}
		if options.embeddingModel == "" {
			return options, nil, ErrMissingAzureEmbeddingModel
		}
	}

	if len(options.token) == 0 {
		return options, nil, ErrMissingToken
	}

	var clientOptions []openaiclient.Option
	if options.embeddingDimensions != 0 {
		clientOptions = append(clientOptions, openaiclient.WithEmbeddingDimensions(options.embeddingDimensions))
	}
	cli, err := openaiclient.New(options.token, options.model, options.baseURL, options.organization,
		openaiclient.APIType(options.apiType), options.apiVersion, options.httpClient, options.embeddingModel,
		options.responseFormat, clientOptions...,
	)
	return options, cli, err
}
