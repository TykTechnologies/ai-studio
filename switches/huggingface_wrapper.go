package switches

import (
	"context"

	"github.com/tmc/langchaingo/llms/huggingface"
)

type HFWrapper struct {
	hdLLM     *huggingface.LLM
	ModelName string
}

// The signature for the huggingface embedclient is wrong so it dos not implement
// the EmbedClient interface, this fixes that
func NewHFWrapper(modelName string, opts ...huggingface.Option) (*HFWrapper, error) {
	x, err := huggingface.New(opts...)
	if err != nil {
		return nil, err
	}

	return &HFWrapper{hdLLM: x}, nil
}

func (o *HFWrapper) CreateEmbedding(ctx context.Context, inputTexts []string) ([][]float32, error) {
	return o.hdLLM.CreateEmbedding(ctx, inputTexts, o.ModelName, "embedding")
}
