// Command agent-tracing-demo is a small agent that exercises Agent Call
// Tracing through a Tyk AI Studio gateway (the embedded gateway or a
// microgateway edge). It runs one session:
//
//   - an agent loop as "orchestrator": the model is offered the App's Studio
//     MCP tool (discovered with tools/list over /tools/{slug}/mcp) and a
//     local read_file tool; MCP calls go through the gateway, read_file runs
//     here (a client-side tool), and the final step streams;
//   - one call from a declared sub-agent (X-Tyk-Agent-Path);
//   - optionally a Responses API chain (previous_response_id);
//   - one call to an LLM whose guardrail blocks it.
//
// With -mock-vendor it also serves a scripted OpenAI-shaped vendor, so the
// run is repeatable without vendor keys: point the App's LLMs at it.
//
//	go run ./examples/agent-tracing-demo -mock-vendor 127.0.0.1:28999   # vendor only
//	go run ./examples/agent-tracing-demo -gateway http://localhost:29090 \
//	  -token $APP_SECRET -llm demo-llm -guarded demo-guarded -tool demo-tool
//
// The App credential is read from -token or DEMO_APP_TOKEN. Nothing here
// stores a credential.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type config struct {
	gateway, token, llm, guarded, tool, model, mode, session string
	responses                                                bool
	steps                                                    int
}

func main() {
	var c config
	var mock string
	flag.StringVar(&c.gateway, "gateway", "", "gateway base URL, e.g. http://localhost:29090 (embedded) or http://localhost:28081 (edge)")
	flag.StringVar(&c.token, "token", os.Getenv("DEMO_APP_TOKEN"), "App credential (default $DEMO_APP_TOKEN)")
	flag.StringVar(&c.llm, "llm", "", "LLM slug the agent uses")
	flag.StringVar(&c.guarded, "guarded", "", "LLM slug whose guardrail blocks every request (optional)")
	flag.StringVar(&c.tool, "tool", "", "Studio tool slug served over MCP (optional)")
	flag.StringVar(&c.model, "model", "gpt-4o", "model name")
	flag.StringVar(&c.mode, "route", "rest", "rest: /llm/rest|stream/{slug}/v1/chat/completions (OpenAI-shaped LLMs); ai: /ai/{slug}/v1/chat/completions (any vendor)")
	flag.StringVar(&c.session, "session", "", "X-Tyk-Session-Id (default: generated)")
	flag.BoolVar(&c.responses, "responses", false, "also run a Responses API chain (OpenAI-shaped LLMs, rest route)")
	flag.IntVar(&c.steps, "max-steps", 4, "agent loop step limit")
	flag.StringVar(&mock, "mock-vendor", "", "serve the scripted mock vendor on this address")
	flag.Parse()

	if mock != "" {
		go func() {
			log.Printf("mock vendor on http://%s", mock)
			log.Fatal(http.ListenAndServe(mock, http.HandlerFunc(serveMock)))
		}()
	}
	if c.gateway == "" {
		if mock == "" {
			flag.Usage()
			os.Exit(2)
		}
		select {} // vendor only
	}
	if c.token == "" || c.llm == "" {
		log.Fatal("-token (or DEMO_APP_TOKEN) and -llm are required")
	}
	if c.session == "" {
		c.session = fmt.Sprintf("demo-%d", time.Now().Unix())
	}
	if err := run(c); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("done: session %s\n", c.session)
}

