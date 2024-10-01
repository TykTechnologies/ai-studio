package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/sashabaranov/go-openai"
)

func main() {
	// Command-line flags will be defined here
	// Main logic will be implemented here
	modelName := flag.String("model", "", "OpenAI model name (required)")
	prompt := flag.String("prompt", "", "Prompt to send to the model (required)")
	apiKey := flag.String("api-key", "", "OpenAI API key (optional, defaults to OPENAI_API_KEY env var)")
	apiEndpoint := flag.String("api-endpoint", "", "OpenAI API endpoint (optional)")

	flag.Parse()

	if *modelName == "" || *prompt == "" {
		fmt.Println("Error: model and prompt are required")
		flag.Usage()
		os.Exit(1)
	}

	if *apiKey == "" {
		*apiKey = os.Getenv("OPENAI_API_KEY")
		if *apiKey == "" {
			log.Fatal("Error: API key is required. Set it using the -api-key flag or OPENAI_API_KEY environment variable")
		}
	}

	config := openai.DefaultConfig(*apiKey)
	if *apiEndpoint != "" {
		config.BaseURL = *apiEndpoint
	}

	client := openai.NewClientWithConfig(config)

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
	fmt.Println(resp.Choices[0].Message.Content)
}
