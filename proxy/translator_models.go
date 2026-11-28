package proxy

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms/googleai"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms/openai"
	"github.com/google/uuid"
)

// Models
type ChatCompletionRequest struct {
	Messages            []Message       `json:"messages"`
	Model               string          `json:"model"`
	Store               *bool           `json:"store,omitempty"`
	Metadata            map[string]any  `json:"metadata,omitempty"`
	FrequencyPenalty    *float64        `json:"frequency_penalty,omitempty"`
	LogitBias           map[string]int  `json:"logit_bias,omitempty"`
	LogProbs            *bool           `json:"logprobs,omitempty"`
	TopLogProbs         *int            `json:"top_logprobs,omitempty"`
	MaxCompletionTokens *int            `json:"max_completion_tokens,omitempty"`
	MaxTokens           *int            `json:"max_tokens,omitempty"`
	ReasoningEffort     *string         `json:"reasoning_effort,omitempty"`
	N                   *int            `json:"n,omitempty"`
	Modalities          []string        `json:"modalities,omitempty"`
	Audio               *AudioConfig    `json:"audio,omitempty"`
	PresencePenalty     *float64        `json:"presence_penalty,omitempty"`
	ResponseFormat      *ResponseFormat `json:"response_format,omitempty"`
	Seed                *int            `json:"seed,omitempty"`
	ServiceTier         *string         `json:"service_tier,omitempty"`
	Stop                any             `json:"stop,omitempty"`
	Stream              *bool           `json:"stream,omitempty"`
	StreamOptions       *StreamOptions  `json:"stream_options,omitempty"`
	Temperature         *float64        `json:"temperature,omitempty"`
	TopP                *float64        `json:"top_p,omitempty"`
	Tools               []Tool          `json:"tools,omitempty"`
	ToolChoice          any             `json:"tool_choice,omitempty"`
	ParallelToolCalls   *bool           `json:"parallel_tool_calls,omitempty"`
	User                *string         `json:"user,omitempty"`
	PromptCacheKey      *string         `json:"prompt_cache_key,omitempty"`
	SafetyIdentifier    *string         `json:"safety_identifier,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
	// Name is a legacy function message's function name.
	Name string `json:"name,omitempty"`
	// ToolCalls are an assistant message's tool calls.
	ToolCalls []MessageToolCall `json:"tool_calls,omitempty"`
	// ToolCallID is the call a tool message answers.
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// MessageToolCall is a tool call in an assistant message of the history.
type MessageToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type AudioConfig struct {
	Voice  string `json:"voice"`
	Format string `json:"format"`
}

type ResponseFormat struct {
	Type       string `json:"type"`
	JSONSchema *any   `json:"json_schema,omitempty"`
}

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type Tool struct {
	Type     string         `json:"type"`
	Function FunctionConfig `json:"function"`
}

type FunctionConfig struct {
	Description string         `json:"description,omitempty"`
	Name        string         `json:"name"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	Strict      *bool          `json:"strict,omitempty"`
}

func (t Tool) ToLangchainTool() llms.Tool {
	return llms.Tool{
		Type: t.Type,
		Function: &llms.FunctionDefinition{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
		},
	}
}

