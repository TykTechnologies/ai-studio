package proxy

import (
	"net/http"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	bedrockVendor "github.com/TykTechnologies/midsommar/v2/vendors/bedrock"
)

// profileARN is an inference profile, which the AWS SDKs send escaped:
// arn%3Aaws%3Abedrock%3A...%3Ainference-profile%2Fus.anthropic.claude-3.
const profileARN = "arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-3"

// sdkEscapedARN is profileARN as the AWS SDKs put it in the path.
const sdkEscapedARN = "arn%3Aaws%3Abedrock%3Aus-east-1%3A123456789012%3Ainference-profile%2Fus.anthropic.claude-3"

// pathRecorder wraps a fake Bedrock handler, keeping the escaped path each
// request arrived with.
type pathRecorder struct {
	mu    sync.Mutex
	paths []string
}

func (p *pathRecorder) wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.paths = append(p.paths, r.URL.EscapedPath())
		p.mu.Unlock()
		next(w, r)
	}
}

func (p *pathRecorder) last() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.paths) == 0 {
		return ""
	}
	return p.paths[len(p.paths)-1]
}

// bedrockARNHarness is a Bedrock LLM that allows only profileARN (anchored),
// so a truncated model ID fails model validation.
func bedrockARNHarness(t *testing.T, serve http.HandlerFunc) *failoverHarness {
	h := newFailoverHarness(t, serve, serveOpenAIText("never"), func(primary, _ *models.LLM) {
		primary.Vendor = models.BEDROCK
		primary.Failover = models.LLMFailover{}
		primary.DefaultModel = "anthropic.claude-3"
		primary.AllowedModels = []string{`^arn:aws:bedrock:us-east-1:123456789012:inference-profile/us\.anthropic\.claude-3$`}
		primary.Metadata = models.JSONMap{"aws_access_key_id": "AKIDTEST", "aws_secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "aws_region": "us-east-1"}
	})
	bedrockVendor.InvalidateClientCache(h.primary.ID)
	t.Cleanup(func() { bedrockVendor.InvalidateClientCache(h.primary.ID) })
	return h
}

// An escaped ARN model ID on /llm/rest/ and /llm/call/ passes model
// validation whole and reaches the vendor escaped as the client sent it
// (the "/" in it used to become a path separator), signed for that path.
func TestBedrockREST_EscapedARNModelID(t *testing.T) {
	var received http.Header
	rec := &pathRecorder{}
	h := bedrockARNHarness(t, rec.wrap(sigV4VerifyHandler("AKIDTEST", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1", &received)))

	for _, route := range []string{"/llm/rest/primary", "/llm/call/primary"} {
		t.Run(route, func(t *testing.T) {
			resp, body := h.post(route+"/model/"+sdkEscapedARN+"/converse", `{"messages":[{"role":"user","content":[{"text":"hi"}]}]}`)
			require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)
			assert.Equal(t, "/model/"+sdkEscapedARN+"/converse", rec.last())
		})
	}
}

// The native stream sends the whole ARN upstream as one escaped segment.
func TestBedrockNativeStream_EscapedARNModelID(t *testing.T) {
	var received http.Header
	rec := &pathRecorder{}
	h := bedrockARNHarness(t, rec.wrap(sigV4StreamHandler(t, "us-east-1", &received)))

	for _, route := range []string{"/llm/stream/primary", "/llm/call/primary"} {
		t.Run(route, func(t *testing.T) {
			resp, body := h.post(route+"/model/"+sdkEscapedARN+"/converse-stream", `{"messages":[{"role":"user","content":[{"text":"hi"}]}]}`)
			require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)
			assert.NotEmpty(t, decodeFrames(t, body))
			got := rec.last()
			assert.Equal(t, profileARN, ExtractBedrockModelIDFromPath(got), "upstream path %s", got)
			assert.Equal(t, "/model/"+url.PathEscape(profileARN)+"/converse-stream", got)
		})
	}
}

// A truncated ARN (the model validator used to see only the part before the
// "/") is still refused: validation runs on the whole ID.
func TestBedrockREST_ARNModelValidationRefusesOthers(t *testing.T) {
	var received http.Header
	h := bedrockARNHarness(t, sigV4VerifyHandler("AKIDTEST", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1", &received))
	other := url.PathEscape("arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.meta.llama3")
	resp, body := h.post("/llm/rest/primary/model/"+other+"/converse", `{"messages":[{"role":"user","content":[{"text":"hi"}]}]}`)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "%s", body)
}

// An LLM whose endpoint is a bare region works on /llm/rest/ (it failed:
// the region was parsed as a URL): the request goes to that region's
// runtime endpoint, signed for it.
func TestBedrockREST_RegionOnlyEndpoint(t *testing.T) {
	var received http.Header
	h, host := bedrockOverrideHarness(t, sigV4VerifyHandler("AKIDTEST", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "eu-west-1", &received), "eu-west-1")

	resp, body := h.post("/llm/rest/primary/model/anthropic.claude-3/converse", `{"messages":[{"role":"user","content":[{"text":"hi"}]}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)
	assert.Equal(t, "bedrock-runtime.eu-west-1.amazonaws.com", host())
	assert.Contains(t, received.Get("Authorization"), "/eu-west-1/bedrock/aws4_request")
}
