package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	mgmt "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
)

//go:embed plugin.manifest.json
var manifestFile []byte

//go:embed config.schema.json
var configSchemaFile []byte

const (
	PluginName    = "echo-agent"
	PluginVersion = "1.0.0"
)

type Config struct {
	Prefix          string `json:"prefix"`
	Suffix          string `json:"suffix"`
	IncludeMetadata bool   `json:"include_metadata"`
	DefaultLLMID    uint32 `json:"default_llm_id"`
	Model           string `json:"model"`
}

type EchoAgentPlugin struct {
	plugin_sdk.BasePlugin
	prefix          string
	suffix          string
	includeMetadata bool
	defaultLLMID    uint32
	model           string
}

// NewEchoAgentPlugin creates a new echo agent plugin
func NewEchoAgentPlugin() *EchoAgentPlugin {
	return &EchoAgentPlugin{
		BasePlugin:      plugin_sdk.NewBasePlugin(PluginName, PluginVersion, "Echo Agent with LLM wrapping"),
		prefix:          "<<",
		suffix:          ">>",
		includeMetadata: false,
		defaultLLMID:    0,
	}
}

// Initialize implements plugin_sdk.Plugin
func (p *EchoAgentPlugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
	log.Printf("🤖 %s: Initialized in %s runtime", PluginName, ctx.Runtime)

	// Note: Broker ID is now handled automatically by the SDK via OpenSession.
	// Service API connections are established in OnSessionReady.

	// Parse config for prefix, suffix, etc.
	if prefix, ok := config["prefix"]; ok {
		p.prefix = prefix
		log.Printf("🤖 %s: Using config prefix: %s", PluginName, p.prefix)
	}

	if suffix, ok := config["suffix"]; ok {
		p.suffix = suffix
		log.Printf("🤖 %s: Using config suffix: %s", PluginName, p.suffix)
	}

	if metadataStr, ok := config["include_metadata"]; ok {
		p.includeMetadata = (metadataStr == "true")
		log.Printf("🤖 %s: Using config include_metadata: %v", PluginName, p.includeMetadata)
	}

	if llmIDStr, ok := config["default_llm_id"]; ok {
		var llmID uint64
		if _, err := fmt.Sscanf(llmIDStr, "%d", &llmID); err == nil {
			p.defaultLLMID = uint32(llmID)
			log.Printf("🤖 %s: Using config default_llm_id: %d", PluginName, p.defaultLLMID)
		}
	}

	if modelStr, ok := config["model"]; ok {
		p.model = modelStr
		log.Printf("🤖 %s: Using config model: %s", PluginName, p.model)
	}

	log.Printf("🤖 %s: Initialized successfully - prefix=%s, suffix=%s, metadata=%v, llmID=%d, model=%s",
		PluginName, p.prefix, p.suffix, p.includeMetadata, p.defaultLLMID, p.model)
	return nil
}

// Shutdown implements plugin_sdk.Plugin
func (p *EchoAgentPlugin) Shutdown(ctx plugin_sdk.Context) error {
	log.Printf("🤖 %s: Shutdown called", PluginName)
	return nil
}

// OnSessionReady implements plugin_sdk.SessionAware - called when the session broker is ready
func (p *EchoAgentPlugin) OnSessionReady(ctx plugin_sdk.Context) {
	log.Printf("🤖 %s: OnSessionReady called - session broker is now active", PluginName)

	// Warm up the Service API connection by making a simple call.
	// This establishes the broker connection early, avoiding timeout errors on first LLM call.
	if ai_studio_sdk.IsInitialized() {
		log.Printf("🤖 %s: Warming up Service API connection...", PluginName)
		_, err := ai_studio_sdk.GetPluginsCount(context.Background())
		if err != nil {
			log.Printf("🤖 %s: Service API warmup failed: %v (this may be OK if not in Studio runtime)", PluginName, err)
		} else {
			log.Printf("🤖 %s: Service API connection established successfully", PluginName)
		}
	} else {
		log.Printf("🤖 %s: SDK not initialized yet, skipping warmup", PluginName)
	}
}

