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
				})
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

func TestBlockedPluginResponseDefaultsTo403(t *testing.T) {
	rec := httptest.NewRecorder()
	writeBlockedPluginResponse(rec, &interfaces.PluginResponse{Block: true, Body: []byte("no")})
	if rec.Code != http.StatusForbidden || rec.Body.String() != "no" {
		t.Fatalf("got %d %q, want 403 \"no\"", rec.Code, rec.Body.String())
	}
}
