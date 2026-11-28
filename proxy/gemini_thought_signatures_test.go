package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms/googleai"
)

// The carrier moves a functionCall part's signature into its args in every
// top-level object of a stream, however the reads are cut, and passes the
// rest through as it is.
func TestSignatureCarrier(t *testing.T) {
	plain := `{"candidates":[{"content":{"role":"model","parts":[{"text":"a } \" { ] ["}]}}]}`
	signed := `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"f","args":{"x":1}},"thoughtSignature":"SIG"},{"functionCall":{"name":"g"}}]}}]}`
	stream := "[" + plain + "\n,\r\n" + signed + "\n]"

	for name, r := range map[string]io.Reader{
		"whole":       strings.NewReader(stream),
		"one byte":    iotest.OneByteReader(strings.NewReader(stream)),
		"half chunks": iotest.HalfReader(strings.NewReader(stream)),
	} {
		t.Run(name, func(t *testing.T) {
			out, err := io.ReadAll(newSignatureCarrier(io.NopCloser(r)))
			require.NoError(t, err)
			var chunks []map[string]any
			require.NoError(t, json.Unmarshal(out, &chunks), string(out))
			require.Len(t, chunks, 2)
			assert.True(t, strings.HasPrefix(string(out), "["+plain+"\n,\r\n"), "unsigned objects and separators pass as they are")

			parts := chunks[1]["candidates"].([]any)[0].(map[string]any)["content"].(map[string]any)["parts"].([]any)
			first := parts[0].(map[string]any)
			assert.NotContains(t, first, "thoughtSignature")
			args := first["functionCall"].(map[string]any)["args"].(map[string]any)
			assert.Equal(t, "SIG", args[googleai.ThoughtSignatureArg])
			assert.EqualValues(t, 1, args["x"])
			assert.NotContains(t, parts[1].(map[string]any)["functionCall"], "args", "an unsigned call is left alone")
		})
	}

	// A single object (not an array) is carried too.
	out, err := io.ReadAll(newSignatureCarrier(io.NopCloser(strings.NewReader(signed))))
	require.NoError(t, err)
	assert.Contains(t, string(out), googleai.ThoughtSignatureArg)
}

func TestRestoreThoughtSignatures(t *testing.T) {
	body := `{"contents":[{"role":"user","parts":[{"text":"hi"}]},{"role":"model","parts":[` +
		`{"functionCall":{"name":"f","args":{"x":1,"` + googleai.ThoughtSignatureArg + `":"SIG"}}},` +
		`{"functionCall":{"name":"g","args":{}}}]}],"generationConfig":{"temperature":0.5}}`
	out := restoreThoughtSignatures([]byte(body))
	assert.NotContains(t, string(out), googleai.ThoughtSignatureArg)
	var req struct {
		Contents []struct {
			Parts []map[string]any `json:"parts"`
		} `json:"contents"`
		GenerationConfig map[string]any `json:"generationConfig"`
	}
	require.NoError(t, json.Unmarshal(out, &req))
	parts := req.Contents[1].Parts
	assert.Equal(t, "SIG", parts[0]["thoughtSignature"])
	assert.Equal(t, map[string]any{"x": float64(1)}, parts[0]["functionCall"].(map[string]any)["args"])
	assert.NotContains(t, parts[1], "thoughtSignature")
	assert.Equal(t, 0.5, req.GenerationConfig["temperature"])

	// A model turn whose first call has nothing to restore gets the
	// placeholder on that call only; a user turn is never touched.
	noSig := `{"contents":[{"role":"model","parts":[{"text":"x"},{"functionCall":{"name":"f","args":{}}},{"functionCall":{"name":"g","args":{}}}]},` +
		`{"role":"user","parts":[{"functionCall":{"name":"h","args":{"` + googleai.ThoughtSignatureArg + `":"S"}}}]}]}`
	var fallback struct {
		Contents []struct {
			Parts []map[string]any `json:"parts"`
		} `json:"contents"`
	}
	require.NoError(t, json.Unmarshal(restoreThoughtSignatures([]byte(noSig)), &fallback))
	assert.NotContains(t, fallback.Contents[0].Parts[0], "thoughtSignature")
	assert.Equal(t, placeholderThoughtSignature, fallback.Contents[0].Parts[1]["thoughtSignature"])
	assert.NotContains(t, fallback.Contents[0].Parts[2], "thoughtSignature")
	assert.NotContains(t, fallback.Contents[1].Parts[0], "thoughtSignature", "only role:model parts")
	assert.Equal(t, map[string]any{}, fallback.Contents[1].Parts[0]["functionCall"].(map[string]any)["args"], "the reserved key is dropped anyway")

	// Gemini 1.x and 2.x get no placeholder; a real signature is still restored.
	for _, model := range []string{"models/gemini-2.5-flash", "gemini-1.5-pro"} {
		older := `{"model":"` + model + `","contents":[{"role":"model","parts":[{"functionCall":{"name":"f","args":{}}}]},` +
			`{"role":"model","parts":[{"functionCall":{"name":"g","args":{"` + googleai.ThoughtSignatureArg + `":"S"}}}]}]}`
		got := string(restoreThoughtSignatures([]byte(older)))
		assert.NotContains(t, got, placeholderThoughtSignature, model)
		assert.Contains(t, got, `"thoughtSignature":"S"`, model)
	}
	assert.True(t, placeholderAllowed("models/gemini-3.5-flash"))
	assert.True(t, placeholderAllowed("gemini-3.1-flash-lite"))

	unchanged := `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`
	assert.Equal(t, unchanged, string(restoreThoughtSignatures([]byte(unchanged))))
}

