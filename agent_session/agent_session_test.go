package agent_session

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// MockMessageQueue implements MessageQueue interface for testing
type MockMessageQueue struct {
	streamData     [][]byte
	errors         []error
	closed         bool
	publishErr     error // Error to return on PublishStream
	wg             sync.WaitGroup
	expectingCount int // Track if we're expecting chunks
}

func NewMockMessageQueue() *MockMessageQueue {
	return &MockMessageQueue{
		streamData: make([][]byte, 0),
		errors:     make([]error, 0),
	}
}

func (m *MockMessageQueue) PublishStream(ctx context.Context, data []byte) error {
	if m.expectingCount > 0 {
		defer m.wg.Done()
	}
	if m.publishErr != nil {
		return m.publishErr
	}
	m.streamData = append(m.streamData, data)
	return nil
}

func (m *MockMessageQueue) PublishError(ctx context.Context, err error) error {
	m.errors = append(m.errors, err)
	return nil
}

func (m *MockMessageQueue) ConsumeStream(ctx context.Context) <-chan []byte {
	ch := make(chan []byte, len(m.streamData))
	for _, data := range m.streamData {
		ch <- data
	}
	close(ch)
	return ch
}

func (m *MockMessageQueue) ConsumeErrors(ctx context.Context) <-chan error {
	ch := make(chan error, len(m.errors))
	for _, err := range m.errors {
		ch <- err
	}
	close(ch)
	return ch
}

func (m *MockMessageQueue) Close() error {
	m.closed = true
	return nil
}

// ExpectChunks sets up the WaitGroup to wait for n chunks
func (m *MockMessageQueue) ExpectChunks(n int) {
	m.expectingCount = n
	m.wg.Add(n)
}

// Wait blocks until all expected chunks are received
func (m *MockMessageQueue) Wait() {
	m.wg.Wait()
}

// MockPluginServiceClient implements pb.PluginServiceClient for testing
type MockPluginServiceClient struct {
	chunks    []*pb.AgentMessageChunk
	streamErr error
}

func (m *MockPluginServiceClient) HandleAgentMessage(ctx context.Context, in *pb.AgentMessageRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pb.AgentMessageChunk], error) {
	if m.streamErr != nil {
		return nil, m.streamErr
	}
	return &MockAgentMessageStream{chunks: m.chunks}, nil
}

