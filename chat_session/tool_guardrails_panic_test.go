//go:build enterprise

package chat_session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
)

// A chat guardrail that panics on a tool result blocks that result; the
// session goroutine, which nothing else protects, goes on.
func TestChatToolResult_PanickingGuardrailBlocks(t *testing.T) {
	guardrails.RegisterSpec(guardrails.Spec{
		Name: "chat_test_panicking", DisplayName: "panics",
		Detectors: []guardrails.DetectorSpec{{Name: "x", Category: "other"}},
	})
	guardrails.RegisterFactory("chat_test_panicking", func(guardrails.Config) (guardrails.Provider, error) {
		panic("boom: secret-tool-output")
	})
	f := newChatToolFilterFixture(t, injectedPage, nil)
	f.session.filters = []*models.Filter{{ID: 88, Name: "panicking", Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider": "chat_test_panicking", "detectors": []any{map[string]any{"name": "x"}}, "on_detect": guardrails.ActionLog,
	}}}

	for i := 0; i < 2; i++ {
		responses := f.call(t, `{}`)
		require.Len(t, responses, 1)
		assert.Contains(t, responses[0].Content, scripting.ToolBlockedMessage)
		assert.NotContains(t, responses[0].Content, "secret-tool-output")
	}
}
