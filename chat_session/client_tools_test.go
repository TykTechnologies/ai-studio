package chat_session

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// scriptedModel answers the first GenerateContent with a client tool call and
// every later one with plain text.
type scriptedModel struct {
	mu    sync.Mutex
	calls int
}

func (m *scriptedModel) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	m.mu.Lock()
	m.calls++
	n := m.calls
	m.mu.Unlock()
	if n == 1 {
		return &llms.ContentResponse{Choices: []*llms.ContentChoice{{
			Content: "Let me confirm.",
			ToolCalls: []llms.ToolCall{{ID: "call_1", Type: "function", FunctionCall: &llms.FunctionCall{
				Name: "ask-approval", Arguments: `{"action":"delete the report"}`,
			}}},
		}}}, nil
	}
	return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: "Done, thanks for confirming."}}}, nil
}

func (m *scriptedModel) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	return "title", nil
}

// setupSharedDB opens a named shared-cache in-memory SQLite database. The
// session goroutine writes on its own pooled connection, and a plain
// ":memory:" DSN would give it an empty database of its own.
func setupSharedDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, models.InitModels(db))
	return db
}

func waitFor(t *testing.T, ch <-chan ChatEvent, kind string) ChatEvent {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Kind == EventError {
				t.Logf("error event: %s", ev.Data)
			}
			if ev.Kind == kind {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", kind)
		}
	}
}

func TestClientTools_ParkAndResume(t *testing.T) {
	db := setupSharedDB(t, "hitl1")
	svc := services.NewService(db)
	chat := &models.Chat{
		Name:          "HITL",
		LLM:           &models.LLM{Name: "Mock", Vendor: models.MOCK_VENDOR, PrivacyScore: 5},
		LLMSettings:   &models.LLMSettings{ModelName: "dummy", MaxLength: 10000},
		SupportsTools: true,
	}
	require.NoError(t, chat.Create(db))
	user := uint(1)
	sessionID := "hitl-session"

	cs, err := NewChatSession(chat, ChatStream, db, svc, nil, &user, &sessionID)
	require.NoError(t, err)
	cs.SetOutputMode(OutputModeEvents)
	require.NoError(t, cs.Start())
	t.Cleanup(cs.Stop)

	model := &scriptedModel{}
	cs.caller = model
	tool := models.Tool{
		ID: 7, Name: "Ask approval", Slug: "ask-approval", ToolType: models.ToolTypeClient,
		Description: "Ask the user to approve an action",
		OASSpec:     `{"parameters":{"type":"object","properties":{"action":{"type":"string"}}},"ui":{"kind":"approval"}}`,
	}
	cs.stateMu.Lock()
	cs.tools["ask-approval"] = tool
	cs.stateMu.Unlock()

	// The model sees the client tool as a function.
	defs := cs.prepareTools()
	require.Len(t, defs, 1)
	assert.Equal(t, "ask-approval", defs[0].Function.Name)

	infos := cs.ClientTools()
	require.Len(t, infos, 1)
	assert.Equal(t, "approval", infos[0].UI.Kind)

	// Turn 1: the model calls the client tool; the turn parks.
	events, unsub := cs.Subscribe("r1", 64)
	defer unsub()
	cs.Input() <- &models.UserMessage{Payload: "Please delete the report", RunID: "r1"}

	start := waitFor(t, events, EventToolCallStart)
	var sd ToolCallStartData
	require.NoError(t, json.Unmarshal(start.Data, &sd))
	assert.Equal(t, "call_1", sd.ToolCallID)
	assert.Equal(t, "ask-approval", sd.ToolName)

	fin := waitFor(t, events, EventFinish)
	var fd FinishData
	require.NoError(t, json.Unmarshal(fin.Data, &fd))
	assert.Equal(t, FinishToolCalls, fd.Reason)
	assert.True(t, cs.AwaitingClientTools())
	assert.Equal(t, []string{"call_1"}, cs.PendingClientCallIDs())

	rows, err := svc.GetCMessagesForSession(sessionID)
	require.NoError(t, err)
	// system prompt? none configured; human, ai text, ai tool-call request
	assert.Len(t, rows, 3)

	// Turn 2: the user answers; the model is called again with the result.
	events2, unsub2 := cs.Subscribe("r2", 64)
	defer unsub2()
	cs.Input() <- &models.UserMessage{RunID: "r2", ToolResults: []models.ToolResult{{ToolCallID: "call_1", Result: `{"approved":true}`}}}

	delta := waitFor(t, events2, EventTextDelta)
	var td TextDeltaData
	require.NoError(t, json.Unmarshal(delta.Data, &td))
	assert.Contains(t, td.Delta, "Done")

	fin2 := waitFor(t, events2, EventFinish)
	require.NoError(t, json.Unmarshal(fin2.Data, &fd))
	assert.Equal(t, FinishStop, fd.Reason)
	assert.False(t, cs.AwaitingClientTools())

	rows, err = svc.GetCMessagesForSession(sessionID)
	require.NoError(t, err)
	require.Len(t, rows, 5, "tool-response row and the follow-up reply were added")
	var last llms.MessageContent
	require.NoError(t, json.Unmarshal(rows[4].Content, &last))
	assert.Equal(t, llms.ChatMessageTypeAI, last.Role)
	var toolRow llms.MessageContent
	require.NoError(t, json.Unmarshal(rows[3].Content, &toolRow))
	assert.Equal(t, llms.ChatMessageTypeTool, toolRow.Role)
	resp, ok := toolRow.Parts[0].(llms.ToolCallResponse)
	require.True(t, ok)
	// Stored as a labelled envelope: the model sees user input, not a
	// trusted tool response.
	answer, isEnvelope := UnwrapClientAnswer(resp.Content)
	require.True(t, isEnvelope, resp.Content)
	assert.Equal(t, map[string]interface{}{"approved": true}, answer)
	assert.Contains(t, resp.Content, `"untrusted":true`)
}

