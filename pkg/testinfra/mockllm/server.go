package mockllm

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// Server is a benchmark-grade mock LLM upstream. Unlike MockLLMBackend it
// records nothing per request (so memory stays flat under sustained load),
// speaks both the OpenAI chat-completions and Anthropic messages wire formats,
// streams SSE when the request body sets "stream": true, and paces its output
// according to a Profile.
//
// Routing is by path suffix, so it works behind any base URL a gateway LLM
// row is configured with:
//
//	*/chat/completions  OpenAI format
//	*/messages          Anthropic format
//	/health, /healthz   liveness
//	/stats              request counters (JSON)
//
// A path beginning /p/{spec}/ applies the ParseProfile overrides in {spec}
// (URL-escaped) before the ProfileHeader ones. Putting the profile in the base
// URL of a gateway LLM row makes it survive every gateway path, including the
// /ai/ loopback, which does not forward client headers.
type Server struct {
	profile atomic.Pointer[Profile]

	requests atomic.Int64
	active   atomic.Int64
	failures atomic.Int64
	streams  atomic.Int64
}

// NewServer returns a Server whose default profile is p.
func NewServer(p Profile) *Server {
	s := &Server{}
	s.SetProfile(p)
	return s
}

// SetProfile replaces the default profile. Safe to call while serving.
func (s *Server) SetProfile(p Profile) {
	if p.OutputTokens < 1 {
		p.OutputTokens = 1
	}
	if p.TokenText == "" {
		p.TokenText = "tok "
	}
	s.profile.Store(&p)
}

// Stats is the /stats payload.
type Stats struct {
	Requests int64 `json:"requests"`
	Active   int64 `json:"active"`
	Failures int64 `json:"failures"`
	Streams  int64 `json:"streams"`
}

// Stats returns the current counters.
func (s *Server) Stats() Stats {
	return Stats{
		Requests: s.requests.Load(),
		Active:   s.active.Load(),
		Failures: s.failures.Load(),
		Streams:  s.streams.Load(),
	}
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/health" || path == "/healthz":
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
		return
	case path == "/stats":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.Stats())
		return
	}

	// Arrival time anchors every delay, so time spent reading a large body
	// counts against the first-token delay the way it would at a vendor.
	start := time.Now()
	s.requests.Add(1)
	s.active.Add(1)
	defer s.active.Add(-1)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "reading body: "+err.Error(), http.StatusBadRequest)
		return
	}

	p := *s.profile.Load()
	if rest, ok := strings.CutPrefix(path, "/p/"); ok {
		rawSpec, _, _ := strings.Cut(rest, "/")
		spec, uerr := url.PathUnescape(rawSpec)
		if uerr == nil {
			p, err = ParseProfile(p, spec)
		}
		if uerr != nil || err != nil {
			http.Error(w, fmt.Sprintf("bad profile in path: %v %v", uerr, err), http.StatusBadRequest)
			return
		}
	}
	if spec := r.Header.Get(ProfileHeader); spec != "" {
		if p, err = ParseProfile(p, spec); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	var req struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	_ = json.Unmarshal(body, &req)
	model := req.Model
	if model == "" {
		model = p.Model
	}
	promptTokens := len(body) / 4
	if promptTokens < 1 {
		promptTokens = 1
	}

	anthropic := strings.HasSuffix(path, "/messages")

	if p.FailureRate > 0 && rand.Float64() < p.FailureRate {
		s.failures.Add(1)
		sleepUntil(start.Add(p.sampleTTFT()))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(BuildErrorResponse("server_error", "simulated failure", "internal_error"))
		return
	}

	firstToken := start.Add(p.sampleTTFT())
	gen := generation{
		model:        model,
		promptTokens: promptTokens,
		profile:      p,
		firstToken:   firstToken,
		interval:     p.tokenInterval(),
	}

	switch {
	case req.Stream && anthropic:
		s.streams.Add(1)
		gen.streamAnthropic(w, r)
	case req.Stream:
		s.streams.Add(1)
		gen.streamOpenAI(w, r)
	case anthropic:
		gen.restAnthropic(w)
	default:
		gen.restOpenAI(w)
	}
}

// generation holds everything needed to write one response.
type generation struct {
	model        string
	promptTokens int
	profile      Profile
	firstToken   time.Time
	interval     time.Duration
}

// tokenAt is when token i (0-based) is due.
func (g generation) tokenAt(i int) time.Time {
	return g.firstToken.Add(time.Duration(i) * g.interval)
}

func (g generation) text() string {
	return strings.Repeat(g.profile.TokenText, g.profile.OutputTokens)
}

func (g generation) restOpenAI(w http.ResponseWriter) {
	sleepUntil(g.tokenAt(g.profile.OutputTokens - 1))
	resp := map[string]any{
		"id":      fmt.Sprintf("chatcmpl-mock-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   g.model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]string{"role": "assistant", "content": g.text()},
			"finish_reason": "stop",
		}},
		"usage": map[string]int{
			"prompt_tokens":     g.promptTokens,
			"completion_tokens": g.profile.OutputTokens,
			"total_tokens":      g.promptTokens + g.profile.OutputTokens,
		},
	}
	writeJSON(w, resp)
}

