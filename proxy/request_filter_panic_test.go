//go:build enterprise

package proxy

import (
	"bytes"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Issue #535: the request-filter chain runs on the caller's goroutine, so a
// script that panicked inside the Tengo VM unwound straight through the proxy
// handler and killed the process.
//
// Request filters fail closed, so the recovered panic must surface as a
// rejected request rather than a silently forwarded one.
func TestScreenProxyRequestByVendor_PanickingScriptRejectsTheRequest(t *testing.T) {
	p := &Proxy{}

	extractors := NewMessageExtractorRegistry()
	extractors.Register(&OpenAIMessageExtractor{})
	p.messageExtractorRegistry = extractors

	reconstructors := NewMessageReconstructorRegistry()
	reconstructors.Register(&OpenAIMessageReconstructor{})
	p.messageReconstructorRegistry = reconstructors

	llm := &models.LLM{
		Vendor: models.OPENAI,
		Filters: []*models.Filter{
			{
				Name:   "panicking-filter",
				Script: []byte(`output := 5 / 0`),
			},
		},
	}

	body := `{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`
	r := httptest.NewRequest("POST", "/openai/v1/chat/completions", strings.NewReader(body))

	// If the panic escaped RunScript, this call would kill the test binary.
	err := p.screenProxyRequestByVendor(llm, r, false)

	if err == nil {
		t.Fatal("screenProxyRequestByVendor() returned no error, want the request rejected (request filters fail closed)")
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Errorf("screenProxyRequestByVendor() error = %q, want it to name the panic", err)
	}
	if !strings.Contains(err.Error(), "panicking-filter") {
		t.Errorf("screenProxyRequestByVendor() error = %q, want it to name the offending filter", err)
	}
}

// A healthy filter behind the same code path must still be able to rewrite the
// request body, proving the recover did not change normal behaviour.
func TestScreenProxyRequestByVendor_HealthyFilterStillModifiesRequest(t *testing.T) {
	p := &Proxy{}

	extractors := NewMessageExtractorRegistry()
	extractors.Register(&OpenAIMessageExtractor{})
	p.messageExtractorRegistry = extractors

	reconstructors := NewMessageReconstructorRegistry()
	reconstructors.Register(&OpenAIMessageReconstructor{})
	p.messageReconstructorRegistry = reconstructors

	llm := &models.LLM{
		Vendor: models.OPENAI,
		Filters: []*models.Filter{
			{
				Name:   "blocking-filter",
				Script: []byte(`output := {block: true, payload: input.raw_input, message: "nope"}`),
			},
		},
	}

	body := `{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`
	r := httptest.NewRequest("POST", "/openai/v1/chat/completions", strings.NewReader(body))

	err := p.screenProxyRequestByVendor(llm, r, false)
	if err == nil {
		t.Fatal("screenProxyRequestByVendor() returned no error for a blocking filter")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("screenProxyRequestByVendor() error = %q, want the filter's block message", err)
	}
}

// The request body must survive a panicking filter intact — a recovered panic
// should not leave a half-consumed body behind for anything reading it after.
func TestScreenProxyRequestByVendor_PanicLeavesBodyReadable(t *testing.T) {
	p := &Proxy{}

	extractors := NewMessageExtractorRegistry()
	extractors.Register(&OpenAIMessageExtractor{})
	p.messageExtractorRegistry = extractors

	llm := &models.LLM{
		Vendor: models.OPENAI,
		Filters: []*models.Filter{
			{Name: "panicking-filter", Script: []byte(`output := 5 / 0`)},
		},
	}

	body := `{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`
	r := httptest.NewRequest("POST", "/openai/v1/chat/completions", strings.NewReader(body))

	if err := p.screenProxyRequestByVendor(llm, r, false); err == nil {
		t.Fatal("expected an error from the panicking filter")
	}

	got, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("reading request body after recovered panic: %v", err)
	}
	if !bytes.Equal(got, []byte(body)) {
		t.Errorf("request body after recovered panic = %q, want %q", got, body)
	}
}
