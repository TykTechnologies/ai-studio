package chat_session

import (
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func trimSession(maxLength int) *ChatSession {
	return &ChatSession{chatRef: &models.Chat{LLMSettings: &models.LLMSettings{MaxLength: maxLength}}}
}

func text(role llms.ChatMessageType, s string) llms.MessageContent {
	return llms.MessageContent{Role: role, Parts: []llms.ContentPart{llms.TextContent{Text: s}}}
}

func call(id, args string) llms.MessageContent {
	return llms.MessageContent{Role: llms.ChatMessageTypeAI, Parts: []llms.ContentPart{llms.ToolCall{
		ID: id, Type: "function", FunctionCall: &llms.FunctionCall{Name: "lookup", Arguments: args},
	}}}
}

func result(id, content string) llms.MessageContent {
	return llms.MessageContent{Role: llms.ChatMessageTypeTool, Parts: []llms.ContentPart{llms.ToolCallResponse{
		ToolCallID: id, Name: "lookup", Content: content,
	}}}
}

// roles renders the conversation as one letter per message.
func roles(msgs []llms.MessageContent) string {
	var b strings.Builder
	for _, m := range msgs {
		switch {
		case m.Role == llms.ChatMessageTypeSystem:
			b.WriteString("S")
		case m.Role == llms.ChatMessageTypeHuman:
			b.WriteString("U")
		case callsTools(m):
			b.WriteString("C")
		case m.Role == llms.ChatMessageTypeAI:
			b.WriteString("A")
		case m.Role == llms.ChatMessageTypeTool:
			b.WriteString("T")
		}
	}
	return b.String()
}

// requireWellFormed fails if a tool result follows anything but its call
// (or another result of the same call), or a call is not followed by its
// results: what vendors refuse.
func requireWellFormed(t *testing.T, msgs []llms.MessageContent) {
	t.Helper()
	r := roles(msgs)
	for i := 0; i < len(r); i++ {
		if r[i] == 'T' {
			require.True(t, i > 0 && (r[i-1] == 'C' || r[i-1] == 'T'), "orphaned tool result in %s", r)
		}
		if r[i] == 'C' {
			require.True(t, i+1 < len(r) && r[i+1] == 'T', "tool call without its result in %s", r)
		}
	}
}

// MaxLength 0 (the API default when the field is left out) is no limit:
// nothing is trimmed.
func TestPreflight_ZeroMaxLengthKeepsHistory(t *testing.T) {
	msgs := []llms.MessageContent{
		text(llms.ChatMessageTypeSystem, "sys"), text(llms.ChatMessageTypeHuman, "q"),
		call("c1", `{"q":"x"}`), result("c1", "r"),
	}
	out := trimSession(0).PreflightTokenLengthCheck(msgs)
	assert.Equal(t, "SUCT", roles(out))
}

// Trimming in a tool loop removes a call together with its results, never
// one without the other, and keeps the system prompt and the latest turn.
func TestPreflight_TrimsWholeToolTurns(t *testing.T) {
	long := strings.Repeat("word ", 200)
	msgs := []llms.MessageContent{
		text(llms.ChatMessageTypeSystem, "sys"),
		text(llms.ChatMessageTypeHuman, long),
		call("c1", `{"q":"`+long+`"}`), result("c1", long), result("c1b", long),
		text(llms.ChatMessageTypeAI, long),
		text(llms.ChatMessageTypeHuman, "latest question"),
		call("c2", `{"q":"y"}`), result("c2", "short"),
	}
	s := trimSession(50)
	out := s.PreflightTokenLengthCheck(msgs)
	requireWellFormed(t, out)
	assert.Equal(t, "S", roles(out)[:1], "the system prompt stays")
	assert.Equal(t, "UCT", roles(out)[1:], "older turns go whole; the latest turn stays")
	assert.LessOrEqual(t, s.estimateTokenLength(out), 50)
}

// The latest turn is never cut, even when it alone is over the limit: a
// call and its results are sent whole or the history is left as it is.
func TestPreflight_NeverSplitsTheLatestTurn(t *testing.T) {
	long := strings.Repeat("word ", 200)
	msgs := []llms.MessageContent{
		text(llms.ChatMessageTypeSystem, "sys"),
		call("c1", `{"q":"x"}`), result("c1", long),
	}
	out := trimSession(10).PreflightTokenLengthCheck(msgs)
	assert.Equal(t, "SCT", roles(out))
}

// The user's latest message is part of the turn being answered: it is
// never trimmed, however long, and neither is what follows it.
func TestPreflight_KeepsTheLatestUserMessage(t *testing.T) {
	long := strings.Repeat("word ", 200)
	msgs := []llms.MessageContent{
		text(llms.ChatMessageTypeSystem, "sys"),
		text(llms.ChatMessageTypeHuman, long),
		text(llms.ChatMessageTypeHuman, "latest "+long),
		call("c1", `{"q":"x"}`), result("c1", "r"),
	}
	out := trimSession(50).PreflightTokenLengthCheck(msgs)
	assert.Equal(t, "SUCT", roles(out))
	assert.Contains(t, out[1].Parts[0].(llms.TextContent).Text, "latest")
}

// Results that follow their call in the first message are not orphans.
func TestPreflight_KeepsResultsOfAFirstMessageCall(t *testing.T) {
	msgs := []llms.MessageContent{call("c1", `{}`), result("c1", "r"), text(llms.ChatMessageTypeHuman, "q")}
	out := trimSession(0).PreflightTokenLengthCheck(msgs)
	assert.Equal(t, "CTU", roles(out))
}

// Tool results whose call was trimmed earlier are dropped, not sent orphaned.
func TestPreflight_DropsOrphanedLeadingResults(t *testing.T) {
	msgs := []llms.MessageContent{
		text(llms.ChatMessageTypeSystem, "sys"),
		result("old", "r"), result("old2", "r"),
		text(llms.ChatMessageTypeHuman, "q"),
	}
	out := trimSession(0).PreflightTokenLengthCheck(msgs)
	assert.Equal(t, "SU", roles(out))
}

// A message that only calls tools is not free: its name and arguments count.
func TestEstimateTokenLengthCountsToolCalls(t *testing.T) {
	s := trimSession(0)
	assert.Greater(t, s.estimateTokenLength([]llms.MessageContent{call("c", `{"query":"a fairly long argument"}`)}), 3)

	named := func(name string) llms.MessageContent {
		return llms.MessageContent{Role: llms.ChatMessageTypeAI, Parts: []llms.ContentPart{llms.ToolCall{
			ID: "c", Type: "function", FunctionCall: &llms.FunctionCall{Name: name, Arguments: `{}`},
		}}}
	}
	assert.Greater(t, s.estimateTokenLength([]llms.MessageContent{named("a_much_longer_function_name_for_counting")}),
		s.estimateTokenLength([]llms.MessageContent{named("f")}), "the name counts")
}