func (r *ChatCompletionRequest) ToLangchainOptions(conf *models.LLM) []llms.CallOption {
	options := make([]llms.CallOption, 0)

	// The request model has already been validated against the config's
	// AllowedModels by the shim handler, so it wins; DefaultModel is only a
	// fallback when the request omits the field. Do not mutate r.Model - the
	// handler echoes it back in the response body.
	model := r.Model
	if model == "" {
		model = conf.DefaultModel
	}
	if model != "" {
		options = append(options, llms.WithModel(model))
	}

	if limit := r.OutputTokenLimit(); limit != nil {
		options = append(options, llms.WithMaxTokens(*limit))
	}

	// reasoning_effort goes to OpenAI-wire upstreams verbatim, and only when
	// the client sent it: accepted values and defaults differ per model, so
	// the gateway neither checks nor defaults it. Other vendors express
	// effort differently and do not receive it.
	if r.ReasoningEffort != nil && *r.ReasoningEffort != "" && conf.Vendor == models.OPENAI {
		options = append(options, openai.WithReasoningEffort(*r.ReasoningEffort))
	}

	if r.N != nil {
		options = append(options, llms.WithN(*r.N))
	}

	if r.Temperature != nil {
		options = append(options, llms.WithTemperature(*r.Temperature))
	}

	if r.TopP != nil {
		options = append(options, llms.WithTopP(*r.TopP))
	}

	if r.FrequencyPenalty != nil {
		options = append(options, llms.WithFrequencyPenalty(*r.FrequencyPenalty))
	}

	if r.PresencePenalty != nil {
		options = append(options, llms.WithPresencePenalty(*r.PresencePenalty))
	}

	if r.Seed != nil {
		options = append(options, llms.WithSeed(*r.Seed))
	}

	// Handle Stop words
	if r.Stop != nil {
		if stopWords, ok := r.GetStop(); ok {
			options = append(options, llms.WithStopWords(stopWords))
		}
	}

	// Handle Tools and ToolChoice
	if len(r.Tools) > 0 {
		langchainTools := make([]llms.Tool, len(r.Tools))
		for i, tool := range r.Tools {
			langchainTools[i] = tool.ToLangchainTool()
		}
		options = append(options, llms.WithTools(langchainTools))
	}

	// Parameters only OpenAI has (service_tier, user, store, ...) go to an
	// OpenAI upstream verbatim; they used to be dropped, so flex and
	// priority never took effect through the bridges.
	if conf.Vendor == models.OPENAI {
		options = append(options, openai.WithExtraBody(r.openAIOnlyParams()))
	}

	// tool_choice goes to the driver as the client sent it: the OpenAI
	// driver forwards it, and the Anthropic, Google AI and Vertex drivers
	// read it with llms.ResolveToolChoice (they used to drop it).
	if r.ToolChoice != nil {
		options = append(options, llms.WithToolChoice(r.ToolChoice))
	}

	// Handle JSON mode
	if r.ResponseFormat != nil && r.ResponseFormat.Type == "json_object" {
		options = append(options, llms.WithJSONMode())
	}

	// Handle Metadata
	if r.Metadata != nil {
		options = append(options, llms.WithMetadata(r.Metadata))
	}

	return options
}

// CompletionCount is the number of completions the caller asked for: the
// request's `n`, defaulting to OpenAI's own default of 1.
// OutputTokenLimit is the cap on generated tokens the client asked for:
// max_completion_tokens when set, otherwise max_tokens (the older name, still
// what most clients send); nil when neither is.
func (r *ChatCompletionRequest) OutputTokenLimit() *int {
	if r.MaxCompletionTokens != nil {
		return r.MaxCompletionTokens
	}
	return r.MaxTokens
}

func (r *ChatCompletionRequest) CompletionCount() int {
	if r == nil || r.N == nil || *r.N < 1 {
		return 1
	}
	return *r.N
}

// Getter functions for ambiguous fields
func (r *ChatCompletionRequest) GetStop() ([]string, bool) {
	switch v := r.Stop.(type) {
	case string:
		return []string{v}, true
	case []string:
		return v, true
	case []any:
		// A JSON array decodes to []any: this is the shape clients send,
		// and it used to fall through, so a list of stop sequences was
		// dropped for every vendor behind a langchaingo driver.
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, len(out) > 0
	default:
		return nil, false
	}
}

// openAIOnlyParams are the request parameters the bridge forwards to an
// OpenAI upstream as the client sent them: those that exist only in OpenAI's
// API (and are not sent to other vendors), and top_p
// (docs/site/docs/proxy.md has the table). A parameter the client did not
// send is not in the map.
func (r *ChatCompletionRequest) openAIOnlyParams() map[string]any {
	out := map[string]any{}
	// top_p is here too: the OpenAI driver has never sent its TopP option,
	// and Studio Chat relies on that for models that refuse top_p, so the
	// bridge forwards the client's own value instead of changing the driver.
	if r.TopP != nil {
		out["top_p"] = *r.TopP
	}
	if r.ServiceTier != nil && *r.ServiceTier != "" {
		out["service_tier"] = *r.ServiceTier
	}
	if r.User != nil && *r.User != "" {
		out["user"] = *r.User
	}
	if r.Store != nil {
		out["store"] = *r.Store
	}
	if r.ParallelToolCalls != nil {
		out["parallel_tool_calls"] = *r.ParallelToolCalls
	}
	if r.LogProbs != nil {
		out["logprobs"] = *r.LogProbs
	}
	if r.TopLogProbs != nil {
		out["top_logprobs"] = *r.TopLogProbs
	}
	if r.PromptCacheKey != nil && *r.PromptCacheKey != "" {
		out["prompt_cache_key"] = *r.PromptCacheKey
	}
	if r.SafetyIdentifier != nil && *r.SafetyIdentifier != "" {
		out["safety_identifier"] = *r.SafetyIdentifier
	}
	return out
}