func TestClientTools_AbandonedOnNextMessage(t *testing.T) {
	db := setupSharedDB(t, "hitl2")
	svc := services.NewService(db)
	chat := &models.Chat{
		Name:        "HITL2",
		LLM:         &models.LLM{Name: "Mock", Vendor: models.MOCK_VENDOR, PrivacyScore: 5},
		LLMSettings: &models.LLMSettings{ModelName: "dummy", MaxLength: 10000},
	}
	require.NoError(t, chat.Create(db))
	user := uint(1)
	sessionID := "hitl-session-2"
	cs, err := NewChatSession(chat, ChatStream, db, svc, nil, &user, &sessionID)
	require.NoError(t, err)
	cs.SetOutputMode(OutputModeEvents)
	require.NoError(t, cs.Start())
	t.Cleanup(cs.Stop)
	cs.caller = &scriptedModel{}
	cs.stateMu.Lock()
	cs.tools["ask-approval"] = models.Tool{ID: 7, Name: "Ask approval", Slug: "ask-approval", ToolType: models.ToolTypeClient}
	cs.stateMu.Unlock()

	events, unsub := cs.Subscribe("r1", 64)
	defer unsub()
	cs.Input() <- &models.UserMessage{Payload: "go", RunID: "r1"}
	waitFor(t, events, EventFinish)
	require.True(t, cs.AwaitingClientTools())

	events2, unsub2 := cs.Subscribe("r2", 64)
	defer unsub2()
	cs.Input() <- &models.UserMessage{Payload: "never mind", RunID: "r2"}
	waitFor(t, events2, EventFinish)
	assert.False(t, cs.AwaitingClientTools())

	rows, err := svc.GetCMessagesForSession(sessionID)
	require.NoError(t, err)
	var toolRow llms.MessageContent
	require.NoError(t, json.Unmarshal(rows[3].Content, &toolRow))
	assert.Equal(t, llms.ChatMessageTypeTool, toolRow.Role, "the parked call was closed with an error result before the new turn")
	assert.Contains(t, toolRow.Parts[0].(llms.ToolCallResponse).Content, "did not respond")
}
