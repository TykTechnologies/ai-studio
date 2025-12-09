package scripting

import (
	"github.com/tmc/langchaingo/llms"
)

// GetLastUserMessage extracts the content of the last user message
func GetLastUserMessage(messages []llms.MessageContent) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llms.ChatMessageTypeHuman {
			for _, part := range messages[i].Parts {
				if textPart, ok := part.(llms.TextContent); ok {
					return textPart.Text
				}
			}
		}
	}
	return ""
}

// GetSystemPrompt extracts the content of the system message
func GetSystemPrompt(messages []llms.MessageContent) string {
	for _, msg := range messages {
		if msg.Role == llms.ChatMessageTypeSystem {
			for _, part := range msg.Parts {
				if textPart, ok := part.(llms.TextContent); ok {
					return textPart.Text
				}
			}
		}
	}
	return ""
}

// GetAllUserMessages returns all user message contents
func GetAllUserMessages(messages []llms.MessageContent) []string {
	var userMessages []string
	for _, msg := range messages {
		if msg.Role == llms.ChatMessageTypeHuman {
			for _, part := range msg.Parts {
				if textPart, ok := part.(llms.TextContent); ok {
					userMessages = append(userMessages, textPart.Text)
				}
			}
		}
	}
	return userMessages
}

// GetMessagesByRole returns all messages with the specified role
func GetMessagesByRole(messages []llms.MessageContent, role llms.ChatMessageType) []string {
	var result []string
	for _, msg := range messages {
		if msg.Role == role {
			for _, part := range msg.Parts {
				if textPart, ok := part.(llms.TextContent); ok {
					result = append(result, textPart.Text)
				}
			}
		}
	}
	return result
}

// HasImages checks if any message contains image content
func HasImages(messages []llms.MessageContent) bool {
	for _, msg := range messages {
		for _, part := range msg.Parts {
			if _, ok := part.(llms.ImageURLContent); ok {
				return true
			}
		}
	}
	return false
}

// MessageCount returns the total number of messages
func MessageCount(messages []llms.MessageContent) int {
	return len(messages)
}

// GetAllText concatenates all text content from all messages
func GetAllText(messages []llms.MessageContent) string {
	var allText string
	for _, msg := range messages {
		for _, part := range msg.Parts {
			if textPart, ok := part.(llms.TextContent); ok {
				if allText != "" {
					allText += "\n"
				}
				allText += textPart.Text
			}
		}
	}
	return allText
}

// CountUserMessages counts the number of user messages
func CountUserMessages(messages []llms.MessageContent) int {
	count := 0
	for _, msg := range messages {
		if msg.Role == llms.ChatMessageTypeHuman {
			count++
		}
	}
	return count
}

// CountAssistantMessages counts the number of assistant messages
func CountAssistantMessages(messages []llms.MessageContent) int {
	count := 0
	for _, msg := range messages {
		if msg.Role == llms.ChatMessageTypeAI {
			count++
		}
	}
	return count
}
