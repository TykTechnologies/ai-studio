package chat_session

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// titleRecorder records the call options of the title request.
type titleRecorder struct{ opts llms.CallOptions }

func (m *titleRecorder) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	for _, o := range options {
		o(&m.opts)
	}
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: `"Weather in Auckland"`}}}, nil
}

func (m *titleRecorder) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	return "", nil
}

// The title call sends no sampling parameters: Claude Sonnet 5 and later
// reject temperature and top_p, so a hard-coded temperature meant titles
// never generated on them (lsj.31).
func TestGenerateChatTitleSendsNoSamplingParameters(t *testing.T) {
	model := &titleRecorder{}
	cs := &ChatSession{caller: model}

	title, err := cs.generateChatTitle("What's the weather in Auckland?")
	require.NoError(t, err)
	assert.Equal(t, "Weather in Auckland", title)
	assert.False(t, model.opts.TemperatureSet, "temperature must not be sent")
	assert.Zero(t, model.opts.TopP)
	assert.Equal(t, 50, model.opts.MaxTokens)
}