// Implement minimal stubs for other required PluginServiceClient methods
func (m *MockPluginServiceClient) Initialize(ctx context.Context, in *pb.InitRequest, opts ...grpc.CallOption) (*pb.InitResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) Ping(ctx context.Context, in *pb.PingRequest, opts ...grpc.CallOption) (*pb.PingResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) Shutdown(ctx context.Context, in *pb.ShutdownRequest, opts ...grpc.CallOption) (*pb.ShutdownResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) ProcessPreAuth(ctx context.Context, in *pb.PluginRequest, opts ...grpc.CallOption) (*pb.PluginResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) Authenticate(ctx context.Context, in *pb.AuthRequest, opts ...grpc.CallOption) (*pb.AuthResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) GetAppByCredential(ctx context.Context, in *pb.GetAppRequest, opts ...grpc.CallOption) (*pb.GetAppResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) GetUserByCredential(ctx context.Context, in *pb.GetUserRequest, opts ...grpc.CallOption) (*pb.GetUserResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) ProcessPostAuth(ctx context.Context, in *pb.EnrichedRequest, opts ...grpc.CallOption) (*pb.PluginResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) OnBeforeWriteHeaders(ctx context.Context, in *pb.HeadersRequest, opts ...grpc.CallOption) (*pb.HeadersResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) OnBeforeWrite(ctx context.Context, in *pb.ResponseWriteRequest, opts ...grpc.CallOption) (*pb.ResponseWriteResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) HandleProxyLog(ctx context.Context, in *pb.ProxyLogRequest, opts ...grpc.CallOption) (*pb.DataCollectionResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) HandleAnalytics(ctx context.Context, in *pb.AnalyticsRequest, opts ...grpc.CallOption) (*pb.DataCollectionResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) HandleBudgetUsage(ctx context.Context, in *pb.BudgetUsageRequest, opts ...grpc.CallOption) (*pb.DataCollectionResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) GetAsset(ctx context.Context, in *pb.GetAssetRequest, opts ...grpc.CallOption) (*pb.GetAssetResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) ListAssets(ctx context.Context, in *pb.ListAssetsRequest, opts ...grpc.CallOption) (*pb.ListAssetsResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) GetManifest(ctx context.Context, in *pb.GetManifestRequest, opts ...grpc.CallOption) (*pb.GetManifestResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) Call(ctx context.Context, in *pb.CallRequest, opts ...grpc.CallOption) (*pb.CallResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) PortalCall(ctx context.Context, in *pb.PortalCallRequest, opts ...grpc.CallOption) (*pb.PortalCallResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) GetConfigSchema(ctx context.Context, in *pb.GetConfigSchemaRequest, opts ...grpc.CallOption) (*pb.GetConfigSchemaResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) GetObjectHookRegistrations(ctx context.Context, in *pb.GetObjectHookRegistrationsRequest, opts ...grpc.CallOption) (*pb.GetObjectHookRegistrationsResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) HandleObjectHook(ctx context.Context, in *pb.ObjectHookRequest, opts ...grpc.CallOption) (*pb.ObjectHookResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) OnStreamComplete(ctx context.Context, in *pb.StreamCompleteRequest, opts ...grpc.CallOption) (*pb.StreamCompleteResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) ExecuteScheduledTask(ctx context.Context, in *pb.ExecuteScheduledTaskRequest, opts ...grpc.CallOption) (*pb.ExecuteScheduledTaskResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) AcceptEdgePayload(ctx context.Context, in *pb.EdgePayloadRequest, opts ...grpc.CallOption) (*pb.EdgePayloadResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) OpenSession(ctx context.Context, in *pb.OpenSessionRequest, opts ...grpc.CallOption) (*pb.OpenSessionResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) CloseSession(ctx context.Context, in *pb.CloseSessionRequest, opts ...grpc.CallOption) (*pb.CloseSessionResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) GetEndpointRegistrations(ctx context.Context, in *pb.GetEndpointRegistrationsRequest, opts ...grpc.CallOption) (*pb.GetEndpointRegistrationsResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) HandleEndpointRequest(ctx context.Context, in *pb.EndpointRequest, opts ...grpc.CallOption) (*pb.EndpointResponse, error) {
	return nil, nil
}

func (m *MockPluginServiceClient) HandleEndpointRequestStream(ctx context.Context, in *pb.EndpointRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pb.EndpointResponseChunk], error) {
	return nil, nil
}

func (m *MockPluginServiceClient) GetResourceTypeRegistrations(ctx context.Context, in *pb.GetResourceTypeRegistrationsRequest, opts ...grpc.CallOption) (*pb.GetResourceTypeRegistrationsResponse, error) {
	return &pb.GetResourceTypeRegistrationsResponse{}, nil
}
func (m *MockPluginServiceClient) ListResourceInstances(ctx context.Context, in *pb.ListResourceInstancesRequest, opts ...grpc.CallOption) (*pb.ListResourceInstancesResponse, error) {
	return &pb.ListResourceInstancesResponse{}, nil
}
func (m *MockPluginServiceClient) GetResourceInstance(ctx context.Context, in *pb.GetResourceInstanceRequest, opts ...grpc.CallOption) (*pb.GetResourceInstanceResponse, error) {
	return &pb.GetResourceInstanceResponse{}, nil
}
func (m *MockPluginServiceClient) ValidateResourceSelection(ctx context.Context, in *pb.ValidateResourceSelectionRequest, opts ...grpc.CallOption) (*pb.ValidateResourceSelectionResponse, error) {
	return &pb.ValidateResourceSelectionResponse{Valid: true}, nil
}
func (m *MockPluginServiceClient) CreateResourceInstance(ctx context.Context, in *pb.CreateResourceInstanceRequest, opts ...grpc.CallOption) (*pb.CreateResourceInstanceResponse, error) {
	return &pb.CreateResourceInstanceResponse{}, nil
}

