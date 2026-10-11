package proxy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func messageTexts(msgs []llms.MessageContent) (roles []llms.ChatMessageType, texts []string) {
	for _, m := range msgs {
		roles = append(roles, m.Role)
		var b strings.Builder
		for _, p := range m.Parts {
			if tp, ok := p.(llms.TextContent); ok {
				b.WriteString(tp.Text)
			}
		}
		texts = append(texts, b.String())
	}
	return roles, texts
}

func TestDecisionFilterFormat(t *testing.T) {
	assert.Equal(t, requestFilterFormatOpenAIDecisions, decisionFilterFormat("/llm/rest/openai/v1/decisions"))
	assert.Equal(t, requestFilterFormatSystemOne, decisionFilterFormat("/llm/rest/jev/v1/systemone"))
	assert.Equal(t, "", decisionFilterFormat("/llm/rest/openai/v1/chat/completions"))
	assert.Equal(t, models.DecisionRequest, requestKindForPath("/v1/systemone"))
	assert.Equal(t, models.GenerativeRequest, requestKindForPath("/v1/chat/completions"))
}

func TestDecisionExtractors_EvidenceIsTheLastUserMessage(t *testing.T) {
	for name, tc := range map[string]struct {
		extractor MessageExtractor
		body      string
	}{
		"openai":    {&OpenAIDecisionsMessageExtractor{}, decisionRequestOpenAI},
		"systemone": {&SystemOneMessageExtractor{}, decisionRequestSystemOne},
	} {
		t.Run(name, func(t *testing.T) {
			msgs, err := tc.extractor.ExtractMessages(nil, []byte(tc.body))
			require.NoError(t, err)
			roles, texts := messageTexts(msgs)
			assert.Equal(t, []llms.ChatMessageType{llms.ChatMessageTypeSystem, llms.ChatMessageTypeSystem, llms.ChatMessageTypeSystem, llms.ChatMessageTypeHuman}, roles)
			assert.Equal(t, []string{"Is the customer angry?", "Which team?", "How urgent?", "I was charged twice for my order."}, texts)
		})
	}
}

func TestOpenAIDecisionsReconstructor_RewritesEvidenceAndInstructions(t *testing.T) {
	r := &OpenAIDecisionsMessageReconstructor{}
	out, err := r.Reconstruct([]map[string]interface{}{
		{"role": "system", "content": "Is the customer angry?"},
		{"role": "system", "content": "Which team?"},
		{"role": "system", "content": "How urgent? [edited]"},
		{"role": "user", "content": "I was charged twice for order [REDACTED]."},
	}, []byte(decisionRequestOpenAI))
	require.NoError(t, err)

	var req map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out, &req))
	assert.JSONEq(t, `"I was charged twice for order [REDACTED]."`, string(req["input"]))
	assert.JSONEq(t, `"gpt-6-luna"`, string(req["model"]))
	var questions []map[string]any
	require.NoError(t, json.Unmarshal(req["questions"], &questions))
	require.Len(t, questions, 3)
	assert.Equal(t, "How urgent? [edited]", questions[2]["instructions"])
	assert.Equal(t, "topic", questions[1]["name"])
	assert.Len(t, questions[1]["choices"], 2, "choices survive")
}

func TestOpenAIDecisionsReconstructor_KeepsImageParts(t *testing.T) {
	body := `{"model":"gpt-6-luna","input":[{"role":"user","content":[` +
		`{"type":"input_text","text":"card 4111 1111 1111 1111"},` +
		`{"type":"input_image","image_url":"data:image/png;base64,AAAA"},` +
		`{"type":"input_text","text":"second part"}]}],` +
		`"questions":[{"type":"predicate","name":"q","instructions":"?"}]}`
	r := &OpenAIDecisionsMessageReconstructor{}
	out, err := r.Reconstruct([]map[string]interface{}{
		{"role": "system", "content": "?"},
		{"role": "user", "content": "card [REDACTED]\nsecond part"},
	}, []byte(body))
	require.NoError(t, err)

	var req struct {
		Input []struct {
			Content []map[string]string `json:"content"`
		} `json:"input"`
	}
	require.NoError(t, json.Unmarshal(out, &req))
	require.Len(t, req.Input, 1)
	parts := req.Input[0].Content
	require.Len(t, parts, 2, "the text collapses into one part; the image stays")
	assert.Equal(t, "card [REDACTED]\nsecond part", parts[0]["text"])
	assert.Equal(t, "input_image", parts[1]["type"])
	assert.NotContains(t, string(out), "4111")

	// Unchanged text leaves the input exactly as sent.
	out, err = r.Reconstruct([]map[string]interface{}{
		{"role": "system", "content": "?"},
		{"role": "user", "content": "card 4111 1111 1111 1111\nsecond part"},
	}, []byte(body))
	require.NoError(t, err)
	assert.Contains(t, string(out), `"second part"`)
}

