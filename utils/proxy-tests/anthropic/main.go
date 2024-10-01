package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/liushuangls/go-anthropic/v2"
)

func main() {
	// Command-line flags
	modelName := flag.String("model", "", "Anthropic model name (required)")
	prompt := flag.String("prompt", "", "Prompt to send to the model (required)")
	apiKey := flag.String("api-key", "", "Anthropic API key (optional, defaults to ANTHROPIC_API_KEY env var)")
	apiEndpoint := flag.String("api-endpoint", "", "Anthropic API endpoint (optional)")
	streaming := flag.Bool("streaming", false, "Enable streaming mode")

	flag.Parse()

	if *modelName == "" || *prompt == "" {
		fmt.Println("Error: model and prompt are required")
		flag.Usage()
		os.Exit(1)
	}

	if *apiKey == "" {
		*apiKey = os.Getenv("ANTHROPIC_API_KEY")
		if *apiKey == "" {
			log.Fatal("Error: API key is required. Set it using the -api-key flag or ANTHROPIC_API_KEY environment variable")
		}
	}

	options := []anthropic.ClientOption{}
	if *apiEndpoint != "" {
		options = append(options, anthropic.WithBaseURL(*apiEndpoint))
	}
	client := anthropic.NewClient(*apiKey, options...)

	ctx := context.Background()

	model := anthropic.Model(*modelName)

	if *streaming {
		resp, err := client.CreateMessagesStream(ctx, anthropic.MessagesStreamRequest{
			MessagesRequest: anthropic.MessagesRequest{
				Model: model,
				Messages: []anthropic.Message{
					anthropic.NewUserTextMessage(*prompt),
				},
				MaxTokens: 1000,
			},
			OnContentBlockDelta: func(data anthropic.MessagesEventContentBlockDeltaData) {
				fmt.Print(*data.Delta.Text)
			},
		})
		if err != nil {
			var apiErr *anthropic.APIError
			if errors.As(err, &apiErr) {
				log.Fatalf("Anthropic API error: %s - %s", apiErr.Type, apiErr.Message)
			}
			log.Fatalf("Error creating streaming message: %v", err)
		}
		fmt.Println("\nFinal response:")
		fmt.Println(resp.Content[0].GetText())
	} else {
		resp, err := client.CreateMessages(ctx, anthropic.MessagesRequest{
			Model: model,
			Messages: []anthropic.Message{
				anthropic.NewUserTextMessage(*prompt),
			},
			MaxTokens: 1000,
		})
		if err != nil {
			var apiErr *anthropic.APIError
			if errors.As(err, &apiErr) {
				log.Fatalf("Anthropic API error: %s - %s", apiErr.Type, apiErr.Message)
			}
			log.Fatalf("Error creating message: %v", err)
		}
		fmt.Println("Response:")
		fmt.Println(resp.Content[0].GetText())
	}
}
