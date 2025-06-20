package proxy

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/tmc/langchaingo/llms"
)

// testCallOptions is a helper struct to capture the options applied by llms.CallOption functions
type testCallOptions struct {
	model            string
	maxTokens        int
	temperature      float64
	topP             float64
	n                int
	seed             int
	frequencyPenalty float64
	presencePenalty  float64
	stopWords        []string
	jsonMode         bool
	metadata         map[string]any
	tools            []llms.Tool
	toolChoice       any
}

// Implement the necessary methods to satisfy the llms option interfaces
func (t *testCallOptions) SetModel(model string) {
	t.model = model
}

func (t *testCallOptions) SetMaxTokens(maxTokens int) {
	t.maxTokens = maxTokens
}

func (t *testCallOptions) SetTemperature(temperature float64) {
	t.temperature = temperature
}

func (t *testCallOptions) SetTopP(topP float64) {
	t.topP = topP
}

func (t *testCallOptions) SetN(n int) {
	t.n = n
}

func (t *testCallOptions) SetSeed(seed int) {
	t.seed = seed
}

func (t *testCallOptions) SetFrequencyPenalty(penalty float64) {
	t.frequencyPenalty = penalty
}

func (t *testCallOptions) SetPresencePenalty(penalty float64) {
	t.presencePenalty = penalty
}

func (t *testCallOptions) SetStopWords(stopWords []string) {
	t.stopWords = stopWords
}

func (t *testCallOptions) SetJSONMode() {
	t.jsonMode = true
}

func (t *testCallOptions) SetMetadata(metadata map[string]any) {
	t.metadata = metadata
}

func (t *testCallOptions) SetTools(tools []llms.Tool) {
	t.tools = tools
}

func (t *testCallOptions) SetToolChoice(toolChoice any) {
	t.toolChoice = toolChoice
}

func TestChatCompletionRequest_ToLangchainOptions_ModelSelection(t *testing.T) {
	tests := []struct {
		name           string
		vendor         string
		defaultModel   string
		requestModel   string
		expectedModel  string
		description    string
	}{
		{
			name:          "OpenAI vendor with request model specified",
			vendor:        models.OPENAI,
			defaultModel:  "gpt-4",
			requestModel:  "gpt-3.5-turbo",
			expectedModel: "gpt-3.5-turbo",
			description:   "Should use request model when specified for OpenAI",
		},
		{
			name:          "OpenAI vendor with no request model",
			vendor:        models.OPENAI,
			defaultModel:  "gpt-4",
			requestModel:  "",
			expectedModel: "gpt-4",
			description:   "Should use default model when no request model for OpenAI",
		},
		{
			name:          "OpenAI vendor with no request model and no default",
			vendor:        models.OPENAI,
			defaultModel:  "",
			requestModel:  "",
			expectedModel: "gpt-3.5-turbo",
			description:   "Should fallback to gpt-3.5-turbo when no models specified for OpenAI",
		},
		{
			name:          "Non-OpenAI vendor with request model specified",
			vendor:        "anthropic",
			defaultModel:  "claude-3-opus",
			requestModel:  "gpt-4",
			expectedModel: "claude-3-opus",
			description:   "Should use default model ignoring request model for non-OpenAI",
		},
		{
			name:          "Non-OpenAI vendor with no request model",
			vendor:        "anthropic",
			defaultModel:  "claude-3-opus",
			requestModel:  "",
			expectedModel: "claude-3-opus",
			description:   "Should use default model for non-OpenAI",
		},
		{
			name:          "Non-OpenAI vendor with empty default model",
			vendor:        "anthropic",
			defaultModel:  "",
			requestModel:  "gpt-4",
			expectedModel: "",
			description:   "Should not set model when default is empty for non-OpenAI",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create LLM configuration
			conf := &models.LLM{
				Vendor:       tt.vendor,
				DefaultModel: tt.defaultModel,
			}

			// Create request
			req := &ChatCompletionRequest{
				Model: tt.requestModel,
			}

			// Get options
			options := req.ToLangchainOptions(conf)

			// Apply all options to our test struct
			testOpts := &testCallOptions{}
			for _, opt := range options {
				opt(testOpts)
			}

			if tt.expectedModel == "" {
				if testOpts.model != "" {
					t.Errorf("%s: expected no model to be set, but got %q", tt.description, testOpts.model)
				}
			} else {
				if testOpts.model != tt.expectedModel {
					t.Errorf("%s: expected model %q, got %q", tt.description, tt.expectedModel, testOpts.model)
				}
			}
		})
	}
}