// MockAgentMessageStream implements the gRPC stream interface
type MockAgentMessageStream struct {
	grpc.ClientStream
	chunks []*pb.AgentMessageChunk
	index  int
}

func (m *MockAgentMessageStream) Recv() (*pb.AgentMessageChunk, error) {
	if m.index >= len(m.chunks) {
		return nil, errors.New("EOF")
	}
	chunk := m.chunks[m.index]
	m.index++
	return chunk, nil
}

// Helper function to setup test database
func setupAgentSessionTest(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	// Auto-migrate models
	err = db.AutoMigrate(
		&models.User{},
		&models.App{},
		&models.Plugin{},
		&models.LLM{},
		&models.Tool{},
		&models.Datasource{},
		&models.AgentConfig{},
		&models.Group{},
	)
	assert.NoError(t, err)

	return db
}

// Helper function to create test data
func createTestAgentConfig(t *testing.T, db *gorm.DB) *models.AgentConfig {
	// Create user
	user := &models.User{
		Email: "test@example.com",
		Name:  "Test User",
	}
	assert.NoError(t, db.Create(user).Error)

	// Create plugin
	plugin := &models.Plugin{
		Name:        "Test Agent Plugin",
		Description: "Test plugin for agent",
		Command:     "/usr/bin/test-agent",
		HookType:    "agent",
	}
	assert.NoError(t, db.Create(plugin).Error)

	// Create LLM
	llm := &models.LLM{
		Name:             "Test LLM",
		Vendor:           models.OPENAI,
		ShortDescription: "Test LLM",
		DefaultModel:     "gpt-4",
		Active:           true,
	}
	assert.NoError(t, db.Create(llm).Error)

	// Create tool
	tool := &models.Tool{
		Name:        "Test Tool",
		Description: "Test tool description",
		ToolType:    "api",
	}
	assert.NoError(t, db.Create(tool).Error)

	// Create datasource
	datasource := &models.Datasource{
		Name:             "Test Datasource",
		ShortDescription: "Test datasource",
		DBSourceType:     "postgresql",
	}
	assert.NoError(t, db.Create(datasource).Error)

	// Create app (first without associations)
	app := &models.App{
		Name:   "Test App",
		UserID: user.ID,
	}
	assert.NoError(t, db.Create(app).Error)

	// Add associations
	assert.NoError(t, db.Model(app).Association("LLMs").Append(&llm))
	assert.NoError(t, db.Model(app).Association("Tools").Append(&tool))
	assert.NoError(t, db.Model(app).Association("Datasources").Append(&datasource))

	// Create agent config
	config := map[string]interface{}{
		"system_prompt": "You are a helpful assistant",
		"temperature":   0.7,
	}
	agentConfig := &models.AgentConfig{
		Name:     "Test Agent",
		Slug:     "test-agent",
		PluginID: plugin.ID,
		AppID:    app.ID,
		Config:   config,
	}
	assert.NoError(t, db.Create(agentConfig).Error)

	return agentConfig
}

