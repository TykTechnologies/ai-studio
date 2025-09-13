// examples/standalone-grpc-plugin/main.go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	pb "github.com/TykTechnologies/midsommar/microgateway/plugins/proto"
	"google.golang.org/grpc"
)

// MessageModifierPlugin modifies outbound LLM requests to add instructions
type MessageModifierPlugin struct {
	instruction string
}

// Initialize implements BasePlugin
func (p *MessageModifierPlugin) Initialize(config map[string]interface{}) error {
	if instruction, ok := config["instruction"]; ok {
		p.instruction = instruction.(string)
	} else {
		p.instruction = "Say Moo! at the end of your response"
	}
	log.Printf("MessageModifierPlugin initialized with instruction: %s", p.instruction)
	return nil
}

// GetHookType implements BasePlugin
func (p *MessageModifierPlugin) GetHookType() interfaces.HookType {
	return interfaces.HookTypePreAuth
}

// GetName implements BasePlugin
func (p *MessageModifierPlugin) GetName() string {
	return "standalone-message-modifier"
}

// GetVersion implements BasePlugin
func (p *MessageModifierPlugin) GetVersion() string {
	return "1.0.0"
}

// Shutdown implements BasePlugin
func (p *MessageModifierPlugin) Shutdown() error {
	log.Println("MessageModifierPlugin shutting down")
	return nil
}

// ProcessRequest implements PreAuthPlugin
func (p *MessageModifierPlugin) ProcessRequest(ctx context.Context, req *interfaces.PluginRequest, pluginCtx *interfaces.PluginContext) (*interfaces.PluginResponse, error) {
	log.Printf("Processing request: %s %s (App ID: %d)", req.Method, req.Path, pluginCtx.AppID)

	// Only modify POST requests to LLM endpoints
	if req.Method != "POST" {
		return &interfaces.PluginResponse{Modified: false}, nil
	}

	// Parse the JSON body
	var requestBody map[string]interface{}
	if err := json.Unmarshal(req.Body, &requestBody); err != nil {
		// If we can't parse JSON, don't modify
		log.Printf("Failed to parse JSON body: %v", err)
		return &interfaces.PluginResponse{Modified: false}, nil
	}

	// Check if this is a chat completion request
	messages, hasMessages := requestBody["messages"]
	if !hasMessages {
		log.Println("No messages field found, skipping modification")
		return &interfaces.PluginResponse{Modified: false}, nil
	}

	// Convert messages to slice of maps
	messageSlice, ok := messages.([]interface{})
	if !ok {
		log.Println("Messages field is not an array, skipping modification")
		return &interfaces.PluginResponse{Modified: false}, nil
	}

	// Add instruction to the last user message
	for i := len(messageSlice) - 1; i >= 0; i-- {
		messageMap, ok := messageSlice[i].(map[string]interface{})
		if !ok {
			continue
		}

		role, hasRole := messageMap["role"].(string)
		if hasRole && role == "user" {
			// Found the last user message, modify it
			content, hasContent := messageMap["content"].(string)
			if hasContent {
				originalContent := content
				messageMap["content"] = content + "\n\n" + p.instruction

				// Marshal the modified request body
				modifiedBody, err := json.Marshal(requestBody)
				if err != nil {
					log.Printf("Failed to marshal modified body: %v", err)
					return &interfaces.PluginResponse{Modified: false}, nil
				}

				log.Printf("Modified user message from %d chars to %d chars", len(originalContent), len(messageMap["content"].(string)))
				return &interfaces.PluginResponse{
					Modified: true,
					Headers:  map[string]string{"Content-Type": "application/json"},
					Body:     modifiedBody,
				}, nil
			}
			break
		}
	}

	// No modification needed
	log.Println("No user message found to modify")
	return &interfaces.PluginResponse{Modified: false}, nil
}

// StandaloneServer wraps the plugin for gRPC server operation
type StandaloneServer struct {
	pb.UnimplementedPluginServiceServer
	plugin *MessageModifierPlugin
}