type message struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func run(c config) error {
	h := func(extra ...string) []string {
		return append([]string{"X-Tyk-Session-Id", c.session}, extra...)
	}
	orch := h("X-Tyk-Agent-Name", "orchestrator")

	var mcp *mcpClient
	tools := []any{fnTool("read_file", "Read a local notes file", `{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)}
	if c.tool != "" {
		var err error
		if mcp, err = openMCP(c, orch); err != nil {
			return err
		}
		listed, err := mcp.list()
		if err != nil {
			return err
		}
		tools = append(tools, listed...)
		fmt.Printf("MCP session %s, %d tool(s) offered\n", mcp.session, len(listed))
	}

	msgs := []message{{Role: "user", Content: "What is the weather today? Use the weather tool and read notes.txt, then answer in one sentence."}}
	for step := 0; step < c.steps; step++ {
		last := step == c.steps-1
		stream := step > 0 // follow-up steps stream
		body := map[string]any{"model": c.model, "messages": msgs, "tools": tools}
		if last {
			delete(body, "tools")
		}
		reply, err := c.chat(c.llm, body, stream, orch...)
		if err != nil {
			return fmt.Errorf("step %d: %w", step+1, err)
		}
		fmt.Printf("step %d: %d tool call(s), text %q\n", step+1, len(reply.ToolCalls), clip(fmt.Sprint(reply.Content)))
		if len(reply.ToolCalls) == 0 {
			break
		}
		msgs = append(msgs, reply)
		for _, tc := range reply.ToolCalls {
			var out string
			switch {
			case tc.Function.Name == "read_file":
				out = "notes.txt: remember the umbrella" // the client-side tool
			case mcp != nil:
				if out, err = mcp.call(tc.Function.Name, tc.Function.Arguments); err != nil {
					return err
				}
			default:
				out = "unknown tool"
			}
			msgs = append(msgs, message{Role: "tool", ToolCallID: tc.ID, Content: out})
		}
	}

	if _, err := c.chat(c.llm, map[string]any{"model": c.model,
		"messages": []message{{Role: "user", Content: "Research umbrellas in one line."}}}, false,
		h("X-Tyk-Agent-Path", "/orchestrator/researcher")...); err != nil {
		return fmt.Errorf("sub-agent: %w", err)
	}
	fmt.Println("sub-agent call ok")

	if c.responses {
		id, err := c.respond(map[string]any{"model": c.model, "input": "Summarise the weather."}, orch)
		if err != nil {
			return err
		}
		if _, err := c.respond(map[string]any{"model": c.model, "input": "Shorter.", "previous_response_id": id}, orch); err != nil {
			return err
		}
		fmt.Println("responses chain ok")
	}

	if c.guarded != "" {
		_, err := c.chat(c.guarded, map[string]any{"model": c.model,
			"messages": []message{{Role: "user", Content: "Do the forbidden thing."}}}, false, orch...)
		var se *statusError
		if !errors.As(err, &se) || se.code < 400 || se.code >= 500 {
			return fmt.Errorf("guarded call: want a 4xx block, got %v", err)
		}
		fmt.Printf("guardrail blocked: HTTP %d\n", se.code)
	}
	return nil
}

func fnTool(name, desc, params string) map[string]any {
	return map[string]any{"type": "function", "function": map[string]any{
		"name": name, "description": desc, "parameters": json.RawMessage(params)}}
}

type statusError struct {
	code int
	body string
}

func (e *statusError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.code, clip(e.body)) }

func (c config) post(path string, body any, headers []string, accept string) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(c.gateway, "/")+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return nil, &statusError{resp.StatusCode, string(b)}
	}
	return resp, nil
}

func (c config) chatPath(slug string, stream bool) string {
	if c.mode == "ai" {
		return "/ai/" + slug + "/v1/chat/completions"
	}
	if stream {
		return "/llm/stream/" + slug + "/v1/chat/completions"
	}
	return "/llm/rest/" + slug + "/v1/chat/completions"
}

// chat sends one chat completion and returns the assistant message,
// assembling a streamed reply from its deltas.
func (c config) chat(slug string, body map[string]any, stream bool, headers ...string) (message, error) {
	if stream {
		body["stream"] = true
		body["stream_options"] = map[string]any{"include_usage": true}
	}
	resp, err := c.post(c.chatPath(slug, stream), body, headers, "")
	if err != nil {
		return message{}, err
	}
	defer resp.Body.Close()
	if !stream {
		var r struct {
			Choices []struct {
				Message message `json:"message"`
			} `json:"choices"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&r); err != nil || len(r.Choices) == 0 {
			return message{}, fmt.Errorf("unexpected reply: %v", err)
		}
		return r.Choices[0].Message, nil
	}
	out := message{Role: "assistant"}
	var text strings.Builder
	calls := map[int]*toolCall{}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok || data == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		for _, ch := range chunk.Choices {
			text.WriteString(ch.Delta.Content)
			for _, d := range ch.Delta.ToolCalls {
				tc := calls[d.Index]
				if tc == nil {
					tc = &toolCall{Type: "function"}
					calls[d.Index] = tc
				}
				if d.ID != "" {
					tc.ID = d.ID
				}
				tc.Function.Name += d.Function.Name
				tc.Function.Arguments += d.Function.Arguments
			}
		}
	}
	for i := 0; i < len(calls); i++ {
		if tc := calls[i]; tc != nil {
			out.ToolCalls = append(out.ToolCalls, *tc)
		}
	}
	out.Content = text.String()
	return out, sc.Err()
}

