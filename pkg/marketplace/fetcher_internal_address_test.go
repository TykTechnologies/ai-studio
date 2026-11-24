package marketplace

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testIndexYAML = `apiVersion: v1
generated: "2026-01-01T00:00:00Z"
plugins:
  demo:
    - id: demo
      version: 1.0.0
      oci_registry: registry.example.com
      oci_repository: plugins/demo
`

// internalIndexServer is a marketplace index on 127.0.0.1 that counts the
// requests it gets: an internal service, as the fetcher must see it.
func internalIndexServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/html":
			_, _ = w.Write([]byte("<!DOCTYPE html><html><title>internal-admin-console</title></html>"))
		case "/missing":
			http.NotFound(w, r)
		case "/unauthorized":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			_, _ = w.Write([]byte(testIndexYAML))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// fetchAll runs every fetch the marketplace makes against u and returns
// their errors.
func fetchAll(f *Fetcher, u string) []error {
	ctx := context.Background()
	_, _, errIndex := f.FetchIndex(ctx, u)
	_, _, _, errCond := f.FetchIndexConditional(ctx, u, "", time.Time{}, false)
	_, _, _, errForce := f.FetchIndexConditional(ctx, u, `"etag"`, time.Now(), true)
	_, errManifest := f.FetchManifest(ctx, u)
	return []error{errIndex, errCond, errForce, errManifest}
}

// The marketplace index URL is under the internal-address block plugin
// registries are under. The fetcher used a plain http.Client, so anyone who
// could make Studio fetch an index (validate, add, sync) could reach
// loopback, private and link-local addresses, and the errors told apart
// what answered there.
func TestFetcherRefusesInternalAddresses(t *testing.T) {
	srv, hits := internalIndexServer(t)
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closedAddr := closed.Addr().String()
	require.NoError(t, closed.Close())

	f := NewFetcher(3 * time.Second)
	portStr := srv.URL[strings.LastIndex(srv.URL, ":"):]
	for name, u := range map[string]string{
		"loopback, something listening":         srv.URL + "/index.yaml",
		"loopback, a page that is not an index": srv.URL + "/html",
		"loopback, 404":                         srv.URL + "/missing",
		"loopback, 401":                         srv.URL + "/unauthorized",
		"loopback, nothing listening":           "http://" + closedAddr + "/index.yaml",
		"a name that resolves to loopback":      "http://localhost" + portStr + "/index.yaml",
		"a name with a trailing dot":            "http://localhost." + portStr + "/index.yaml",
		"IPv6 loopback":                         "http://[::1]" + portStr + "/index.yaml",
		"IPv4-mapped IPv6 loopback":             "http://[::ffff:127.0.0.1]" + portStr + "/index.yaml",
		"userinfo before an internal host":      "http://public.example.com@127.0.0.1" + portStr + "/index.yaml",
		"decimal spelling of 127.0.0.1":         "http://2130706433" + portStr + "/index.yaml",
		"hex spelling of 127.0.0.1":             "http://0x7f000001" + portStr + "/index.yaml",
		"link-local metadata address":           "http://169.254.169.254/latest/meta-data/",
		"private range 10/8":                    "http://10.255.255.1/index.yaml",
		"private range 192.168/16":              "http://192.168.0.1/index.yaml",
		"unique local IPv6":                     "http://[fd00::1]/index.yaml",
		"file scheme":                           "file:///etc/passwd",
		"gopher scheme":                         "gopher://127.0.0.1" + portStr + "/",
	} {
		t.Run(name, func(t *testing.T) {
			for i, err := range fetchAll(f, u) {
				require.Error(t, err, "fetch %d", i)
				// The same error value for every cause: nothing to tell a
				// listening port from a closed one, or a 401 from a 404.
				assert.Equal(t, ErrFetchFailed, err, "fetch %d", i)
			}
			_, err := f.FetchChangelog(context.Background(), u)
			assert.Error(t, err)
		})
	}
	assert.Zero(t, hits.Load(), "no request may reach the internal server")
	assert.NotContains(t, ErrFetchFailed.Error(), "127.0.0.1")
}

// A URL that is allowed may not lead to one that is not: every redirect hop
// is dialled through the same check.
func TestFetcherRefusesARedirectToAnInternalAddress(t *testing.T) {
	target, targetHits := internalIndexServer(t)
	var entryHits atomic.Int32
	entry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entryHits.Add(1)
		http.Redirect(w, r, target.URL+"/index.yaml", http.StatusFound)
	}))
	t.Cleanup(entry.Close)

	// The entry is reached by a name the operator allowed; the redirect
	// target, 127.0.0.1, is not allowed.
	t.Setenv("LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS", "localhost")
	entryURL := "http://localhost" + entry.URL[strings.LastIndex(entry.URL, ":"):] + "/index.yaml"

	f := NewFetcher(3 * time.Second)
	for i, err := range fetchAll(f, entryURL) {
		assert.Equal(t, ErrFetchFailed, err, "fetch %d", i)
	}
	assert.Positive(t, entryHits.Load(), "the allowed host is fetched")
	assert.Zero(t, targetHits.Load(), "the redirect to an internal address is not followed")
}