func (g generation) restAnthropic(w http.ResponseWriter) {
	sleepUntil(g.tokenAt(g.profile.OutputTokens - 1))
	resp := map[string]any{
		"id":            fmt.Sprintf("msg_mock_%d", time.Now().UnixNano()),
		"type":          "message",
		"role":          "assistant",
		"model":         g.model,
		"content":       []map[string]string{{"type": "text", "text": g.text()}},
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage": map[string]int{
			"input_tokens":  g.promptTokens,
			"output_tokens": g.profile.OutputTokens,
		},
	}
	writeJSON(w, resp)
}

func (g generation) streamOpenAI(w http.ResponseWriter, r *http.Request) {
	id := fmt.Sprintf("chatcmpl-mock-%d", time.Now().UnixNano())
	created := time.Now().Unix()
	chunk := func(delta map[string]string, finish any) []byte {
		b, _ := json.Marshal(map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": created, "model": g.model,
			"choices": []map[string]any{{"index": 0, "delta": delta, "finish_reason": finish}},
		})
		return sseData(b)
	}
	// Every content frame is identical, so marshal it once.
	first := chunk(map[string]string{"role": "assistant", "content": g.profile.TokenText}, nil)
	content := chunk(map[string]string{"content": g.profile.TokenText}, nil)
	finish := chunk(map[string]string{}, "stop")
	usageJSON, _ := json.Marshal(map[string]any{
		"id": id, "object": "chat.completion.chunk", "created": created, "model": g.model,
		"choices": []any{},
		"usage": map[string]int{
			"prompt_tokens":     g.promptTokens,
			"completion_tokens": g.profile.OutputTokens,
			"total_tokens":      g.promptTokens + g.profile.OutputTokens,
		},
	})

	sw, ok := startSSE(w, g.firstToken)
	if !ok {
		return
	}
	if !sw.write(first) {
		return
	}
	for i := 1; i < g.profile.OutputTokens; i++ {
		if !sw.writeAt(r, g.tokenAt(i), content) {
			return
		}
	}
	sw.write(finish)
	sw.write(sseData(usageJSON))
	sw.write([]byte("data: [DONE]\n\n"))
}

func (g generation) streamAnthropic(w http.ResponseWriter, r *http.Request) {
	id := fmt.Sprintf("msg_mock_%d", time.Now().UnixNano())
	event := func(name string, payload any) []byte {
		b, _ := json.Marshal(payload)
		return []byte("event: " + name + "\ndata: " + string(b) + "\n\n")
	}
	start := event("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": id, "type": "message", "role": "assistant", "model": g.model,
			"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]int{"input_tokens": g.promptTokens, "output_tokens": 1},
		},
	})
	blockStart := event("content_block_start", map[string]any{
		"type": "content_block_start", "index": 0,
		"content_block": map[string]string{"type": "text", "text": ""},
	})
	delta := event("content_block_delta", map[string]any{
		"type": "content_block_delta", "index": 0,
		"delta": map[string]string{"type": "text_delta", "text": g.profile.TokenText},
	})
	blockStop := event("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
	msgDelta := event("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": map[string]int{"output_tokens": g.profile.OutputTokens},
	})
	msgStop := event("message_stop", map[string]string{"type": "message_stop"})

	sw, ok := startSSE(w, g.firstToken)
	if !ok {
		return
	}
	var head []byte
	head = append(head, start...)
	head = append(head, blockStart...)
	head = append(head, delta...)
	if !sw.write(head) {
		return
	}
	for i := 1; i < g.profile.OutputTokens; i++ {
		if !sw.writeAt(r, g.tokenAt(i), delta) {
			return
		}
	}
	sw.write(blockStop)
	sw.write(msgDelta)
	sw.write(msgStop)
}

// sseWriter writes and flushes SSE frames, stopping on the first write error
// (the client went away).
type sseWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

// startSSE waits for the first token, then sends the SSE headers.
func startSSE(w http.ResponseWriter, firstToken time.Time) (*sseWriter, bool) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return nil, false
	}
	sleepUntil(firstToken)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	return &sseWriter{w: w, f: f}, true
}

func (s *sseWriter) write(b []byte) bool {
	if _, err := s.w.Write(b); err != nil {
		return false
	}
	s.f.Flush()
	return true
}

// writeAt writes b once its due time arrives, or gives up if the client left.
func (s *sseWriter) writeAt(r *http.Request, due time.Time, b []byte) bool {
	if d := time.Until(due); d > 0 {
		t := time.NewTimer(d)
		select {
		case <-t.C:
		case <-r.Context().Done():
			t.Stop()
			return false
		}
	}
	return s.write(b)
}

func sseData(b []byte) []byte {
	out := make([]byte, 0, len(b)+8)
	out = append(out, "data: "...)
	out = append(out, b...)
	return append(out, '\n', '\n')
}

func writeJSON(w http.ResponseWriter, v any) {
	b, _ := json.Marshal(v)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

// sleepUntil sleeps until t. Scheduling against an absolute time keeps a long
// stream from accumulating per-token sleep overshoot.
func sleepUntil(t time.Time) {
	if d := time.Until(t); d > 0 {
		time.Sleep(d)
	}
}
