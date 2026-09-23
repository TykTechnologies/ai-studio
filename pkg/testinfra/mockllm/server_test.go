package mockllm

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func post(t *testing.T, url, body string, header map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestServerOpenAIRestUsageAndModel(t *testing.T) {
	srv := httptest.NewServer(NewServer(Profile{OutputTokens: 5, TokenText: "ab"}))
	defer srv.Close()

	resp := post(t, srv.URL+"/v1/chat/completions", `{"model":"gpt-x","messages":[]}`, nil)
	defer resp.Body.Close()
	var out struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
		Usage struct {
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Model != "gpt-x" || out.Choices[0].Message.Content != "ababababab" || out.Usage.CompletionTokens != 5 {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestServerOpenAIStreamFramesAndPacing(t *testing.T) {
	p := Profile{TTFT: 30 * time.Millisecond, TokensPerSecond: 100, OutputTokens: 5, TokenText: "x"}
	srv := httptest.NewServer(NewServer(p))
	defer srv.Close()

	start := time.Now()
	resp := post(t, srv.URL+"/v1/chat/completions", `{"model":"m","stream":true}`, nil)
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type %q", ct)
	}

	var firstAt time.Duration
	var content, sawUsage, sawDone int
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			sawDone++
			continue
		}
		if strings.Contains(data, `"content":"x"`) {
			if content == 0 {
				firstAt = time.Since(start)
			}
			content++
		}
		if strings.Contains(data, `"usage"`) {
			sawUsage++
		}
	}
	total := time.Since(start)

	if content != 5 || sawUsage != 1 || sawDone != 1 {
		t.Fatalf("content=%d usage=%d done=%d", content, sawUsage, sawDone)
	}
	if firstAt < 30*time.Millisecond {
		t.Fatalf("first token after %v, want >= 30ms", firstAt)
	}
	// 4 more tokens at 10ms each after the first.
	if total < 70*time.Millisecond {
		t.Fatalf("stream finished after %v, want >= 70ms", total)
	}
}

func TestServerAnthropicStreamEvents(t *testing.T) {
	srv := httptest.NewServer(NewServer(Profile{OutputTokens: 3, TokenText: "y"}))
	defer srv.Close()

	resp := post(t, srv.URL+"/v1/messages", `{"model":"claude-x","stream":true}`, nil)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	body := string(b)
	for _, ev := range []string{"message_start", "content_block_start", "content_block_stop", "message_delta", "message_stop"} {
		if strings.Count(body, "event: "+ev+"\n") != 1 {
			t.Fatalf("want exactly one %s event in:\n%s", ev, body)
		}
	}
	if n := strings.Count(body, "event: content_block_delta\n"); n != 3 {
		t.Fatalf("want 3 deltas, got %d", n)
	}
	if !strings.Contains(body, `"output_tokens":3`) {
		t.Fatalf("message_delta usage missing:\n%s", body)
	}
}

func TestServerAnthropicRest(t *testing.T) {
	srv := httptest.NewServer(NewServer(Profile{OutputTokens: 2, TokenText: "z"}))
	defer srv.Close()

	resp := post(t, srv.URL+"/v1/messages", `{"model":"claude-x"}`, nil)
	defer resp.Body.Close()
	var out struct {
		Type    string                  `json:"type"`
		Content []struct{ Text string } `json:"content"`
		Usage   struct {
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Type != "message" || out.Content[0].Text != "zz" || out.Usage.OutputTokens != 2 {
		t.Fatalf("unexpected: %+v", out)
	}
}

func TestServerProfileHeaderOverride(t *testing.T) {
	s := NewServer(Profile{OutputTokens: 1})
	srv := httptest.NewServer(s)
	defer srv.Close()

	resp := post(t, srv.URL+"/v1/chat/completions", `{}`, map[string]string{ProfileHeader: "tokens=4,token_text=q"})
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), `"content":"qqqq"`) {
		t.Fatalf("override not applied: %s", b)
	}

	resp = post(t, srv.URL+"/v1/chat/completions", `{}`, map[string]string{ProfileHeader: "bogus=1"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad override should 400, got %d", resp.StatusCode)
	}
	if got := s.Stats().Requests; got != 2 {
		t.Fatalf("requests=%d", got)
	}
}

func TestServerFailureRate(t *testing.T) {
	s := NewServer(Profile{FailureRate: 1})
	srv := httptest.NewServer(s)
	defer srv.Close()

	resp := post(t, srv.URL+"/v1/chat/completions", `{}`, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError || s.Stats().Failures != 1 {
		t.Fatalf("status=%d failures=%d", resp.StatusCode, s.Stats().Failures)
	}
}

func TestProfileLognormalTTFT(t *testing.T) {
	p := Profile{TTFT: 100 * time.Millisecond, TTFTP99: 400 * time.Millisecond}
	var below int
	const n = 20000
	for i := 0; i < n; i++ {
		if p.sampleTTFT() <= 100*time.Millisecond {
			below++
		}
	}
	// The median is TTFT, so about half the samples fall at or below it.
	if frac := float64(below) / n; frac < 0.47 || frac > 0.53 {
		t.Fatalf("fraction below median = %.3f", frac)
	}
}

func TestServerProfileInPath(t *testing.T) {
	srv := httptest.NewServer(NewServer(Profile{OutputTokens: 1}))
	defer srv.Close()

	resp := post(t, srv.URL+"/p/tokens=3,token_text=k/v1/chat/completions", `{}`, nil)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), `"content":"kkk"`) {
		t.Fatalf("path profile not applied: %s", b)
	}
	// The header override applies on top of the path profile.
	resp = post(t, srv.URL+"/p/tokens=3,token_text=k/v1/messages", `{}`, map[string]string{ProfileHeader: "tokens=2"})
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), `"text":"kk"`) {
		t.Fatalf("header override not layered on path profile: %s", b)
	}
}