// respond sends one Responses API request and returns the response id the
// gateway handed out.
func (c config) respond(body map[string]any, headers []string) (string, error) {
	resp, err := c.post("/llm/rest/"+c.llm+"/v1/responses", body, headers, "")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var r struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil || r.ID == "" {
		return "", fmt.Errorf("responses: no id (%v)", err)
	}
	return r.ID, nil
}

type mcpClient struct {
	c       config
	headers []string
	session string
	id      int
}

func openMCP(c config, headers []string) (*mcpClient, error) {
	m := &mcpClient{c: c, headers: headers}
	resp, err := m.rpc("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "agent-tracing-demo", "version": "1"}})
	if err != nil {
		return nil, fmt.Errorf("MCP initialize: %w", err)
	}
	m.session = resp.Header.Get("Mcp-Session-Id")
	resp.Body.Close()
	if r, err := m.notify("notifications/initialized"); err == nil {
		r.Body.Close()
	}
	return m, nil
}

func (m *mcpClient) send(body map[string]any) (*http.Response, error) {
	h := append([]string(nil), m.headers...)
	if m.session != "" {
		h = append(h, "Mcp-Session-Id", m.session)
	}
	return m.c.post("/tools/"+m.c.tool+"/mcp", body, h, "application/json, text/event-stream")
}

func (m *mcpClient) rpc(method string, params any) (*http.Response, error) {
	m.id++
	return m.send(map[string]any{"jsonrpc": "2.0", "id": m.id, "method": method, "params": params})
}

func (m *mcpClient) notify(method string) (*http.Response, error) {
	return m.send(map[string]any{"jsonrpc": "2.0", "method": method})
}

// result reads a JSON-RPC result from a JSON or SSE response.
func result(resp *http.Response, into any) error {
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		for _, line := range strings.Split(string(b), "\n") {
			if d, ok := strings.CutPrefix(line, "data: "); ok {
				b = []byte(d)
			}
		}
	}
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return err
	}
	if env.Error != nil {
		return errors.New(env.Error.Message)
	}
	return json.Unmarshal(env.Result, into)
}

func (m *mcpClient) list() ([]any, error) {
	resp, err := m.rpc("tools/list", map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("MCP tools/list: %w", err)
	}
	var r struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := result(resp, &r); err != nil {
		return nil, err
	}
	var out []any
	for _, t := range r.Tools {
		schema := string(t.InputSchema)
		if schema == "" || schema == "null" {
			schema = `{"type":"object","properties":{}}`
		}
		out = append(out, fnTool(t.Name, t.Description, schema))
	}
	return out, nil
}