func TestNewAgentSession(t *testing.T) {
	db := setupAgentSessionTest(t)
	agentConfig := createTestAgentConfig(t, db)
	mockQueue := NewMockMessageQueue()
	mockClient := &MockPluginServiceClient{}

	t.Run("Create new agent session successfully", func(t *testing.T) {
		session, err := NewAgentSession(agentConfig, mockClient, 12345, mockQueue, db)

		assert.NoError(t, err)
		assert.NotNil(t, session)
		assert.NotEmpty(t, session.id)
		assert.Equal(t, agentConfig, session.agentConfig)
		assert.Equal(t, mockQueue, session.queue)
		assert.Equal(t, mockClient, session.pluginClient)
		assert.Equal(t, uint32(12345), session.serviceBrokerID)
		assert.Equal(t, db, session.db)
		assert.NotNil(t, session.ctx)
		assert.NotNil(t, session.cancel)
	})

	t.Run("Session ID is unique UUID format", func(t *testing.T) {
		session1, err1 := NewAgentSession(agentConfig, mockClient, 12345, mockQueue, db)
		session2, err2 := NewAgentSession(agentConfig, mockClient, 12345, mockQueue, db)

		assert.NoError(t, err1)
		assert.NoError(t, err2)
		assert.NotEqual(t, session1.id, session2.id)
		assert.Len(t, session1.id, 36) // UUID v4 format
	})
}

