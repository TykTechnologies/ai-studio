package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"

	pb "github.com/TykTechnologies/midsommar/microgateway/plugins/proto"
	ai_studio_sdk "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
	"google.golang.org/grpc"
)

type EchoAgentPlugin struct {
	pb.UnimplementedPluginServiceServer
	pluginID        uint32
	prefix          string
	suffix          string
	includeMetadata bool
}

type Config struct {
	Prefix          string `json:"prefix"`
	Suffix          string `json:"suffix"`
	IncludeMetadata bool   `json:"include_metadata"`
}

func (p *EchoAgentPlugin) Initialize(ctx context.Context, req *pb.InitRequest) (*pb.InitResponse, error) {
	log.Println("EchoAgent: Initialize called")

	// Set defaults
	p.prefix = "<<"
	p.suffix = ">>"
	p.includeMetadata = false

	// Extract plugin ID from config and set it for SDK
	if pluginIDStr, ok := req.Config["plugin_id"]; ok {
		fmt.Sscanf(pluginIDStr, "%d", &p.pluginID)
		ai_studio_sdk.SetPluginID(p.pluginID)
		log.Printf("EchoAgent: Plugin ID set to %d", p.pluginID)
	}

	// Parse custom configuration
	if configJSON, ok := req.Config["config"]; ok {
		var config Config
		if err := json.Unmarshal([]byte(configJSON), &config); err == nil {
			if config.Prefix != "" {
				p.prefix = config.Prefix
			}
			if config.Suffix != "" {
				p.suffix = config.Suffix
			}
			p.includeMetadata = config.IncludeMetadata
			log.Printf("EchoAgent: Using custom config - prefix: %s, suffix: %s, metadata: %v",
				p.prefix, p.suffix, p.includeMetadata)
		}
	}

	return &pb.InitResponse{
		Success: true,
		Message: "EchoAgent initialized successfully",
	}, nil
}

func (p *EchoAgentPlugin) Ping(ctx context.Context, req *pb.Empty) (*pb.PingResponse, error) {
	return &pb.PingResponse{
		Status:  "healthy",
		Message: "EchoAgent is running",
	}, nil
}

func (p *EchoAgentPlugin) Shutdown(ctx context.Context, req *pb.Empty) (*pb.ShutdownResponse, error) {
	log.Println("EchoAgent: Shutdown called")
	return &pb.ShutdownResponse{
		Success: true,
		Message: "EchoAgent shutdown successfully",
	}, nil
}

func (p *EchoAgentPlugin) GetManifest(ctx context.Context, req *pb.Empty) (*pb.ManifestResponse, error) {
	manifest, err := os.ReadFile("plugin.manifest.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	return &pb.ManifestResponse{
		ManifestJson: string(manifest),
	}, nil
}

func (p *EchoAgentPlugin) GetConfigSchema(ctx context.Context, req *pb.GetConfigSchemaRequest) (*pb.GetConfigSchemaResponse, error) {
	schema, err := os.ReadFile("config.schema.json")
	if err != nil {
		return &pb.GetConfigSchemaResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("failed to read config schema: %v", err),
		}, nil
	}

	return &pb.GetConfigSchemaResponse{
		Success:    true,
		SchemaJson: string(schema),
	}, nil
}

func (p *EchoAgentPlugin) HandleAgentMessage(req *pb.AgentMessageRequest, stream grpc.ServerStreamingServer[pb.AgentMessageChunk]) error {
	log.Printf("EchoAgent: Received message: %s", req.UserMessage)

	// Check if we have any LLMs available
	if len(req.AvailableLlms) == 0 {
		return stream.Send(&pb.AgentMessageChunk{
			Type:    pb.AgentMessageChunk_ERROR,
			Content: "No LLMs available for this agent",
			IsFinal: true,
		})
	}

	// Use the first available LLM
	llmID := req.AvailableLlms[0].Id
	log.Printf("EchoAgent: Using LLM ID %d", llmID)

	// Call the LLM via SDK
	llmResponse, err := ai_studio_sdk.CallLLM(context.Background(), llmID, req.UserMessage, nil)
	if err != nil {
		log.Printf("EchoAgent: Error calling LLM: %v", err)
		return stream.Send(&pb.AgentMessageChunk{
			Type:    pb.AgentMessageChunk_ERROR,
			Content: fmt.Sprintf("Failed to call LLM: %v", err),
			IsFinal: true,
		})
	}

	// Parse the LLM response
	var responseData struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(llmResponse, &responseData); err != nil {
		log.Printf("EchoAgent: Error parsing LLM response: %v", err)
		return stream.Send(&pb.AgentMessageChunk{
			Type:    pb.AgentMessageChunk_ERROR,
			Content: fmt.Sprintf("Failed to parse LLM response: %v", err),
			IsFinal: true,
		})
	}

	// Extract content
	var content string
	if len(responseData.Choices) > 0 {
		content = responseData.Choices[0].Message.Content
	} else {
		content = "No response from LLM"
	}

	// Wrap the content with configured prefix/suffix
	wrappedContent := fmt.Sprintf("%s %s %s", p.prefix, content, p.suffix)
	log.Printf("EchoAgent: Sending wrapped response: %s", wrappedContent)

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

func main() {
	port := os.Getenv("PLUGIN_PORT")
	if port == "" {
		port = "50051"
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterPluginServiceServer(grpcServer, &EchoAgentPlugin{})

	log.Printf("EchoAgent plugin server listening on port %s", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
