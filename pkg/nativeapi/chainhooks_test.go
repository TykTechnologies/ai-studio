package nativeapi

import (
	"net/http"
	"strings"
	"testing"
)

func chainSpec() Spec {
	s := batchSpec()
	s.Resources = append([]ResourceType{}, s.Resources...)
	s.ChainHooks = &ChainHooks{Hooks: []ChainHook{
		{
			Name: "create", Method: http.MethodPost, Path: "/v1/thing",
			RequestRefs: []BodyRef{{Path: "previous_id", Resource: testBatch.Name}},
			ResponseIDs: []BodyRef{{Path: "id", Resource: testBatch.Name}},
			EventObject: "response",
		},
		{Name: "count", Method: http.MethodPost, Path: "/v1/thing/count"},
	}}
	return s
}

func TestChainHooksValidate(t *testing.T) {
	if _, err := NewMatcher(chainSpec()); err != nil {
		t.Fatalf("valid chain spec refused: %v", err)
	}
	bad := []struct {
		name string
		mut  func(*Spec)
		want string
	}{
		{"no name", func(s *Spec) { s.ChainHooks.Hooks[0].Name = "" }, "has no name"},
		{"bad method", func(s *Spec) { s.ChainHooks.Hooks[0].Method = "post" }, "invalid method"},
		{"bad path", func(s *Spec) { s.ChainHooks.Hooks[0].Path = "v1/thing" }, "must start"},
		{"param path", func(s *Spec) { s.ChainHooks.Hooks[0].Path = "/v1/thing/{id}" }, "must not have parameters"},
		{"duplicate", func(s *Spec) { s.ChainHooks.Hooks[1].Path = "/v1/thing" }, "duplicate chain hook"},
		{"shadows an operation", func(s *Spec) { s.ChainHooks.Hooks[1].Path = "/v1/messages/batches" }, "serves the same requests"},
		{"bad request ref", func(s *Spec) { s.ChainHooks.Hooks[0].RequestRefs[0].Resource = "nope" }, "body ref"},
		{"bad response ref", func(s *Spec) { s.ChainHooks.Hooks[0].ResponseIDs[0].Path = "" }, "body ref"},
		{"event object without ids", func(s *Spec) { s.ChainHooks.Hooks[1].EventObject = "response" }, "EventObject"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			s := chainSpec()
			tc.mut(&s)
			if _, err := NewMatcher(s); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestChainHooksHook(t *testing.T) {
	s := chainSpec()
	if h, ok := s.ChainHooks.Hook(http.MethodPost, "/v1/thing"); !ok || h.Name != "create" {
		t.Fatalf("Hook = %v %v", h, ok)
	}
	if h, ok := s.ChainHooks.Hook(http.MethodPost, "/v1/thing"); !ok || h != &s.ChainHooks.Hooks[0] {
		t.Fatal("Hook must point into the spec")
	}
	for _, c := range [][2]string{{http.MethodGet, "/v1/thing"}, {http.MethodPost, "/v1/thing/"}, {http.MethodPost, "/v1/other"}} {
		if _, ok := s.ChainHooks.Hook(c[0], c[1]); ok {
			t.Errorf("Hook(%s %s) matched", c[0], c[1])
		}
	}
	var none *ChainHooks
	if _, ok := none.Hook(http.MethodPost, "/v1/thing"); ok {
		t.Fatal("nil ChainHooks matched")
	}
}

func TestAsyncSpecValidateUsageSource(t *testing.T) {
	poll := func(string) UpstreamReq { return UpstreamReq{} }
	lineUsage := func([]byte) (Usage, bool, error) { return Usage{}, false, nil }
	results := func(string, []byte) (UpstreamReq, bool) { return UpstreamReq{}, false }
	objUsage := func([]byte) (Usage, bool, error) { return Usage{}, false, nil }
	cases := []struct {
		name  string
		async *AsyncSpec
		ok    bool
	}{
		{"results and line usage", &AsyncSpec{Poll: poll, Results: results, LineUsage: lineUsage}, true},
		{"object usage", &AsyncSpec{Poll: poll, ObjectUsage: objUsage}, true},
		{"neither", &AsyncSpec{Poll: poll}, true},
		{"results without line usage", &AsyncSpec{Poll: poll, Results: results}, false},
		{"line usage without results", &AsyncSpec{Poll: poll, LineUsage: lineUsage}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := batchSpec()
			rt := testBatch
			rt.Async = tc.async
			s.Resources = []ResourceType{rt}
			_, err := NewMatcher(s)
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

func TestAsyncSpecIsTracked(t *testing.T) {
	var none *AsyncSpec
	if none.IsTracked([]byte(`{}`)) {
		t.Fatal("a resource without Async is never tracked")
	}
	if !(&AsyncSpec{}).IsTracked([]byte(`{}`)) {
		t.Fatal("without Tracked every object is tracked")
	}
	only := &AsyncSpec{Tracked: func(obj []byte) bool { return strings.Contains(string(obj), "bg") }}
	if !only.IsTracked([]byte(`{"bg":true}`)) || only.IsTracked([]byte(`{}`)) {
		t.Fatal("Tracked not consulted")
	}
}

func TestSpecValidateChildRefs(t *testing.T) {
	s := batchSpec()
	s.Operations[2].ResponseIDs = []BodyRef{{Path: "output_id", Resource: testBatch.Name, Child: true}}
	if _, err := NewMatcher(s); err != nil {
		t.Fatalf("a child response ID refused: %v", err)
	}
	s.Operations[0].RequestRefs = []BodyRef{{Path: "input_id", Resource: testBatch.Name, Child: true}}
	if _, err := NewMatcher(s); err == nil || !strings.Contains(err.Error(), "cannot be a child") {
		t.Fatalf("err = %v", err)
	}
}

func TestMatcherMatchesFold(t *testing.T) {
	m := mustMatcher(t, chainSpec())
	for _, p := range []string{
		"/v1/Messages/Batches", "/V1/messages/batches/x/Results", "/v1/messages/batches/", "//v1/messages/batches",
		"/v1/Thing", "/v1/thing/", "/V1/THING/COUNT",
	} {
		if !m.MatchesFold(p) {
			t.Errorf("MatchesFold(%q) = false", p)
		}
	}
	for _, p := range []string{"/v1/messages", "/v1/messages/batches/x/y/z", "/v1/other"} {
		if m.MatchesFold(p) {
			t.Errorf("MatchesFold(%q) = true", p)
		}
	}
	h := chainSpec().ChainHooks
	if !h.Declares("/v1/thing") || h.Declares("/v1/Thing") || (*ChainHooks)(nil).Declares("/v1/thing") {
		t.Fatal("Declares")
	}
}