// OnSessionClosing implements plugin_sdk.SessionAware - called when the session is closing
func (p *EchoAgentPlugin) OnSessionClosing(ctx plugin_sdk.Context) {
	log.Printf("🤖 %s: OnSessionClosing called - cleaning up session resources", PluginName)
}

// GetManifest implements plugin_sdk.UIProvider
func (p *EchoAgentPlugin) GetManifest() ([]byte, error) {
	return manifestFile, nil
}

// GetConfigSchema implements plugin_sdk.ConfigProvider
func (p *EchoAgentPlugin) GetConfigSchema() ([]byte, error) {
	return configSchemaFile, nil
}

// ListAssets implements plugin_sdk.UIProvider
func (p *EchoAgentPlugin) ListAssets(pathPrefix string) ([]*pb.AssetInfo, error) {
	return []*pb.AssetInfo{}, nil
}

// HandleAgentMessage implements plugin_sdk.AgentPlugin
func (p *EchoAgentPlugin) HandleAgentMessage(req *pb.AgentMessageRequest, stream pb.PluginService_HandleAgentMessageServer) error {
	log.Printf("EchoAgent: Received message: %s", req.UserMessage)
	log.Printf("EchoAgent: Available LLMs: %d", len(req.AvailableLlms))

	// Set service broker ID if provided (needed for LLM calls via service API)
	if req.ServiceBrokerId != 0 {
		ai_studio_sdk.SetServiceBrokerID(req.ServiceBrokerId)
		log.Printf("EchoAgent: Set service broker ID: %d", req.ServiceBrokerId)
	}

	// Parse config from request if present
	if req.ConfigJson != "" {
		var config Config
		if err := json.Unmarshal([]byte(req.ConfigJson), &config); err == nil {
			if config.Prefix != "" {
				p.prefix = config.Prefix
			}
			if config.Suffix != "" {
				p.suffix = config.Suffix
			}
			p.includeMetadata = config.IncludeMetadata
			p.defaultLLMID = config.DefaultLLMID
			if config.Model != "" {
				p.model = config.Model
			}
			log.Printf("EchoAgent: Using custom config - prefix: %s, suffix: %s, metadata: %v, default_llm_id: %d, model: %s",
				p.prefix, p.suffix, p.includeMetadata, p.defaultLLMID, p.model)
		}
	}

	// Select LLM to use
	var selectedLLM *pb.AgentLLMInfo
	if p.defaultLLMID > 0 {
		// Use configured default
		for _, llm := range req.AvailableLlms {
			if llm.Id == p.defaultLLMID {
				selectedLLM = llm
				break
			}
		}
		if selectedLLM == nil {
			log.Printf("EchoAgent: WARNING - Configured LLM ID %d not found, using first available", p.defaultLLMID)
		}
	}

	// Fall back to first available LLM
	if selectedLLM == nil && len(req.AvailableLlms) > 0 {
		selectedLLM = req.AvailableLlms[0]
	}

	// If no LLM available, fall back to echo mode
	if selectedLLM == nil {
		log.Println("EchoAgent: No LLM available, using echo mode")
		return p.echoMode(req.UserMessage, stream)
	}

	log.Printf("EchoAgent: Using LLM: %s (ID: %d, Vendor: %s, Model: %s)",
		selectedLLM.Name, selectedLLM.Id, selectedLLM.Vendor, selectedLLM.DefaultModel)

	// Call LLM via SDK helper
	return p.callLLM(req, selectedLLM, p.model, stream)
}

// echoMode is the fallback mode that just echoes the message
func (p *EchoAgentPlugin) echoMode(userMessage string, stream pb.PluginService_HandleAgentMessageServer) error {
	wrappedContent := fmt.Sprintf("%s %s %s", p.prefix, userMessage, p.suffix)
	log.Printf("EchoAgent: Sending wrapped echo response: %s", wrappedContent)

	// Send content chunk
	if err := stream.Send(&pb.AgentMessageChunk{
		Type:    pb.AgentMessageChunk_CONTENT,
		Content: wrappedContent,
		IsFinal: false,
	}); err != nil {
		return err
	}

	// Send done chunk
	return stream.Send(&pb.AgentMessageChunk{
		Type:    pb.AgentMessageChunk_DONE,
		Content: "completed",
		IsFinal: true,
	})
}

