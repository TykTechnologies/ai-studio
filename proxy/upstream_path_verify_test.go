package proxy

import (
	"net/http"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// Added during independent verification (ut0.11).

func TestCanonicalVendorPath_VerifyShapes(t *testing.T) {
	for _, tc := range []struct{ upstream, remaining, want string }{
		{"/gw/v1", "/messages/batches/msgbatch_x/results", "/v1/messages/batches/msgbatch_x/results"},
		{"/gw/v1", "/v1/messages/batches/msgbatch_x/cancel", "/v1/messages/batches/msgbatch_x/cancel"},
		{"/gw/version", "/v1/messages/batches", "/v1/messages/batches"}, // "version" is not a version segment
		{"/gw/v1beta2", "/v1beta2/models", "/v1beta2/models"},
		{"/v1", "/messages/batches", "/v1/messages/batches"},
		{"/a/b/c/v1/", "/v1/messages/count_tokens", "/v1/messages/count_tokens"},
	} {
		if got := canonicalVendorPath(tc.upstream, tc.remaining); got != tc.want {
			t.Errorf("canonicalVendorPath(%q, %q) = %q, want %q", tc.upstream, tc.remaining, got, tc.want)
		}
	}
}

// canonicalVendorPath resolves ".." (via joinUpstreamPath), so a remaining
// path can climb out of an endpoint's corporate prefix and still produce a
// native-looking canonical path. The native dispatcher must therefore reject
// traversal before matching: every such escape is caught by
// hasTraversalSegment.
func TestCanonicalVendorPath_EscapesAreTraversal(t *testing.T) {
	for _, tc := range []struct{ upstream, remaining string }{
		{"/anthropic/v1", "/../../v1/messages/batches"},
		{"/anthropic/v1", "/messages/../../../v1/messages/batches"},
		{"/acct/gw/anthropic", "/../../../v1/messages/batches"},
	} {
		joined := joinUpstreamPath(tc.upstream, tc.remaining)
		base := "/" + strings.Trim(tc.upstream, "/")
		escaped := !strings.HasPrefix(joined, base+"/") && joined != base
		if !escaped {
			continue
		}
		if !hasTraversalSegment(tc.remaining) {
			t.Errorf("%q + %q escapes to %q without a traversal segment", tc.upstream, tc.remaining, joined)
		}
		t.Logf("%q + %q -> joined %q, canonical %q (must be refused by the traversal check)",
			tc.upstream, tc.remaining, joined, canonicalVendorPath(tc.upstream, tc.remaining))
	}
}

// ut0.20 fix: a count_tokens call through the legacy native chain is logged
// (honouring DontLogBodies) without a chat record, and an unrelated unknown
// path still logs nothing, as on main.
func TestAnalyzeResponse_CountTokensLogVerify(t *testing.T) {
	for _, tc := range []struct {
		path        string
		dontLog     bool
		logs, recs  int
		wantBodyLog bool
	}{
		{"/llm/rest/claude/v1/messages/count_tokens", false, 1, 0, true},
		{"/llm/call/claude/v1/messages/count_tokens/", true, 1, 0, false},
		{"/llm/rest/claude/v1/models", false, 0, 0, false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			h := &chatRecordCounter{}
			analytics.SetHandler(h)
			defer analytics.ResetHandler()
			svc := new(MockService) // no expectations: any pricing/budget call panics
			req, _ := http.NewRequest(http.MethodPost, "http://gw"+tc.path, nil)
			AnalyzeResponse(svc, &models.LLM{ID: 1, Vendor: models.ANTHROPIC, DontLogBodies: tc.dontLog}, &models.App{ID: 2},
				http.StatusOK, []byte(`{"input_tokens":42}`), []byte(`{"model":"m","messages":[]}`), req)
			if len(h.logs) != tc.logs || h.count() != tc.recs {
				t.Fatalf("logs=%d records=%d, want %d/%d", len(h.logs), h.count(), tc.logs, tc.recs)
			}
			if tc.logs == 1 {
				l := h.logs[0]
				if (l.ResponseBody != "") != tc.wantBodyLog || (l.RequestBody != "") != tc.wantBodyLog {
					t.Errorf("bodies logged = %v/%v, want %v", l.RequestBody != "", l.ResponseBody != "", tc.wantBodyLog)
				}
				if l.AppID != 2 || l.LLMID != 1 || l.ResponseCode != http.StatusOK {
					t.Errorf("unexpected log %+v", l)
				}
			}
		})
	}
}