// Initialize the plugin
func (s *StandaloneServer) Initialize(ctx context.Context, req *pb.InitRequest) (*pb.InitResponse, error) {
	log.Printf("Received initialization request with config: %v", req.Config)

	// Convert string config to interface{} map
	config := make(map[string]interface{})
	for k, v := range req.Config {
		config[k] = v
	}

	err := s.plugin.Initialize(config)
	if err != nil {
		return &pb.InitResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.InitResponse{
		Success: true,
	}, nil
}

// Ping responds to health checks
func (s *StandaloneServer) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	return &pb.PingResponse{
		Healthy:   true,
		Timestamp: req.Timestamp,
	}, nil
}

// Shutdown handles graceful shutdown
func (s *StandaloneServer) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Println("Received shutdown request")
	err := s.plugin.Shutdown()
	return &pb.ShutdownResponse{
		Success: err == nil,
	}, err
}

// ProcessPreAuth handles pre-auth plugin requests
func (s *StandaloneServer) ProcessPreAuth(ctx context.Context, req *pb.PluginRequest) (*pb.PluginResponse, error) {
	// Convert protobuf request to interfaces format
	pluginReq := &interfaces.PluginRequest{
		Method:  req.Method,
		Path:    req.Path,
		Headers: req.Headers,
		Body:    req.Body,
	}

	// Convert metadata from string map to interface map
	metadata := make(map[string]interface{})
	for k, v := range req.Context.Metadata {
		metadata[k] = v
	}

	pluginCtx := &interfaces.PluginContext{
		RequestID: req.Context.RequestId,
		LLMSlug:   req.Context.LlmSlug,
		LLMID:     uint(req.Context.LlmId),
		AppID:     uint(req.Context.AppId),
		UserID:    uint(req.Context.UserId),
		Metadata:  metadata,
	}

	// Call the plugin
	resp, err := s.plugin.ProcessRequest(ctx, pluginReq, pluginCtx)
	if err != nil {
		return &pb.PluginResponse{
			Modified: false,
			Block:    false,
			ErrorMessage: err.Error(),
		}, err
	}

	// Convert back to protobuf format
	return &pb.PluginResponse{
		Modified:     resp.Modified,
		Block:        resp.Block,
		Headers:      resp.Headers,
		Body:         resp.Body,
		ErrorMessage: resp.ErrorMessage,
	}, nil
}

func main() {
	var (
		port        = flag.String("port", "8080", "gRPC server port")
		instruction = flag.String("instruction", "Say Moo! at the end of your response", "Instruction to add to messages")
	)
	flag.Parse()

	// Create plugin instance
	plugin := &MessageModifierPlugin{
		instruction: *instruction,
	}

	// Initialize plugin with config
	config := map[string]interface{}{
		"instruction": *instruction,
	}
	if err := plugin.Initialize(config); err != nil {
		log.Fatalf("Failed to initialize plugin: %v", err)
	}

	// Create gRPC server
	lis, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", *port, err)
	}

	grpcServer := grpc.NewServer()
	server := &StandaloneServer{plugin: plugin}

	pb.RegisterPluginServiceServer(grpcServer, server)

	log.Printf("🚀 Standalone Message Modifier Plugin starting on port %s", *port)
	log.Printf("📝 Instruction: %s", *instruction)
	log.Printf("🔧 Plugin: %s v%s", plugin.GetName(), plugin.GetVersion())
	log.Printf("⚡ Hook Type: %s", plugin.GetHookType())

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("🛑 Received shutdown signal, stopping server...")
		grpcServer.GracefulStop()
	}()

	// Start serving
	fmt.Printf("✅ gRPC server listening on :%s\n", *port)
	fmt.Printf("📊 Test with: grpc://localhost:%s\n", *port)
	fmt.Println("🔄 Use Ctrl+C to stop")

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

	log.Println("👋 Server stopped")
}