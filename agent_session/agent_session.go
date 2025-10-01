package agent_session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/gofrs/uuid"
	"github.com/gosimple/slug"
	"gorm.io/gorm"
)

// MessageQueue is a minimal interface for publishing agent responses
// This avoids importing chat_session to prevent circular dependencies
type MessageQueue interface {
	PublishStream(ctx context.Context, data []byte) error
	PublishError(ctx context.Context, err error) error
	ConsumeStream(ctx context.Context) <-chan []byte
	ConsumeErrors(ctx context.Context) <-chan error
	Close() error
}

// AgentSession manages the runtime lifecycle of an agent plugin conversation
type AgentSession struct {
	id           string
	agentConfig  *models.AgentConfig
	queue        MessageQueue
	pluginClient pb.PluginServiceClient
	db           *gorm.DB
	ctx          context.Context
	cancel       context.CancelFunc
}

// AgentMessageChunk represents a chunk of agent response
type AgentMessageChunk struct {
	Type     string                 `json:"type"`     // CONTENT, TOOL_CALL, TOOL_RESULT, THINKING, ERROR, DONE
	Content  string                 `json:"content"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	IsFinal  bool                   `json:"is_final"`
}

// NewAgentSession creates a new agent session
func NewAgentSession(
	agentConfig *models.AgentConfig,
	pluginClient pb.PluginServiceClient,
	queue MessageQueue,
	db *gorm.DB,
) (*AgentSession, error) {
	// Generate session ID
	uid, _ := uuid.NewV4()
	id := uid.String()

	// Create context with cancel
	ctx, cancel := context.WithCancel(context.Background())

	as := &AgentSession{
		id:           id,
		agentConfig:  agentConfig,
		queue:        queue,
		pluginClient: pluginClient,
		db:           db,
		ctx:          ctx,
		cancel:       cancel,
	}

	return as, nil
}

// SendMessage sends a user message to the agent plugin and streams responses back
func (as *AgentSession) SendMessage(userMessage string, history []map[string]interface{}) error {
	// Build context from App resources
	req, err := as.buildAgentRequest(userMessage, history)
	if err != nil {
		return fmt.Errorf("failed to build agent request: %w", err)
	}

	// Call plugin HandleAgentMessage via gRPC
	stream, err := as.pluginClient.HandleAgentMessage(as.ctx, req)
	if err != nil {
		return fmt.Errorf("failed to call plugin HandleAgentMessage: %w", err)
	}

	// Start goroutine to receive chunks and forward to queue
	go as.receiveChunks(stream)

	return nil
}

// receiveChunks receives chunks from plugin gRPC stream and forwards to message queue
func (as *AgentSession) receiveChunks(stream pb.PluginService_HandleAgentMessageClient) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in receiveChunks", "error", r, "session_id", as.id)
			as.queue.PublishError(as.ctx, fmt.Errorf("panic: %v", r))
		}
	}()

	for {
		chunk, err := stream.Recv()
		if err != nil {
			// Stream ended or error occurred
			if err.Error() == "EOF" {
				slog.Debug("agent stream completed", "session_id", as.id)
			} else {
				slog.Error("error receiving chunk", "error", err, "session_id", as.id)
				as.queue.PublishError(as.ctx, err)
			}
			return
		}

		// Convert proto chunk to internal format and publish
		agentChunk := &AgentMessageChunk{
			Type:    chunk.GetType().String(),
			Content: chunk.GetContent(),
			IsFinal: chunk.GetIsFinal(),
		}

		// Parse metadata JSON if present
		if chunk.GetMetadataJson() != "" {
			var metadata map[string]interface{}
			if err := json.Unmarshal([]byte(chunk.GetMetadataJson()), &metadata); err == nil {
				agentChunk.Metadata = metadata
			}
		}

		// Publish as stream chunk (raw bytes for compatibility with chat_session queue)
		chunkJSON, err := json.Marshal(agentChunk)
		if err != nil {
			slog.Error("failed to marshal agent chunk", "error", err, "session_id", as.id)
			continue
		}

		ctx, cancel := context.WithTimeout(as.ctx, 5*time.Second)
		if err := as.queue.PublishStream(ctx, chunkJSON); err != nil {
			cancel()
			slog.Error("failed to publish chunk to queue", "error", err, "session_id", as.id)
			return
		}
		cancel()

		// If this is the final chunk, we're done
		if chunk.GetIsFinal() {
			slog.Debug("received final chunk", "session_id", as.id)
			return
		}
	}
}

// buildAgentRequest builds the gRPC request with context from App resources
func (as *AgentSession) buildAgentRequest(userMessage string, history []map[string]interface{}) (*pb.AgentMessageRequest, error) {
	// Load AgentConfig with all preloaded App resources
	var agentConfig models.AgentConfig
	if err := as.db.
		Preload("App.LLMs").
		Preload("App.Tools").
		Preload("App.Datasources").
		Preload("Plugin").
		First(&agentConfig, as.agentConfig.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to load agent config: %w", err)
	}

	// Convert tools to proto format
	availableTools := make([]*pb.AgentToolInfo, 0, len(agentConfig.App.Tools))
	for _, tool := range agentConfig.App.Tools {
		availableTools = append(availableTools, &pb.AgentToolInfo{
			Id:          uint32(tool.ID),
			Name:        tool.Name,
			Slug:        slug.Make(tool.Name), // Generate slug from name
			Description: tool.Description,
		})
	}

	// Convert datasources to proto format
	availableDatasources := make([]*pb.AgentDatasourceInfo, 0, len(agentConfig.App.Datasources))
	for _, ds := range agentConfig.App.Datasources {
		availableDatasources = append(availableDatasources, &pb.AgentDatasourceInfo{
			Id:           uint32(ds.ID),
			Name:         ds.Name,
			Description:  ds.ShortDescription, // Use short description
			DbSourceType: ds.DBSourceType,
		})
	}

	// Convert LLMs to proto format
	availableLLMs := make([]*pb.AgentLLMInfo, 0, len(agentConfig.App.LLMs))
	for _, llm := range agentConfig.App.LLMs {
		availableLLMs = append(availableLLMs, &pb.AgentLLMInfo{
			Id:           uint32(llm.ID),
			Name:         llm.Name,
			Vendor:       string(llm.Vendor),
			DefaultModel: llm.DefaultModel,
		})
	}

	// Convert config to JSON
	configJSON, err := json.Marshal(agentConfig.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal agent config: %w", err)
	}

	// Convert history to proto format
	protoHistory := make([]*pb.AgentConversationMessage, 0, len(history))
	for _, msg := range history {
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)
		protoHistory = append(protoHistory, &pb.AgentConversationMessage{
			Role:    role,
			Content: content,
		})
	}

	// Build plugin context
	pluginContext := &pb.PluginContext{
		AppId: uint32(agentConfig.App.ID),
		Metadata: map[string]string{
			"agent_config_id": fmt.Sprintf("%d", agentConfig.ID),
			"plugin_id":       fmt.Sprintf("%d", agentConfig.PluginID),
			"session_id":      as.id,
		},
	}

	return &pb.AgentMessageRequest{
		SessionId:            as.id,
		UserMessage:          userMessage,
		AvailableTools:       availableTools,
		AvailableDatasources: availableDatasources,
		AvailableLlms:        availableLLMs,
		ConfigJson:           string(configJSON),
		History:              protoHistory,
		Context:              pluginContext,
	}, nil
}

// GetQueue returns the message queue for consuming responses
func (as *AgentSession) GetQueue() MessageQueue {
	return as.queue
}

// GetID returns the session ID
func (as *AgentSession) GetID() string {
	return as.id
}

// Close closes the agent session and cleans up resources
func (as *AgentSession) Close() error {
	slog.Debug("closing agent session", "session_id", as.id)

	// Cancel context to stop any ongoing operations
	as.cancel()

	// Close queue
	if err := as.queue.Close(); err != nil {
		slog.Error("failed to close queue", "error", err, "session_id", as.id)
		return err
	}

	return nil
}
