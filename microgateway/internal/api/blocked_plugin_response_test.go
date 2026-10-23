package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
)

// A plugin that answers a request itself (a cache hit, a rate-limit refusal)
// may hand back headers copied from another response. Framing headers among
// them must not decide how the body is sent: a stale Content-Length made
// net/http refuse to write a longer body, so the client got an empty
// response (the community llm-cache replaying a stored JSON Content-Length
// with a streamed body).
func TestBlockedPluginResponseFramingComesFromTheBody(t *testing.T) {
	body := "event: message_start\ndata: {}\n\nevent: message_stop\ndata: {}\n\n"
	cases := map[string]map[string]string{
		"shorter Content-Length": {"Content-Length": "3", "Content-Type": "text/event-stream"},
		"longer Content-Length":  {"Content-Length": "4096", "Content-Type": "text/event-stream"},
		"Transfer-Encoding":      {"Transfer-Encoding": "gzip", "Content-Type": "text/event-stream"},
	}
	for name, headers := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeBlockedPluginResponse(w, &interfaces.PluginResponse{
					Block: true, StatusCode: http.StatusOK, Headers: headers, Body: []byte(body),
				}, false)
			}))
			defer srv.Close()

			resp, err := http.Get(srv.URL)
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer resp.Body.Close()
			got, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("reading body: %v (Content-Length %q)", err, resp.Header.Get("Content-Length"))
			}
			if string(got) != body {
				t.Fatalf("body = %q, want %q", got, body)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
				t.Errorf("Content-Type = %q, want the plugin's", ct)
			}
		})
	}
}

// A plugin's refusal of a native Bedrock runtime call goes out in the AWS
// JSON protocol's shape, which an AWS SDK reads (8xf.30): X-Amzn-ErrorType
// by status and {"message"} with the plugin's message. A plugin that shaped
// it itself, a 2xx answer (a cache hit) and other calls are left alone.
func TestBlockedPluginResponseAWSShapeOnBedrock(t *testing.T) {
	firewall := &interfaces.PluginResponse{Block: true, StatusCode: http.StatusForbidden,
		Headers: map[string]string{"Content-Type": "application/json", "X-Firewall": "1"},
		Body:    []byte(`{"error":{"message":"prompt injection detected","type":"content_policy_violation"}}`)}

	rec := httptest.NewRecorder()
	writeBlockedPluginResponse(rec, firewall, true)
	if rec.Code != http.StatusForbidden || rec.Header().Get("X-Amzn-ErrorType") != "AccessDeniedException" ||
		rec.Body.String() != `{"message":"prompt injection detected"}` || rec.Header().Get("X-Firewall") != "1" {
		t.Fatalf("got %d %v %q", rec.Code, rec.Header(), rec.Body.String())
	}

	rec = httptest.NewRecorder()
	writeBlockedPluginResponse(rec, &interfaces.PluginResponse{Block: true, StatusCode: http.StatusTooManyRequests,
		Body: []byte(`{"error":"rate limit exceeded"}`)}, true)
	if rec.Header().Get("X-Amzn-ErrorType") != "ThrottlingException" || rec.Body.String() != `{"message":"rate limit exceeded"}` {
		t.Fatalf("got %v %q", rec.Header(), rec.Body.String())
	}

	shaped := &interfaces.PluginResponse{Block: true, StatusCode: http.StatusBadRequest,
		Headers: map[string]string{"X-Amzn-ErrorType": "ValidationException"}, Body: []byte(`{"message":"mine"}`)}
	for name, tc := range map[string]struct {
		resp *interfaces.PluginResponse
		aws  bool
	}{
		"shaped by the plugin": {shaped, true},
		"a cache hit":          {&interfaces.PluginResponse{Block: true, StatusCode: http.StatusOK, Body: []byte(`{"output":{}}`)}, true},
		"not a runtime call":   {firewall, false},
	} {
		rec := httptest.NewRecorder()
		writeBlockedPluginResponse(rec, tc.resp, tc.aws)
		if rec.Body.String() != string(tc.resp.Body) {
			t.Errorf("%s: body %q, want the plugin's %q", name, rec.Body.String(), tc.resp.Body)
		}
	}
}

func TestIsBedrockRuntimeCall(t *testing.T) {
	for path, want := range map[string]bool{
		"/llm/rest/br/model/anthropic.claude-3/converse":                                                 true,
		"/llm/call/br/model/arn%3Aaws%3Abedrock%3Aus-east-1%3A1%3Ainference-profile%2Fx/converse-stream": true,
		"/llm/rest/br/v1/chat/completions":                                                               false,
	} {
		r := httptest.NewRequest(http.MethodPost, path, nil)
		if got := isBedrockRuntimeCall("bedrock", r); got != want {
			t.Errorf("%s: %v, want %v", path, got, want)
		}
	}
	if isBedrockRuntimeCall("openai", httptest.NewRequest(http.MethodPost, "/llm/rest/x/model/m/converse", nil)) {
		t.Error("another vendor's call is not Bedrock's")
	}
}

func TestBlockedPluginResponseDefaultsTo403(t *testing.T) {
	rec := httptest.NewRecorder()
	writeBlockedPluginResponse(rec, &interfaces.PluginResponse{Block: true, Body: []byte("no")}, false)
	if rec.Code != http.StatusForbidden || rec.Body.String() != "no" {
		t.Fatalf("got %d %q, want 403 \"no\"", rec.Code, rec.Body.String())
	}
}
