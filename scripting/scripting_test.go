package scripting

import (
	"testing"

	"github.com/tmc/langchaingo/llms"
)

// Type alias for convenience in tests
type MessageContent = llms.MessageContent

func TestRunScript(t *testing.T) {
	tests := []struct {
		name          string
		sourceCode    string
		input         *ScriptInput
		wantBlock     bool
		wantPayload   string
		wantMessage   string
		wantErr       bool
		errMsg        string
	}{
		{
			name: "pass through - no blocking, no modification",
			sourceCode: `
				output := {
					block: false,
					payload: input.raw_input,
					message: ""
				}
			`,
			input: &ScriptInput{
				RawInput:   `{"test": "data"}`,
				Messages:   []MessageContent{},
				VendorName: "openai",
				ModelName:  "gpt-4",
			},
			wantBlock:   false,
			wantPayload: `{"test": "data"}`,
			wantMessage: "",
			wantErr:     false,
		},
		{
			name: "block request",
			sourceCode: `
				output := {
					block: true,
					payload: "",
					message: "Blocked by policy"
				}
			`,
			input: &ScriptInput{
				RawInput:   `{"prompt": "restricted"}`,
				Messages:   []MessageContent{},
				VendorName: "anthropic",
				ModelName:  "claude-3",
			},
			wantBlock:   true,
			wantPayload: "",
			wantMessage: "Blocked by policy",
			wantErr:     false,
		},
		{
			name: "modify payload",
			sourceCode: `
				output := {
					block: false,
					payload: "[REDACTED]",
					message: "Content redacted"
				}
			`,
			input: &ScriptInput{
				RawInput:   `{"prompt": "secret info"}`,
				Messages:   []MessageContent{},
				VendorName: "openai",
				ModelName:  "gpt-4",
			},
			wantBlock:   false,
			wantPayload: "[REDACTED]",
			wantMessage: "Content redacted",
			wantErr:     false,
		},
		{
			name: "access vendor and model metadata",
			sourceCode: `
				// Block if not using OpenAI gpt-4
				should_block := input.vendor_name != "openai" || input.model_name != "gpt-4"

				output := {
					block: should_block,
					payload: input.raw_input,
					message: should_block ? "Only OpenAI GPT-4 allowed" : ""
				}
			`,
			input: &ScriptInput{
				RawInput:   `{"test": "data"}`,
				Messages:   []MessageContent{},
				VendorName: "anthropic",
				ModelName:  "claude-3",
			},
			wantBlock:   true,
			wantPayload: `{"test": "data"}`,
			wantMessage: "Only OpenAI GPT-4 allowed",
			wantErr:     false,
		},
		{
			name: "access context metadata",
			sourceCode: `
				// Block if app_id is missing
				has_app_id := false
				if input.context && input.context.app_id {
					has_app_id = true
				}

				output := {
					block: !has_app_id,
					payload: input.raw_input,
					message: has_app_id ? "" : "Missing app_id"
				}
			`,
			input: &ScriptInput{
				RawInput:   `{"test": "data"}`,
				Messages:   []MessageContent{},
				VendorName: "openai",
				ModelName:  "gpt-4",
				Context: map[string]interface{}{
					"app_id":  int64(123),
					"user_id": int64(456),
				},
			},
			wantBlock:   false,
			wantPayload: `{"test": "data"}`,
			wantMessage: "",
			wantErr:     false,
		},
		{
			name: "access message arrays with roles",
			sourceCode: `
				text := import("text")

				// Count messages by role
				user_count := 0
				system_count := 0

				for msg in input.messages {
					if msg.role == "user" {
						user_count = user_count + 1
					} else if msg.role == "system" {
						system_count = system_count + 1
					}
				}

				output := {
					block: user_count == 0,
					payload: input.raw_input,
					message: user_count == 0 ? "No user messages found" : ""
				}
			`,
			input: &ScriptInput{
				RawInput:   `{"messages": [{"role": "user", "content": "Hello"}]}`,
				Messages: []MessageContent{
					{
						Role:  llms.ChatMessageTypeSystem,
						Parts: []llms.ContentPart{llms.TextPart("You are helpful")},
					},
					{
						Role:  llms.ChatMessageTypeHuman,
						Parts: []llms.ContentPart{llms.TextPart("Hello world")},
					},
				},
				VendorName: "openai",
				ModelName:  "gpt-4",
			},
			wantBlock:   false,
			wantPayload: `{"messages": [{"role": "user", "content": "Hello"}]}`,
			wantMessage: "",
			wantErr:     false,
		},
		{
			name: "filter messages by content - block on email",
			sourceCode: `
				text := import("text")

				should_block := false
				block_reason := ""

				for msg in input.messages {
					if msg.role == "user" {
						if text.contains(msg.content, "@") {
							should_block = true
							block_reason = "Email addresses not allowed"
						}
					}
				}

				output := {
					block: should_block,
					payload: input.raw_input,
					message: block_reason
				}
			`,
			input: &ScriptInput{
				RawInput:   `{"messages": [{"role": "user", "content": "Contact me@example.com"}]}`,
				Messages: []MessageContent{
					{
						Role:  llms.ChatMessageTypeHuman,
						Parts: []llms.ContentPart{llms.TextPart("Contact me@example.com")},
					},
				},
				VendorName: "anthropic",
				ModelName:  "claude-3",
			},
			wantBlock:   true,
			wantPayload: `{"messages": [{"role": "user", "content": "Contact me@example.com"}]}`,
			wantMessage: "Email addresses not allowed",
			wantErr:     false,
		},
		{
			name: "filter messages by content - allow clean message",
			sourceCode: `
				text := import("text")

				should_block := false

				for msg in input.messages {
					if msg.role == "user" {
						if text.contains(msg.content, "@") {
							should_block = true
						}
					}
				}

				output := {
					block: should_block,
					payload: input.raw_input,
					message: ""
				}
			`,
			input: &ScriptInput{
				RawInput:   `{"messages": [{"role": "user", "content": "Hello world"}]}`,
				Messages: []MessageContent{
					{
						Role:  llms.ChatMessageTypeHuman,
						Parts: []llms.ContentPart{llms.TextPart("Hello world")},
					},
				},
				VendorName: "openai",
				ModelName:  "gpt-4",
			},
			wantBlock:   false,
			wantPayload: `{"messages": [{"role": "user", "content": "Hello world"}]}`,
			wantMessage: "",
			wantErr:     false,
		},
		{
			name: "access system prompt from messages",
			sourceCode: `
				// Find system message
				system_prompt := ""
				for msg in input.messages {
					if msg.role == "system" {
						system_prompt = msg.content
						break
					}
				}

				// Block if system prompt is missing
				output := {
					block: system_prompt == "",
					payload: input.raw_input,
					message: system_prompt == "" ? "System prompt required" : ""
				}
			`,
			input: &ScriptInput{
				RawInput:   `{"system": "You are helpful", "messages": [...]}`,
				Messages: []MessageContent{
					{
						Role:  llms.ChatMessageTypeSystem,
						Parts: []llms.ContentPart{llms.TextPart("You are a helpful assistant")},
					},
					{
						Role:  llms.ChatMessageTypeHuman,
						Parts: []llms.ContentPart{llms.TextPart("Hello")},
					},
				},
				VendorName: "anthropic",
				ModelName:  "claude-3",
			},
			wantBlock:   false,
			wantPayload: `{"system": "You are helpful", "messages": [...]}`,
			wantMessage: "",
			wantErr:     false,
		},
		{
			name: "empty messages array",
			sourceCode: `
				output := {
					block: len(input.messages) == 0,
					payload: input.raw_input,
					message: len(input.messages) == 0 ? "No messages provided" : ""
				}
			`,
			input: &ScriptInput{
				RawInput:   `{"messages": []}`,
				Messages:   []MessageContent{},
				VendorName: "openai",
				ModelName:  "gpt-4",
			},
			wantBlock:   true,
			wantPayload: `{"messages": []}`,
			wantMessage: "No messages provided",
			wantErr:     false,
		},
		{
			name: "multiple user messages",
			sourceCode: `
				text := import("text")

				// Collect all user messages
				user_messages := []
				for msg in input.messages {
					if msg.role == "user" {
						user_messages = append(user_messages, msg.content)
					}
				}

				// Block if any user message is too short
				should_block := false
				for content in user_messages {
					if len(content) < 5 {
						should_block = true
						break
					}
				}

				output := {
					block: should_block,
					payload: input.raw_input,
					message: should_block ? "User message too short" : ""
				}
			`,
			input: &ScriptInput{
				RawInput:   `{"messages": [{"role": "user", "content": "Hi"}]}`,
				Messages: []MessageContent{
					{
						Role:  llms.ChatMessageTypeHuman,
						Parts: []llms.ContentPart{llms.TextPart("Hello")},
					},
					{
						Role:  llms.ChatMessageTypeAI,
						Parts: []llms.ContentPart{llms.TextPart("Hi there!")},
					},
					{
						Role:  llms.ChatMessageTypeHuman,
						Parts: []llms.ContentPart{llms.TextPart("Hi")},
					},
				},
				VendorName: "openai",
				ModelName:  "gpt-4",
			},
			wantBlock:   true,
			wantPayload: `{"messages": [{"role": "user", "content": "Hi"}]}`,
			wantMessage: "User message too short",
			wantErr:     false,
		},
		{
			name: "access is_chat flag",
			sourceCode: `
				// Different behavior for chat vs proxy
				msg := ""
				if input.is_chat {
					msg = "Chat context"
				} else {
					msg = "Proxy context"
				}

				output := {
					block: false,
					payload: input.raw_input,
					message: msg
				}
			`,
			input: &ScriptInput{
				RawInput:   `{"test": "data"}`,
				Messages:   []MessageContent{},
				VendorName: "openai",
				ModelName:  "gpt-4",
				IsChat:     true,
			},
			wantBlock:   false,
			wantPayload: `{"test": "data"}`,
			wantMessage: "Chat context",
			wantErr:     false,
		},
		{
			name: "missing output variable",
			sourceCode: `
				// Script doesn't set output
				dummy := "test"
			`,
			input: &ScriptInput{
				RawInput: `{"test": "data"}`,
			},
			wantErr: true,
			errMsg:  "output must be a map",
		},
		{
			name: "invalid output format",
			sourceCode: `
				// Output is not a map
				output := "invalid"
			`,
			input: &ScriptInput{
				RawInput: `{"test": "data"}`,
			},
			wantErr: true,
			errMsg:  "output must be a map",
		},
		{
			name: "syntax error",
			sourceCode: `
				output := {
					block: false,
					payload: input.raw_input
					// missing closing brace
			`,
			input: &ScriptInput{
				RawInput: `{"test": "data"}`,
			},
			wantErr: true,
			errMsg:  "compilation error",
		},
		{
			name: "modify messages using messages array output - OpenAI",
			sourceCode: `
				text := import("text")

				// Modify all user messages to redact email addresses
				modified := []
				for msg in input.messages {
					new_msg := {
						role: msg.role,
						content: msg.content
					}
					if msg.role == "user" {
						new_msg.content = text.replace(msg.content, "@", "[REDACTED]", -1)
					}
					modified = append(modified, new_msg)
				}

				output := {
					block: false,
					messages: modified,
					message: "Email redacted"
				}
			`,
			input: &ScriptInput{
				RawInput:   `{"model":"gpt-4","messages":[{"role":"user","content":"Email me@example.com"}]}`,
				Messages: []MessageContent{
					{
						Role:  llms.ChatMessageTypeHuman,
						Parts: []llms.ContentPart{llms.TextPart("Email me@example.com")},
					},
				},
				VendorName: "openai",
				ModelName:  "gpt-4",
			},
			wantBlock:   false,
			wantPayload: "",
			wantMessage: "Email redacted",
			wantErr:     false,
		},
		{
			name: "modify messages using messages array output - with system",
			sourceCode: `
				// Modify system prompt
				modified := []
				for msg in input.messages {
					new_msg := {
						role: msg.role,
						content: msg.content
					}
					if msg.role == "system" {
						new_msg.content = "[MODIFIED] " + msg.content
					}
					modified = append(modified, new_msg)
				}

				output := {
					block: false,
					messages: modified,
					message: ""
				}
			`,
			input: &ScriptInput{
				RawInput:   `{"model":"gpt-4","messages":[{"role":"system","content":"You are helpful"},{"role":"user","content":"Hello"}]}`,
				Messages: []MessageContent{
					{
						Role:  llms.ChatMessageTypeSystem,
						Parts: []llms.ContentPart{llms.TextPart("You are helpful")},
					},
					{
						Role:  llms.ChatMessageTypeHuman,
						Parts: []llms.ContentPart{llms.TextPart("Hello")},
					},
				},
				VendorName: "openai",
				ModelName:  "gpt-4",
			},
			wantBlock:   false,
			wantPayload: "",
			wantMessage: "",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := NewScriptRunner([]byte(tt.sourceCode))
			output, err := runner.RunScript(tt.input, nil)

			if (err != nil) != tt.wantErr {
				t.Errorf("RunScript() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if err != nil && tt.errMsg != "" {
					if got := err.Error(); !contains(got, tt.errMsg) {
						t.Errorf("RunScript() error message = %v, want substring %v", got, tt.errMsg)
					}
				}
				return
			}

			if output.Block != tt.wantBlock {
				t.Errorf("RunScript() Block = %v, want %v", output.Block, tt.wantBlock)
			}

			if output.Payload != tt.wantPayload {
				t.Errorf("RunScript() Payload = %v, want %v", output.Payload, tt.wantPayload)
			}

			if output.Message != tt.wantMessage {
				t.Errorf("RunScript() Message = %v, want %v", output.Message, tt.wantMessage)
			}
		})
	}
}