func TestAgentSession_SendMessage(t *testing.T) {
	db := setupAgentSessionTest(t)
	agentConfig := createTestAgentConfig(t, db)

	t.Run("Send message successfully", func(t *testing.T) {
		mockQueue := NewMockMessageQueue()
		mockClient := &MockPluginServiceClient{
			chunks: []*pb.AgentMessageChunk{
				{
					Type:    pb.AgentMessageChunk_CONTENT,
					Content: "Hello, I'm an agent",
					IsFinal: true,
				},
			},
		}

		session, err := NewAgentSession(agentConfig, mockClient, 12345, mockQueue, db)
		assert.NoError(t, err)

		history := []map[string]interface{}{
			{"role": "user", "content": "Previous message"},
			{"role": "assistant", "content": "Previous response"},
		}

		mockQueue.ExpectChunks(1)
		err = session.SendMessage("Hello agent", history)
		assert.NoError(t, err)

		// Wait for goroutine to complete
		mockQueue.Wait()

		// Verify chunks were published to queue
		assert.GreaterOrEqual(t, len(mockQueue.streamData), 1)
	})

	t.Run("Handle plugin error gracefully", func(t *testing.T) {
		mockQueue := NewMockMessageQueue()
		mockClient := &MockPluginServiceClient{
			streamErr: errors.New("plugin connection failed"),
		}

		session, err := NewAgentSession(agentConfig, mockClient, 12345, mockQueue, db)
		assert.NoError(t, err)

		err = session.SendMessage("Hello agent", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to call plugin HandleAgentMessage")
	})

	t.Run("Handle empty history", func(t *testing.T) {
		mockQueue := NewMockMessageQueue()
		mockClient := &MockPluginServiceClient{
			chunks: []*pb.AgentMessageChunk{
				{Type: pb.AgentMessageChunk_CONTENT, Content: "Response", IsFinal: true},
			},
		}

		session, err := NewAgentSession(agentConfig, mockClient, 12345, mockQueue, db)
		assert.NoError(t, err)

		mockQueue.ExpectChunks(1)
		err = session.SendMessage("Hello", nil)
		assert.NoError(t, err)
		mockQueue.Wait()
	})
}

func TestAgentSession_ReceiveChunks(t *testing.T) {
	db := setupAgentSessionTest(t)
	agentConfig := createTestAgentConfig(t, db)

	t.Run("Receive and publish multiple chunks", func(t *testing.T) {
		mockQueue := NewMockMessageQueue()
		mockClient := &MockPluginServiceClient{
			chunks: []*pb.AgentMessageChunk{
				{Type: pb.AgentMessageChunk_CONTENT, Content: "Chunk 1", IsFinal: false},
				{Type: pb.AgentMessageChunk_CONTENT, Content: "Chunk 2", IsFinal: false},
				{Type: pb.AgentMessageChunk_CONTENT, Content: "Chunk 3", IsFinal: true},
			},
		}

		session, err := NewAgentSession(agentConfig, mockClient, 12345, mockQueue, db)
		assert.NoError(t, err)

		mockQueue.ExpectChunks(3)
		err = session.SendMessage("Test", nil)
		assert.NoError(t, err)

		// Wait for goroutine to complete
		mockQueue.Wait()

		// Verify all chunks were published
		assert.Equal(t, 3, len(mockQueue.streamData))

		// Verify chunk content
		var chunk1 AgentMessageChunk
		json.Unmarshal(mockQueue.streamData[0], &chunk1)
		assert.Equal(t, "CONTENT", chunk1.Type)
		assert.Equal(t, "Chunk 1", chunk1.Content)
		assert.False(t, chunk1.IsFinal)
	})

	t.Run("Handle chunk with metadata", func(t *testing.T) {
		mockQueue := NewMockMessageQueue()
		metadataJSON := `{"tool_id": "123", "execution_time": 1.5}`
		mockClient := &MockPluginServiceClient{
			chunks: []*pb.AgentMessageChunk{
				{
					Type:         pb.AgentMessageChunk_TOOL_CALL,
					Content:      "Calling tool",
					MetadataJson: metadataJSON,
					IsFinal:      true,
				},
			},
		}

		session, err := NewAgentSession(agentConfig, mockClient, 12345, mockQueue, db)
		assert.NoError(t, err)

		mockQueue.ExpectChunks(1)
		err = session.SendMessage("Test", nil)
		assert.NoError(t, err)

		mockQueue.Wait()

		assert.Equal(t, 1, len(mockQueue.streamData))

		var chunk AgentMessageChunk
		json.Unmarshal(mockQueue.streamData[0], &chunk)
		assert.NotNil(t, chunk.Metadata)
		assert.Equal(t, "123", chunk.Metadata["tool_id"])
	})

	t.Run("Stop after final chunk", func(t *testing.T) {
		mockQueue := NewMockMessageQueue()
		mockClient := &MockPluginServiceClient{
			chunks: []*pb.AgentMessageChunk{
				{Type: pb.AgentMessageChunk_CONTENT, Content: "First", IsFinal: false},
				{Type: pb.AgentMessageChunk_DONE, Content: "", IsFinal: true},
			},
		}

		session, err := NewAgentSession(agentConfig, mockClient, 12345, mockQueue, db)
		assert.NoError(t, err)

		mockQueue.ExpectChunks(2)
		err = session.SendMessage("Test", nil)
		assert.NoError(t, err)

		mockQueue.Wait()

		// Should only have 2 chunks (stops after IsFinal)
		assert.Equal(t, 2, len(mockQueue.streamData))
	})

	t.Run("Handle publish error", func(t *testing.T) {
		mockQueue := NewMockMessageQueue()
		mockQueue.publishErr = errors.New("queue full")

		mockClient := &MockPluginServiceClient{
			chunks: []*pb.AgentMessageChunk{
				{Type: pb.AgentMessageChunk_CONTENT, Content: "Test", IsFinal: true},
			},
		}

		session, err := NewAgentSession(agentConfig, mockClient, 12345, mockQueue, db)
		assert.NoError(t, err)

		mockQueue.ExpectChunks(1)
		err = session.SendMessage("Test", nil)
		assert.NoError(t, err)

		mockQueue.Wait()

		// Should not have published due to error
		assert.Equal(t, 0, len(mockQueue.streamData))
	})
}

func TestAgentSession_BuildAgentRequest(t *testing.T) {
	db := setupAgentSessionTest(t)
	agentConfig := createTestAgentConfig(t, db)
	mockQueue := NewMockMessageQueue()
	mockClient := &MockPluginServiceClient{}

	t.Run("Build request with all resources", func(t *testing.T) {
		session, err := NewAgentSession(agentConfig, mockClient, 12345, mockQueue, db)
		assert.NoError(t, err)

		history := []map[string]interface{}{
			{"role": "user", "content": "Hello"},
			{"role": "assistant", "content": "Hi there"},
		}

		req, err := session.buildAgentRequest("Test message", history)

		assert.NoError(t, err)
		assert.NotNil(t, req)
		assert.Equal(t, session.id, req.SessionId)
		assert.Equal(t, "Test message", req.UserMessage)
		assert.Equal(t, uint32(12345), req.ServiceBrokerId)

		// Verify tools (may be 0 or more depending on association loading)
		assert.GreaterOrEqual(t, len(req.AvailableTools), 0)
		if len(req.AvailableTools) > 0 {
			assert.Equal(t, "Test Tool", req.AvailableTools[0].Name)
		}

		// Verify datasources
		assert.GreaterOrEqual(t, len(req.AvailableDatasources), 0)
		if len(req.AvailableDatasources) > 0 {
			assert.Equal(t, "Test Datasource", req.AvailableDatasources[0].Name)
		}

		// Verify LLMs
		assert.GreaterOrEqual(t, len(req.AvailableLlms), 0)
		if len(req.AvailableLlms) > 0 {
			assert.Equal(t, "Test LLM", req.AvailableLlms[0].Name)
			assert.Equal(t, "OPENAI", req.AvailableLlms[0].Vendor)
		}

		// Verify config
		assert.NotEmpty(t, req.ConfigJson)

		// Verify history
		assert.Len(t, req.History, 2)
		assert.Equal(t, "user", req.History[0].Role)
		assert.Equal(t, "Hello", req.History[0].Content)

		// Verify context
		assert.NotNil(t, req.Context)
		assert.Equal(t, uint32(agentConfig.AppID), req.Context.AppId)
		assert.Equal(t, session.id, req.Context.Metadata["session_id"])
		assert.Equal(t, "12345", req.Context.Metadata["_service_broker_id"])
	})

	t.Run("Build request with empty history", func(t *testing.T) {
		session, err := NewAgentSession(agentConfig, mockClient, 12345, mockQueue, db)
		assert.NoError(t, err)

		req, err := session.buildAgentRequest("Test", nil)

		assert.NoError(t, err)
		assert.Len(t, req.History, 0)
	})

	t.Run("Handle invalid agent config ID", func(t *testing.T) {
		invalidConfig := &models.AgentConfig{ID: 99999}
		session, err := NewAgentSession(invalidConfig, mockClient, 12345, mockQueue, db)
		assert.NoError(t, err)

		_, err = session.buildAgentRequest("Test", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load agent config")
	})
}

func TestAgentSession_GetQueue(t *testing.T) {
	db := setupAgentSessionTest(t)
	agentConfig := createTestAgentConfig(t, db)
	mockQueue := NewMockMessageQueue()
	mockClient := &MockPluginServiceClient{}

	t.Run("Get queue returns correct instance", func(t *testing.T) {
		session, err := NewAgentSession(agentConfig, mockClient, 12345, mockQueue, db)
		assert.NoError(t, err)

		queue := session.GetQueue()
		assert.Equal(t, mockQueue, queue)
	})
}

func TestAgentSession_GetID(t *testing.T) {
	db := setupAgentSessionTest(t)
	agentConfig := createTestAgentConfig(t, db)
	mockQueue := NewMockMessageQueue()
	mockClient := &MockPluginServiceClient{}

	t.Run("Get ID returns session ID", func(t *testing.T) {
		session, err := NewAgentSession(agentConfig, mockClient, 12345, mockQueue, db)
		assert.NoError(t, err)

		id := session.GetID()
		assert.Equal(t, session.id, id)
		assert.NotEmpty(t, id)
	})
}

func TestAgentSession_Close(t *testing.T) {
	db := setupAgentSessionTest(t)
	agentConfig := createTestAgentConfig(t, db)
	mockQueue := NewMockMessageQueue()
	mockClient := &MockPluginServiceClient{}

	t.Run("Close session successfully", func(t *testing.T) {
		session, err := NewAgentSession(agentConfig, mockClient, 12345, mockQueue, db)
		assert.NoError(t, err)

		err = session.Close()
		assert.NoError(t, err)
		assert.True(t, mockQueue.closed)

		// Verify context is cancelled
		select {
		case <-session.ctx.Done():
			// Expected - context should be cancelled
		case <-time.After(100 * time.Millisecond):
			t.Error("Context should be cancelled after Close()")
		}
	})

	t.Run("Close cancels ongoing operations", func(t *testing.T) {
		mockClient := &MockPluginServiceClient{
			chunks: []*pb.AgentMessageChunk{
				{Type: pb.AgentMessageChunk_CONTENT, Content: "Chunk 1", IsFinal: false},
				{Type: pb.AgentMessageChunk_CONTENT, Content: "Chunk 2", IsFinal: false},
				{Type: pb.AgentMessageChunk_CONTENT, Content: "Chunk 3", IsFinal: true},
			},
		}

		session, err := NewAgentSession(agentConfig, mockClient, 12345, mockQueue, db)
		assert.NoError(t, err)

		// Start message sending
		err = session.SendMessage("Test", nil)
		assert.NoError(t, err)

		// Close immediately
		err = session.Close()
		assert.NoError(t, err)

		// Context should be cancelled
		assert.Error(t, session.ctx.Err())
	})
}

func TestAgentMessageChunk(t *testing.T) {
	t.Run("Marshal and unmarshal chunk", func(t *testing.T) {
		chunk := &AgentMessageChunk{
			Type:    "CONTENT",
			Content: "Test content",
			Metadata: map[string]interface{}{
				"key": "value",
			},
			IsFinal: true,
		}

		data, err := json.Marshal(chunk)
		assert.NoError(t, err)

		var decoded AgentMessageChunk
		err = json.Unmarshal(data, &decoded)
		assert.NoError(t, err)

		assert.Equal(t, chunk.Type, decoded.Type)
		assert.Equal(t, chunk.Content, decoded.Content)
		assert.Equal(t, chunk.IsFinal, decoded.IsFinal)
		assert.Equal(t, "value", decoded.Metadata["key"])
	})
}

func TestAgentSession_Integration(t *testing.T) {
	db := setupAgentSessionTest(t)
	agentConfig := createTestAgentConfig(t, db)

	t.Run("Full conversation flow", func(t *testing.T) {
		mockQueue := NewMockMessageQueue()
		mockClient := &MockPluginServiceClient{
			chunks: []*pb.AgentMessageChunk{
				{Type: pb.AgentMessageChunk_THINKING, Content: "Let me think...", IsFinal: false},
				{Type: pb.AgentMessageChunk_TOOL_CALL, Content: "Calling API", MetadataJson: `{"tool":"weather"}`, IsFinal: false},
				{Type: pb.AgentMessageChunk_TOOL_RESULT, Content: "Result received", IsFinal: false},
				{Type: pb.AgentMessageChunk_CONTENT, Content: "Here's your answer", IsFinal: false},
				{Type: pb.AgentMessageChunk_DONE, Content: "", IsFinal: true},
			},
		}

		session, err := NewAgentSession(agentConfig, mockClient, 12345, mockQueue, db)
		assert.NoError(t, err)

		// Send message
		history := []map[string]interface{}{
			{"role": "user", "content": "What's the weather?"},
		}
		mockQueue.ExpectChunks(5)
		err = session.SendMessage("Is it raining?", history)
		assert.NoError(t, err)

		// Wait for processing
		mockQueue.Wait()

		// Verify all chunks published
		assert.Equal(t, 5, len(mockQueue.streamData))

		// Verify chunk types
		var chunks []AgentMessageChunk
		for _, data := range mockQueue.streamData {
			var chunk AgentMessageChunk
			json.Unmarshal(data, &chunk)
			chunks = append(chunks, chunk)
		}

		assert.Equal(t, "THINKING", chunks[0].Type)
		assert.Equal(t, "TOOL_CALL", chunks[1].Type)
		assert.Equal(t, "TOOL_RESULT", chunks[2].Type)
		assert.Equal(t, "CONTENT", chunks[3].Type)
		assert.Equal(t, "DONE", chunks[4].Type)
		assert.True(t, chunks[4].IsFinal)

		// Close session
		err = session.Close()
		assert.NoError(t, err)
		assert.True(t, mockQueue.closed)
	})
}