// callLLM calls the LLM via the SDK and streams back the wrapped response
func (p *EchoAgentPlugin) callLLM(req *pb.AgentMessageRequest, llm *pb.AgentLLMInfo, model string, stream pb.PluginService_HandleAgentMessageServer) error {
	ctx := stream.Context()

	// Build LLM messages from user message and history
	messages := []*mgmt.LLMMessage{}

	// Add history
	for _, histMsg := range req.History {
		messages = append(messages, &mgmt.LLMMessage{
			Role:    histMsg.Role,
			Content: histMsg.Content,
		})
	}

	// Add current user message
	messages = append(messages, &mgmt.LLMMessage{
		Role:    "user",
		Content: req.UserMessage,
	})

	// Determine which model to use
	modelToUse := model
	if modelToUse == "" {
		// Fallback to LLM's default model
		modelToUse = llm.DefaultModel
		log.Printf("EchoAgent: No model specified, using LLM default: %s", modelToUse)
	} else {
		log.Printf("EchoAgent: Using configured model: %s", modelToUse)
	}

	log.Printf("EchoAgent: Calling LLM %d with %d messages via SDK", llm.Id, len(messages))

	// Use SDK's CallLLM helper to call the LLM proxy
	llmStream, err := ai_studio_sdk.CallLLM(
		ctx,
		llm.Id,
		modelToUse,
		messages,
		0.7,  // temperature
		1000, // max tokens
		nil,  // no tools
		false, // non-streaming
	)
	if err != nil {
		log.Printf("EchoAgent: Failed to call LLM via SDK: %v", err)
		return stream.Send(&pb.AgentMessageChunk{
			Type:    pb.AgentMessageChunk_ERROR,
			Content: fmt.Sprintf("Failed to call LLM: %v", err),
			IsFinal: true,
		})
	}

	// Receive response from LLM
	var llmContent string
	for {
		resp, err := llmStream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("EchoAgent: Error receiving from LLM: %v", err)
			return stream.Send(&pb.AgentMessageChunk{
				Type:    pb.AgentMessageChunk_ERROR,
				Content: fmt.Sprintf("Error receiving LLM response: %v", err),
				IsFinal: true,
			})
		}

		if !resp.Success {
			log.Printf("EchoAgent: LLM returned error: %s", resp.ErrorMessage)
			return stream.Send(&pb.AgentMessageChunk{
				Type:    pb.AgentMessageChunk_ERROR,
				Content: fmt.Sprintf("LLM error: %s", resp.ErrorMessage),
				IsFinal: true,
			})
		}

		llmContent += resp.Content

		if resp.Done {
			break
		}
	}

	log.Printf("EchoAgent: Received LLM response (%d chars)", len(llmContent))

	// Wrap LLM response with prefix/suffix
	wrappedContent := fmt.Sprintf("%s %s %s", p.prefix, llmContent, p.suffix)

	// Send wrapped content
	if err := stream.Send(&pb.AgentMessageChunk{
		Type:    pb.AgentMessageChunk_CONTENT,
		Content: wrappedContent,
		IsFinal: false,
	}); err != nil {
		return err
	}

	// Send done chunk
	return stream.Send(&pb.AgentMessageChunk{
		Type:    pb.AgentMessageChunk_DONE,
		Content: "completed",
		IsFinal: true,
	})
}

func main() {
	log.Printf("🤖 Starting %s Plugin v%s", PluginName, PluginVersion)
	log.Printf("Agent plugin with LLM wrapping using unified SDK")

	plugin := NewEchoAgentPlugin()
	plugin_sdk.Serve(plugin)
}