// toolChoiceTranslated reports whether the bridge translates tool_choice for
// a vendor itself (the vendor does not speak OpenAI's shape). For the other
// vendors the value is forwarded untouched and the vendor judges it.
func toolChoiceTranslated(vendor models.Vendor) bool {
	switch vendor {
	case models.ANTHROPIC, models.GOOGLEAI, models.VERTEX, models.BEDROCK:
		return true
	}
	return false
}

// ValidateToolChoice checks a tool_choice the bridge has to translate: it
// must be "auto", "none", "required" or a named function; "required" and a
// named function need tools; and a named function must be one of them. The
// error is for the client (400). Nothing is checked for vendors that take
// OpenAI's tool_choice as it is.
func (r *ChatCompletionRequest) ValidateToolChoice(vendor models.Vendor) error {
	if r.ToolChoice == nil || !toolChoiceTranslated(vendor) {
		return nil
	}
	choice, err := llms.ResolveToolChoice(r.ToolChoice)
	if err != nil {
		return fmt.Errorf("%w for %s models through this endpoint", err, vendor)
	}
	switch choice.Mode {
	case llms.ToolChoiceRequired:
		if len(r.Tools) == 0 {
			return errors.New(`tool_choice "required" needs at least one tool in tools`)
		}
	case llms.ToolChoiceFunction:
		for _, tool := range r.Tools {
			if tool.Function.Name == choice.Name {
				return nil
			}
		}
		return fmt.Errorf("tool_choice names the function %q, which is not in tools", choice.Name)
	}
	return nil
}

// hasToolHistory reports whether the conversation already holds tool calls
// or tool results.
func (r *ChatCompletionRequest) hasToolHistory() bool {
	for _, msg := range r.Messages {
		if msg.Role == "tool" || len(msg.ToolCalls) > 0 {
			return true
		}
	}
	return false
}

func (r *ChatCompletionRequest) GetToolChoice() (string, *FunctionConfig, bool) {
	switch v := r.ToolChoice.(type) {
	case string:
		return v, nil, true
	case map[string]any:
		if typeStr, ok := v["type"].(string); ok && typeStr == "function" {
			if funcMap, ok := v["function"].(map[string]any); ok {
				return "", &FunctionConfig{
					Name: funcMap["name"].(string),
				}, true
			}
		}
	}
	return "", nil, false
}

func (r *ChatCompletionRequest) GetMessageContent(msg Message) (string, []map[string]any, bool) {
	switch v := msg.Content.(type) {
	case string:
		return v, nil, true
	case []map[string]any:
		return "", v, true
	case []any:
		// What encoding/json decodes an array of parts into.
		parts := make([]map[string]any, 0, len(v))
		for _, p := range v {
			if m, ok := p.(map[string]any); ok {
				parts = append(parts, m)
			}
		}
		return "", parts, true
	default:
		return "", nil, false
	}
}

type ChatCompletionMessage struct {
	Role      string                   `json:"role"`
	Content   string                   `json:"content"`
	Name      string                   `json:"name,omitempty"`
	ToolCalls []map[string]interface{} `json:"tool_calls,omitempty"`
}

type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   CompletionUsage        `json:"usage"`
	// ServiceTier is the tier the vendor says it served the call at
	// (OpenAI: "default", "flex", "priority"); left out when it said none.
	ServiceTier string `json:"service_tier,omitempty"`
}

type ChatCompletionChoice struct {
	Index        int                   `json:"index"`
	Message      ChatCompletionMessage `json:"message"`
	FinishReason string                `json:"finish_reason"`
}

