package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Only the gateway's own loopback hop is recognised: it needs this process's
// token and a loopback connection. A client sending the header, or a reverse
// proxy on the same host forwarding it, must not pass.
func TestIsInternalHop(t *testing.T) {
	req := func(remote, hop string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/llm/call/x", nil)
		r.RemoteAddr = remote
		if hop != "" {
			r.Header.Set(hdrInternalHop, hop)
		}
		return r
	}
	cases := []struct {
		name   string
		r      *http.Request
		expect bool
	}{
		{"own hop over IPv4 loopback", req("127.0.0.1:40000", internalHopToken), true},
		{"own hop over IPv6 loopback", req("[::1]:40000", internalHopToken), true},
		{"no header", req("127.0.0.1:40000", ""), false},
		{"old presence value via a local reverse proxy", req("127.0.0.1:40000", "1"), false},
		{"guessed token via a local reverse proxy", req("127.0.0.1:40000", newInternalHopToken()), false},
		{"right token from outside", req("203.0.113.5:40000", internalHopToken), false},
	}
	for _, c := range cases {
		if got := IsInternalHop(c.r); got != c.expect {
			t.Errorf("%s: IsInternalHop = %v, want %v", c.name, got, c.expect)
		}
	}
}

// The loopback hop sends the token, so the receiving side recognises it.
func TestInternalRoutingTransportSendsHopToken(t *testing.T) {
	var got string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(hdrInternalHop)
		if !IsInternalHop(r) {
			t.Errorf("hop not recognised: header %q from %s", got, r.RemoteAddr)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	tr := newInternalRoutingTransport(http.DefaultTransport, "Bearer x")
	resp, err := tr.RoundTrip(httptest.NewRequest(http.MethodPost, upstream.URL+"/llm/call/x", nil).WithContext(t.Context()))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got != internalHopToken {
		t.Fatalf("hop header = %q, want the process token", got)
	}
}