// fakeGemini3 answers the first turn with two parallel function calls, the
// first signed (as Gemini 3 signs only the first of parallel calls), and
// refuses a later turn whose first call lacks that signature, as Gemini 3
// does.
type fakeGemini3 struct {
	mu     sync.Mutex
	bodies []string
}

const gemini3Signature = "c2lnbmF0dXJlLWJ5dGVz+/="

func (f *fakeGemini3) handler(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.bodies = append(f.bodies, string(raw))
	f.mu.Unlock()

	var req struct {
		Contents []struct {
			Role  string           `json:"role"`
			Parts []map[string]any `json:"parts"`
		} `json:"contents"`
	}
	_ = json.Unmarshal(raw, &req)
	w.Header().Set("Content-Type", "application/json")
	usage := `"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"totalTokenCount":7}`
	if len(req.Contents) == 1 {
		_, _ = w.Write([]byte(`[{"candidates":[{"content":{"role":"model","parts":[` +
			`{"functionCall":{"name":"get_time","args":{"zone":"UTC"}},"thoughtSignature":"` + gemini3Signature + `"},` +
			`{"functionCall":{"name":"get_weather","args":{"city":"Paris"}}}]},"finishReason":1}],` + usage + `}]`))
		return
	}
	for _, c := range req.Contents {
		if c.Role != "model" {
			continue
		}
		for _, p := range c.Parts {
			if _, ok := p["functionCall"]; ok {
				if p["thoughtSignature"] != gemini3Signature && p["thoughtSignature"] != placeholderThoughtSignature {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"error":{"code":400,"message":"Function call is missing a thought_signature in functionCall parts.","status":"INVALID_ARGUMENT"}}`))
					return
				}
				break // only the first call of a turn is signed
			}
		}
	}
	_, _ = w.Write([]byte(`[{"candidates":[{"content":{"role":"model","parts":[{"text":"12:00, sunny"}]},"finishReason":1}],` + usage + `}]`))
}

const gemini3Tools = `"tools":[{"type":"function","function":{"name":"get_time","parameters":{"type":"object","properties":{"zone":{"type":"string"}}}}},` +
	`{"type":"function","function":{"name":"get_weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}}]`

type gemini3ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// turnOneCalls reads the tool calls of a buffered or streamed answer.
func turnOneCalls(t *testing.T, out []byte, stream bool) []gemini3ToolCall {
	t.Helper()
	if !stream {
		var resp struct {
			Choices []struct {
				Message struct {
					ToolCalls []gemini3ToolCall `json:"tool_calls"`
				} `json:"message"`
			} `json:"choices"`
		}
		require.NoError(t, json.Unmarshal(out, &resp), string(out))
		require.Len(t, resp.Choices, 1)
		return resp.Choices[0].Message.ToolCalls
	}
	var calls []gemini3ToolCall
	for _, line := range strings.Split(string(out), "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok || data == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					ToolCalls []gemini3ToolCall `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		require.NoError(t, json.Unmarshal([]byte(data), &chunk), data)
		for _, c := range chunk.Choices {
			calls = append(calls, c.Delta.ToolCalls...)
		}
	}
	return calls
}

// A Gemini 3 tool loop on /ai: the first call's signature comes back to the
// client inside its tool call ID, never in its arguments, and is on the part
// again when the client sends the calls back, so the second turn is served.
// Buffered and streamed, Google AI and Vertex, two parallel calls.
func TestGemini3ToolRoundTrip(t *testing.T) {
	t.Parallel()
	for _, vendor := range []models.Vendor{models.VERTEX, models.GOOGLEAI} {
		for _, stream := range []bool{false, true} {
			t.Run(string(vendor)+map[bool]string{false: "/buffered", true: "/stream"}[stream], func(t *testing.T) {
				fake := &fakeGemini3{}
				h := newFailoverHarness(t, fake.handler, serveOpenAIText("never"), func(primary, _ *models.LLM) {
					primary.Vendor = vendor
					primary.APIKey = vertexTestToken
					if vendor == models.VERTEX {
						primary.APIEndpoint += vertexEndpointSuffix
					}
					primary.DefaultModel = "gemini-3.5-flash"
					primary.Failover = models.LLMFailover{}
				})
				streamField := `"stream":` + map[bool]string{false: "false", true: "true"}[stream]

				resp, out := h.post("/ai/primary/v1/chat/completions",
					`{"model":"gemini-3.5-flash",`+streamField+`,"messages":[{"role":"user","content":"time and weather?"}],`+gemini3Tools+`}`)
				require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
				assert.NotContains(t, string(out), googleai.ThoughtSignatureArg, "the reserved key never reaches the client")
				calls := turnOneCalls(t, out, stream)
				require.Len(t, calls, 2, string(out))
				assert.Equal(t, "get_time", calls[0].Function.Name)
				assert.JSONEq(t, `{"zone":"UTC"}`, calls[0].Function.Arguments)
				sig, ok := googleai.DecodeSignatureID(calls[0].ID)
				require.True(t, ok, "the signed call's ID carries the signature: %s", calls[0].ID)
				assert.Equal(t, gemini3Signature, sig)
				assert.True(t, strings.HasPrefix(calls[1].ID, "call_"), calls[1].ID)
				_, ok = googleai.DecodeSignatureID(calls[1].ID)
				assert.False(t, ok, "an unsigned call gets a plain ID")
				assert.NotEqual(t, calls[0].ID, calls[1].ID)

				// The client echoes the calls back unchanged, with the results.
				echoed, err := json.Marshal(calls)
				require.NoError(t, err)
				turn2 := `{"model":"gemini-3.5-flash",` + streamField + `,"messages":[{"role":"user","content":"time and weather?"},` +
					`{"role":"assistant","content":null,"tool_calls":` + string(echoed) + `},` +
					`{"role":"tool","tool_call_id":"` + calls[0].ID + `","content":"12:00"},` +
					`{"role":"tool","tool_call_id":"` + calls[1].ID + `","content":"sunny"}],` + gemini3Tools + `}`
				resp, out = h.post("/ai/primary/v1/chat/completions", turn2)
				require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
				assert.Contains(t, string(out), "12:00, sunny")

				fake.mu.Lock()
				defer fake.mu.Unlock()
				require.Len(t, fake.bodies, 2)
				assert.NotContains(t, fake.bodies[1], googleai.ThoughtSignatureArg, "the reserved key never reaches the API")
				assert.Contains(t, fake.bodies[1], `"thoughtSignature":"`+gemini3Signature+`"`)
				assert.Equal(t, 1, strings.Count(fake.bodies[1], `"thoughtSignature"`), "only the signed call gets one")
			})
		}
	}
}

// The unified /v1 route carries the signature the same way.
func TestGemini3ToolRoundTrip_UnifiedRoute(t *testing.T) {
	t.Parallel()
	fake := &fakeGemini3{}
	h := newFailoverHarness(t, fake.handler, serveOpenAIText("never"), func(primary, _ *models.LLM) {
		primary.Vendor = models.VERTEX
		primary.APIKey = vertexTestToken
		primary.APIEndpoint += vertexEndpointSuffix
		primary.DefaultModel = "gemini-3.5-flash"
		primary.Failover = models.LLMFailover{}
	})
	resp, out := h.post("/v1/chat/completions", `{"model":"primary/gemini-3.5-flash","messages":[{"role":"user","content":"time and weather?"}],`+gemini3Tools+`}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	calls := turnOneCalls(t, out, false)
	require.Len(t, calls, 2)
	echoed, err := json.Marshal(calls)
	require.NoError(t, err)
	resp, out = h.post("/v1/chat/completions", `{"model":"primary/gemini-3.5-flash","messages":[{"role":"user","content":"time and weather?"},`+
		`{"role":"assistant","content":null,"tool_calls":`+string(echoed)+`},`+
		`{"role":"tool","tool_call_id":"`+calls[0].ID+`","content":"12:00"},{"role":"tool","tool_call_id":"`+calls[1].ID+`","content":"sunny"}],`+gemini3Tools+`}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	assert.Contains(t, string(out), "12:00, sunny")
}

// When the signature cannot be recovered from the ID (a client rewrote it,
// the ID is garbage, or its signature is over the cap), the turn's first call
// goes upstream with the placeholder and the request is served.
func TestGemini3ToolRoundTrip_PlaceholderFallback(t *testing.T) {
	t.Parallel()
	for name, id := range map[string]string{
		"rewritten id": "call_client_renamed_1",
		"garbage id":   "call_gs1_zzzzzzzz_***",
		"oversize id":  googleai.EncodeSignatureID(strings.Repeat("s", googleai.MaxThoughtSignatureBytes+1)),
	} {
		for _, vendor := range []models.Vendor{models.VERTEX, models.GOOGLEAI} {
			t.Run(name+"/"+string(vendor), func(t *testing.T) {
				fake := &fakeGemini3{}
				h := newFailoverHarness(t, fake.handler, serveOpenAIText("never"), func(primary, _ *models.LLM) {
					primary.Vendor = vendor
					primary.APIKey = vertexTestToken
					if vendor == models.VERTEX {
						primary.APIEndpoint += vertexEndpointSuffix
					}
					primary.DefaultModel = "gemini-3.5-flash"
					primary.Failover = models.LLMFailover{}
				})
				turn2 := `{"model":"gemini-3.5-flash","messages":[{"role":"user","content":"time and weather?"},` +
					`{"role":"assistant","content":null,"tool_calls":[` +
					`{"id":"` + id + `","type":"function","function":{"name":"get_time","arguments":"{\"zone\":\"UTC\"}"}},` +
					`{"id":"call_2","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Paris\"}"}}]},` +
					`{"role":"tool","tool_call_id":"` + id + `","content":"12:00"},{"role":"tool","tool_call_id":"call_2","content":"sunny"}],` + gemini3Tools + `}`
				resp, out := h.post("/ai/primary/v1/chat/completions", turn2)
				require.Equal(t, http.StatusOK, resp.StatusCode, string(out))

				fake.mu.Lock()
				defer fake.mu.Unlock()
				require.Len(t, fake.bodies, 1)
				assert.Contains(t, fake.bodies[0], `"thoughtSignature":"`+placeholderThoughtSignature+`"`)
				assert.Equal(t, 1, strings.Count(fake.bodies[0], `"thoughtSignature"`), "only the turn's first call")
				assert.NotContains(t, fake.bodies[0], googleai.ThoughtSignatureArg)
			})
		}
	}
}