type CreateCompletionRequest struct {
	Model            string      `json:"model"`
	Prompt           string      `json:"prompt,omitempty"`
	MaxTokens        *int        `json:"max_tokens,omitempty"`
	Temperature      *float64    `json:"temperature,omitempty"`
	TopP             *float64    `json:"top_p,omitempty"`
	N                *int        `json:"n,omitempty"`
	Stream           *bool       `json:"stream,omitempty"`
	LogProbs         *int        `json:"logprobs,omitempty"`
	Echo             *bool       `json:"echo,omitempty"`
	Stop             interface{} `json:"stop,omitempty"`
	PresencePenalty  *float64    `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64    `json:"frequency_penalty,omitempty"`
	User             string      `json:"user,omitempty"`
}

type CompletionResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []CompletionChoice `json:"choices"`
	Usage   CompletionUsage    `json:"usage"`
}

type CompletionChoice struct {
	Text         string    `json:"text"`
	Index        int       `json:"index"`
	LogProbs     *LogProbs `json:"logprobs"`
	FinishReason string    `json:"finish_reason"`
}

type CompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type LogProbs struct {
	Tokens        []string             `json:"tokens"`
	TokenLogProbs []float32            `json:"token_logprobs"`
	TopLogProbs   []map[string]float32 `json:"top_logprobs"`
	TextOffset    []int                `json:"text_offset"`
}

// Streaming response types for OpenAI-compatible SSE streaming

// ChatCompletionChunk represents a streaming chunk in OpenAI format
type ChatCompletionChunk struct {
	ID      string                      `json:"id"`
	Object  string                      `json:"object"` // "chat.completion.chunk"
	Created int64                       `json:"created"`
	Model   string                      `json:"model"`
	Choices []ChatCompletionChunkChoice `json:"choices"`
	Usage   *CompletionUsage            `json:"usage,omitempty"` // Only in final chunk
}

// ChatCompletionChunkChoice represents a choice in a streaming chunk
type ChatCompletionChunkChoice struct {
	Index        int                 `json:"index"`
	Delta        ChatCompletionDelta `json:"delta"`
	FinishReason *string             `json:"finish_reason"` // null until final chunk
}

// ChatCompletionDelta represents the delta content in a streaming chunk
type ChatCompletionDelta struct {
	Role      string                        `json:"role,omitempty"`    // Only in first chunk
	Content   string                        `json:"content,omitempty"` // Streaming text content
	ToolCalls []ChatCompletionToolCallDelta `json:"tool_calls,omitempty"`
}

// ChatCompletionToolCallDelta is one fragment of a streamed tool call.
//
// Index is what makes reassembly possible and is required on every fragment:
// it is the only thing tying an arguments fragment back to the call it belongs
// to when a model emits several in parallel. Id and Function.Name appear on the
// opening fragment; subsequent fragments carry argument text only, and the
// concatenation of those fragments must parse as JSON.
type ChatCompletionToolCallDelta struct {
	Index    int                          `json:"index"`
	ID       string                       `json:"id,omitempty"`
	Type     string                       `json:"type,omitempty"`
	Function *ChatCompletionFunctionDelta `json:"function,omitempty"`
}

// ChatCompletionFunctionDelta is the function half of a streamed tool call.
type ChatCompletionFunctionDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// ChatCompletionStreamError represents an error in SSE format
type ChatCompletionStreamError struct {
	Error ChatCompletionErrorDetail `json:"error"`
}

// ChatCompletionErrorDetail contains error details
type ChatCompletionErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

func (r *ChatCompletionRequest) GetMessages() []llms.MessageContent {
	messages := make([]llms.MessageContent, len(r.Messages))

	for i, msg := range r.Messages {
		msgContent := llms.MessageContent{
			Role:  convertRole(msg.Role),
			Parts: []llms.ContentPart{},
		}

		// Use the existing GetMessageContent helper
		if textContent, contentArray, ok := r.GetMessageContent(msg); ok {
			if textContent != "" {
				// Simple text content
				msgContent.Parts = append(msgContent.Parts, llms.TextPart(textContent))
			} else if contentArray != nil {
				// Handle multi-part content
				for _, part := range contentArray {
					if contentType, ok := part["type"].(string); ok {
						switch contentType {
						case "text":
							if text, ok := part["text"].(string); ok {
								msgContent.Parts = append(msgContent.Parts, llms.TextPart(text))
							}
						case "image_url":
							if imageUrl, ok := part["image_url"].(map[string]interface{}); ok {
								if url, ok := imageUrl["url"].(string); ok {
									if detail, ok := imageUrl["detail"].(string); ok {
										msgContent.Parts = append(msgContent.Parts,
											llms.ImageURLWithDetailPart(url, detail))
									} else {
										msgContent.Parts = append(msgContent.Parts,
											llms.ImageURLPart(url))
									}
								}
							}
						}
					}
				}
			}
		}

		messages[i] = msgContent
	}

	return messages
}

// DriverMessages is GetMessages for the /ai/ bridge's driver call: on top of
// the text, it carries the history's tool calls (as llms.ToolCall parts of
// the assistant message) and tool results (as llms.ToolCallResponse parts
// with the call's id and tool name), so the vendor gets the conversation the
// client sent, and the inner hop's guardrails read tool results as tool
// results. Consecutive tool messages become one message, as Anthropic and
// Gemini want the results of parallel calls in one turn. Drivers without
// tool parts (Ollama) get GetMessages, as before.
//
// GetMessages itself is unchanged: script filters read its text parts.
//
// A tool call ID minted for a Gemini turn carries its thought signature and
// can be kilobytes long (gemini_thought_signatures.go). A history holding one
// can reach another vendor (a failover, a router, a client switching
// models), whose IDs are limited in length, so for any vendor but Google AI
// and Vertex such IDs are shortened, the same way in the tool calls and the
// tool results (googleai.ShortSignatureID).
func (r *ChatCompletionRequest) DriverMessages(vendor models.Vendor) []llms.MessageContent {
	plain := r.GetMessages()
	switch vendor {
	case models.OPENAI, models.ANTHROPIC, models.GOOGLEAI, models.VERTEX:
	default:
		return plain
	}
	names := map[string]string{}
	toolCallID := func(id string) string { return id }
	if vendor != models.GOOGLEAI && vendor != models.VERTEX {
		toolCallID = googleai.ShortSignatureID
	}
	out := make([]llms.MessageContent, 0, len(plain))
	for i, msg := range r.Messages {
		mc := plain[i]
		switch {
		case msg.Role == "assistant" && len(msg.ToolCalls) > 0:
			for _, c := range msg.ToolCalls {
				c.ID = toolCallID(c.ID)
				names[c.ID] = c.Function.Name
				args := c.Function.Arguments
				if strings.TrimSpace(args) == "" {
					args = "{}"
				}
				typ := c.Type
				if typ == "" {
					typ = "function"
				}
				mc.Parts = append(mc.Parts, llms.ToolCall{ID: c.ID, Type: typ, FunctionCall: &llms.FunctionCall{Name: c.Function.Name, Arguments: args}})
			}
		case msg.Role == "tool":
			var text strings.Builder
			for _, p := range mc.Parts {
				if t, ok := p.(llms.TextContent); ok {
					text.WriteString(t.Text)
				}
			}
			id := toolCallID(msg.ToolCallID)
			part := llms.ToolCallResponse{ToolCallID: id, Name: names[id], Content: text.String()}
			if n := len(out); n > 0 && out[n-1].Role == llms.ChatMessageTypeTool {
				out[n-1].Parts = append(out[n-1].Parts, part)
				continue
			}
			mc = llms.MessageContent{Role: llms.ChatMessageTypeTool, Parts: []llms.ContentPart{part}}
		}
		out = append(out, mc)
	}
	return out
}

// convertRole converts OpenAI role strings to langchaingo ChatMessageType
func convertRole(role string) llms.ChatMessageType {
	switch role {
	case "system":
		return llms.ChatMessageTypeSystem
	case "user":
		return llms.ChatMessageTypeHuman
	case "assistant":
		return llms.ChatMessageTypeAI
	case "function":
		return llms.ChatMessageTypeFunction
	case "tool":
		return llms.ChatMessageTypeTool
	default:
		slog.Warn("role is not supported, defaulting to human", "role", role)
		return llms.ChatMessageTypeHuman
	}
}

// NewChatCompletionResponse converts a langchaingo ContentResponse into the
// OpenAI chat.completion object.
//
// n is the number of completions the caller asked for (the request's `n`, or 1
// when it is omitted). It matters because langchaingo drivers do not agree on
// what a ContentChoice represents: OpenAI's returns one per requested
// completion, while Anthropic's returns one per *content block* of a single
// turn - so a reply carrying text and a tool call arrives as two choices.
//
// OpenAI indexes `choices` by n and puts every part of one turn in the same
// message. Passing the block-per-choice shape straight through produced
// choices[0] with finish_reason:"tool_calls" and no tool_calls array, plus a
// choices[1] holding the call: an SDK reads choices[0], finds nothing to
// invoke, and breaks. Anything beyond the requested n is therefore folded back
// into a single turn.
func NewChatCompletionResponse(llmResponse *llms.ContentResponse, model string, n int) *ChatCompletionResponse {
	response := &ChatCompletionResponse{
		ID:      uuid.New().String(),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: make([]ChatCompletionChoice, 0, len(llmResponse.Choices)),
	}

	for i, group := range groupContentChoices(llmResponse.Choices, n) {
		response.Choices = append(response.Choices, newChatCompletionChoice(i, group))
	}

	// The service tier, when the driver reports one (the OpenAI driver).
	for _, choice := range llmResponse.Choices {
		if choice == nil {
			continue
		}
		if tier, _ := choice.GenerationInfo["ServiceTier"].(string); tier != "" {
			response.ServiceTier = tier
			break
		}
	}

	// Note: Usage stats would need to be set separately as they're not part of ContentResponse
	return response
}

// groupContentChoices partitions the driver's choices into the turns the caller
// actually asked for. With n completions requested and more choices than that
// coming back, the extras are content blocks of one turn rather than
// alternatives, so they all collapse into a single group.
func groupContentChoices(choices []*llms.ContentChoice, n int) [][]*llms.ContentChoice {
	if len(choices) == 0 {
		return nil
	}
	if n < 1 {
		n = 1
	}
	if len(choices) <= n {
		groups := make([][]*llms.ContentChoice, 0, len(choices))
		for _, c := range choices {
			groups = append(groups, []*llms.ContentChoice{c})
		}
		return groups
	}
	return [][]*llms.ContentChoice{choices}
}

// newChatCompletionChoice merges one group of driver choices into a single
// OpenAI choice: text concatenated, tool calls unioned, and the finish reason
// taken from the turn rather than from whichever block happened to be last.
func newChatCompletionChoice(index int, group []*llms.ContentChoice) ChatCompletionChoice {
	choice := ChatCompletionChoice{
		Index:   index,
		Message: ChatCompletionMessage{Role: "assistant"},
	}

	var content strings.Builder
	var toolCalls []map[string]interface{}
	stopReason := ""

	for _, c := range group {
		content.WriteString(c.Content)
		if stopReason == "" {
			stopReason = c.StopReason
		}
		for _, toolCall := range c.ToolCalls {
			if toolCall.FunctionCall == nil {
				continue
			}
			// Anthropic's driver leaves Type empty; OpenAI's schema pins it to
			// the literal "function" and SDKs switch on it.
			callType := toolCall.Type
			if callType == "" {
				callType = "function"
			}
			// Gemini returns tool calls with no id at all. The id is what a
			// client puts in tool_call_id when it sends the result back, so an
			// empty one breaks the round trip - mint one rather than emit "".
			callID := toolCall.ID
			if callID == "" {
				callID = "call_" + uuid.New().String()
			}
			// Clients json.Unmarshal this field directly; "" fails with
			// "unexpected end of JSON input" where "{}" is a no-argument call.
			args := toolCall.FunctionCall.Arguments
			if args == "" {
				args = "{}"
			}
			toolCalls = append(toolCalls, map[string]interface{}{
				"id":   callID,
				"type": callType,
				"function": map[string]interface{}{
					"name":      toolCall.FunctionCall.Name,
					"arguments": args,
				},
			})
		}
	}

	choice.Message.Content = content.String()
	choice.Message.ToolCalls = toolCalls
	choice.FinishReason = convertFinishReason(stopReason)
	// A turn that produced tool calls must say so, whatever the block-level
	// stop reason was: clients branch on finish_reason to decide whether to run
	// a tool and send the result back.
	if len(toolCalls) > 0 {
		choice.FinishReason = "tool_calls"
	}
	return choice
}

// Helper to convert finish reasons
func convertFinishReason(reason string) string {
	// Map langchaingo/vendor stop reasons to OpenAI format
	switch reason {
	case "stop", "end_turn":
		return "stop"
	case "length", "max_tokens":
		return "length"
	case "tool_calls", "tool_use":
		return "tool_calls"
	case "content_filter", "content_filtered":
		return "content_filter"
	default:
		return "stop"
	}
}