// The documented settings still let an internal index through: the named
// host, and nothing wider.
func TestFetcherAllowsAnInternalIndexTheOperatorAllowed(t *testing.T) {
	srv, hits := internalIndexServer(t)
	portStr := srv.URL[strings.LastIndex(srv.URL, ":"):]
	f := NewFetcher(3 * time.Second)

	t.Run("LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS names the host", func(t *testing.T) {
		t.Setenv("LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS", "localhost")
		before := hits.Load()
		index, _, err := f.FetchIndex(context.Background(), "http://localhost"+portStr+"/index.yaml")
		require.NoError(t, err)
		assert.Contains(t, index.Plugins, "demo")
		require.NoError(t, CheckIndexURL("http://localhost"+portStr+"/index.yaml"))

		// The same server by an address that was not named is still refused.
		_, _, err = f.FetchIndex(context.Background(), srv.URL+"/index.yaml")
		assert.Equal(t, ErrFetchFailed, err)
		assert.Equal(t, ErrFetchFailed, CheckIndexURL(srv.URL+"/index.yaml"))
		assert.Equal(t, before+1, hits.Load())
	})

	t.Run("ALLOW_INTERNAL_NETWORK_ACCESS", func(t *testing.T) {
		t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "true")
		index, _, modified, err := f.FetchIndexConditional(context.Background(), srv.URL+"/index.yaml", "", time.Time{}, false)
		require.NoError(t, err)
		assert.True(t, modified)
		assert.Contains(t, index.Plugins, "demo")
		require.NoError(t, CheckIndexURL(srv.URL+"/index.yaml"))
	})
}

// What an allowed host answers is not echoed either: a page that is not an
// index gives one fixed error, without the page's first bytes.
func TestFetcherDoesNotEchoTheAnswer(t *testing.T) {
	srv, _ := internalIndexServer(t)
	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "true")
	f := NewFetcher(3 * time.Second)

	for i, err := range fetchAll(f, srv.URL+"/html") {
		assert.Equal(t, ErrInvalidDocument, err, "fetch %d", i)
	}
	assert.NotContains(t, ErrInvalidDocument.Error(), "DOCTYPE")
	for i, err := range fetchAll(f, srv.URL+"/missing") {
		assert.Equal(t, ErrFetchFailed, err, "fetch %d", i)
	}
	for i, err := range fetchAll(f, srv.URL+"/unauthorized") {
		assert.Equal(t, ErrFetchFailed, err, "fetch %d", i)
	}
}

// An answer larger than the cap is refused rather than read into memory.
func TestFetcherCapsTheAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("id: demo\nname: \"" + strings.Repeat("a", maxManifestBytes) + "\"\n"))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "true")

	_, err := NewFetcher(5*time.Second).FetchManifest(context.Background(), srv.URL)
	assert.Equal(t, ErrFetchFailed, err)
}

// CheckIndexURL is the same rule without a fetch, for a URL that is stored
// now and fetched later.
func TestCheckIndexURL(t *testing.T) {
	netguardtest.StubLookupIP(t, map[string]string{
		"internal.example.com": "10.1.2.3",
		"metadata.example.com": "169.254.169.254",
		"gone.example.com":     "",
	})
	for _, u := range []string{
		"http://127.0.0.1:8080/index.yaml",
		"http://localhost:8080/index.yaml",
		"http://LOCALHOST./index.yaml",
		"http://[::1]/index.yaml",
		"http://[::ffff:10.0.0.1]/index.yaml",
		"http://10.0.0.1/index.yaml",
		"http://172.16.0.1/index.yaml",
		"http://192.168.1.1/index.yaml",
		"http://169.254.169.254/latest/meta-data/",
		"http://[fd00::1]/index.yaml",
		"http://2130706433/index.yaml",
		"http://0x7f.0.0.1/index.yaml",
		"http://0177.0.0.1/index.yaml",
		"http://user:secret@127.0.0.1/index.yaml",
		"https://internal.example.com/index.yaml",
		"https://metadata.example.com/index.yaml",
		"https://gone.example.com/index.yaml",
		"file:///etc/passwd",
		"gopher://example.com/",
		"postgres://postgres:5432/",
		"//127.0.0.1/index.yaml",
		"index.yaml",
		"",
		"http://",
		"http://[::1",
	} {
		assert.Equal(t, ErrFetchFailed, CheckIndexURL(u), u)
	}
	for _, u := range []string{
		"https://marketplace.example.com/index.yaml",
		"http://marketplace.example.com:8443/index.yaml",
		"https://203.0.113.10/index.yaml",
	} {
		assert.NoError(t, CheckIndexURL(u), u)
	}
}
