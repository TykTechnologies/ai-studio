package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

type upstreamRoundTripFunc func(*http.Request) (*http.Response, error)

func (f upstreamRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// routeUpstreamTo sends every upstream request to the fake vendor at target,
// whatever host it names; the Host header (and so the host SigV4 signed)
// stays the one the proxy chose. It returns the last host the proxy chose.
func routeUpstreamTo(t *testing.T, target string) func() string {
	t.Helper()
	u, err := url.Parse(target)
	require.NoError(t, err)
	var mu sync.Mutex
	var last string
	prev := upstreamTransport
	upstreamTransport = upstreamRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		out := r.Clone(r.Context())
		if out.Host == "" {
			out.Host = r.URL.Host
		}
		mu.Lock()
		last = out.Host
		mu.Unlock()
		out.URL.Scheme, out.URL.Host = u.Scheme, u.Host
		return prev.RoundTrip(out)
	})
	t.Cleanup(func() { upstreamTransport = prev })
	return func() string {
		mu.Lock()
		defer mu.Unlock()
		return last
	}
}

// overridePostAuth is a post_auth plugin (a load balancer) that sends the
// request to override.
func overridePostAuth(override string) *AuthHooks {
	return &AuthHooks{PostAuth: func(_ http.ResponseWriter, r *http.Request, _ uint) bool {
		*r = *r.WithContext(context.WithValue(r.Context(), "upstream_override", override))
		return false
	}}
}

// sigV4StreamHandler is sigV4VerifyHandler for converse-stream: a request
// signed for region gets a Converse event stream.
func sigV4StreamHandler(t *testing.T, region string, got *http.Header) http.HandlerFunc {
	stream := bedrockStream(t, "Sure. ", `{"to":"x"}`)
	var mu sync.Mutex
	return func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		var h http.Header
		sigV4VerifyHandler("AKIDTEST", "secret", region, &h)(rec, r)
		mu.Lock()
		*got = h
		mu.Unlock()
		if rec.Code != http.StatusOK {
			w.WriteHeader(rec.Code)
			_, _ = w.Write(rec.Body.Bytes())
			return
		}
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		_, _ = w.Write(stream)
	}
}

func bedrockOverrideHarness(t *testing.T, serve http.HandlerFunc, endpoint string) (*failoverHarness, func() string) {
	h := newFailoverHarness(t, serve, serveOpenAIText("never"), func(primary, _ *models.LLM) {
		primary.Vendor = models.BEDROCK
		primary.Failover = models.LLMFailover{}
		primary.DefaultModel = "anthropic.claude-3"
		if endpoint != "" {
			primary.APIEndpoint = endpoint
		}
		primary.Metadata = models.JSONMap{"aws_access_key_id": "AKIDTEST", "aws_secret_access_key": "secret", "aws_region": "us-east-1"}
	})
	return h, routeUpstreamTo(t, h.primaryVendor.server.URL)
}

// A load balancer's upstream_override into another region: /llm/rest/
// reaches the override host signed for the override's region, not the
// LLM's (AWS answered 403).
func TestBedrockREST_UpstreamOverrideSignsForItsRegion(t *testing.T) {
	var received http.Header
	h, host := bedrockOverrideHarness(t, sigV4VerifyHandler("AKIDTEST", "secret", "eu-west-1", &received), "https://bedrock-runtime.us-east-1.amazonaws.com")
	h.proxy.SetAuthHooks(overridePostAuth("https://bedrock-runtime.eu-west-1.amazonaws.com"))

	resp, body := h.post("/llm/rest/primary/model/anthropic.claude-3/converse", `{"messages":[{"role":"user","content":[{"text":"hi"}]}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "the SigV4 verifier rejected the request: %s", body)
	assert.Equal(t, "bedrock-runtime.eu-west-1.amazonaws.com", host())
	assert.Contains(t, received.Get("Authorization"), "/eu-west-1/bedrock/aws4_request")
}

// The native stream honours the override too (it ignored it), signed for
// the override's region.
func TestBedrockNativeStream_UpstreamOverride(t *testing.T) {
	var received http.Header
	h, host := bedrockOverrideHarness(t, sigV4StreamHandler(t, "eu-west-1", &received), "https://bedrock-runtime.us-east-1.amazonaws.com")
	h.proxy.SetAuthHooks(overridePostAuth("https://bedrock-runtime.eu-west-1.amazonaws.com"))

	resp, body := h.post("/llm/stream/primary/model/anthropic.claude-3/converse-stream", `{"messages":[{"role":"user","content":[{"text":"hi"}]}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "the SigV4 verifier rejected the request: %s", body)
	assert.Equal(t, "bedrock-runtime.eu-west-1.amazonaws.com", host())
	assert.Contains(t, received.Get("Authorization"), "/eu-west-1/bedrock/aws4_request")
	assert.NotEmpty(t, decodeFrames(t, body))
}

// An override whose host names no region (a VPC endpoint, a proxy) is
// signed for the LLM's region.
func TestBedrockNativeStream_UpstreamOverrideWithoutRegion(t *testing.T) {
	var received http.Header
	h, host := bedrockOverrideHarness(t, sigV4StreamHandler(t, "us-east-1", &received), "https://bedrock-runtime.us-east-1.amazonaws.com")
	h.proxy.SetAuthHooks(overridePostAuth("https://vpce-0abc.bedrock.example.internal"))

	resp, body := h.post("/llm/stream/primary/model/anthropic.claude-3/converse-stream", `{"messages":[{"role":"user","content":[{"text":"hi"}]}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "the SigV4 verifier rejected the request: %s", body)
	assert.Equal(t, "vpce-0abc.bedrock.example.internal", host())
	assert.Contains(t, received.Get("Authorization"), "/us-east-1/bedrock/aws4_request")
}

// No override: the stream goes to the LLM's endpoint, signed for its region.
func TestBedrockNativeStream_NoOverrideUnchanged(t *testing.T) {
	var received http.Header
	h, host := bedrockOverrideHarness(t, sigV4StreamHandler(t, "us-east-1", &received), "https://bedrock-runtime.us-east-1.amazonaws.com")

	resp, body := h.post("/llm/stream/primary/model/anthropic.claude-3/converse-stream", `{"messages":[{"role":"user","content":[{"text":"hi"}]}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "the SigV4 verifier rejected the request: %s", body)
	assert.Equal(t, "bedrock-runtime.us-east-1.amazonaws.com", host())
}
