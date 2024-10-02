package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/sashabaranov/go-openai"
)

func handleStreamingResponse(stream *openai.ChatCompletionStream) {
	defer stream.Close()

	fmt.Println("Streaming Response:")
	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			log.Printf("Error receiving stream response: %v", err)
			return
		}
		fmt.Print(response.Choices[0].Delta.Content)
	}
}

func main() {
	// Command-line flags will be defined here
	// Main logic will be implemented here
	modelName := flag.String("model", "", "model name (required)")
	prompt := flag.String("prompt", "", "Prompt to send to the model (required)")
	apiKey := flag.String("api-key", "", "API key (optional, defaults to OPENAI_API_KEY env var)")
	apiEndpoint := flag.String("api-endpoint", "", "API endpoint (optional)")

	streaming := flag.Bool("streaming", false, "Enable streaming mode")

	flag.Parse()

	if *modelName == "" || *prompt == "" {
		fmt.Println("Error: model and prompt are required")
		flag.Usage()
		os.Exit(1)
	}

	if *apiKey == "" {
		log.Fatal("Error: API key is required. Set it using the -api-key flag or OPENAI_API_KEY environment variable")
	}

	config := openai.DefaultConfig(*apiKey)
	if *apiEndpoint != "" {
		config.BaseURL = *apiEndpoint
	}

	client := openai.NewClientWithConfig(config)
	if *streaming {
		stream, err := client.CreateChatCompletionStream(
			context.Background(),
			openai.ChatCompletionRequest{
				StreamOptions: &openai.StreamOptions{IncludeUsage: true},
				Model:         *modelName,
				Messages: []openai.ChatCompletionMessage{
					{
						Role:    openai.ChatMessageRoleUser,
						Content: *prompt,
					},
				},
			},
		)
		if err != nil {
			log.Fatalf("Error creating chat completion stream: %v", err)
		}
		defer stream.Close()

		fmt.Println("Streaming Response:")
		for {
			response, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				log.Printf("Error receiving stream response: %v", err)
				return
			}
			if len(response.Choices) > 0 {
				fmt.Print(response.Choices[0].Delta.Content)
			}
		}
	} else {
		resp, err := client.CreateChatCompletion(
			context.Background(),
			openai.ChatCompletionRequest{
				Model: *modelName,
				Messages: []openai.ChatCompletionMessage{
					{
						Role:    openai.ChatMessageRoleUser,
						Content: *prompt,
					},
				},
			},
		)

		if err != nil {
			log.Fatalf("Error creating chat completion: %v", err)
		}

		fmt.Println("Response:")
		if len(resp.Choices) > 0 {
			fmt.Println(resp.Choices[0].Message.Content)
		}
	}
}