func TestSystemOneReconstructor_RewritesStateKeepsOrder(t *testing.T) {
	r := &SystemOneMessageReconstructor{}
	out, err := r.Reconstruct([]map[string]interface{}{
		{"role": "system", "content": "Is the customer angry?"},
		{"role": "system", "content": "Which team?"},
		{"role": "system", "content": "How urgent?"},
		{"role": "user", "content": "I was charged twice for order [REDACTED]."},
	}, []byte(decisionRequestSystemOne))
	require.NoError(t, err)

	s := string(out)
	assert.Contains(t, s, `"state":"I was charged twice for order [REDACTED]."`)
	assert.Less(t, strings.Index(s, `"angry"`), strings.Index(s, `"topic"`))
	assert.Less(t, strings.Index(s, `"topic"`), strings.Index(s, `"urgency"`))
	assert.Less(t, strings.Index(s, `"shipping"`), strings.Index(s, `"billing"`), "option order survives")
}

// A filter whose output drops the evidence or a question must not corrupt
// the request: unmatched parts are left as they were.
func TestDecisionReconstructors_PartialFilterOutput(t *testing.T) {
	t.Run("openai: no user message keeps the input", func(t *testing.T) {
		out, err := (&OpenAIDecisionsMessageReconstructor{}).Reconstruct([]map[string]interface{}{
			{"role": "system", "content": "Is the customer angry?"},
		}, []byte(decisionRequestOpenAI))
		require.NoError(t, err)
		var req map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(out, &req))
		assert.JSONEq(t, `"I was charged twice for my order."`, string(req["input"]))
	})

	t.Run("openai: an instruction count mismatch leaves instructions alone", func(t *testing.T) {
		out, err := (&OpenAIDecisionsMessageReconstructor{}).Reconstruct([]map[string]interface{}{
			{"role": "system", "content": "only one"},
			{"role": "user", "content": "new evidence"},
		}, []byte(decisionRequestOpenAI))
		require.NoError(t, err)
		assert.Contains(t, string(out), "Which team?")
		assert.NotContains(t, string(out), "only one")
		assert.Contains(t, string(out), "new evidence")
	})

	t.Run("systemone: no user message keeps the state", func(t *testing.T) {
		out, err := (&SystemOneMessageReconstructor{}).Reconstruct([]map[string]interface{}{}, []byte(decisionRequestSystemOne))
		require.NoError(t, err)
		assert.Contains(t, string(out), `"state":"I was charged twice for my order."`)
	})

	t.Run("not JSON is an error, not a silent pass", func(t *testing.T) {
		_, err := (&OpenAIDecisionsMessageReconstructor{}).Reconstruct(nil, []byte(`{"model":`))
		assert.Error(t, err)
		_, err = (&SystemOneMessageReconstructor{}).Reconstruct(nil, []byte(`{"model":`))
		assert.Error(t, err)
	})

	t.Run("extractors reject what they cannot read", func(t *testing.T) {
		_, err := (&OpenAIDecisionsMessageExtractor{}).ExtractMessages(nil, []byte(`{"input":7}`))
		assert.Error(t, err)
		_, err = (&SystemOneMessageExtractor{}).ExtractMessages(nil, []byte(`not json`))
		assert.Error(t, err)
	})
}

func TestSystemOneReconstructor_UntouchedObjectStateKeepsItsShape(t *testing.T) {
	body := `{"model":"jev-latest","state":{"ticket":"T-1","body":"hello"},"questions":{"q":{"type":"noul","instructions":{"ask":"angry?"}}}}`
	r := &SystemOneMessageReconstructor{}
	out, err := r.Reconstruct([]map[string]interface{}{
		{"role": "system", "content": `{"ask":"angry?"}`},
		{"role": "user", "content": `{"ticket":"T-1","body":"hello"}`},
	}, []byte(body))
	require.NoError(t, err)
	assert.JSONEq(t, body, string(out))
}
