//go:build agenttracebench

package proxy

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// BenchmarkAgentTraceCapturePerRequest is the tracing work one 32 KiB
// non-streaming LLM request costs with tracing on, content off: the recorder
// and declared headers on the request path, and the capture analysis
// (extraction, fingerprints, raw capture) on the analytics goroutine.
func BenchmarkAgentTraceCapturePerRequest(b *testing.B) {
	agenttrace.SetEnabled(true)
	agenttrace.SetSink(discardSink{})
	agenttrace.SetFingerprinter(agenttrace.StaticFingerprinter{ID: 1, Key: []byte("k")})
	b.Cleanup(func() {
		agenttrace.SetEnabled(false)
		agenttrace.SetSink(nil)
		agenttrace.SetFingerprinter(nil)
		agenttrace.SetContentProcessor(nil)
	})

	respBody := []byte(toolCallReply)
	llm := &models.LLM{Name: "Primary", Vendor: models.OPENAI}
	app := &models.App{ID: 1}
	for _, c := range []struct {
		size int
		off  bool // an off-mode content pipeline (the default) is installed
	}{{1 << 10, false}, {32 << 10, false}, {32 << 10, true}} {
		pad := strings.Repeat("x", c.size)
		reqBody := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"` + pad + `"}]}`)
		name := strconv.Itoa(c.size>>10) + "KiB"
		if c.off {
			name += "/content-off-pipeline"
			agenttrace.SetContentProcessor(offModeProcessor{})
		} else {
			agenttrace.SetContentProcessor(nil)
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				r := httptest.NewRequest(http.MethodPost, "/llm/rest/primary/v1/chat/completions", nil)
				r.Header.Set("Traceparent", clientTraceparent)
				r, rec := beginRequestCapture(r)
				captureLLM(r, llm, app, http.StatusOK, nil, reqBody, respBody, false)
				rec.Finish()
			}
		})
	}
}

// offModeProcessor resolves every span to content off, as the default
// configuration does.
type offModeProcessor struct{}

func (offModeProcessor) Process(ev *agenttrace.Event) { ev.ContentMode = agenttrace.ContentOff; ev.Raw = nil }
func (offModeProcessor) Mode(*agenttrace.Event) string { return agenttrace.ContentOff }
