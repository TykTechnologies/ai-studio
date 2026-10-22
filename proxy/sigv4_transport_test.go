package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/aws/aws-sdk-go-v2/aws"
	v4signer "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sigV4VerifyHandler is a mock Bedrock endpoint that checks a request's SigV4
// signature the way AWS does: it rebuilds the request from the headers the
// Authorization header says were signed, as they arrived, and signs it
// again. A header changed after signing fails the check.
func sigV4VerifyHandler(accessKey, secret, region string, got *http.Header) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*got = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		auth := r.Header.Get("Authorization")
		signed := sigV4Between(auth, "SignedHeaders=", ",")
		want := sigV4Between(auth+",", "Signature=", ",")
		when, err := time.Parse("20060102T150405Z", r.Header.Get("X-Amz-Date"))
		if err != nil || signed == "" || want == "" {
			http.Error(w, "unsigned", http.StatusForbidden)
			return
		}
		check, _ := http.NewRequest(r.Method, "http://"+r.Host+r.URL.RequestURI(), bytes.NewReader(body))
		for _, h := range strings.Split(signed, ";") {
			if h != "host" {
				check.Header[http.CanonicalHeaderKey(h)] = r.Header.Values(h)
			}
		}
		sum := sha256.Sum256(body)
		err = v4signer.NewSigner().SignHTTP(context.Background(), aws.Credentials{AccessKeyID: accessKey, SecretAccessKey: secret},
			check, hex.EncodeToString(sum[:]), "bedrock", region, when)
		if err != nil || sigV4Between(check.Header.Get("Authorization")+",", "Signature=", ",") != want {
			http.Error(w, "signature mismatch", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}},"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1,"totalTokens":2}}`))
	}
}

func sigV4Between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	s = s[i+len(start):]
	if j := strings.Index(s, end); j >= 0 {
		return s[:j]
	}
	return s
}

// The /llm/rest/ Bedrock passthrough signs after httputil.ReverseProxy has
// changed the headers: a client's X-Forwarded-For (appended to) and
// Connection-listed headers (removed) used to break the signature, a 403
// from AWS.
func TestBedrockREST_SigV4SurvivesForwardedAndConnectionHeaders(t *testing.T) {
	var received http.Header
	h := newFailoverHarness(t, sigV4VerifyHandler("AKIDTEST", "secret", "us-east-1", &received), serveOpenAIText("never"), func(primary, _ *models.LLM) {
		primary.Vendor = models.BEDROCK
		primary.Failover = models.LLMFailover{}
		primary.DefaultModel = "anthropic.claude-3"
		primary.Metadata = models.JSONMap{"aws_access_key_id": "AKIDTEST", "aws_secret_access_key": "secret", "aws_region": "us-east-1"}
	})
	resp, body := h.post("/llm/rest/primary/model/anthropic.claude-3/converse",
		`{"messages":[{"role":"user","content":[{"text":"hi"}]}]}`,
		"X-Forwarded-For", "203.0.113.7", "Connection", "X-Hop", "X-Hop", "drop-me")
	require.Equal(t, http.StatusOK, resp.StatusCode, "the SigV4 verifier rejected the request: %s", body)
	// The signature covers the headers as sent: the appended
	// X-Forwarded-For, and without the Connection-listed one.
	assert.Contains(t, received.Get("X-Forwarded-For"), "203.0.113.7")
	assert.Contains(t, sigV4Between(received.Get("Authorization"), "SignedHeaders=", ","), "x-forwarded-for")
	assert.Empty(t, received.Get("X-Hop"))
}
