package switches

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/llms/googleai/vertex"
	"github.com/tmc/langchaingo/llms/huggingface"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"
)

func setupOpenAIDriver(connDef *models.LLM, llmSettings *models.LLMSettings) (llms.Model, error) {
	var opts = make([]openai.Option, 0)
	if connDef.APIEndpoint != "" {
		opts = append(opts, openai.WithBaseURL(connDef.APIEndpoint))
	}

	if connDef.APIKey != "" {
		opts = append(opts, openai.WithToken(connDef.APIKey))
	}

	opts = append(opts, openai.WithModel(llmSettings.ModelName))

	llm, err := openai.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI driver: %v", err)
	}

	return llm, nil
}

func setupAnthropicDriver(connDef *models.LLM, llmSettings *models.LLMSettings) (llms.Model, error) {
	var opts = make([]anthropic.Option, 0)
	if connDef.APIEndpoint != "" {
		opts = append(opts, anthropic.WithBaseURL(connDef.APIEndpoint))
	}

	if connDef.APIKey != "" {
		opts = append(opts, anthropic.WithToken(connDef.APIKey))
	}

	opts = append(opts, anthropic.WithModel(llmSettings.ModelName))

	llm, err := anthropic.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create anthropic driver: %v", err)
	}

	return llm, nil
}

func setupVertexDriver(connDef *models.LLM, llmSettings *models.LLMSettings) (llms.Model, error) {
	// format for project and location is split with a colon
	split := strings.Split(connDef.APIEndpoint, ":")
	if len(split) != 2 {
		return nil, fmt.Errorf("invalid API endpoint format (must be project:location)")
	}

	project := split[0]
	location := split[1]

	ctx, _ := context.WithTimeout(context.Background(), 240*time.Second)

	llm, err := vertex.New(
		ctx,
		googleai.WithCloudProject(project),
		googleai.WithCloudLocation(location),
		googleai.WithAPIKey(connDef.APIKey),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create vertex driver: %v", err)
	}

	return llm, nil
}

func setupGoogleDriver(connDef *models.LLM, llmSettings *models.LLMSettings) (llms.Model, error) {
	var opts = make([]googleai.Option, 0)
	if connDef.APIKey != "" {
		opts = append(opts, googleai.WithAPIKey(connDef.APIKey))
	}

	opts = append(opts, googleai.WithDefaultModel(llmSettings.ModelName))

	llm, err := googleai.New(context.Background(), opts...)

	if err != nil {
		return nil, fmt.Errorf("failed to create google_ai driver: %v", err)
	}

	return llm, nil
}

func setupHuggingFaceDriver(connDef *models.LLM, llmSettings *models.LLMSettings) (llms.Model, error) {
	var opts = make([]huggingface.Option, 0)

	if connDef.APIKey != "" {
		opts = append(opts, huggingface.WithToken(connDef.APIKey))
	}

	opts = append(opts, huggingface.WithModel(llmSettings.ModelName))

	llm, err := huggingface.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create huggingface driver: %v", err)
	}

	return llm, nil
}

func setupOllamaDriver(connDef *models.LLM, llmSettings *models.LLMSettings) (llms.Model, error) {
	var opts = make([]ollama.Option, 0)

	if connDef.APIEndpoint != "" {
		opts = append(opts, ollama.WithServerURL(connDef.APIEndpoint))
	}

	opts = append(opts, ollama.WithModel(llmSettings.ModelName))

	llm, err := ollama.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create ollama driver: %v", err)
	}

	return llm, nil
}
