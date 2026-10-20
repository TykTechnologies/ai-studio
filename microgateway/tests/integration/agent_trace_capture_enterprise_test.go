//go:build enterprise
// +build enterprise

package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// The edge's content pipeline, as microgateway/cmd/microgateway/main_enterprise.go imports it.
	_ "github.com/TykTechnologies/ai-studio-enterprise/v2/features/agenttracing/content"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// X-Tyk-Trace-Capture: off keeps a request's content out of the capture the
// edge sends, while the install default captures it (jyx.113).
func TestAgentTraceCapture_EdgeCaptureHeaderOff(t *testing.T) {
	baseURL, hub, _ := setupCaptureGateway(t)
	p := agenttrace.NewContentProcessor(agenttrace.ContentGlobals{
		DefaultMode: agenttrace.ContentFull, MaxRequestOverride: agenttrace.ContentFull, StoreCap: agenttrace.ContentFull,
	}, func(uint, agenttrace.Resource) agenttrace.ContentOverrides { return agenttrace.ContentOverrides{} })
	require.NotNil(t, p, "the Enterprise content pipeline is registered")
	agenttrace.SetContentProcessor(p)
	t.Cleanup(func() { agenttrace.SetContentProcessor(nil) })

	chatBody := `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`
	require.Equal(t, http.StatusOK, postWithHeaders(t, baseURL+"/llm/rest/mock-openai/v1/chat/completions", chatBody,
		map[string]string{"X-Tyk-Session-Id": "full"}))
	require.Equal(t, http.StatusOK, postWithHeaders(t, baseURL+"/llm/rest/mock-openai/v1/chat/completions", chatBody,
		map[string]string{"X-Tyk-Session-Id": "off", "X-Tyk-Trace-Capture": "off"}))

	var evs []*agenttrace.Event
	require.Eventually(t, func() bool { evs = hub.llmEvents(); return len(evs) == 2 }, 10*time.Second, 25*time.Millisecond)
	for _, e := range evs {
		switch e.Declared.SessionID {
		case "full":
			assert.Equal(t, agenttrace.ContentFull, e.ContentMode)
			assert.NotEmpty(t, e.Parts, "the default captures content")
		case "off":
			assert.Equal(t, agenttrace.ContentOff, e.ContentMode)
			assert.Empty(t, e.Parts, "X-Tyk-Trace-Capture: off captures no content")
		default:
			t.Fatalf("unexpected session %q", e.Declared.SessionID)
		}
	}
}