func TestRunScriptConcurrency(t *testing.T) {
	sourceCode := `
		output := {
			block: false,
			payload: input.raw_input,
			message: ""
		}
	`

	runner := NewScriptRunner([]byte(sourceCode))
	concurrentRuns := 100

	done := make(chan bool)
	for i := 0; i < concurrentRuns; i++ {
		go func() {
			input := &ScriptInput{
				RawInput:   `{"test": "data"}`,
				Messages:   []MessageContent{},
				VendorName: "openai",
				ModelName:  "gpt-4",
			}
			_, err := runner.RunScript(input, nil)
			if err != nil {
				t.Errorf("Concurrent RunScript() error = %v", err)
			}
			done <- true
		}()
	}

	for i := 0; i < concurrentRuns; i++ {
		<-done
	}
}

// TestHelperFunctions tests the midsommar helper module functions
func TestHelperFunctions(t *testing.T) {
	tests := []struct {
		name         string
		sourceCode   string
		input        *ScriptInput
		wantErr      bool
		checkPayload func(t *testing.T, payload string)
	}{
		{
			name: "redact_pattern helper - email regex",
			sourceCode: `
				tyk := import("tyk")

				modified_payload := tyk.redact_pattern(input, "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}", "[EMAIL]")

				output := {
					block: false,
					payload: modified_payload,
					message: ""
				}
			`,
			input: &ScriptInput{
				RawInput:   `{"model":"gpt-4","messages":[{"role":"user","content":"Contact me@example.com or admin@test.org"}]}`,
				Messages: []MessageContent{
					{
						Role:  llms.ChatMessageTypeHuman,
						Parts: []llms.ContentPart{llms.TextPart("Contact me@example.com or admin@test.org")},
					},
				},
				VendorName: "openai",
				ModelName:  "gpt-4",
			},
			wantErr: false,
			checkPayload: func(t *testing.T, payload string) {
				// Verify the payload contains [EMAIL] redaction
				if !contains(payload, "[EMAIL]") {
					t.Errorf("Payload should contain [EMAIL] redaction, got: %s", payload)
				}
				// The @ symbol should not appear in email addresses (but may appear elsewhere in JSON)
				// Just verify [EMAIL] appears twice (for both emails)
				count := 0
				for i := 0; i < len(payload); i++ {
					if i+6 < len(payload) && payload[i:i+7] == "[EMAIL]" {
						count++
					}
				}
				if count < 2 {
					t.Errorf("Expected 2 [EMAIL] redactions, got %d in: %s", count, payload)
				}
			},
		},
		{
			name: "redact_pattern with system message - Anthropic format",
			sourceCode: `
				tyk := import("tyk")

				modified_payload := tyk.redact_pattern(input, "secret", "[REDACTED]")

				output := {
					block: false,
					payload: modified_payload,
					message: ""
				}
			`,
			input: &ScriptInput{
				RawInput:   `{"model":"claude-3","system":"This is secret info","messages":[{"role":"user","content":"Tell me the secret"}]}`,
				Messages: []MessageContent{
					{
						Role:  llms.ChatMessageTypeSystem,
						Parts: []llms.ContentPart{llms.TextPart("This is secret info")},
					},
					{
						Role:  llms.ChatMessageTypeHuman,
						Parts: []llms.ContentPart{llms.TextPart("Tell me the secret")},
					},
				},
				VendorName: "anthropic",
				ModelName:  "claude-3",
			},
			wantErr: false,
			checkPayload: func(t *testing.T, payload string) {
				// Verify redaction occurred (count [REDACTED] occurrences)
				count := 0
				searchStr := "[REDACTED]"
				for i := 0; i <= len(payload)-len(searchStr); i++ {
					if payload[i:i+len(searchStr)] == searchStr {
						count++
					}
				}
				if count < 2 {
					t.Errorf("Expected at least 2 [REDACTED] redactions, got %d in: %s", count, payload)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := NewScriptRunner([]byte(tt.sourceCode))
			output, err := runner.RunScript(tt.input, nil)

			if (err != nil) != tt.wantErr {
				t.Errorf("RunScript() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.checkPayload != nil {
				tt.checkPayload(t, output.Payload)
			}
		})
	}
}

// TestToolResponseFilters tests filters applied to tool responses
func TestToolResponseFilters(t *testing.T) {
	tests := []struct {
		name         string
		sourceCode   string
		input        *ScriptInput
		wantBlock    bool
		wantMessage  string
		checkPayload func(t *testing.T, payload string)
	}{
		{
			name: "tool response - simple blocking",
			sourceCode: `
				text := import("text")

				// Block if tool response contains error
				should_block := false
				if len(input.messages) > 0 {
					content := input.messages[0].content
					if text.contains(content, "error") {
						should_block = true
					}
				}

				output := {
					block: should_block,
					payload: input.raw_input,
					message: should_block ? "Tool returned error" : ""
				}
			`,
			input: &ScriptInput{
				RawInput:   `{"error": "API call failed"}`,
				Messages: []MessageContent{
					{
						Role:  llms.ChatMessageTypeTool,
						Parts: []llms.ContentPart{llms.TextPart(`{"error": "API call failed"}`)},
					},
				},
				VendorName: "openai",
				ModelName:  "gpt-4",
				IsChat:     true,
			},
			wantBlock:   true,
			wantMessage: "Tool returned error",
		},
		{
			name: "tool response - simple payload modification",
			sourceCode: `
				text := import("text")

				// Redact email from tool response (plain string manipulation)
				modified_content := input.raw_input
				if len(input.messages) > 0 {
					content := input.messages[0].content
					modified_content = text.replace(content, "@", "[AT]", -1)
				}

				output := {
					block: false,
					payload: modified_content,
					message: ""
				}
			`,
			input: &ScriptInput{
				RawInput:   "Contact support@example.com for help",
				Messages: []MessageContent{
					{
						Role:  llms.ChatMessageTypeTool,
						Parts: []llms.ContentPart{llms.TextPart("Contact support@example.com for help")},
					},
				},
				VendorName: "openai",
				ModelName:  "gpt-4",
				IsChat:     true,
			},
			wantBlock: false,
			checkPayload: func(t *testing.T, payload string) {
				if !contains(payload, "support[AT]example.com") {
					t.Errorf("Expected email to be redacted, got: %s", payload)
				}
			},
		},
		{
			name: "tool response - access context metadata",
			sourceCode: `
				// Block based on tool name
				tool_name := ""
				if input.context && input.context.tool_name {
					tool_name = input.context.tool_name
				}

				should_block := tool_name == "restricted_tool"

				output := {
					block: should_block,
					payload: input.raw_input,
					message: should_block ? "Tool is restricted" : ""
				}
			`,
			input: &ScriptInput{
				RawInput:   "Some tool output",
				Messages: []MessageContent{
					{
						Role:  llms.ChatMessageTypeTool,
						Parts: []llms.ContentPart{llms.TextPart("Some tool output")},
					},
				},
				VendorName: "openai",
				ModelName:  "gpt-4",
				IsChat:     true,
				Context: map[string]interface{}{
					"tool_name": "allowed_tool",
					"tool_id":   int64(123),
				},
			},
			wantBlock:   false,
			wantMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := NewScriptRunner([]byte(tt.sourceCode))
			output, err := runner.RunScript(tt.input, nil)

			if err != nil {
				t.Errorf("RunScript() unexpected error = %v", err)
				return
			}

			if output.Block != tt.wantBlock {
				t.Errorf("RunScript() Block = %v, want %v", output.Block, tt.wantBlock)
			}

			if output.Message != tt.wantMessage {
				t.Errorf("RunScript() Message = %v, want %v", output.Message, tt.wantMessage)
			}

			if tt.checkPayload != nil {
				tt.checkPayload(t, output.Payload)
			}
		})
	}
}

func contains(s, substr string) bool {
	// Simple substring check
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

var test_data = `
{"expand":"schema,names","startAt":0,"maxResults":50,"total":7,"issues":[{"expand":"operations,versionedRepresentations,editmeta,changelog,renderedFields","id":"45156","self":"https://tyktech.atlassian.net/rest/api/3/issue/45156","key":"TD-3482","fields":{"summary":"Allow 'Artnight' to rollback to v5.5.0","created":"2024-11-13T20:27:56.526+0000","status":{"self":"https://tyktech.atlassian.net/rest/api/3/status/10005","description":"","iconUrl":"https://tyktech.atlassian.net/","name":"Done","id":"10005","statusCategory":{"self":"https://tyktech.atlassian.net/rest/api/3/statuscategory/3","id":3,"key":"done","colorName":"green","name":"Done"}}}},{"expand":"operations,versionedRepresentations,editmeta,changelog,renderedFields","id":"45155","self":"https://tyktech.atlassian.net/rest/api/3/issue/45155","key":"TD-3481","fields":{"summary":"Create runbook for bulk custom domain add","created":"2024-11-13T19:31:41.972+0000","status":{"self":"https://tyktech.atlassian.net/rest/api/3/status/10003","description":"","iconUrl":"https://tyktech.atlassian.net/","name":"New","id":"10003","statusCategory":{"self":"https://tyktech.atlassian.net/rest/api/3/statuscategory/2","id":2,"key":"new","colorName":"blue-gray","name":"To Do"}}}},{"expand":"operations,versionedRepresentations,editmeta,changelog,renderedFields","id":"45154","self":"https://tyktech.atlassian.net/rest/api/3/issue/45154","key":"TT-13567","fields":{"summary":"[Docs] Review & update developer portal integration for streams","created":"2024-11-13T17:24:21.690+0000","status":{"self":"https://tyktech.atlassian.net/rest/api/3/status/1","description":"The issue is open and ready for the assignee to start work on it.","iconUrl":"https://tyktech.atlassian.net/images/icons/statuses/open.png","name":"Open","id":"1","statusCategory":{"self":"https://tyktech.atlassian.net/rest/api/3/statuscategory/2","id":2,"key":"new","colorName":"blue-gray","name":"To Do"}}}},{"expand":"operations,versionedRepresentations,editmeta,changelog,renderedFields","id":"45153","self":"https://tyktech.atlassian.net/rest/api/3/issue/45153","key":"TD-3480","fields":{"summary":"FiscalNote ","created":"2024-11-13T17:23:33.246+0000","status":{"self":"https://tyktech.atlassian.net/rest/api/3/status/3","description":"This issue is being actively worked on at the moment by the assignee.","iconUrl":"https://tyktech.atlassian.net/images/icons/statuses/inprogress.png","name":"In Progress","id":"3","statusCategory":{"self":"https://tyktech.atlassian.net/rest/api/3/statuscategory/4","id":4,"key":"indeterminate","colorName":"yellow","name":"In Progress"}}}},{"expand":"operations,versionedRepresentations,editmeta,changelog,renderedFields","id":"45151","self":"https://tyktech.atlassian.net/rest/api/3/issue/45151","key":"TT-13566","fields":{"summary":"Make upstream auth oauth password client secret not required in oas schema","created":"2024-11-13T15:03:48.591+0000","status":{"self":"https://tyktech.atlassian.net/rest/api/3/status/10037","description":"","iconUrl":"https://tyktech.atlassian.net/images/icons/statuses/generic.png","name":"In Dev","id":"10037","statusCategory":{"self":"https://tyktech.atlassian.net/rest/api/3/statuscategory/4","id":4,"key":"indeterminate","colorName":"yellow","name":"In Progress"}}}},{"expand":"operations,versionedRepresentations,editmeta,changelog,renderedFields","id":"45149","self":"https://tyktech.atlassian.net/rest/api/3/issue/45149","key":"TD-3479","fields":{"summary":"Liip/SBB Limits","created":"2024-11-13T13:43:55.347+0000","status":{"self":"https://tyktech.atlassian.net/rest/api/3/status/3","description":"This issue is being actively worked on at the moment by the assignee.","iconUrl":"https://tyktech.atlassian.net/images/icons/statuses/inprogress.png","name":"In Progress","id":"3","statusCategory":{"self":"https://tyktech.atlassian.net/rest/api/3/statuscategory/4","id":4,"key":"indeterminate","colorName":"yellow","name":"In Progress"}}}},{"expand":"operations,versionedRepresentations,editmeta,changelog,renderedFields","id":"45148","self":"https://tyktech.atlassian.net/rest/api/3/issue/45148","key":"TT-13565","fields":{"summary":"Disable \"Hybrid data plane configuration\" option inside Type drop down if Control Plane is not deployed","created":"2024-11-13T13:15:05.346+0000","status":{"self":"https://tyktech.atlassian.net/rest/api/3/status/1","description":"The issue is open and ready for the assignee to start work on it.","iconUrl":"https://tyktech.atlassian.net/images/icons/statuses/open.png","name":"Open","id":"1","statusCategory":{"self":"https://tyktech.atlassian.net/rest/api/3/statuscategory/2","id":2,"key":"new","colorName":"blue-gray","name":"To Do"}}}}]}
`
