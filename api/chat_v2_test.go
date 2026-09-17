package api_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	gotest "testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/api"
	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/group_access"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uiChunk is one decoded `data:` line of a UI message stream.
type uiChunk map[string]any

// readUIStream decodes a UI message stream body until [DONE] or EOF.
func readUIStream(t *gotest.T, body *bufio.Scanner) (chunks []uiChunk, sawDone bool) {
	t.Helper()
	for body.Scan() {
		line := body.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			return chunks, true
		}
		var c uiChunk
		require.NoError(t, json.Unmarshal([]byte(payload), &c), "chunk must be JSON: %s", payload)
		chunks = append(chunks, c)
	}
	return chunks, false
}

func indexOf(list []string, want string) int {
	for i, v := range list {
		if v == want {
			return i
		}
	}
	return -1
}

func chunkTypes(chunks []uiChunk) []string {
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, c["type"].(string))
	}
	return out
}

func TestChatV2(t *gotest.T) {
	os.Setenv("ENVIRONMENT", "test")
	os.Setenv("ENABLE_ANALYTICS", "false")
	defer os.Unsetenv("ENVIRONMENT")
	defer os.Unsetenv("ENABLE_ANALYTICS")
	gin.SetMode(gin.TestMode)

	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	a := api.NewAPI(service, true, authService, config, nil, apitest.EmptyFile, nil)

	user := &models.User{Email: "v2@test.com", Name: "V2 User", IsAdmin: true, EmailVerified: true, ShowChat: true}
	require.NoError(t, user.Create(db))
	other := &models.User{Email: "other@test.com", Name: "Other", EmailVerified: true, ShowChat: true}
	require.NoError(t, other.Create(db))

	group := &models.Group{Name: "Default"}
	require.NoError(t, group.Create(db))
	require.NoError(t, service.AddUserToGroup(user.ID, group.ID))

	chat := &models.Chat{
		LLM:           &models.LLM{Name: "Dummy LLM", Vendor: models.MOCK_VENDOR},
		LLMSettings:   &models.LLMSettings{ModelName: "dummy"},
		Name:          "V2 Chat",
		Description:   "A chat for the v2 API",
		Groups:        []models.Group{*group},
		SupportsTools: true,
		SystemPrompt:  "You are a helpful assistant.",
	}
	require.NoError(t, chat.SetPromptTemplates([]models.PromptTemplate{{ID: 1, Name: "Hi", Prompt: "Say hi"}}))
	require.NoError(t, chat.Create(db))

	newRouter := func(u *models.User) *gin.Engine {
		router := gin.New()
		authed := router.Group("/common")
		authed.Use(func(c *gin.Context) { c.Set("user", u); c.Next() })
		a.SetupChatRoutes(authed)
		return router
	}
	router := newRouter(user)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// 1. Create a session.
	w := apitest.PerformRequest(router, "POST", fmt.Sprintf("/common/chat/%d/sessions", chat.ID), map[string]any{})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var sess api.V2SessionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sess))
	require.NotEmpty(t, sess.SessionID)
	assert.Equal(t, "V2 Chat", sess.Chat.Name)
	assert.True(t, sess.Chat.ToolSupport)
	require.Len(t, sess.Chat.PromptTemplates, 1)
	assert.Equal(t, "Say hi", sess.Chat.PromptTemplates[0].Prompt)

	// 2. Run a turn and read the stream.
	runURL := fmt.Sprintf("%s/common/chat-sessions/%s/runs", ts.URL, sess.SessionID)
	body, _ := json.Marshal(api.V2RunRequest{Message: "Hello, assistant!"})
	resp, err := http.Post(runURL, "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "v1", resp.Header.Get("x-vercel-ai-ui-message-stream"))
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")

	chunks, done := readUIStream(t, bufio.NewScanner(resp.Body))
	require.True(t, done, "stream must end with [DONE]")
	types := chunkTypes(chunks)
	assert.Equal(t, "start", types[0])
	assert.Equal(t, "start-step", types[1])
	assert.Equal(t, "text-start", types[2])
	assert.Equal(t, "finish", types[len(types)-1])
	assert.Equal(t, "finish-step", types[len(types)-2])
	assert.Less(t, indexOf(types, "text-start"), indexOf(types, "text-delta"))
	assert.Less(t, indexOf(types, "text-delta"), indexOf(types, "text-end"))
	assert.Less(t, indexOf(types, "text-end"), indexOf(types, "finish-step"))
	assert.Equal(t, "stop", chunks[len(chunks)-1]["finishReason"])

	var text strings.Builder
	for _, c := range chunks {
		if c["type"] == "text-delta" {
			text.WriteString(c["delta"].(string))
		}
	}
	// The mock vendor streams the sentence word by word (no separators).
	assert.Equal(t, "thisisatenwordsentencethatshouldbesent.", text.String())

	// 3. History reflects the turn.
	w = apitest.PerformRequest(router, "GET", fmt.Sprintf("/common/chat-sessions/%s/messages/v2", sess.SessionID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var hist struct {
		Messages []api.V2Message `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &hist))
	require.Len(t, hist.Messages, 2)
	assert.Equal(t, "user", hist.Messages[0].Role)
	assert.Equal(t, "Hello, assistant!", hist.Messages[0].Parts[0].Text)
	assert.Equal(t, "assistant", hist.Messages[1].Role)
	assert.Equal(t, "this is a ten word sentence that should be sent.", hist.Messages[1].Parts[0].Text)

	// The finish carries the row ids of the turn (transient data chunk).
	var ids map[string]any
	for _, c := range chunks {
		if c["type"] == "data-message-ids" {
			ids = c["data"].(map[string]any)
		}
	}
	require.NotNil(t, ids)
	assert.Equal(t, hist.Messages[0].ID, ids["user_message_id"])
	assert.Equal(t, hist.Messages[1].ID, ids["assistant_message_id"])
	firstAssistantID := hist.Messages[1].ID

	// 4. Regenerate replaces the last reply instead of appending.
	body, _ = json.Marshal(api.V2RunRequest{Regenerate: true})
	resp2, err := http.Post(runURL, "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	chunks2, done := readUIStream(t, bufio.NewScanner(resp2.Body))
	resp2.Body.Close()
	require.True(t, done)
	assert.Contains(t, chunkTypes(chunks2), "text-delta")

	w = apitest.PerformRequest(router, "GET", fmt.Sprintf("/common/chat-sessions/%s/messages/v2", sess.SessionID), nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &hist))
	require.Len(t, hist.Messages, 2, "regenerate must not grow the history")
	assert.Equal(t, "Hello, assistant!", hist.Messages[0].Parts[0].Text, "the user turn is kept")
	assert.NotEqual(t, firstAssistantID, hist.Messages[1].ID, "the reply row was replaced")

	// 4b. An edit rewinds to after a given message, then appends.
	root := "root"
	body, _ = json.Marshal(api.V2RunRequest{Message: "Edited opener", AfterMessageID: &root})
	resp2b, err := http.Post(runURL, "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	_, done = readUIStream(t, bufio.NewScanner(resp2b.Body))
	resp2b.Body.Close()
	require.True(t, done)
	w = apitest.PerformRequest(router, "GET", fmt.Sprintf("/common/chat-sessions/%s/messages/v2", sess.SessionID), nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &hist))
	require.Len(t, hist.Messages, 2)
	assert.Equal(t, "Edited opener", hist.Messages[0].Parts[0].Text)

	// 5. Resume the session by id returns the same session.
	w = apitest.PerformRequest(router, "POST", fmt.Sprintf("/common/chat/%d/sessions", chat.ID), map[string]any{"session_id": sess.SessionID})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resumed api.V2SessionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resumed))
	assert.Equal(t, sess.SessionID, resumed.SessionID)

	// 6. Another user cannot touch the session.
	otherRouter := newRouter(other)
	w = apitest.PerformRequest(otherRouter, "GET", fmt.Sprintf("/common/chat-sessions/%s/messages/v2", sess.SessionID), nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w = apitest.PerformRequest(otherRouter, "POST", fmt.Sprintf("/common/chat-sessions/%s/runs", sess.SessionID), api.V2RunRequest{Message: "hi"})
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 7. A v1 SSE reader may not attach to a v2 session.
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/common/chat/%d?session_id=%s", ts.URL, chat.ID, sess.SessionID), nil)
	client := &http.Client{Timeout: 5 * time.Second}
	resp3, err := client.Do(req)
	require.NoError(t, err)
	first := make([]byte, 512)
	n, _ := resp3.Body.Read(first)
	resp3.Body.Close()
	assert.Contains(t, string(first[:n]), "different chat API version")

	// 8. Cancel on an idle session reports nothing to cancel.
	w = apitest.PerformRequest(router, "POST", fmt.Sprintf("/common/chat-sessions/%s/cancel", sess.SessionID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"cancelled":false`)

	// 9. Validation.
	w = apitest.PerformRequest(router, "POST", fmt.Sprintf("/common/chat-sessions/%s/runs", sess.SessionID), api.V2RunRequest{})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 10. The v1 mutation endpoints (sidebar tool / datasource toggles) serve
	// v2 sessions too: removing a tool that is not attached is a no-op 200,
	// not a "different API version" 409.
	w = apitest.PerformRequest(router, "DELETE", fmt.Sprintf("/common/chat-sessions/%s/tools/999", sess.SessionID), nil)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// 10b. A client tool picked in the chat window: the built-in generative
	// UI tool was first seeded with a raw JSON definition (not base64), which
	// used to fail with "illegal base64 data at input byte 0". Either form
	// attaches, and the reply carries the session's client tools so the
	// browser can register the renderer without a new session.
	service.GroupAccessService = group_access.NewService(db) // AddTool checks entitlements
	require.NoError(t, models.GetOrCreateDefaultClientTools(db))
	var present models.Tool
	require.NoError(t, db.Where("tool_type = ?", models.ToolTypeClient).First(&present).Error)
	catalogue := &models.ToolCatalogue{Name: "V2 tools"}
	require.NoError(t, db.Create(catalogue).Error)
	require.NoError(t, catalogue.AddTool(db, &present))
	require.NoError(t, db.Model(group).Association("ToolCatalogues").Append(catalogue))
	for _, spec := range []string{present.OASSpec, `{"ui":{"kind":"present","title":"Generative UI"}}`} {
		require.NoError(t, db.Model(&models.Tool{}).Where("id = ?", present.ID).UpdateColumn("oas_spec", spec).Error)
		w = apitest.PerformRequest(router, "POST", fmt.Sprintf("/common/chat-sessions/%s/tools", sess.SessionID), map[string]any{"tool_id": fmt.Sprint(present.ID)})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var added struct {
			ClientTools []api.V2ClientToolInfo `json:"client_tools"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &added))
		require.Len(t, added.ClientTools, 1)
		assert.Equal(t, models.PresentToolOperation, added.ClientTools[0].Name)
		assert.Contains(t, string(added.ClientTools[0].UI), `"kind":"present"`)
		assert.Contains(t, string(added.ClientTools[0].Schema), `"component"`)

		w = apitest.PerformRequest(router, "DELETE", fmt.Sprintf("/common/chat-sessions/%s/tools/%d", sess.SessionID, present.ID), nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), `"client_tools":[]`)
	}

	// 11. History pages backwards from the newest row.
	w = apitest.PerformRequest(router, "GET", fmt.Sprintf("/common/chat-sessions/%s/messages/v2?limit=1", sess.SessionID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	var page struct {
		Messages   []api.V2Message `json:"messages"`
		HasMore    bool            `json:"has_more"`
		NextBefore uint            `json:"next_before"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Len(t, page.Messages, 1)
	assert.Equal(t, "assistant", page.Messages[0].Role, "the newest row comes first when paging")
	assert.True(t, page.HasMore)
	require.NotZero(t, page.NextBefore)
	w = apitest.PerformRequest(router, "GET", fmt.Sprintf("/common/chat-sessions/%s/messages/v2?limit=1&before=%d", sess.SessionID, page.NextBefore), nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Len(t, page.Messages, 1)
	assert.Equal(t, "user", page.Messages[0].Role)
	// Paging counts stored rows; the system prompt row precedes the user
	// turn, so one more (empty) page follows.
	assert.True(t, page.HasMore)
	w = apitest.PerformRequest(router, "GET", fmt.Sprintf("/common/chat-sessions/%s/messages/v2?limit=1&before=%d", sess.SessionID, page.NextBefore), nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	assert.Empty(t, page.Messages)
	assert.False(t, page.HasMore)
}
