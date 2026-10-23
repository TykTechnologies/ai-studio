package netguard

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

// stubProxy sends every plugin transport request through proxyURL, except
// those whose host is in noProxy (as NO_PROXY would).
func stubProxy(t *testing.T, proxyURL string, noProxy ...string) {
	t.Helper()
	u, err := url.Parse(proxyURL)
	if err != nil {
		t.Fatal(err)
	}
	orig := proxyFromEnvironment
	proxyFromEnvironment = func(req *http.Request) (*url.URL, error) {
		for _, h := range noProxy {
			if req.URL.Hostname() == h {
				return nil, nil
			}
		}
		return u, nil
	}
	t.Cleanup(func() { proxyFromEnvironment = orig })
}

// Behind a proxy on an internal address, a public registry is reached
// through it, an internal registry is refused by name before anything is
// sent to the proxy, and a target that bypasses the proxy (NO_PROXY) is
// dial-guarded (midsommar-clean-c1g.33).
func TestPluginHTTPTransport_InternalProxy(t *testing.T) {
	t.Setenv(EnvAllowInternal, "")
	stubPluginLookup(t, map[string]string{
		"registry.example.com":   "203.0.113.10",
		"registry.corp.internal": "10.0.0.5",
	})
	var proxied atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied.Add(1)
		w.WriteHeader(http.StatusOK) // answers as the target would
	}))
	t.Cleanup(proxy.Close)
	direct := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(direct.Close)
	directURL, _ := url.Parse(direct.URL)
	stubProxy(t, proxy.URL, directURL.Hostname())
	client := &http.Client{Transport: PluginHTTPTransport()}

	resp, err := client.Get("http://registry.example.com/v2/")
	if err != nil {
		t.Fatalf("public registry through an internal proxy: %v", err)
	}
	resp.Body.Close()
	if proxied.Load() != 1 {
		t.Fatalf("proxy saw %d requests, want 1", proxied.Load())
	}

	for _, target := range []string{"http://registry.corp.internal/v2/", "http://10.0.0.6/v2/", "http://unresolvable.example/v2/"} {
		_, err := client.Get(target)
		if !errors.Is(err, ErrInternalPluginHost) {
			t.Errorf("%s through the proxy = %v, want ErrInternalPluginHost", target, err)
		}
	}
	if proxied.Load() != 1 {
		t.Fatalf("refused targets reached the proxy (%d requests)", proxied.Load())
	}

	// An exempt internal registry goes through the proxy.
	t.Setenv(EnvAllowedInternalHosts, "registry.corp.internal")
	resp, err = client.Get("http://registry.corp.internal/v2/")
	if err != nil {
		t.Fatalf("exempt registry through the proxy: %v", err)
	}
	resp.Body.Close()
	t.Setenv(EnvAllowedInternalHosts, "")

	// NO_PROXY: dialled directly, so the dial guard applies.
	if _, err := client.Get(direct.URL); !errors.Is(err, ErrInternalPluginHost) {
		t.Fatalf("direct dial to %s = %v, want ErrInternalPluginHost", direct.URL, err)
	}
}
