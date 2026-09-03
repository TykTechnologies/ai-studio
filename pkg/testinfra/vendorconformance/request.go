package vendorconformance

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Prompts are deliberately short, deterministic in shape, and cheap. The suite
// asserts response *structure*, so the model's actual words never matter — but
// a prompt that reliably elicits a short answer keeps the bill down and reduces
// the chance of hitting max_tokens and getting finish_reason "length" when the
// test expects "stop".
const (
	promptBasic     = "Reply with exactly the word: pong"
	promptMultiturn = "Reply with exactly the word: three"
	promptSystem    = "What is your designated call sign? Reply with the call sign only."
	systemPrompt    = "You are a test fixture. Your designated call sign is BLUEBIRD. Always answer in one word."
	promptTool      = "What is the weather in Wellington? Use the get_weather tool."
	promptJSON      = "Return a JSON object with keys \"city\" (string) and \"population\" (integer) for Wellington, New Zealand."
	promptVision    = "Reply with exactly one word naming the dominant colour of this image."
)

// A 1x1 solid red PNG. Small enough to inline in every request, and the
// expected answer is unambiguous enough to confirm the image actually arrived.
const redPixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// ChatRequest is an OpenAI-format chat completion request. It is built as a
// map rather than a struct so scenarios can add vendor-specific fields without
// growing a union type, and so omitted fields are genuinely absent from the
// wire rather than present-and-zero.
type ChatRequest map[string]any

// JSON renders the request body.
func (r ChatRequest) JSON() []byte {
	b, err := json.Marshal(map[string]any(r))
	if err != nil {
		panic(fmt.Sprintf("vendorconformance: unmarshalable request: %v", err))
	}
	return b
}

// WithModel returns a copy with the model field replaced. Used by the universal
// surface, which needs "{slug}/{model}" where the shim needs a bare model name.
func (r ChatRequest) WithModel(model string) ChatRequest {
	out := ChatRequest{}
	for k, v := range r {
		out[k] = v
	}
	out["model"] = model
	return out
}

// IsStreaming reports whether this request asked for SSE.
func (r ChatRequest) IsStreaming() bool {
	s, ok := r["stream"].(bool)
	return ok && s
}

// BuildRequest returns the OpenAI-format request body for a scenario, or false
// if the scenario has no request of its own (errors, which builds its own
// deliberately-broken requests).
func BuildRequest(c Case, cfg *Config) (ChatRequest, bool) {
	base := ChatRequest{
		"model":      c.Model,
		"max_tokens": cfg.MaxTokens,
	}

	switch c.Scenario {
	case ScenarioBasic:
		base["messages"] = userMessages(promptBasic)

	case ScenarioStreaming:
		base["messages"] = userMessages(promptBasic)
		base["stream"] = true

	case ScenarioUsage:
		base["messages"] = userMessages(promptBasic)
		base["stream"] = true
		// The whole point of this scenario: usage must arrive on the final
		// chunk when the client asks for it, and match the non-streaming call.
		base["stream_options"] = map[string]any{"include_usage": true}

	case ScenarioSystem:
		// A system prompt is a top-level field for Anthropic (`system`) and
		// Gemini (`systemInstruction`) but a message for OpenAI. Whether the
		// translator moves it correctly is the assertion.
		base["messages"] = []any{
			map[string]any{"role": "system", "content": systemPrompt},
			map[string]any{"role": "user", "content": promptSystem},
		}

	case ScenarioMultiturn:
		base["messages"] = []any{
			map[string]any{"role": "user", "content": "Reply with exactly the word: one"},
			map[string]any{"role": "assistant", "content": "one"},
			map[string]any{"role": "user", "content": "Reply with exactly the word: two"},
			map[string]any{"role": "assistant", "content": "two"},
			map[string]any{"role": "user", "content": promptMultiturn},
		}

	case ScenarioTools:
		base["messages"] = userMessages(promptTool)
		base["tools"] = weatherTool()
		base["tool_choice"] = "auto"

	case ScenarioToolsStreaming:
		base["messages"] = userMessages(promptTool)
		base["tools"] = weatherTool()
		base["tool_choice"] = "auto"
		base["stream"] = true

	case ScenarioJSONMode:
		base["messages"] = userMessages(promptJSON)
		base["response_format"] = map[string]any{"type": "json_object"}

	case ScenarioVision:
		base["messages"] = []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": promptVision},
					map[string]any{
						"type":      "image_url",
						"image_url": map[string]any{"url": "data:image/png;base64," + redPixelPNG},
					},
				},
			},
		}

	case ScenarioLongContext:
		base["messages"] = []any{
			map[string]any{"role": "user", "content": longContextPrompt()},
		}

	case ScenarioErrors:
		return nil, false

	default:
		return nil, false
	}

	return base, true
}

func userMessages(text string) []any {
	return []any{map[string]any{"role": "user", "content": text}}
}

// weatherTool is the single tool definition used by both tool scenarios. Its
// schema is intentionally boring: the assertion is about argument round-tripping
// through three different vendor encodings, not about schema complexity.
func weatherTool() []any {
	return []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "get_weather",
				"description": "Get the current weather for a city.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{
							"type":        "string",
							"description": "The city name, e.g. Wellington",
						},
					},
					"required":             []any{"city"},
					"additionalProperties": false,
				},
			},
		},
	}
}

// ToolResultRequest builds the second leg of a tool round-trip: the original
// exchange plus the assistant's tool call and our synthetic tool result. This is
// where tool_call_id round-tripping either works or silently breaks.
func ToolResultRequest(original ChatRequest, toolCallID, toolName, result string) ChatRequest {
	msgs, _ := original["messages"].([]any)
	next := append([]any{}, msgs...)
	next = append(next,
		map[string]any{
			"role":    "assistant",
			"content": nil,
			"tool_calls": []any{
				map[string]any{
					"id":   toolCallID,
					"type": "function",
					"function": map[string]any{
						"name":      toolName,
						"arguments": `{"city":"Wellington"}`,
					},
				},
			},
		},
		map[string]any{
			"role":         "tool",
			"tool_call_id": toolCallID,
			"content":      result,
		},
	)

	out := ChatRequest{}
	for k, v := range original {
		out[k] = v
	}
	out["messages"] = next
	delete(out, "tools")
	delete(out, "tool_choice")
	delete(out, "stream")
	return out
}

// longContextPrompt builds a prompt of roughly 8k tokens. The filler is varied
// rather than one repeated token so prompt-caching and dedup optimisations
// cannot collapse it into something much smaller than intended.
func longContextPrompt() string {
	var b strings.Builder
	b.WriteString("Below is a numbered list. After it, reply with exactly the word: done\n\n")
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(&b, "%d. item %d of the inventory manifest for sector %d\n", i, i*7%997, i%13)
	}
	b.WriteString("\nReply with exactly the word: done")
	return b.String()
}

// ExpectedFinishReason is the finish_reason a scenario should produce. An empty
// return means any terminal reason is acceptable.
func ExpectedFinishReason(s Scenario) string {
	switch s {
	case ScenarioTools, ScenarioToolsStreaming:
		return "tool_calls"
	default:
		return ""
	}
}