func (m *mcpClient) call(name, args string) (string, error) {
	var a map[string]any
	if strings.TrimSpace(args) != "" {
		if err := json.Unmarshal([]byte(args), &a); err != nil {
			return "", fmt.Errorf("tool %s arguments: %w", name, err)
		}
	}
	if a == nil {
		a = map[string]any{}
	}
	resp, err := m.rpc("tools/call", map[string]any{"name": name, "arguments": a})
	if err != nil {
		return "", fmt.Errorf("MCP tools/call %s: %w", name, err)
	}
	var r struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := result(resp, &r); err != nil {
		return "", err
	}
	var parts []string
	for _, c := range r.Content {
		parts = append(parts, c.Text)
	}
	return strings.Join(parts, "\n"), nil
}

func clip(s string) string {
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}

// serveMock is a scripted OpenAI-shaped vendor (and, under /tool/, a Studio
// tool upstream with weather and advice operations): the first step of a turn
// asks for every offered tool (first MCP tool with {} and read_file),
// a step carrying tool results answers (streamed when asked), Responses
// requests get ids that chain, and anything else gets "done".
func serveMock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Stream   bool `json:"stream"`
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
		Tools []struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
		PreviousResponseID string `json:"previous_response_id"`
	}
	if strings.HasPrefix(r.URL.Path, "/tool/") { // a Studio tool's upstream
		w.Header().Set("Content-Type", "application/json")
		switch strings.TrimPrefix(r.URL.Path, "/tool/") {
		case "weather":
			fmt.Fprint(w, `{"forecast":"sunny","high_c":19}`)
		default:
			fmt.Fprint(w, `{"advice":"take an umbrella"}`)
		}
		return
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	usage := `"usage":{"prompt_tokens":20,"completion_tokens":8,"total_tokens":28}`
	w.Header().Set("Content-Type", "application/json")
	if strings.HasSuffix(r.URL.Path, "/responses") {
		id := "resp_mock_1"
		if req.PreviousResponseID != "" {
			id = "resp_mock_2"
		}
		fmt.Fprintf(w, `{"id":%q,"object":"response","created_at":1,"status":"completed","model":"gpt-4o",`+
			`"output":[{"type":"message","id":"msg_%s","status":"completed","role":"assistant","content":[{"type":"output_text","text":"noted","annotations":[]}]}],`+
			`"usage":{"input_tokens":20,"output_tokens":8,"total_tokens":28}}`, id, id)
		return
	}
	hasResults := false
	for _, m := range req.Messages {
		hasResults = hasResults || m.Role == "tool"
	}
	if !hasResults && len(req.Tools) > 0 {
		var calls []string
		for i, t := range req.Tools {
			args := "{}"
			if t.Function.Name == "read_file" {
				args = `{\"path\":\"notes.txt\"}`
			} else if i > 2 {
				continue // two MCP tools are enough
			}
			calls = append(calls, fmt.Sprintf(`{"id":"call_%d","type":"function","function":{"name":%q,"arguments":"%s"}}`, i, t.Function.Name, args))
		}
		fmt.Fprintf(w, `{"id":"c1","object":"chat.completion","created":1,"model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[%s]},"finish_reason":"tool_calls"}],%s}`,
			strings.Join(calls, ","), usage)
		return
	}
	text := "done"
	if hasResults {
		text = "It is sunny; take the umbrella anyway."
	}
	if req.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		f, _ := w.(http.Flusher)
		for _, piece := range strings.SplitAfter(text, " ") {
			fmt.Fprintf(w, "data: {\"id\":\"c2\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":%q}}]}\n\n", piece)
			if f != nil {
				f.Flush()
			}
		}
		fmt.Fprintf(w, "data: {\"id\":\"c2\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],%s}\n\n", usage)
		fmt.Fprint(w, "data: [DONE]\n\n")
		return
	}
	fmt.Fprintf(w, `{"id":"c3","object":"chat.completion","created":1,"model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}],%s}`, text, usage)
}
