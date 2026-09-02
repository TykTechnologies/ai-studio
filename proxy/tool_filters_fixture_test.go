package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// toolFilterFixture is a proxy wired to a real tool pointing at a mock
// upstream, with an app credential authorised to call it.
type toolFilterFixture struct {
	service    *services.Service
	router     http.Handler
	apiKey     string
	tool       *models.Tool
	upstream   *mockServerConfig
	db         *gorm.DB
	teardownFn []func()
}

func (f *toolFilterFixture) teardown() {
	for i := len(f.teardownFn) - 1; i >= 0; i-- {
		f.teardownFn[i]()
	}
}

// attachFilter creates a filter and attaches it to the fixture's tool.
// responseFilter false governs tool input, true governs tool output.
func (f *toolFilterFixture) attachFilter(t *testing.T, name, script string, responseFilter bool) {
	t.Helper()
	filter, err := f.service.CreateFilter(name, "test filter", []byte(script), responseFilter, "")
	require.NoError(t, err)
	require.NoError(t, f.service.AddFilterToTool(f.tool.ID, filter.ID))
}

// call issues a tool call through the proxy exactly as a client would.
func (f *toolFilterFixture) call(t *testing.T, operationID string, payload map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(map[string]interface{}{
		"operation_id": operationID,
		"parameters":   map[string][]string{},
		"payload":      payload,
		"headers":      map[string][]string{},
	})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "/tools/"+testToolSlug, bytes.NewBuffer(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+f.apiKey)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)
	return rr
}

func newToolFilterFixture(t *testing.T, upstreamBody string) *toolFilterFixture {
	t.Helper()
	return newToolFilterFixtureWithService(t, upstreamBody, nil)
}

// newToolFilterFixtureWithService allows a test to wrap the gateway service,
// e.g. to return a tool result of a type Studio's client would never produce
// but the microgateway's does.
func newToolFilterFixtureWithService(t *testing.T, upstreamBody string, wrap func(*services.Service) services.ServiceInterface) *toolFilterFixture {
	t.Helper()

	db, cancel := setupTest(t)
	service := services.NewService(db)
	budgetService := budget.NewService(db, services.NewTestNotificationService(db))

	user, err := service.CreateUser(services.UserDTO{
		Email:    "tool-filters@example.com",
		Name:     "toolfilters",
		Password: "password123",
	})
	require.NoError(t, err)

	upstream := &mockServerConfig{
		ResponseStatus: http.StatusOK,
		ResponseBody:   upstreamBody,
	}
	mockServer, mockTeardown := newMockServer(t, upstream)

	toolDef := newTestToolDefinition(mockServer.URL)
	_, err = service.CreateTool(toolDef.Name, toolDef.Description, toolDef.ToolType,
		toolDef.OASSpec, toolDef.PrivacyScore, "", "")
	require.NoError(t, err)

	// CreateTool does not carry the operation whitelist, which the MCP
	// transport requires before it will expose any tool.
	require.NoError(t, service.DB.Model(&models.Tool{}).
		Where("slug = ?", testToolSlug).
		Update("available_operations", toolDef.AvailableOperations).Error)

	tool, err := service.GetToolBySlug(testToolSlug)
	require.NoError(t, err)

	app, err := service.CreateApp("Tool Filter App", "governance test", user.ID,
		[]uint{}, []uint{}, []uint{tool.ID}, nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, service.ActivateAppCredential(app.ID))

	app, err = service.GetAppByID(app.ID)
	require.NoError(t, err)
	require.NotNil(t, app.Credential)

	var gatewayService services.ServiceInterface = service
	if wrap != nil {
		gatewayService = wrap(service)
	}

	p := New(gatewayService, budgetService, &Config{Port: 9998})
	require.NoError(t, p.loadResources())

	return &toolFilterFixture{
		service:  service,
		router:   p.createHandler(),
		apiKey:   app.Credential.Secret,
		tool:     tool,
		upstream: upstream,
		db:       db,
		teardownFn: []func(){
			func() { tearDownTest(db, cancel) },
			mockTeardown,
		},
	}
}

// complianceEvents returns the compliance events recorded so far, waiting
// briefly because the analytics pipeline records them asynchronously.
func (f *toolFilterFixture) complianceEvents(t *testing.T, expected int) []models.ComplianceEvent {
	t.Helper()

	deadline := time.Now().Add(12 * time.Second)
	var events []models.ComplianceEvent
	for time.Now().Before(deadline) {
		events = nil
		if err := f.db.Order("id asc").Find(&events).Error; err == nil && len(events) >= expected {
			return events
		}
		time.Sleep(50 * time.Millisecond)
	}
	return events
}

// mcpSession drives a tool's MCP StreamableHTTP endpoint the way a real MCP
// client does: initialize, then call. There is no other MCP transport test in
// the repo, so this is written against the wire protocol rather than a helper.
type mcpSession struct {
	fixture   *toolFilterFixture
	sessionID string
}

func (s *mcpSession) post(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, "/tools/"+testToolSlug+"/mcp", bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+s.fixture.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if s.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", s.sessionID)
	}

	rr := httptest.NewRecorder()
	s.fixture.router.ServeHTTP(rr, req)
	return rr
}

func newMCPSession(t *testing.T, f *toolFilterFixture) *mcpSession {
	t.Helper()

	s := &mcpSession{fixture: f}
	rr := s.post(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	require.Equal(t, http.StatusOK, rr.Code, "MCP initialize failed: %s", rr.Body.String())

	s.sessionID = rr.Header().Get("Mcp-Session-Id")
	require.NotEmpty(t, s.sessionID)

	s.post(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	return s
}

// callTool performs tools/call and returns the text content plus whether the
// result was flagged as an error.
func (s *mcpSession) callTool(t *testing.T, name string, arguments map[string]interface{}) (string, bool) {
	t.Helper()

	params, err := json.Marshal(map[string]interface{}{"name": name, "arguments": arguments})
	require.NoError(t, err)

	rr := s.post(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":`+string(params)+`}`)
	require.Equal(t, http.StatusOK, rr.Code, "tools/call transport failure: %s", rr.Body.String())

	var envelope struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &envelope), "body: %s", rr.Body.String())
	require.NotEmpty(t, envelope.Result.Content, "body: %s", rr.Body.String())

	return envelope.Result.Content[0].Text, envelope.Result.IsError
}
