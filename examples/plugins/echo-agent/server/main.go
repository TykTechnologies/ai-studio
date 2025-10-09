package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	mgmt "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
)

//go:embed plugin.manifest.json
var manifestFile []byte

//go:embed config.schema.json
var configSchemaFile []byte

type EchoAgentPlugin struct {
	serviceAPI      mgmt.AIStudioManagementServiceClient
	pluginID        uint32
	prefix          string
	suffix          string
	includeMetadata bool
	defaultLLMID    uint32
}

type Config struct {
	Prefix          string `json:"prefix"`
	Suffix          string `json:"suffix"`
	IncludeMetadata bool   `json:"include_metadata"`
	DefaultLLMID    uint32 `json:"default_llm_id"`
}

// OnInitialize is called when the plugin starts up
func (p *EchoAgentPlugin) OnInitialize(serviceAPI mgmt.AIStudioManagementServiceClient, pluginID uint32) error {
	log.Printf("🤖 EchoAgent: Initialize called with plugin ID %d", pluginID)

	p.serviceAPI = serviceAPI
	p.pluginID = pluginID

	// Set defaults
	p.prefix = "<<"
	p.suffix = ">>"
	p.includeMetadata = false
	p.defaultLLMID = 0 // 0 means use first available

	log.Println("✅ EchoAgent: Initialized successfully with SDK")
	return nil
}

// OnShutdown is called when the plugin is shutting down
func (p *EchoAgentPlugin) OnShutdown() error {
	log.Println("EchoAgent: Shutdown called")
	return nil
}

// GetManifest returns the plugin manifest
func (p *EchoAgentPlugin) GetManifest() ([]byte, error) {
	return manifestFile, nil
}

// GetConfigSchema returns the configuration schema
func (p *EchoAgentPlugin) GetConfigSchema() ([]byte, error) {
	return configSchemaFile, nil
}

// HandleAgentMessage processes incoming agent messages
func (p *EchoAgentPlugin) HandleAgentMessage(req *pb.AgentMessageRequest, stream pb.PluginService_HandleAgentMessageServer) error {
	log.Printf("EchoAgent: Received message: %s", req.UserMessage)
	log.Printf("EchoAgent: Available LLMs: %d", len(req.AvailableLlms))

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
			log.Printf("EchoAgent: Using custom config - prefix: %s, suffix: %s, metadata: %v, default_llm_id: %d",
				p.prefix, p.suffix, p.includeMetadata, p.defaultLLMID)
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
	return p.callLLM(req, selectedLLM, stream)
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
func (p *EchoAgentPlugin) callLLM(req *pb.AgentMessageRequest, llm *pb.AgentLLMInfo, stream pb.PluginService_HandleAgentMessageServer) error {
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

	log.Printf("EchoAgent: Calling LLM %d with %d messages via SDK", llm.Id, len(messages))

	// Use SDK's CallLLM helper to call the LLM proxy
	llmStream, err := ai_studio_sdk.CallLLM(
		ctx,
		llm.Id,
		llm.DefaultModel,
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
	log.Printf("🤖 Starting Echo Agent Plugin with AI Studio SDK")

	// Create plugin implementation
	plugin := &EchoAgentPlugin{}

	// Use SDK's ServeAgentPlugin helper - this handles all the go-plugin boilerplate
	ai_studio_sdk.ServeAgentPlugin(plugin)
}
