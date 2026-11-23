package scripting

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// The request the judge sends: a model that reasons gets the larger output
// limit (its reasoning tokens count against it), any other model the
// 64-token answer limit (midsommar-clean-6bg5.26).
func TestJudgeCall_OutputLimitPerModel(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]json.RawMessage
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c1","object":"chat.completion","created":1,"model":"m",` +
			`"choices":[{"index":0,"message":{"role":"assistant","content":"{\"verdict\":\"benign\",\"reason\":\"code_or_data\"}"},"finish_reason":"stop"}],` +
			`"usage":{"prompt_tokens":50,"completion_tokens":12,"total_tokens":62}}`))
	}))
	defer srv.Close()

	llm := &models.LLM{ID: 91, Name: "judge", Vendor: models.OPENAI, APIEndpoint: srv.URL + "/v1", APIKey: "sk-test", AllowInternalHost: true, Active: true}
	for _, tc := range []struct {
		model string
		want  string
	}{
		{"gpt-5.6-luna", "2048"},
		{"gpt-4o-mini", "64"},
	} {
		out, err := judgeCall(context.Background(), llm, tc.model, "system", "user")
		if err != nil {
			t.Fatalf("%s: %v", tc.model, err)
		}
		if _, ok := parseJudgeAnswer(out.Text); !ok {
			t.Fatalf("%s: answer not parsed: %q", tc.model, out.Text)
		}
		mu.Lock()
		body := bodies[len(bodies)-1]
		mu.Unlock()
		limit := string(body["max_completion_tokens"])
		if limit == "" {
			limit = string(body["max_tokens"])
		}
		if limit != tc.want {
			t.Errorf("%s: output limit sent %q, want %s (body keys %v)", tc.model, limit, tc.want, body)
		}
	}
}
