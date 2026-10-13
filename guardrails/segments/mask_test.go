package segments

import (
	"strings"
	"testing"
)

// Errors about a body name its schema, never its data: inside a tool result
// the keys are the tool's output.
func TestErrorsMaskDataKeys(t *testing.T) {
	cases := []struct {
		name, format, body string
		want               string
		secret             string
	}{
		{
			name:   "repeated key inside a functionResponse",
			format: FormatGoogleAI,
			body:   `{"contents":[{"role":"user","parts":[{"functionResponse":{"name":"f","response":{"AKIASECRETKEY":1,"AKIASECRETKEY":2}}}]}]}`,
			want:   "repeated key at /contents/0/parts/0/functionResponse/response",
			secret: "AKIASECRETKEY",
		},
		{
			name:   "repeated key deeper in tool data",
			format: FormatGoogleAI,
			body:   `{"contents":[{"role":"user","parts":[{"functionResponse":{"name":"f","response":{"alice@example.com":{"x":1,"x":2}}}}]}]}`,
			want:   "repeated key at /contents/0/parts/0/functionResponse/response/…",
			secret: "alice@example.com",
		},
		{
			name:   "repeated top-level parameter is named",
			format: FormatOpenAI,
			body:   `{"model":"m","max_tokens":1,"max_tokens":2,"messages":[]}`,
			want:   `repeated key "max_tokens" at /`,
		},
		{
			name:   "case variant of a schema field is named",
			format: FormatOpenAI,
			body:   `{"messages":[{"role":"user","Content":"x"}]}`,
			want:   `a case variant of "content" at /messages/0`,
		},
		{
			name:   "case-variant data keys in an unknown block",
			format: FormatAnthropic,
			body:   `{"messages":[{"role":"user","content":[{"type":"future","Secret@Mail":1,"secret@mail":2}]}]}`,
			want:   "case-variant keys at /messages/0/content/0",
			secret: "ecret@",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Extract(c.format, []byte(c.body))
			if err == nil {
				t.Fatal("no error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q, want it to contain %q", err, c.want)
			}
			if c.secret != "" && strings.Contains(err.Error(), c.secret) {
				t.Errorf("error %q leaks a data key", err)
			}
		})
	}

	// The format-blind check and the generic fallback mask the same way.
	body := []byte(`{"input":[{"call_output":{"bob@example.com":1,"bob@example.com":2}}]}`)
	for name, err := range map[string]error{"CheckKeys": CheckKeys(body), "Generic": func() error { _, err := Generic(body); return err }()} {
		if err == nil || strings.Contains(err.Error(), "bob@") || !strings.Contains(err.Error(), "at /input/0/…") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestMaskPath(t *testing.T) {
	for in, want := range map[string]string{
		"":                              "/",
		"/messages/3/content":           "/messages/3/content",
		"/messages/3/a~1b/c":            "/messages/3/…",
		"/contents/0/parts/x~0y":        "/contents/0/parts/…",
		"/" + strings.Repeat("0/", 200): "/" + strings.Repeat("0/", 99) + "0…",
	} {
		if got := maskPath(in); got != want {
			t.Errorf("maskPath(%q) = %q, want %q", in, got, want)
		}
	}
}
