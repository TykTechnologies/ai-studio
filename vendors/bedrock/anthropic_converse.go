package bedrockVendor

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// This file holds pure converters between a simplified Anthropic Messages shape
// and the AWS Bedrock Converse API. The simplified types live here (mirroring the
// existing ChatMessage / ToolDef pattern) so the vendor package stays free of any
// dependency on the proxy package — the proxy translator maps its wire structs
// into these before calling in.

// --- Simplified Anthropic input types (proxy maps its wire structs into these) ---

// AnthropicMsg is one conversation turn with already-decoded content blocks.
type AnthropicMsg struct {
	Role   string // "user" | "assistant"
	Blocks []AnthropicBlock
}

// AnthropicBlock is a single content block. Only the fields relevant to its Type
// are populated.
type AnthropicBlock struct {
	Type string // "text" | "tool_use" | "tool_result"

	Text string // text

	// tool_use
	ToolUseID string
	Name      string
	Input     map[string]any

	// tool_result
	ResultText string
	IsError    bool

	// prompt caching: a cache breakpoint sits at (and includes) this block
	CachePoint bool
}

// AnthropicSystemBlock is a system-prompt block.
type AnthropicSystemBlock struct {
	Text       string
	CachePoint bool
}

// AnthropicToolSpec is a tool definition.
type AnthropicToolSpec struct {
	Name        string
	Description string
	InputSchema map[string]any
	CachePoint  bool
}

// AnthropicToolChoiceSpec controls tool selection ("auto" | "any" | "tool").
type AnthropicToolChoiceSpec struct {
	Type string
	Name string
}

// AnthropicOutBlock is a simplified output block (text or tool_use) the proxy
// maps back onto the Anthropic response wire format.
type AnthropicOutBlock struct {
	Type  string // "text" | "tool_use"
	Text  string
	ID    string
	Name  string
	Input map[string]any
}

// --- Converters ---

// AnthropicMessagesToConverse maps Anthropic messages + system blocks to the
// Converse request shape. cache_control markers become Bedrock cache points
// inserted immediately after the block they apply to.
func AnthropicMessagesToConverse(msgs []AnthropicMsg, system []AnthropicSystemBlock) ([]types.Message, []types.SystemContentBlock) {
	var converseMsgs []types.Message
	for _, m := range msgs {
		var content []types.ContentBlock
		for _, b := range m.Blocks {
			switch b.Type {
			case "text":
				if b.Text == "" {
					continue // Bedrock rejects empty text blocks
				}
				content = append(content, &types.ContentBlockMemberText{Value: b.Text})
			case "tool_use":
				content = append(content, &types.ContentBlockMemberToolUse{
					Value: types.ToolUseBlock{
						ToolUseId: aws.String(b.ToolUseID),
						Name:      aws.String(b.Name),
						Input:     document.NewLazyDocument(b.Input),
					},
				})
			case "tool_result":
				status := types.ToolResultStatusSuccess
				if b.IsError {
					status = types.ToolResultStatusError
				}
				content = append(content, &types.ContentBlockMemberToolResult{
					Value: types.ToolResultBlock{
						ToolUseId: aws.String(b.ToolUseID),
						Status:    status,
						Content: []types.ToolResultContentBlock{
							&types.ToolResultContentBlockMemberText{Value: b.ResultText},
						},
					},
				})
			}
			if b.CachePoint {
				content = append(content, &types.ContentBlockMemberCachePoint{
					Value: types.CachePointBlock{Type: types.CachePointTypeDefault},
				})
			}
		}
		if len(content) == 0 {
			continue
		}
		converseMsgs = append(converseMsgs, types.Message{
			Role:    convertToConverseRole(m.Role),
			Content: content,
		})
	}

	var systemBlocks []types.SystemContentBlock
	for _, s := range system {
		if s.Text != "" {
			systemBlocks = append(systemBlocks, &types.SystemContentBlockMemberText{Value: s.Text})
		}
		if s.CachePoint {
			systemBlocks = append(systemBlocks, &types.SystemContentBlockMemberCachePoint{
				Value: types.CachePointBlock{Type: types.CachePointTypeDefault},
			})
		}
	}

	return converseMsgs, systemBlocks
}

// AnthropicToolsToToolConfig converts Anthropic tool definitions and tool_choice
// to a Bedrock ToolConfiguration. Returns nil when there are no tools.
func AnthropicToolsToToolConfig(tools []AnthropicToolSpec, choice *AnthropicToolChoiceSpec) *types.ToolConfiguration {
	if len(tools) == 0 {
		return nil
	}

	bedrockTools := make([]types.Tool, 0, len(tools))
	for _, t := range tools {
		bedrockTools = append(bedrockTools, &types.ToolMemberToolSpec{
			Value: types.ToolSpecification{
				Name:        aws.String(t.Name),
				Description: aws.String(t.Description),
				InputSchema: &types.ToolInputSchemaMemberJson{
					Value: document.NewLazyDocument(t.InputSchema),
				},
			},
		})
		if t.CachePoint {
			bedrockTools = append(bedrockTools, &types.ToolMemberCachePoint{
				Value: types.CachePointBlock{Type: types.CachePointTypeDefault},
			})
		}
	}

	cfg := &types.ToolConfiguration{Tools: bedrockTools}

	if choice != nil {
		switch choice.Type {
		case "any":
			cfg.ToolChoice = &types.ToolChoiceMemberAny{Value: types.AnyToolChoice{}}
		case "tool":
			if choice.Name != "" {
				cfg.ToolChoice = &types.ToolChoiceMemberTool{
					Value: types.SpecificToolChoice{Name: aws.String(choice.Name)},
				}
			}
		case "auto":
			cfg.ToolChoice = &types.ToolChoiceMemberAuto{Value: types.AutoToolChoice{}}
		}
	}

	return cfg
}

// ConverseOutputToAnthropicBlocks converts a Converse response message into
// Anthropic output blocks (text and tool_use).
func ConverseOutputToAnthropicBlocks(output *bedrockruntime.ConverseOutput) []AnthropicOutBlock {
	var blocks []AnthropicOutBlock
	if output == nil {
		return blocks
	}
	msgOutput, ok := output.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		return blocks
	}
	for _, block := range msgOutput.Value.Content {
		switch b := block.(type) {
		case *types.ContentBlockMemberText:
			blocks = append(blocks, AnthropicOutBlock{Type: "text", Text: b.Value})
		case *types.ContentBlockMemberToolUse:
			var input map[string]any
			if b.Value.Input != nil {
				_ = b.Value.Input.UnmarshalSmithyDocument(&input)
			}
			blocks = append(blocks, AnthropicOutBlock{
				Type:  "tool_use",
				ID:    aws.ToString(b.Value.ToolUseId),
				Name:  aws.ToString(b.Value.Name),
				Input: input,
			})
		}
	}
	return blocks
}

// ConvertConverseStopReasonAnthropic maps Bedrock stop reasons to Anthropic
// stop_reason values.
func ConvertConverseStopReasonAnthropic(reason types.StopReason) string {
	switch reason {
	case types.StopReasonEndTurn:
		return "end_turn"
	case types.StopReasonToolUse:
		return "tool_use"
	case types.StopReasonMaxTokens:
		return "max_tokens"
	case types.StopReasonStopSequence:
		return "stop_sequence"
	default:
		return "end_turn"
	}
}
